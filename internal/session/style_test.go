package session_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/session"
)

func styleTmux(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %q: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

func TestStyleOnlyAffectsMiaSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-style-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	styleTmux(t, "-f", "/dev/null", "new-session", "-d", "-s", "unrelated", "sleep 60")
	styleTmux(t, "set-option", "-g", "default-command", "sleep 60")
	styleTmux(t, "set-option", "-g", "status-style", "fg=colour1,bg=colour0")
	styleTmux(t, "set-option", "-gw", "pane-border-style", "fg=colour2")
	styleTmux(t, "set-option", "-gw", "window-status-format", "ORIGINAL")
	styleTmux(t, "new-session", "-d", "-s", session.Name("existing"), "sleep 60")

	// The gateway uses a persistent tmux control client for ordinary commands.
	session.UseControl()
	host := session.Here()
	if err := host.Ensure("existing", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := host.Ensure("fresh", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"existing", "fresh"} {
		target := session.Target(name, "")
		if got := styleTmux(t, "show-options", "-t", target, "-v", "status-style"); got != "fg=default,bg=default" {
			t.Errorf("%s status style = %q", name, got)
		}
		if got := styleTmux(t, "show-options", "-w", "-t", target, "-v", "pane-border-style"); got != "fg=colour8,bg=default" {
			t.Errorf("%s pane border = %q", name, got)
		}
		if got := styleTmux(t, "show-options", "-t", target, "-v", "window-status-format"); !strings.Contains(got, "colour8") || !strings.Contains(got, "#I:#W#F") {
			t.Errorf("%s window status format = %q", name, got)
		}
		if got := styleTmux(t, "show-options", "-t", target, "-v", "window-status-current-format"); !strings.Contains(got, "fg=default,bold") || strings.Contains(got, "colour4") {
			t.Errorf("%s current window status format = %q", name, got)
		}
		if got := styleTmux(t, "show-options", "-t", target, "-v", "status-left"); !strings.Contains(got, "fg=default,bold") || strings.Contains(got, "colour4") {
			t.Errorf("%s status left = %q", name, got)
		}
	}

	styleTmux(t, "new-window", "-d", "-t", session.Target("existing", ""), "-n", "later", "sleep 60")
	if err := host.Style("existing"); err != nil {
		t.Fatal(err)
	}
	if got := styleTmux(t, "show-options", "-w", "-t", session.Target("existing", "later"), "-v", "pane-border-style"); got != "fg=colour8,bg=default" {
		t.Errorf("new window pane border = %q", got)
	}
	if err := host.StartWindow("fresh", t.TempDir(), "chat", []string{"sleep", "60"}); err != nil {
		t.Fatal(err)
	}
	if got := styleTmux(t, "show-options", "-w", "-t", session.Target("fresh", "chat"), "-v", "pane-border-style"); got != "fg=colour8,bg=default" {
		t.Errorf("Mia-created window pane border = %q", got)
	}

	if got := styleTmux(t, "show-options", "-g", "-v", "status-style"); got != "fg=colour1,bg=colour0" {
		t.Errorf("global status changed to %q", got)
	}
	if got := styleTmux(t, "show-options", "-gw", "-v", "pane-border-style"); got != "fg=colour2" {
		t.Errorf("global border changed to %q", got)
	}
	if got := styleTmux(t, "show-options", "-gw", "-v", "window-status-format"); got != "ORIGINAL" {
		t.Errorf("global window status format changed to %q", got)
	}
	if got := styleTmux(t, "show-options", "-q", "-t", "=unrelated:", "status-style"); got != "" {
		t.Errorf("unrelated session received local status style %q", got)
	}
	if got := styleTmux(t, "show-options", "-wq", "-t", "=unrelated:", "pane-border-style"); got != "" {
		t.Errorf("unrelated window received local pane border %q", got)
	}
	if got := styleTmux(t, "show-options", "-wq", "-t", "=unrelated:", "window-status-format"); got != "" {
		t.Errorf("unrelated window received local status format %q", got)
	}
}

func TestConfiguredShellIsLocalToMiaSession(t *testing.T) {
	shell, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-shell-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	styleTmux(t, "-f", "/dev/null", "new-session", "-d", "-s", "unrelated", "sleep 60")
	styleTmux(t, "set-option", "-g", "default-shell", "/bin/sh")
	styleTmux(t, "new-session", "-d", "-s", session.Name("existing"), "sleep 60")
	styleTmux(t, "set-option", "-t", session.Target("existing", ""), "default-command", "sleep 60")

	host := session.Host{Shell: "zsh"}
	if err := host.Ensure("existing", dir); err != nil {
		t.Fatal(err)
	}
	if err := host.Ensure("fresh", dir); err != nil {
		t.Fatal(err)
	}
	if err := host.StartWindow("fresh", dir, "later", nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"existing", "fresh"} {
		if got := styleTmux(t, "show-options", "-q", "-t", session.Target(name, ""), "-v", "default-shell"); got != shell {
			t.Errorf("%s shell = %q, want %q", name, got, shell)
		}
	}
	for _, window := range []string{"0", "later"} {
		got := ""
		for deadline := time.Now().Add(3 * time.Second); got != "zsh" && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			got = styleTmux(t, "display-message", "-p", "-t", session.Target("fresh", window), "#{pane_current_command}")
		}
		if got != "zsh" {
			t.Errorf("%s command = %q, want zsh", window, got)
		}
	}
	if got := styleTmux(t, "show-options", "-t", session.Target("existing", ""), "-v", "default-command"); got != "sleep 60" {
		t.Errorf("existing environment command changed to %q", got)
	}
	if got := styleTmux(t, "show-options", "-g", "-v", "default-shell"); got != "/bin/sh" {
		t.Errorf("global default shell changed to %q", got)
	}
	if got := styleTmux(t, "show-options", "-q", "-t", "=unrelated:", "-v", "default-shell"); got != "" {
		t.Errorf("unrelated session received local default shell %q", got)
	}
}

