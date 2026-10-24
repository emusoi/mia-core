package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/gateway"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type doctorReport struct {
	Checks []doctorCheck `json:"checks"`
	OK     bool          `json:"ok"`
}

func (r *doctorReport) add(name, status, detail string) {
	r.Checks = append(r.Checks, doctorCheck{Name: name, Status: status, Detail: detail})
	if status == "fail" {
		r.OK = false
	}
}

func cmdDoctor(args []string, asJSON bool) int {
	if len(args) > 0 {
		return usageErr("mia doctor")
	}
	report := doctorReport{Checks: []doctorCheck{}, OK: true}
	finish := func() int {
		if asJSON {
			if code := emit(report); code != exitOK {
				return code
			}
		} else {
			for _, check := range report.Checks {
				fmt.Printf("%-5s %-24s %s\n", strings.ToUpper(check.Status), check.Name, check.Detail)
			}
		}
		if !report.OK {
			return exitFailed
		}
		return exitOK
	}

	for _, tool := range []struct {
		name, binary, arg string
	}{{"git", "git", "--version"}, {"tmux", "tmux", "-V"}} {
		out, err := run.Local(tool.binary).Capture(tool.arg)
		if err != nil {
			report.add(tool.name, "fail", err.Error())
		} else {
			report.add(tool.name, "ok", run.FirstLine(out))
		}
	}
	engine, engineErr := container.Find()
	if engineErr != nil {
		report.add("container engine", "fail", engineErr.Error())
	} else if err := engine.Ready(); err != nil {
		report.add("container engine", "fail", err.Error())
	} else {
		report.add("container engine", "ok", engine.Binary)
	}
	if gateway.Running() {
		report.add("gateway", "ok", "listening on "+gateway.Addr)
	} else {
		report.add("gateway", "warn", "not running; `mia env up` starts it")
	}
	report.add("https", "info", "certificate: "+trustState()+"; PAC is a browser setting; `mia env browse --direct` needs neither")

	here, err := os.Getwd()
	if err != nil {
		report.add("repository", "fail", err.Error())
		return finish()
	}
	a, err := app.Open(here)
	if errors.Is(err, git.ErrNotARepository) {
		report.add("repository", "info", "not inside a git repository; repository checks skipped")
		return finish()
	}
	if err != nil {
		report.add("configuration", "fail", err.Error())
		return finish()
	}
	report.add("configuration", "ok", "effective configuration is valid")
	if a.Config.Env.Compose.Enabled() && engineErr == nil {
		if err := engine.ComposeReady(); err != nil {
			report.add("compose", "fail", err.Error())
		} else if manager, err := managerFor(a); err != nil {
			report.add("compose", "fail", err.Error())
		} else if err := manager.CheckCompose(model.Record{Path: a.Root, Name: filepath.Base(a.Root)}); err != nil {
			report.add("compose", "fail", err.Error())
		} else {
			report.add("compose", "ok", "compose plugin and resolved config are valid")
		}
	}

	machines, err := a.Runtimes.All()
	if err != nil {
		report.add("machines", "fail", err.Error())
	} else {
		for _, machine := range machines {
			ok, detail := a.Reachable(machine)
			if !ok {
				report.add("machine "+machine.Name, "fail", detail)
				continue
			}
			report.add("machine "+machine.Name, "ok", detail)
			if _, err := runtime.Run(machine.SSH, "sh", "-c", "command -v tmux"); err != nil {
				report.add("tmux on "+machine.Name, "fail", err.Error())
			} else {
				report.add("tmux on "+machine.Name, "ok", "installed")
			}
		}
	}

	records, err := a.Store.Load()
	if err != nil {
		report.add("worktree records", "fail", err.Error())
		return finish()
	}
	for _, record := range records {
		if machine := env.Placement(record).Runtime(); machine != "" {
			if _, err := a.Runtimes.Get(machine); err != nil {
				report.add("placement "+record.Name, "fail", err.Error())
			}
			if a.Config.Env.Session != "local" {
				report.add("session "+record.Name, "warn", "remote staging has no .git; use `[env] session = \"local\"` for agents that commit or inspect Git")
			}
			if a.Config.Env.SyncSecrets {
				report.add("sync "+record.Name, "warn", "sync_secrets = true allows .env, keys and credential files on this machine")
			}
		}
	}
	if engineErr == nil {
		manager, err := managerFor(a)
		if err == nil {
			for _, record := range records {
				drift, err := manager.Drift(record)
				if err != nil {
					report.add("environment "+record.Name, "warn", err.Error())
				} else if drift {
					report.add("environment "+record.Name, "warn", "configuration changed; `mia env up --recreate "+record.Name+"` applies it")
				}
			}
		}
	}

	plugins, err := pluginHost(a.Config).List()
	if err != nil {
		report.add("plugins", "fail", err.Error())
	} else {
		for _, p := range plugins {
			if p.Enabled && p.Problem != "" {
				report.add("plugin "+p.Name, "fail", p.Problem)
			}
		}
		for _, collision := range verbConflicts(plugins) {
			report.add("plugin verb "+collision.Verb, "fail", strings.Join(collision.Plugins, ", "))
		}
	}
	return finish()
}
