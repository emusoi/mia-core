package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/runtime"
)

func TestFailedEnvironmentCleanupKeepsWorktreeAndRecord(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("keep-me")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := `case "$1" in
inspect) echo running ;;
rm) echo 'cannot remove' >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if code := cmdRemove(a, []string{record.Name}); code != exitFailed {
		t.Fatalf("remove with failed cleanup exited %d", code)
	}
	if _, err := os.Stat(record.Path); err != nil {
		t.Fatalf("worktree was removed after cleanup failed: %v", err)
	}
	if _, err := a.RecordOrHere(record.Name); err != nil {
		t.Fatalf("record was forgotten after cleanup failed: %v", err)
	}
}

func TestMovePreflightFailureLeavesCurrentEnvironmentAlone(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("stay-here")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Runtimes.Add(runtime.Machine{Name: "box", SSH: "box.invalid", Engine: "podman"}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	t.Setenv("MIA_MOVE_CALLS", log)
	podman := `printf '%s\n' "$*" >> "$MIA_MOVE_CALLS"
case "$1" in inspect) echo running ;; esac
`
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\n"+podman), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\necho offline >&2\nexit 255\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	manager, err := managerFor(a)
	if err != nil {
		t.Fatal(err)
	}

	if code := cmdHost(a, manager, record, []string{"box"}, false); code != exitFailed {
		t.Fatalf("failed move preflight exited %d", code)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "rm -f") {
		t.Fatalf("failed preflight removed the old environment: %s", calls)
	}
	got, err := a.RecordOrHere(record.Name)
	if err != nil || got.Env != nil {
		t.Fatalf("failed preflight changed placement: %+v, %v", got.Env, err)
	}
}
