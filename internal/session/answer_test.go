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

func TestAnAgentsTitleSaysWhatItIsDoing(t *testing.T) {
	for title, want := range map[string]string{
		"✳ Fix the retry loop": "Fix the retry loop",
		"⠐ Fix the retry loop": "Fix the retry loop",
		"mia-core.kurasini":    "",
		"MacBookPro.lan":       "",
		"":                     "",
	} {
		if got := AgentTitle(title); got != want {
			t.Errorf("AgentTitle(%q) = %q, want %q", title, got, want)
		}
	}
	w := WindowsFrom([]Pane{{Index: 1, Window: "2.1.316", Command: "2.1.316", Title: "✳ Ship it"}})
	if w[0].Name != "claude" || w[0].Command != "claude" || w[0].Agent != "Ship it" {
		t.Errorf("claude's versioned binary is called claude: %+v", w[0])
	}
	w = WindowsFrom([]Pane{{Index: 1, Window: "2.1.316", Command: "2.1.316", Title: "resume the task"}, {Index: 2, Window: "2.1.316", Command: "2.1.316", Title: "Claude Code"}})
	if w[0].Agent != "resume the task" || w[1].Agent != "" {
		t.Errorf("a claude title unmarked is still its own; its bare name says nothing: %+v", w)
	}
}

// A pane no program has titled is titled with the machine's name by tmux,
// short or with its domain: that is not what an agent in it is doing.
func TestTheMachinesNameIsNoAgentTitle(t *testing.T) {
	for _, c := range []struct {
		title, host string
		want        bool
	}{
		{"MacBookPro.lan", "MacBookPro.lan", true},
		{"MacBookPro.lan", "MacBookPro", true},
		{"MacBookPro", "MacBookPro.local", true},
		{"macbookpro.local", "MacBookPro.lan", true},
		{"fedora", "fedora", true},
		{"fedora.home.arpa", "fedora", true},
		{"MacBookPro.lan fix the build", "MacBookPro.lan", false},
		{"MacBookProject", "MacBookPro", false},
		{"resume the task", "MacBookPro.lan", false},
		{"fedora", "MacBookPro.lan", false},
		{"", "MacBookPro.lan", false},
		{"anything", "", false},
	} {
		if got := hostTitle(c.title, c.host); got != c.want {
			t.Errorf("hostTitle(%q, %q) = %v, want %v", c.title, c.host, got, c.want)
		}
	}
	host, err := os.Hostname()
	if err != nil {
		t.Skip(err)
	}
	w := WindowsFrom([]Pane{{Index: 1, Window: "claude", Command: "claude", Title: host}})
	if w[0].Agent != "" {
		t.Errorf("a claude that has not titled its pane says nothing, not %q", w[0].Agent)
	}
}
