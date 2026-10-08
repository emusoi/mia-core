package cli

import (
	"fmt"
	"os"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/gateway"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
)

func cmdEnv(a *app.App, args []string, asJSON bool) int {
	if len(args) == 0 {
		return usageErr(usageOf("env"))
	}
	if args[0] == "-h" || args[0] == "--help" {
		verbUsage(os.Stdout, "env")
		return exitOK
	}
	sub, rest := args[0], args[1:]
	var withCredentials, once bool
	switch sub {
	case "status", "ports":
		asJSON = take(&rest, "--json") || asJSON
	case "tools":
		withCredentials = take(&rest, "--credentials")
	case "shell":
		once = take(&rest, "--once")
	case "run":
		return envRun(a, rest)
	}

	manager, err := managerFor(a)
	if err != nil {
		return fail(err)
	}

	target, rest := splitEnvWords(sub, rest,
		func(word string) bool { _, err := manager.Runtimes.Get(word); return err == nil },
		func(word string) bool { return declares(a.Config.Services, word) },
		func(word string) bool { return namesAWorktree(a, word) })
	record, err := a.RecordOrHere(target)
	if err != nil {
		return fail(err)
	}

	switch sub {
	case "dotfiles":
		return exitFor(manager.CopyMachineDotfiles(record, os.Stderr))
	case "up":
		environment, err := manager.Up(record, os.Stderr)
		if err != nil {
			if environment.Container == "" {
				return fail(err)
			}
			fmt.Fprintf(os.Stderr, "mia: %v\n", err)
		}
		if err := manager.CarryTools(record, a.Config.Tools, a.Config.Tools.CarryCredentials, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "mia: the environment is up but %v\n", err)
		}
		if err := manager.ApplyDotfiles(record, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "mia: the environment is up but %v\n", err)
		}
		if gatewayErr := EnsureGateway(); gatewayErr != nil {
			fmt.Fprintf(os.Stderr, "mia: the environment is up but %v\n", gatewayErr)
		}
		a.Emit("env.up", record, environment)
		if asJSON {
			return emit(environment)
		}
		fmt.Printf("%s  %s  %s\n", environment.Name, environment.State, environment.Image.Ref)
		for _, port := range environment.Listening {
			fmt.Printf("          https://%s%s:%d/\n", environment.Name, gateway.Suffix, port)
		}
		return exitOK

	case "setup":
		if err := manager.Setup(record, os.Stderr); err != nil {
			return fail(err)
		}
		fmt.Printf("setup ran inside %s\n", record.Name)
		return exitOK

	case "down":
		if err := manager.Down(record); err != nil {
			return fail(err)
		}
		a.Emit("env.down", record, envGone{Removed: false})
		fmt.Printf("stopped %s\n", record.Name)
		return exitOK

	case "rm":
		if err := manager.Remove(record); err != nil {
			return fail(err)
		}
		a.Emit("env.down", record, envGone{Removed: true})
		fmt.Printf("removed the environment for %s\n", record.Name)
		return exitOK

	case "sync":
		action := ""
		if len(rest) > 0 {
			action = rest[0]
		}
		if action == "auto" || action == "manual" {
			if record.Env == nil {
				return fail(fmt.Errorf("%s has no environment — `mia env host %s <machine>` first", record.Name, record.Name))
			}
			record.Env.Sync = map[string]string{"auto": "", "manual": model.SyncManual}[action]
			if err := a.Store.Put(record); err != nil {
				return fail(err)
			}
		}
		session, err := manager.Sync(record, action)
		if err != nil {
			return fail(err)
		}
		mode := "auto — live, both ways"
		if record.Manual() {
			mode = "manual — `mia env sync " + record.Name + " now` moves changes"
		}
		fmt.Printf("%s  sync %s\n          %s\n", record.Name, mode, session.Describe())
		return exitOK

	case "shell":
		// A window named env in the worktree's session, on the machine the
		// environment runs on, with a shell inside the container. Detaching
		// leaves it running; the same command lands back in it, as it was.
		// --once is the bare shell that ends with the connection.
		if once {
			if err := manager.Shell(record); err != nil {
				return fail(err)
			}
			return exitOK
		}
		argv, err := manager.ShellArgv(record)
		if err != nil {
			return fail(err)
		}
		host, err := a.SessionHostOf(record)
		if err != nil {
			return fail(err)
		}
		dir, err := a.SessionDir(record)
		if err != nil {
			return fail(err)
		}
		if err := host.StartWindow(record.Name, dir, envWindow, argv); err != nil {
			return fail(err)
		}
		if err := host.AttachWindow(record.Name, envWindow); err != nil {
			return fail(err)
		}
		return exitOK

	case "exec":
		if len(rest) == 0 {
			return usageErr("mia env exec <worktree> <command...>")
		}
		if err := manager.Exec(record, rest); err != nil {
			return fail(err)
		}
		return exitOK

	case "ports":
		ports, err := manager.PortsOf(record)
		if err != nil {
			return fail(err)
		}
		if asJSON {
			if ports.Listening == nil {
				ports.Listening = []int{}
			}
			return emit(ports)
		}
		if len(ports.Listening) == 0 {
			fmt.Println("nothing listening")
			return exitOK
		}
		for _, port := range ports.Listening {
			fmt.Printf("%d  https://%s%s:%d/\n", port, record.Name, gateway.Suffix, port)
		}
		return exitOK

	case "tools":
		if err := manager.CarryTools(record, a.Config.Tools, withCredentials, os.Stderr); err != nil {
			return fail(err)
		}
		return exitOK

	case "image":
		tag, err := manager.BuildImage(record, os.Stderr)
		if err != nil {
			return fail(err)
		}
		fmt.Println(tag)
		return exitOK

	case "host":
		return cmdHost(a, manager, record, rest)

	case "browse":
		return cmdBrowse(manager, record, rest)

	case "service":
		if len(rest) == 0 {
			statuses, err := manager.ServiceStatus(record)
			if err != nil {
				return fail(err)
			}
			if asJSON {
				return emit(statuses)
			}
			if len(statuses) == 0 {
				fmt.Println("no services declared — add a [[service]] block")
				return exitOK
			}
			for _, status := range statuses {
				state := "stopped"
				if status.Error != "" {
					state = "unavailable — " + status.Error
				} else if status.Running {
					state = "running"
					if status.Healthy != nil {
						if *status.Healthy {
							state += " · healthy"
						} else {
							state += " · NOT ANSWERING"
						}
					}
				}
				fmt.Printf("%-14s %s\n", status.ID, state)
			}
			return exitOK
		}
		action, ids := rest[0], rest[1:]
		switch action {
		case "start", "stop", "restart":
			if len(ids) != 1 {
				return usageErr("mia env service " + action + " <worktree> <id>")
			}
			if action == "stop" || action == "restart" {
				if err := manager.StopService(record, ids[0]); err != nil {
					return fail(err)
				}
			}
			if action == "start" || action == "restart" {
				if err := manager.StartService(record, ids[0]); err != nil {
					return fail(err)
				}
			}
			fmt.Printf("%s %s\n", map[string]string{
				"start": "started", "stop": "stopped", "restart": "restarted",
			}[action], ids[0])
			return exitOK
		case "logs":
			if len(ids) != 1 {
				return usageErr("mia env service logs <worktree> <id>")
			}
			out, err := manager.Logs(record, ids[0], 40)
			if err != nil {
				return fail(err)
			}
			fmt.Println(out)
			return exitOK
		default:
			return usageErr("mia env service [start|stop|restart|logs] <worktree> <id>")
		}

	case "status":
		environment := manager.Describe(record)
		if asJSON {
			return emit(environment)
		}
		state := environment.State
		if environment.Error != "" {
			state = "unavailable — " + environment.Error
		} else if state == "" {
			state = "no environment — `mia env up` starts one"
		}
		fmt.Printf("%s  %s\n", environment.Name, state)
		fmt.Printf("machine   %s\n", environment.Machine)
		fmt.Printf("image     %s  (%s)\n", environment.Image.Ref, environment.Image.Source)
		if environment.Mirror != "" {
			fmt.Printf("tree      %s\n", environment.Mirror)
		}
		for _, port := range environment.Listening {
			fmt.Printf("serving   https://%s%s:%d/\n", environment.Name, gateway.Suffix, port)
		}
		return exitOK

	default:
		fmt.Fprintf(os.Stderr, "mia: no env command %q\n", sub)
		return exitUsage
	}
}

