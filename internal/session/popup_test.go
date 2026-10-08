package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/run"
)

func TestPopupProcess(t *testing.T) {
	if os.Getenv("MIA_TEST_POPUP") != "1" {
		return
	}
	other := filepath.Join(os.Getenv("MIA_TEST_POPUP_DIR"), "unused")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", other)
	if err := Here().AttachPopup("fixture", os.Getenv("MIA_TEST_POPUP_DIR")); err != nil {
		os.WriteFile(filepath.Join(os.Getenv("MIA_TEST_POPUP_DIR"), "popup-error"), []byte(err.Error()), 0o600)
		t.Fatal(err)
	}
}

func TestPopupLinksOnlySafeWindowsAndCleansUp(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-popup-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("MIA_TEST_POPUP", "1")
	t.Setenv("MIA_TEST_POPUP_DIR", dir)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %s", args, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	until := func(why string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		detail, _ := os.ReadFile(filepath.Join(dir, "popup-error"))
		t.Fatalf("%s: %s", why, detail)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tmux("-f", "/dev/null", "new-session", "-d", "-s", "unrelated")
	editor := tmux("new-session", "-d", "-P", "-F", "#{window_id}", "-s", "mia-fixture", "-n", "editor", "-x", "110", "-y", "34", binary, "-test.run=^TestPopupProcess$")
	view := ""
	attached := func() bool {
		for line := range strings.SplitSeq(tmux("list-clients", "-F", "#{session_name}"), "\n") {
			if strings.HasPrefix(line, "mia-popup-") {
				view = "=" + line
				return true
			}
		}
		return false
	}
	until("popup did not attach", attached)
	shell := tmux("display-message", "-p", "-t", view+":", "#{window_id}")
	if shell == editor || tmux("display-message", "-p", "-t", "=mia-fixture:", "#{window_id}") != editor {
		t.Fatal("popup selected or moved the outer editor")
	}
	if tmux("list-windows", "-t", "=mia-fixture:", "-F", "#{window_id}") != editor+"\n"+shell {
		t.Fatal("popup shell is not in the real worktree session")
	}
	if got := tmux("list-windows", "-t", view+":", "-F", "#{window_id}"); got != shell {
		t.Fatalf("popup exposed the outer editor window: %s", got)
	}
	created := tmux("new-window", "-d", "-P", "-F", "#{window_id}", "-t", view+":", "-n", "created")
	until("popup-created window did not persist in the worktree session", func() bool {
		out, err := exec.Command("tmux", "display-message", "-p", "-t", "=mia-fixture:"+created, "#{window_id}").CombinedOutput()
		return err == nil && strings.TrimSpace(string(out)) == created
	})
	if tmux("display-message", "-p", "-t", "=mia-fixture:", "#{window_id}") != editor {
		t.Fatal("creating a popup window changed the outer session selection")
	}
	proof := filepath.Join(dir, "proof")
	tmux("send-keys", "-t", view+":", "MIA_POPUP_TEST=alive", "Enter")
	tmux("detach-client", "-s", view)
	until("detached popup left a temporary session", func() bool {
		return !attached() && !strings.Contains(tmux("list-sessions", "-F", "#{session_name}"), "mia-popup-")
	})
	tmux("send-keys", "-t", "=mia-fixture:"+shell, "printf '%s' \"$MIA_POPUP_TEST\" > "+run.ShellJoin([]string{proof}), "Enter")
	until("popup close lost the running shell", func() bool { value, _ := os.ReadFile(proof); return string(value) == "alive" })

	editor = tmux("new-window", "-P", "-F", "#{window_id}", "-t", "=mia-fixture:", "-n", "editor", binary, "-test.run=^TestPopupProcess$")
	until("popup did not reopen", attached)
	selected := tmux("display-message", "-p", "-t", view+":", "#{window_id}")
	if selected == editor {
		t.Fatal("reopening selected the outer editor")
	}
	visible := strings.Fields(tmux("list-windows", "-t", view+":", "-F", "#{window_id}"))
	if len(visible) != 2 || !slices.Contains(visible, shell) || !slices.Contains(visible, created) {
		detail := tmux("list-windows", "-t", view+":", "-F", "#{window_id}\t#{window_name}\t#{pane_current_command}\t#{window_active}")
		t.Fatalf("reopening did not preserve exactly the safe windows: %v\n%s", visible, detail)
	}
	if out, err := exec.Command("tmux", "select-window", "-t", view+":"+editor).CombinedOutput(); err == nil {
		t.Fatalf("popup unexpectedly allowed selecting the editor: %s", out)
	}
	tmux("detach-client", "-s", view)
	until("second popup did not close", func() bool { return !attached() })

	tmux("new-window", "-t", "=mia-fixture:", "-n", "editor", binary, "-test.run=^TestPopupProcess$")
	until("third popup did not attach", attached)
	if err := Here().Kill("fixture"); err != nil {
		t.Fatal(err)
	}
	if got := tmux("list-sessions", "-F", "#{session_name}"); got != "unrelated" {
		t.Fatalf("removal left popup windows or affected unrelated sessions: %s", got)
	}
}
