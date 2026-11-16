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
	auto := func() string {
		out, _ := exec.Command("tmux", "list-windows", "-t", "="+session.Name(kept.Name), "-F", "#{window_name}=#{automatic-rename}").Output()
		return string(out)
	}
	before := auto() // the session's first window tmux names, agent a person did
	// the agent window is split: a second pane runs beside it
	if out, err := exec.Command("tmux", "split-window", "-d", "-t", "="+session.Name(kept.Name)+":agent", "sleep", "200").CombinedOutput(); err != nil {
		t.Fatalf("split: %s", out)
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
	i := slices.IndexFunc(got[0].Windows, func(w app.Window) bool { return w.Name == "agent" })
	if i < 0 || got[0].Windows[i].Command != "sleep 300" {
		t.Fatalf("its program runs again: %+v", got[0].Windows)
	}
	if splits := got[0].Windows[i].Splits; len(splits) != 1 || splits[0].Command != "sleep 200" {
		t.Errorf("and so does the pane beside it: %+v", got[0].Windows[i])
	}
	if _, err := a.Resurrect(false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(before, "=1") || !strings.Contains(before, "agent=0") || strings.Count(auto(), "=1") != strings.Count(before, "=1") || !strings.Contains(auto(), "agent=0") {
		t.Errorf("a window tmux named it names still, one a person named keeps its name: before %q, after %q", before, auto())
	}
	if panes, _ := exec.Command("tmux", "list-panes", "-t", "="+session.Name(kept.Name)+":agent").Output(); strings.Count(string(panes), "\n") != 2 {
		t.Errorf("the agent window is split in two again:\n%s", panes)
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

// A session whose worktree was removed is not one to bring back: resurrect
// says the worktree is gone, not that it is already running, and the list
// leaves it out.
func TestASessionOfARemovedWorktreeIsNotBroughtBack(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-resurrect-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	gone, err := a.New("gone")
	if err != nil {
		t.Fatal(err)
	}
	stays, err := a.New("stays")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []struct{ name, path string }{{gone.Name, gone.Path}, {stays.Name, stays.Path}} {
		if out, err := exec.Command("tmux", "new-session", "-d", "-s", session.Name(record.name), "-c", record.path).CombinedOutput(); err != nil {
			t.Fatalf("session: %s", out)
		}
	}
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	removal, err := a.PlanRemoval(gone.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Remove(removal, false, true); err != nil {
		t.Fatal(err)
	}
	revived, err := a.Resurrect(false, gone.Name)
	if err == nil || !strings.Contains(err.Error(), "was removed") || len(revived) != 0 {
		t.Fatalf("resurrecting a removed worktree: %v, %v", revived, err)
	}
	for _, s := range a.Sessions() {
		if s.Name == gone.Name {
			t.Errorf("the list still has %s, whose worktree is gone", s.Name)
		}
	}
}

// A session is remembered when mia makes or changes it, not only when the
// status line next polls: one made and used with no terminal comes back.
func TestASessionIsRememberedWhenItIsMadeOrChanged(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-resurrect-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("headless")
	if err != nil {
		t.Fatal(err)
	}
	for name, seconds := range map[string]string{"agent": "300", "extra": "200"} {
		if _, err := a.NewWindow(record, name, []string{"sleep", seconds}); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.CloseWindow(record, "extra"); err != nil {
		t.Fatal(err)
	}
	exec.Command("tmux", "kill-session", "-t", "="+session.Name(record.Name)).Run()
	got, err := a.Resurrect(true, record.Name)
	if err != nil || len(got) != 1 {
		t.Fatalf("a session nothing polled is remembered all the same: %+v %v", got, err)
	}
	if !slices.ContainsFunc(got[0].Windows, func(w app.Window) bool { return w.Name == "agent" && w.Command == "sleep 300" }) {
		t.Errorf("its window comes back with its program: %+v", got[0].Windows)
	}
	if slices.ContainsFunc(got[0].Windows, func(w app.Window) bool { return w.Name == "extra" }) {
		t.Errorf("the window that was closed is not one to bring back: %+v", got[0].Windows)
	}
}

// What mia kills it looks at first: the session of a worktree being removed
// is remembered as it was then. And the next worktree, handed the same name,
// starts with no session of its own to bring back.
func TestARemovedWorktreesSessionIsSeenLastAndNotInherited(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-resurrect-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	gone, err := a.New("gone")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(gone, "agent", []string{"sleep", "300"}); err != nil {
		t.Fatal(err)
	}
	// a window mia did not make, and so has not seen
	if out, err := exec.Command("tmux", "new-window", "-d", "-t", "="+session.Name(gone.Name)+":", "-n", "late", "sleep", "100").CombinedOutput(); err != nil {
		t.Fatalf("window: %s", out)
	}
	removal, err := a.PlanRemoval(gone.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Remove(removal, false, true); err != nil {
		t.Fatal(err)
	}
	if kept, _ := os.ReadFile(filepath.Join(repo.MiaDir(), "sessions.json")); !strings.Contains(string(kept), `"late"`) {
		t.Errorf("the session was not looked at before it was killed:\n%s", kept)
	}
	next, err := a.New("next")
	if err != nil {
		t.Fatal(err)
	}
	if next.Name != gone.Name {
		t.Skipf("%s was not handed the name %s again", next.Name, gone.Name)
	}
	if got, err := a.Resurrect(true, next.Name); err == nil || !strings.Contains(err.Error(), "remembers no session") {
		t.Errorf("a new worktree was handed the session of the one removed before it: %+v %v", got, err)
	}
}