func splitEnvWords(sub string, rest []string, isRuntime, isService, isWorktree func(string) bool) (target string, left []string) {
	if len(rest) == 0 {
		return "", nil
	}
	switch {
	case sub == "host" && len(rest) > 1 && isRuntime(rest[0]) && !isRuntime(rest[1]):
		return rest[1], rest[:1]

	case sub == "host" && isRuntime(rest[len(rest)-1]):
		machine := rest[len(rest)-1]
		if len(rest) > 1 {
			target = rest[0]
		}
		return target, []string{machine}

	case sub == "service" && isServiceAction(rest[0]):
		action, rest := rest[0], rest[1:]
		if len(rest) > 1 || (len(rest) == 1 && !isService(rest[0])) {
			target, rest = rest[0], rest[1:]
		}
		return target, append([]string{action}, rest...)

	case sub == "exec" && !isWorktree(rest[0]):
		return "", rest
	}
	return rest[0], rest[1:]
}

func isServiceAction(word string) bool {
	switch word {
	case "start", "stop", "restart", "logs":
		return true
	}
	return false
}

func cmdBrowse(manager env.Manager, record model.Record, rest []string) int {
	if err := EnsureGateway(); err != nil {
		return fail(err)
	}
	ports, err := manager.PortsOf(record)
	if err != nil {
		return fail(err)
	}

	port := 0
	if len(rest) > 0 {
		if _, err := fmt.Sscan(rest[0], &port); err != nil || port <= 0 {
			return usageErr("mia env browse [worktree] [port]")
		}
	} else {
		port = choosePort(ports)
	}
	if port == 0 {
		fmt.Fprintf(os.Stderr, "mia: %s is not serving anything yet — `mia env service %s` shows what should be\n", record.Name, record.Name)
		return exitFailed
	}

	url := fmt.Sprintf("https://%s%s:%d/", record.Name, gateway.Suffix, port)
	fmt.Println(url)
	if _, err := run.Local("open").Combined(url); err != nil {
		fmt.Fprintf(os.Stderr, "mia: could not open a browser (%v)\n", err)
	}
	return exitOK
}

