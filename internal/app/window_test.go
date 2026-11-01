package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
)

func TestRemoteNewWindowUsesTheRemoteSessionShell(t *testing.T) {
	dir := t.TempDir()
	ssh := `#!/bin/sh
for arg do command=$arg; done
printf '%s\n' "$command" >> "$MIA_TEST_SSH_LOG"
case "$command" in
  *echo*) printf '/home/koala\n' ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("SHELL", "/mac-only/zsh")
	log := filepath.Join(dir, "ssh.log")
	t.Setenv("MIA_TEST_SSH_LOG", log)

	store := runtime.Store{Path: filepath.Join(dir, "runtimes.json")}
	if err := store.Add(runtime.Machine{Name: "fedora", SSH: "fedora.invalid"}); err != nil {
		t.Fatal(err)
	}
	placement, err := model.OnRuntime("fedora", "/home/koala/.mia/app/example")
	if err != nil {
		t.Fatal(err)
	}
	a := &app.App{Root: filepath.Join(dir, "repo"), Runtimes: store}
	record := model.Record{Name: "example", Env: &model.Environment{Placement: placement}}
	if _, err := a.NewWindow(record, "zsh", nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !strings.Contains(line, "'new-window'") {
			continue
		}
		if !strings.HasSuffix(line, " '--'") || strings.Contains(line, "/mac-only/zsh") {
			t.Fatalf("new window over SSH = %q, want tmux session default command", line)
		}
		return
	}
	t.Fatalf("no remote new-window call in %q", data)
}

func TestAShellInAWorktreeWithoutASessionIsTheSessionsOwnShell(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-window-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	dir := t.TempDir()
	a := &app.App{Root: dir}
	record := model.Record{Name: "lonely", Path: dir}
	opened, err := a.NewWindow(record, "shell", nil)
	if err != nil {
		t.Fatal(err)
	}
	if windows := session.Here().Windows("lonely"); len(windows) != 1 || windows[0] != opened {
		t.Errorf("windows = %v, opened %q; want one shell", windows, opened)
	}
}

func TestTextSentToAWindowCanBeReadBack(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-send-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	dir := filepath.Join(socket, "talker")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	a := &app.App{Root: dir}
	record := model.Record{Name: "talker", Path: dir}
	if _, err := a.NewWindow(record, "agent", []string{"cat"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SendToWindow(record, "agent", "hello there"); err != nil {
		t.Fatal(err)
	}
	var text string
	for range 50 {
		if text, err = a.ReadWindow(record, "agent"); err == nil && strings.Count(text, "hello there") == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Count(text, "hello there") != 2 {
		t.Fatalf("read back %q, want the typed line and cat's echo", text)
	}
	if _, err := a.ReadWindow(record, "nobody"); err == nil {
		t.Error("reading a window that isn't there succeeded")
	}
}

func TestRenamingAWorktreeRenamesItsSession(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-rename-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("busy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "shell", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Rename(record.Name, "renamed"); err != nil {
		t.Fatal(err)
	}
	if !session.Here().Exists("renamed") || session.Here().Exists(record.Name) {
		t.Error("the session kept the old name, so the worktree lost it")
	}
}
