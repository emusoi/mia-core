package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestEnvDownAllAndFollowLogs(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	second := repo.Worktree("second", "second")
	for _, record := range []model.Record{{Name: "main", Path: repo.Root}, {Name: "second", Path: second}} {
		if err := a.Store.Put(record); err != nil {
			t.Fatal(err)
		}
	}
	a.Config.Services = []env.Service{{ID: "web", Run: []string{"serve"}}}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	t.Setenv("MIA_BULK_CALLS", log)
	script := `printf '%s\n' "$*" >> "$MIA_BULK_CALLS"
case "$1" in
inspect) echo running ;;
stop|exec) exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var events []string
	a.Events = func(e app.Event) { events = append(events, e.Name+" "+e.Record.Name) }

	if code := cmdEnv(a, []string{"down", "--all"}, false); code != exitOK {
		t.Fatalf("down --all exited %d", code)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"stop -t 5 mia-main", "stop -t 5 mia-second"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("down --all missed %q: %s", want, calls)
		}
	}
	if len(events) != 2 {
		t.Fatalf("down --all emitted %v", events)
	}

	if code := cmdEnv(a, []string{"service", "logs", "-f", "main", "web"}, false); code != exitOK {
		t.Fatalf("logs -f exited %d", code)
	}
	calls, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "tail -n 40 -f /run/mia/web.log") {
		t.Fatalf("logs -f did not follow the service log: %s", calls)
	}
}
