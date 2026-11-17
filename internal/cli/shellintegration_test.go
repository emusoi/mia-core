package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTheZshIntegrationMarksPromptsSoTmuxJumpsBetweenThem(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	lab, err := os.MkdirTemp("/tmp", "mia-prompts-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(lab) })
	t.Setenv("TMUX_TMPDIR", lab)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	snippet := filepath.Join(lab, "integration.zsh")
	if err := os.WriteFile(snippet, []byte(zshIntegration), 0o644); err != nil {
		t.Fatal(err)
	}
	// sourced twice, as a zshrc read again does
	rc := "PS1='%% '\nsource " + snippet + "\nsource " + snippet + "\n"
	if err := os.WriteFile(filepath.Join(lab, ".zshrc"), []byte(rc), 0o644); err != nil {
		t.Fatal(err)
	}
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %q: %s", args, out)
		}
		return strings.TrimRight(string(out), "\n")
	}
	raw := filepath.Join(lab, "raw")
	tmux("-f", "/dev/null", "new-session", "-d", "-s", "prompts", "-x", "80", "-y", "20", "-e", "ZDOTDIR="+lab, zsh+" -i")
	tmux("pipe-pane", "-o", "-t", "prompts", "cat >> "+raw)
	prompts := func(n int) {
		t.Helper()
		for range 250 {
			drawn := 0
			for _, line := range strings.Split(tmux("capture-pane", "-p", "-t", "prompts"), "\n") {
				if strings.HasPrefix(line, "%") {
					drawn++
				}
			}
			if drawn >= n {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("no prompt %d:\n%s", n, tmux("capture-pane", "-p", "-t", "prompts"))
	}
	prompts(1)
	for i, command := range []string{"echo one", "false", "echo three"} {
		tmux("send-keys", "-t", "prompts", command, "Enter")
		prompts(i + 2)
	}

	cursor := func() string {
		return tmux("display-message", "-p", "-t", "prompts", "#{copy_cursor_line}")
	}
	tmux("copy-mode", "-t", "prompts")
	for _, want := range []string{"% echo three", "% false", "% echo one"} {
		tmux("send-keys", "-t", "prompts", "-X", "previous-prompt")
		if got := cursor(); got != want {
			t.Errorf("previous-prompt is on %q, want %q", got, want)
		}
	}
	tmux("send-keys", "-t", "prompts", "-X", "next-prompt", "-o")
	if got := cursor(); got != "one" {
		t.Errorf("next-prompt -o is on %q, want the first command's output", got)
	}

	data, err := os.ReadFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, mark := range []string{"\x1b]133;A\x1b\\", "\x1b]133;B\x1b\\", "\x1b]133;D;0\x1b\\", "\x1b]133;D;1\x1b\\", "\x1b]7;file://"} {
		if !strings.Contains(out, mark) {
			t.Errorf("the shell wrote no %q:\n%q", mark, out)
		}
	}
	if got := strings.Count(out, "\x1b]133;C\x1b\\"); got != 3 {
		t.Errorf("each command's output is marked once, sourced twice as it was: %d marks for 3 commands", got)
	}

	// with no terminal it says nothing, and adds nothing
	quiet := exec.Command(zsh, "-f", "-i", "-c", "source "+snippet+"; whence _mia_precmd")
	if said, _ := quiet.Output(); len(said) != 0 {
		t.Errorf("with no terminal it said %q", said)
	}
}
