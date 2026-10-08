package session

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCurrentNamesThePaneSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-current-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	if out, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-s", Name("probe"), "sleep 60").CombinedOutput(); err != nil {
		t.Fatalf("new-session: %s %v", out, err)
	}
	pane, err := exec.Command("tmux", "list-panes", "-t", exact(Name("probe")), "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIA_TMUX_PANE", strings.TrimSpace(string(pane)))
	if got := Current(); got != Name("probe") {
		t.Fatalf("Current() = %q, want %q", got, Name("probe"))
	}
	t.Setenv("MIA_TMUX_PANE", "")
	t.Setenv("TMUX_PANE", "")
	if got := Current(); got != "" {
		t.Fatalf("Current() outside tmux = %q, want empty", got)
	}
}
