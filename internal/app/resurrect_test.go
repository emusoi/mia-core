package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/session"
)

func TestWhatWentDownWithTmuxComesBackAndWhatWasClosedStaysClosed(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-resurrect-")
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
	kept, err := a.New("kept")
	if err != nil {
		t.Fatal(err)
	}
	closed, err := a.New("closed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(kept, "agent", []string{"sleep", "300"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(closed, "shell", nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	exec.Command("tmux", "kill-session", "-t", "="+session.Name(closed.Name)).Run()
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Resurrect(true); len(got) != 0 {
		t.Fatalf("tmux is still up: nothing to bring back, got %+v", got)
	}

	exec.Command("tmux", "kill-server").Run()
	got, err := a.Resurrect(true)
	if err != nil || len(got) != 1 || got[0].Name != kept.Name {
		t.Fatalf("only the session that went down with tmux comes back: %+v %v", got, err)
	}
	if i := slices.IndexFunc(got[0].Windows, func(w app.Window) bool { return w.Name == "agent" }); i < 0 || got[0].Windows[i].Command != "sleep 300" {
		t.Errorf("its program runs again: %+v", got[0].Windows)
	}
	if _, err := a.Resurrect(false); err != nil {
		t.Fatal(err)
	}
	if !session.Here().HasWindow(kept.Name, "agent") || session.Here().Exists(closed.Name) {
		t.Errorf("kept is back with its agent window, closed is not: %v", session.Here().Windows(kept.Name))
	}
	if again, _ := a.Resurrect(true); len(again) != 0 {
		t.Errorf("once back, it is not brought back twice: %+v", again)
	}

	// what was closed is still remembered, and comes back by name
	states := map[string]string{}
	for _, s := range a.Sessions() {
		states[s.Name] = s.State
	}
	if states[kept.Name] != "running" || states[closed.Name] != "closed" {
		t.Errorf("every session is listed with its state: %v", states)
	}
	if got, err := a.Resurrect(false, closed.Name); err != nil || len(got) != 1 || !session.Here().Exists(closed.Name) {
		t.Errorf("a closed session comes back when it is named: %+v %v", got, err)
	}
	if _, err := a.Resurrect(true, "nowhere"); err == nil {
		t.Error("a name mia never saw is refused")
	}
}

func TestAClaudeResumesTheConversationItHad(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-resurrect-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	claudeHome := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeHome)
	bin := t.TempDir()
	sleep, _ := exec.LookPath("sleep")
	if err := os.Symlink(sleep, filepath.Join(bin, "claude")); err != nil {
		t.Fatal(err)
	}
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := a.New("talking")
	if err != nil {
		t.Fatal(err)
	}
	// typed into the window's shell, as a person does, so the window outlives it
	if _, err := a.NewWindow(wt, "claude", nil); err != nil {
		t.Fatal(err)
	}
	exec.Command("tmux", "send-keys", "-t", "="+session.Name(wt.Name)+":claude", filepath.Join(bin, "claude")+" 300", "Enter").Run()
	// claude says which session a process runs in sessions/<pid>.json
	var pid string
	for range 50 {
		out, _ := exec.Command("tmux", "display", "-p", "-t", "="+session.Name(wt.Name)+":claude", "#{pane_pid} #{pane_current_command}").Output()
		shell, running, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
		pgid, _ := exec.Command("ps", "-o", "tpgid=", "-p", shell).Output()
		if pid = strings.TrimSpace(string(pgid)); running == "claude" && pid != "" && pid != "-1" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	os.MkdirAll(filepath.Join(claudeHome, "sessions"), 0o755)
	os.WriteFile(filepath.Join(claudeHome, "sessions", pid+".json"), []byte(`{"pid":`+pid+`,"sessionId":"23f98c02-c52b"}`), 0o644)
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	// the claude ends before tmux does: its window, back at a shell, keeps it
	exec.Command("kill", pid).Run()
	for range 50 {
		out, _ := exec.Command("tmux", "display", "-p", "-t", "="+session.Name(wt.Name)+":claude", "#{pane_current_command}").Output()
		if strings.TrimSpace(string(out)) != "claude" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	exec.Command("tmux", "kill-server").Run()
	got, err := a.Resurrect(true)
	if err != nil || len(got) != 1 {
		t.Fatalf("it went down with tmux: %+v %v", got, err)
	}
	i := slices.IndexFunc(got[0].Windows, func(w app.Window) bool { return w.Name == "claude" })
	if i < 0 || !strings.HasSuffix(got[0].Windows[i].Command, "300 --resume 23f98c02-c52b") {
		t.Errorf("the claude window resumes its own conversation: %+v", got[0].Windows)
	}
	// brought back, it is remembered with its claude though that is yet to start
	if _, err := a.Resurrect(false); err != nil {
		t.Fatal(err)
	}
	back := a.Sessions()
	if len(back) != 1 || !slices.ContainsFunc(back[0].Windows, func(w app.Window) bool { return strings.HasSuffix(w.Command, "--resume 23f98c02-c52b") }) {
		t.Errorf("what was just opened is remembered as opened: %+v", back)
	}
}
