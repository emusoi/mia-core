package session

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestAnAnswerGoesToItsPaneOnlyWhileTheWorktreeHoldsIt(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-answer-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	for _, s := range []string{"mia-kijenge", "mia-other"} {
		if out, err := exec.Command("tmux", "new-session", "-d", "-s", s, "cat").CombinedOutput(); err != nil {
			t.Fatalf("%s: %s", s, out)
		}
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	pane := func(s string) string {
		out, _ := exec.Command("tmux", "display-message", "-p", "-t", "="+s+":", "#{pane_id}").Output()
		return strings.TrimSpace(string(out))
	}
	h := Here()
	if err := h.Answer("kijenge", pane("mia-kijenge"), "go on", true); err != nil {
		t.Fatal(err)
	}
	if err := h.Answer("kijenge", pane("mia-other"), "nope", true); err == nil {
		t.Error("a pane of another session must be refused")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		screen, _ := h.Capture("kijenge", "", false)
		if strings.Count(screen, "go on") == 2 { // typed, then cat's echo after Enter
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the reply and its Enter did not arrive: %q", screen)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if screen, _ := h.Capture("other", "", false); strings.Contains(screen, "nope") {
		t.Errorf("nothing typed into the other pane: %q", screen)
	}
}
