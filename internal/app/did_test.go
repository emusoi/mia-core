package app

import (
	"reflect"
	"testing"
)

func TestACommandLineIsSplitAsAShellWouldForThePlainCase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for line, want := range map[string][][]string{
		`mia new fix-login`: {{"mia", "new", "fix-login"}},
		`cd ~/src/app && mia agent send olasiti -- 'a "b"'`:  {{"cd", home + "/src/app"}, {"mia", "agent", "send", "olasiti", "--", `a "b"`}},
		`mia window send x claude -- "it's \"so\"" and\ on`:  {{"mia", "window", "send", "x", "claude", "--", `it's "so"`, "and on"}},
		`mia window send x claude -- ''`:                     {{"mia", "window", "send", "x", "claude", "--", ""}},
		`mia new fix | tail -1`:                              nil,
		`mia new fix > out`:                                  nil,
		`mia new "$BRANCH"`:                                  nil,
		`mia new $(date +%s)`:                                nil,
		`mia new fix; mia new other`:                         nil,
		"mia new fix\nmia new other":                         nil,
		`mia new 'fix`:                                       nil,
		`&& mia new fix`:                                     nil,
		`mia agent send olasiti -- 'costs $5; fine' && true`: {{"mia", "agent", "send", "olasiti", "--", "costs $5; fine"}, {"true"}},
	} {
		got, ok := shellCommands(line)
		if ok != (want != nil) || !reflect.DeepEqual(got, want) {
			t.Errorf("%s\n got %q, %v\nwant %q", line, got, ok, want)
		}
	}
}

func TestAMiaVerbTheMainAgentRanIsSaidAsWhatItDid(t *testing.T) {
	a := &App{Root: "/home/user/.config/mia/main"}
	where := func(dir string) place {
		switch dir {
		case "/src/mia-desk":
			return place{repository: "mia-desk", worktree: "mia-desk"}
		case "/src/mia-desk.olasiti":
			return place{repository: "mia-desk", worktree: "olasiti"}
		}
		return place{}
	}
	type did struct{ kind, verb, text, repository, worktree, machine string }
	for _, c := range []struct {
		cwd, command string
		want         did
	}{
		{"", `cd /src/mia-desk && mia new fix-login`, did{"did", "new", "Made a worktree for fix-login on mia-desk", "mia-desk", "", ""}},
		{"", `cd /src/mia-desk && mia new fix-login --name goba --on fedora`, did{"did", "new", "Made a worktree for fix-login on mia-desk, on fedora", "mia-desk", "goba", "fedora"}},
		{"", `cd /src/mia-desk && mia agent run goba claude`, did{"did", "agent run", "Started goba on mia-desk · claude", "mia-desk", "goba", ""}},
		{"", `cd /src/mia-desk && mia agent run --task 'fix the login page' goba`, did{"did", "agent run", "Started goba on mia-desk", "mia-desk", "goba", ""}},
		{"", `cd /src/mia-desk && mia window new goba codex -- codex 'fix the login page'`, did{"did", "window new", "Started goba on mia-desk · codex", "mia-desk", "goba", ""}},
		{"", `cd /src/mia-desk && mia agent send olasiti claude -- rebase onto main`, did{"did", "agent send", "Told olasiti: rebase onto main", "mia-desk", "olasiti", ""}},
		{"", `cd /src/mia-desk && mia window send olasiti@fedora claude -- 'rebase onto main'`, did{"did", "window send", "Told olasiti: rebase onto main", "mia-desk", "olasiti", "fedora"}},
		{"", `cd /src/mia-desk && mia window press kijenge claude -- 1`, did{"did", "window press", "Answered kijenge: 1", "mia-desk", "kijenge", ""}},
		{"", `cd /src/mia-desk && mia move sakina fedora`, did{"did", "move", "Moved sakina to fedora", "mia-desk", "sakina", "fedora"}},
		{"", `cd /src/mia-desk && mia move sakina@fedora local`, did{"did", "move", "Moved sakina back here", "mia-desk", "sakina", "local"}},
		{"", `cd /src/mia-desk && mia agent stop goba`, did{"did", "agent stop", "Stopped the agent in goba", "mia-desk", "goba", ""}},
		{"", `cd /src/mia-desk && mia rm --force goba`, did{"did", "rm", "Removed goba on mia-desk", "mia-desk", "goba", ""}},
		{"", `cd /src/mia-desk && mia archive goba`, did{"did", "archive", "Archived goba on mia-desk", "mia-desk", "goba", ""}},
		// typed in the worktree, which the command then need not name; and in the folder the agent was in
		{"", `cd /src/mia-desk.olasiti && mia agent send claude -- keep going`, did{"did", "agent send", "Told olasiti: keep going", "mia-desk", "olasiti", ""}},
		{"/src/mia-desk.olasiti", `/usr/local/bin/mia agent send -- 2`, did{"did", "agent send", "Answered olasiti: 2", "mia-desk", "olasiti", ""}},
		// a worktree nobody knows the repository of is still named
		{"", `mia agent send olasiti claude -- hello`, did{"did", "agent send", "Told olasiti: hello", "", "olasiti", ""}},

		// reading, and whatever is more than one plain command, stays a tool's line
		{"", `cd /src/mia-desk && mia decisions`, did{kind: "tool"}},
		{"", `mia api standing`, did{kind: "tool"}},
		{"", `cd /src/mia-desk && mia archive ls`, did{kind: "tool"}},
		{"", `cd /src/mia-desk && mia move --dry-run sakina fedora`, did{kind: "tool"}},
		{"", `cd /src/mia-desk && mia window new goba build -- make`, did{kind: "tool"}},
		{"", `cd /src/mia-desk && mia new fix-login | tail -1`, did{kind: "tool"}},
		{"", `cd /src/mia-desk && git status && mia new fix-login`, did{kind: "tool"}},
		{"", `cd /src/mia-desk && mia new --no-such-flag fix-login`, did{kind: "tool"}},
		{"", `mia agent send -- 1`, did{kind: "tool"}}, // to whom is not known
		{"", `go test ./...`, did{kind: "tool"}},
	} {
		turn := a.did(Turn{N: 7, Kind: turnTool, Name: "Bash", Text: "Ran it", command: c.command, cwd: c.cwd}, where)
		got := did{turn.Kind, turn.Verb, turn.Text, turn.Repository, turn.Worktree, turn.Machine}
		if c.want.kind == "tool" {
			c.want.text = "Ran it"
		}
		if got != c.want || turn.N != 7 {
			t.Errorf("%s\n got %+v\nwant %+v", c.command, got, c.want)
		}
	}
	// an agent a plugin adds gives the line alone: read when whole, left when cut
	if turn := a.did(Turn{Kind: turnTool, Text: "Ran cd /src/mia-desk && mia move sakina fedora"}, where); turn.Kind != turnDid || turn.Text != "Moved sakina to fedora" {
		t.Errorf("a plugin's line: %+v", turn)
	}
	if turn := a.did(Turn{Kind: turnTool, Text: "Ran cd /src/mia-desk && mia agent send sakina -- a long…"}, where); turn.Kind != turnTool {
		t.Errorf("a line that was cut: %+v", turn)
	}
	if turn := a.did(Turn{Kind: turnSaid, Text: "Ran mia move sakina fedora"}, where); turn.Kind != turnSaid {
		t.Errorf("what the agent said: %+v", turn)
	}
}
