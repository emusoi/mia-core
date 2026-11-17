package session_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/session"
)

// titlesLab is a tmux server of the test's own, with a worktree folder
// called config.
func titlesLab(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-titles-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	config := filepath.Join(dir, "config")
	if err := os.Mkdir(config, 0o755); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestPanesAreTitledByWhatTheyAreForInTmuxItself(t *testing.T) {
	config := titlesLab(t)
	host := session.Here()
	host.Shell = "sh"
	if err := host.Ensure("titled", config); err != nil {
		t.Fatal(err)
	}
	for _, w := range []struct {
		name string
		what session.What
		argv []string
	}{
		{"claude", session.What{Kind: session.KindAgent, Label: "claude"}, []string{"sleep", "300"}},
		{"notes", session.What{Kind: session.KindEditor, Label: "nvim · notes.md"}, []string{"sleep", "300"}},
		{"api", session.What{Kind: session.KindServer, Label: "api :8080"}, []string{"sleep", "300"}},
		{"check", session.What{Kind: session.KindTests, Label: "tests"}, []string{"sleep", "300"}},
		{"plain", session.What{Kind: session.KindShell}, []string{"sh"}},
	} {
		if err := host.StartWindow("titled", config, w.name, w.what, w.argv); err != nil {
			t.Fatal(err)
		}
	}
	border := styleTmux(t, "show-options", "-wv", "-t", session.Target("titled", "claude"), "pane-border-format")
	want := map[string]string{"claude": "claude", "notes": "nvim · notes.md", "api": "api :8080", "check": "tests", "plain": "shell · config"}
	for name, title := range want {
		got := styleTmux(t, "display-message", "-p", "-t", session.Target("titled", name), border)
		if strings.TrimSpace(got) != title {
			t.Errorf("window %s's pane is titled %q, want %q", name, got, title)
		}
	}
	if got := styleTmux(t, "display-message", "-p", "-t", session.Target("titled", "api"), "#{@mia_kind}"); got != "server" {
		t.Errorf("the api pane is marked %q, want server", got)
	}

	// a window opened in tmux by hand is named as its pane is titled; one a
	// person renames keeps the name, as tmux turns its own naming off
	styleTmux(t, "new-window", "-d", "-t", session.Target("titled", ""), "-c", config, "tail -n +1 -f /etc/hosts")
	styleTmux(t, "new-window", "-d", "-t", session.Target("titled", ""), "-c", config, "-P", "-F", "#{window_id}")
	named := styleTmux(t, "new-window", "-d", "-t", session.Target("titled", ""), "-c", config, "-P", "-F", "#{window_id}")
	styleTmux(t, "rename-window", "-t", named, "mine")
	if err := host.Style("titled"); err != nil {
		t.Fatal(err)
	}
	var names string
	for range 100 { // tmux names a window on its next look
		names = styleTmux(t, "list-windows", "-t", session.Target("titled", ""), "-F", "#{window_name}")
		if strings.Contains(names, "tail · config") && strings.Count(names, "shell · config") >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(names, "tail · config") || strings.Count(names, "shell · config") < 2 {
		t.Errorf("windows tmux names are named as their panes are titled:\n%s", names)
	}
	if got := styleTmux(t, "display-message", "-p", "-t", named, "#{window_name} #{automatic-rename}"); got != "mine 0" {
		t.Errorf("a window named by hand keeps its name: %q", got)
	}
	for _, name := range []string{"claude", "notes", "api", "check", "plain"} {
		if !strings.Contains(names, name+"\n") && !strings.HasSuffix(names, name) {
			t.Errorf("window %s that mia named keeps its name:\n%s", name, names)
		}
	}
}

func TestAnAgentIsKnownByItsMarkBeforeItsBinarysName(t *testing.T) {
	w := session.WindowsFrom([]session.Pane{{Index: 1, Window: "2.1.316", Command: "2.1.316", Kind: session.KindAgent, Label: "codex", Title: "host"}})
	if w[0].Name != "codex" || w[0].Command != "codex" || w[0].Kind != session.KindAgent {
		t.Errorf("a pane marked as an agent runs that agent: %+v", w[0])
	}
	w = session.WindowsFrom([]session.Pane{{Index: 1, Window: "2.1.316", Command: "2.1.316"}})
	if w[0].Name != "claude" || w[0].Command != "claude" {
		t.Errorf("one with no mark is still known by its binary: %+v", w[0])
	}
}