func choosePort(ports env.Ports) int {
	if ports.Page != 0 {
		return ports.Page
	}
	if len(ports.Listening) > 0 {
		return ports.Listening[0]
	}
	return 0
}

func cmdHost(a *app.App, manager env.Manager, record model.Record, rest []string) int {
	if len(rest) == 0 {
		where, err := manager.Where(record)
		if err != nil {
			return fail(err)
		}
		fmt.Println(env.Placement(record).String())
		if where.Remote() {
			fmt.Printf("staging  %s:%s\n", where.Machine.SSH, where.Dir)
		}
		return exitOK
	}
	target := rest[0]

	runtimes := runtime.Store{Path: config.RuntimesPath()}
	machine, err := runtimes.Get(target)
	if err != nil {
		return fail(err)
	}

	if err := manager.Remove(record); err != nil {
		fmt.Fprintf(os.Stderr, "mia: %v\n", err)
	}
	if record, err = a.Host(record, machine); err != nil {
		return fail(err)
	}
	fmt.Printf("%s will run on %s — `mia env up %s`\n", record.Name, machine.Name, record.Name)
	return exitOK
}

// The window `mia env shell` opens and returns to.
const envWindow = "env"

func declares(services []env.Service, id string) bool {
	for _, service := range services {
		if service.ID == id {
			return true
		}
	}
	return false
}

func namesAWorktree(a *app.App, word string) bool {
	repo, err := a.Resolver()
	if err != nil {
		return false
	}
	_, err = repo.Worktree(word)
	return err == nil
}

func placeOn(a *app.App, record model.Record, on string) (model.Record, int) {
	machine, err := (runtime.Store{Path: config.RuntimesPath()}).Get(on)
	if err != nil {
		return record, fail(err)
	}
	placed, err := a.Host(record, machine)
	if err != nil {
		return record, fail(err)
	}
	fmt.Printf("%s runs on %s\n", placed.Name, machine.Name)
	return placed, cmdEnv(a, []string{"up", placed.Name}, false)
}

type envGone struct {
	Removed bool `json:"removed"`
}

func envRun(a *app.App, args []string) int {
	if len(args) < 2 {
		return usageErr("mia env run <worktree> <command...>")
	}
	record, err := a.RecordOrHere(args[0])
	if err != nil {
		return fail(err)
	}
	var exit int
	var output, where string
	if manager, err := managerFor(a); err == nil {
		exit, output, where = manager.Check(record, args[1:])
	} else {
		exit, output, where = env.CheckHere(record, args[1:])
	}
	fmt.Print(output)
	fmt.Fprintf(os.Stderr, "mia: ran %s\n", where)
	return exit
}