func TestConfiguredShellResolvesOnRemoteHost(t *testing.T) {
	dir := t.TempDir()
	log := dir + "/ssh.log"
	ssh := `#!/bin/sh
for arg do command=$arg; done
printf '%s\n' "$command" >> "$MIA_TEST_SSH_LOG"
case "$command" in
  *"command -v"*) printf '/usr/bin/zsh\n' ;;
  *"has-session"*) exit 1 ;;
  *"list-windows"*) printf '%%7\n' ;;
esac
`
	if err := os.WriteFile(dir+"/ssh", []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("MIA_TEST_SSH_LOG", log)
	if err := (session.Host{SSH: "remote.example", Shell: "zsh"}).Ensure("remote", "/home/koala/repo"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var resolved, firstPane, localOption bool
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		resolved = resolved || strings.Contains(line, "command -v") && strings.Contains(line, "zsh")
		firstPane = firstPane || strings.Contains(line, "'new-session'") && strings.Contains(line, "/usr/bin/zsh")
		localOption = localOption || strings.Contains(line, "'set-option' '-t' '=mia-remote:' 'default-shell' '/usr/bin/zsh'")
		if strings.Contains(line, "'-g'") || strings.Contains(line, "'-f'") {
			t.Errorf("remote session shell touched global tmux scope or config: %q", line)
		}
	}
	if !resolved || !firstPane || !localOption {
		t.Errorf("remote shell setup missing resolution=%t firstPane=%t localOption=%t: %q", resolved, firstPane, localOption, data)
	}
}

func TestStyleDispatchesToRemoteTmux(t *testing.T) {
	dir := t.TempDir()
	log := dir + "/ssh.log"
	ssh := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MIA_TEST_SSH_LOG\"\ncase \"$*\" in\n  *list-windows*) printf '%%7\\n' ;;\nesac\n"
	if err := os.WriteFile(dir+"/ssh", []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("MIA_TEST_SSH_LOG", log)
	if err := (session.Host{SSH: "remote.example"}).Style("remote"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("remote tmux calls = %d, want session style, list windows, window style: %q", len(lines), data)
	}
	if !strings.Contains(lines[0], "'set-option' '-t' '=mia-remote:'") || !strings.Contains(lines[0], "';' 'set-option'") {
		t.Errorf("session options were not batched for the remote Mia session: %q", lines[0])
	}
	if !strings.Contains(lines[1], "'list-windows' '-t' '=mia-remote:'") {
		t.Errorf("remote windows were not listed by exact Mia session: %q", lines[1])
	}
	if !strings.Contains(lines[2], "'set-option' '-w' '-t' '%7'") {
		t.Errorf("window options were not sent to the remote tmux window: %q", lines[2])
	}
	for _, line := range lines {
		if strings.Contains(line, "'-g'") || strings.Contains(line, "'-f'") {
			t.Errorf("remote style touched global tmux scope or config: %q", line)
		}
	}
}
