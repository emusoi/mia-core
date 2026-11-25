package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A shell at rest is brought to where the popup was opened: zsh keeps what was typed (pushed
// aside), another shell's line is cleared; one there already, or running a program, is let be.
func TestComeLineBringsAShellAtRestAndLetsAProgramBe(t *testing.T) {
	for _, c := range []struct{ dir, runs, to, want string }{
		{"/r/other", "zsh", "/r/it's", "\x11 cd -- '/r/it'\\''s'"},
		{"/r/other", "-zsh", "/r/here", "\x11 cd -- '/r/here'"},
		{"/r/other", "bash", "/r/here", "\x15 cd -- '/r/here'"},
		{"/r/here", "zsh", "/r/here", ""},
		{"/r/other", "nvim", "/r/here", ""},
		{"/r/other", "claude", "/r/here", ""},
	} {
		if got := comeLine(c.dir, c.runs, c.to); got != c.want {
			t.Errorf("comeLine(%q, %q, %q) = %q, want %q", c.dir, c.runs, c.to, got, c.want)
		}
	}
}

// A client's trail: a session come to is the newest and what was ahead is let go; the one it
// is at already (a walk brought it there) changes nothing; the oldest go past the most kept.
func TestTrailKeepsWhereAClientHasBeen(t *testing.T) {
	tr := trail{}.seen("a").seen("b").seen("c")
	if !reflect.DeepEqual(tr, trail{Sessions: []string{"a", "b", "c"}, At: 2}) {
		t.Fatalf("three seen = %+v", tr)
	}
	tr.At = 0 // walked back twice
	if same := tr.seen("a"); !reflect.DeepEqual(same, tr) {
		t.Fatalf("come to where it is at = %+v", same)
	}
	if next := tr.seen("d"); !reflect.DeepEqual(next, trail{Sessions: []string{"a", "d"}, At: 1}) {
		t.Fatalf("gone elsewhere from the middle = %+v", next)
	}
	long := trail{}
	for i := 0; i < trailKeep+5; i++ {
		long = long.seen(strings.Repeat("s", i+1))
	}
	if len(long.Sessions) != trailKeep || long.At != trailKeep-1 || len(long.Sessions[0]) != 6 {
		t.Fatalf("past the most kept: %d sessions, at %d, oldest %q", len(long.Sessions), long.At, long.Sessions[0])
	}
}

// The agents' menu: an agent each on a letter of its own, opening its models; an agent's
// models, the first "as it is set", each moving the window's conversation or starting it beside.
func TestAgentsMenuIsALetterEachThenItsModels(t *testing.T) {
	agents := []agentKnown{{Name: "claude", Installed: true}, {Name: "codex", Installed: true}}
	agents[0].Models = append(agents[0].Models, struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}{"opus", "Opus"})
	first := agentsMenu("mia", "c1", "hand", "3", nil, agents)
	if first[2] != "claude" || first[3] != "c" || first[5] != "codex" || first[6] != "o" || !strings.Contains(first[4], "tmux agents c1 hand 3 claude") {
		t.Fatalf("the agents = %q", first)
	}
	models := agentsMenu("mia", "c1", "hand", "3", []string{"claude"}, agents)
	if models[2] != "as it is set" || !strings.Contains(models[4], "hand --window 3 claude'") || models[6] != "Opus" || !strings.Contains(models[8], "hand --window 3 claude --model opus") {
		t.Fatalf("claude's models = %q", models)
	}
	if beside := agentsMenu("mia", "c1", "new", "3", []string{"codex"}, agents); !strings.Contains(beside[4], "window new codex'") {
		t.Fatalf("a new window = %q", beside)
	}
}

// The terminal app's lines send the same key each of tmux's user keys is, by its number.
func TestTerminalKeysSendWhatTmuxBinds(t *testing.T) {
	snippet := tmuxSnippet("mia")
	for _, app := range []string{"kitty", "ghostty"} {
		lines, err := terminalKeys(app)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
			sent := line[strings.Index(line, "[99;"):]
			if !strings.Contains(snippet, `"\e`+sent+`"`) {
				t.Errorf("%s sends %q, which tmux binds nothing to: %s", app, sent, line)
			}
		}
	}
	if lines, _ := terminalKeys("kitty"); !strings.Contains(lines, `map cmd+k send_text all \x1b[99;1~`) {
		t.Errorf("kitty's ⌘K: %s", lines)
	}
	if _, err := terminalKeys("nosuch"); err == nil {
		t.Error("an app mia has no lines for")
	}
}

// tmux reads every line of what `mia tmux install` adds: a server of the test's own, on its
// own socket, sources it and has the keys bound.
func TestTmuxReadsWhatInstallAdds(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	dir := t.TempDir()
	conf := filepath.Join(dir, "mia.conf")
	if err := os.WriteFile(conf, []byte(tmuxSnippet("/usr/local/bin/mia")), 0o644); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	tmux := func(args ...string) (string, error) {
		cmd := exec.Command("tmux", append([]string{"-S", socket, "-f", "/dev/null"}, args...)...)
		cmd.Env = append(os.Environ(), "TMUX=")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := tmux("new-session", "-d", "-s", "t", "sleep 60"); err != nil {
		t.Fatalf("a server of the test's own: %s", out)
	}
	defer tmux("kill-server")
	if out, err := tmux("source-file", conf); err != nil || strings.TrimSpace(out) != "" {
		t.Fatalf("tmux refused mia's block: %s %v", out, err)
	}
	keys, _ := tmux("list-keys")
	for _, want := range []string{"tmux shell", "tmux act", "agents '#{client_name}' hand", "tmux walk", "User40", "User62"} {
		if !strings.Contains(keys, want) {
			t.Errorf("no key bound for %q", want)
		}
	}
	if hooks, _ := tmux("show-hooks", "-g"); !strings.Contains(hooks, "tmux seen") {
		t.Errorf("no hook for where a client has been: %s", hooks)
	}
}
