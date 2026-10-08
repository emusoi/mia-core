package session

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func captureUntil(c *control, args []string, want string) (string, error) {
	var out string
	var err error
	for range 40 {
		out, err = c.run(args)
		if err != nil || strings.Contains(out, want) {
			return out, err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return out, err
}

func TestControlClientAnswersLikeTheCommandLine(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-control-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	exec.Command("tmux", "new-session", "-d", "-s", "mia-probe", "-x", "40", "-y", "5", "sh -c 'printf \"hello control \\033[31mred\\033[0m\\n\"; sleep 60'").Run()
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })

	c, err := startControl()
	if err != nil {
		t.Fatal(err)
	}
	defer c.cmd.Process.Kill()
	out, err := c.run([]string{"list-panes", "-a", "-F", "#{session_name} #{window_name}"})
	if err != nil || !strings.Contains(out, "mia-probe") {
		t.Fatalf("list-panes through the control client: %q %v", out, err)
	}
	text, err := captureUntil(c, []string{"capture-pane", "-p", "-t", "=mia-probe:"}, "hello control")
	if err != nil || !strings.Contains(text, "hello control") {
		t.Fatalf("capture-pane through the control client: %q %v", text, err)
	}
	coloured, err := captureUntil(c, []string{"capture-pane", "-p", "-e", "-t", "=mia-probe:"}, "\x1b[31m")
	if err != nil || !strings.Contains(coloured, "\x1b[31m") {
		t.Fatalf("colour escapes must survive the control client: %q %v", coloured, err)
	}
	if _, err := c.run([]string{"has-session", "-t", "=nowhere"}); err == nil {
		t.Fatal("a failing command must fail through the control client too")
	}
	if out, err := c.run([]string{"display-message", "-p", "it's #{session_name}"}); err != nil || !strings.Contains(out, "it's") {
		t.Fatalf("quoting: %q %v", out, err)
	}
}

func TestControlSessionOutlivesItsClient(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-control-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })

	c, err := startControl()
	if err != nil {
		t.Fatal(err)
	}
	c.cmd.Process.Kill()
	time.Sleep(300 * time.Millisecond)
	if err := exec.Command("tmux", "has-session", "-t", exact(controlSession)).Run(); err != nil {
		t.Fatal("the control session left with its client — destroy-unattached segfaults the tmux 3.6a server")
	}
}
