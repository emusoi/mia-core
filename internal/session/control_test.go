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

func TestAServerMiaStartsKeepsNoPluginCallVars(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	for _, name := range pluginCall {
		t.Setenv(name, "leaked")
	}
	// (What the caller has does reach it: on macOS the server is launchd's, whose own is bare.)
	t.Setenv("MIA_CALLER", "kept")
	starts := map[string]func() error{
		"command line": func() error {
			_, err := Host{}.local("new-session", "-d", "-s", "mia-probe", "sleep 60")
			return err
		},
		"control client": func() error {
			c, err := startControl()
			if err == nil {
				c.cmd.Process.Kill()
			}
			return err
		},
	}
	for how, start := range starts {
		dir, err := os.MkdirTemp("/tmp", "mia-env-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		t.Setenv("TMUX_TMPDIR", dir)
		t.Setenv("TMUX", "")
		if err := start(); err != nil {
			t.Fatalf("%s: %v", how, err)
		}
		out, err := exec.Command("tmux", "show-environment", "-g").Output()
		for _, session := range []string{"mia-probe", controlSession} {
			exec.Command("tmux", "kill-session", "-t", exact(session)).Run()
		}
		if err != nil {
			t.Fatalf("%s: show-environment: %v", how, err)
		}
		for _, name := range pluginCall {
			if strings.Contains(string(out), name+"=") {
				t.Errorf("a server started by the %s kept %s:\n%s", how, name, out)
			}
		}
		if !strings.Contains(string(out), "MIA_CALLER=kept") {
			t.Errorf("a server started by the %s lost the caller's environment:\n%s", how, out)
		}
	}
}

func TestANodePaneTitledPiIsPi(t *testing.T) {
	for _, c := range []struct {
		command, title string
		want           bool
	}{
		{"node", "π - shop", true},
		{"node", "pi - shop:🚧", true},
		{"node", "pipeline - shop", false},
		{"bun", " π - shop", true},
		{"node", "shop", false},    // any other node program
		{"zsh", "π - shop", false}, // pi quit and left its title on the shell
		{"claude", "π - shop", false},
	} {
		if got := piTitle(c.command, c.title); got != c.want {
			t.Errorf("piTitle(%q, %q) = %v, want %v", c.command, c.title, got, c.want)
		}
	}
}
