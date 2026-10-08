package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
)

func TestMachinesPanelSkipsWorktreeSurveysWithoutRemoteMachines(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	path := repo.Worktree("selected", "feature")
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	placement, err := model.OnRuntime("fixture", "/fixture/selected")
	if err != nil {
		t.Fatal(err)
	}
	record := model.Record{Name: "selected", Path: path, Env: &model.Environment{Placement: placement}}
	if err := a.Store.Put(record); err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	for name, body := range map[string]string{
		"git":  "printf 'git %s\\n' \"$*\" >> \"$MIA_MACHINE_PANEL_CALLS\"\nexec " + run.ShellJoin([]string{realGit}) + " \"$@\"\n",
		"tmux": "printf 'tmux %s\\n' \"$*\" >> \"$MIA_MACHINE_PANEL_CALLS\"\nexit 1\n",
		"ps":   "printf 'ps %s\\n' \"$*\" >> \"$MIA_MACHINE_PANEL_CALLS\"\nexit 1\n",
		"ssh":  "printf 'ssh %s\\n' \"$*\" >> \"$MIA_MACHINE_PANEL_CALLS\"\ncase \"$*\" in\n*uname*) printf 'Fixture 1.0\\n/usr/bin/podman\\n' ;;\n*) exit 1 ;;\nesac\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("MIA_MACHINE_PANEL_CALLS", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, registered := range []bool{false, true, false} {
		if registered {
			if err := a.Runtimes.Add(runtime.Machine{Name: "fixture", SSH: "fake-box", Engine: "/usr/bin/podman"}); err != nil {
				t.Fatal(err)
			}
		} else if err := a.Runtimes.Remove("fixture"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(log, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		view, err := buildPanel(a, "machines-panel", "")
		if err != nil || view.ID != "machines" || len(view.Sections) != 1 || len(view.Sections[0].Rows) == 0 || view.Sections[0].Rows[0].ID != runtime.Local {
			t.Fatalf("the local machine panel changed: %+v, %v", view, err)
		}
		calls, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		if !registered {
			if len(view.Sections[0].Rows) != 1 || len(calls) != 0 {
				t.Fatalf("the local-only panel surveyed worktrees: %+v, %s", view, calls)
			}
			continue
		}
		if len(view.Sections[0].Rows) != 2 {
			t.Fatalf("a newly added machine was not loaded: %+v", view)
		}
		row := view.Sections[0].Rows[1]
		if row.ID != "fixture" || row.Cells[3] != "1 here" || row.Note != "" || !slices.Contains(row.Facts, "hosts  selected") || !slices.Contains(row.Facts, "answers  Fixture 1.0 · /usr/bin/podman") || strings.Count(string(calls), "uname -sr;") != 1 {
			t.Fatalf("remote hosting or reachability was lost: %+v, %s", row, calls)
		}
	}
}

func TestMachinesPanelKeepsRuntimeStoreAndReachabilityErrors(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Runtimes.Add(runtime.Machine{Name: "fixture", SSH: "fake-box", Engine: "/usr/bin/podman"}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	for name, body := range map[string]string{"ssh": "echo 'fixture offline' >&2\nexit 255\n", "tmux": "exit 1\n", "ps": "exit 1\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	view, err := buildPanel(a, "machines-panel", "")
	if err != nil || len(view.Sections) != 1 || len(view.Sections[0].Rows) != 2 {
		t.Fatalf("an unreachable machine was hidden: %+v, %v", view, err)
	}
	row := view.Sections[0].Rows[1]
	if row.Note != "unreachable" || !strings.Contains(strings.Join(row.Facts, "\n"), "fixture offline") {
		t.Fatalf("the machine error was lost: %+v", row)
	}
	if err := os.WriteFile(a.Runtimes.Path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := buildPanel(a, "machines-panel", ""); err == nil || !strings.Contains(err.Error(), "read "+a.Runtimes.Path) {
		t.Fatalf("the runtime-store error was hidden: %v", err)
	}
}
