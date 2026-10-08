package cli

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/tui"
)

func TestOnlyAShellLandsInTheSessionDashRunsIn(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-lands-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	if out, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-s", session.Name("engaruka"), "sleep 60").CombinedOutput(); err != nil {
		t.Fatalf("new-session: %s %v", out, err)
	}
	pane, err := exec.Command("tmux", "list-panes", "-t", "="+session.Name("engaruka"), "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIA_TMUX_PANE", strings.TrimSpace(string(pane)))

	if !landsHere(tui.Chosen{RowID: "engaruka", Argv: []string{"shell", "engaruka"}}) {
		t.Error("a shell for the session dash runs in must end the dash loop")
	}
	for _, argv := range [][]string{
		{"chat", "attach", "engaruka"},
		{"window", "open", "engaruka"},
		{"new", "--shell", "engaruka"},
	} {
		if landsHere(tui.Chosen{RowID: "engaruka", Argv: argv}) {
			t.Errorf("mia %s must leave dash up", strings.Join(argv, " "))
		}
	}
	if landsHere(tui.Chosen{RowID: "duluti", Argv: []string{"shell", "duluti"}}) {
		t.Error("a shell for another worktree must leave dash up")
	}
}
