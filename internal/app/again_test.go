package app

import "testing"

func TestAnAgentContinuesItsConversation(t *testing.T) {
	for in, want := range map[string]string{
		"claude":              "claude --continue",
		"claude --model opus": "claude --model opus --continue",
		"claude --resume abc": "claude --resume abc",
		"codex":               "codex resume --last",
		"npm run dev":         "npm run dev",
		"node server.js":      "node server.js",
		// as ps shows them: behind node, by one version's path, with a launcher's arguments
		"/opt/codex/vendor/bin/codex":        "codex resume --last",
		"/opt/codex/vendor/bin/codex --yolo": "codex --yolo resume --last",
		"codex resume 23f9":                  "codex resume 23f9",
		"codex fix it":                       "codex fix it",
		"pi":                                 "pi --continue",
		"pi --session 23f9":                  "pi --session 23f9",
		"opencode":                           "opencode --continue",
		"/usr/local/bin/cursor-agent --use-system-ca /opt/cursor-agent/index.js": "cursor-agent --continue",
		"": "",
	} {
		if got := again(builtin, in, ""); got != want {
			t.Errorf("again(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"claude":                        "claude --resume 23f98c02",
		"claude --model opus -c":        "claude --model opus --resume 23f98c02",
		"claude --resume old --verbose": "claude --verbose --resume 23f98c02",
	} {
		if got := again(builtin, in, "23f98c02"); got != want {
			t.Errorf("again(%q, conversation) = %q, want %q", in, got, want)
		}
	}
	// what again gives, given again, stays as it is
	for _, in := range []string{"claude", "codex", "pi", "opencode", "cursor-agent"} {
		if once := again(builtin, in, ""); again(builtin, once, "") != once {
			t.Errorf("again(%q) twice = %q, want %q", in, again(builtin, once, ""), once)
		}
	}
}

func TestWhatStaysIsKeptAndWhatFinishesIsNot(t *testing.T) {
	for in, want := range map[string]string{
		"claude --model opus":        "claude",
		"node /usr/local/bin/cursor-agent": "cursor-agent",
		"nvim main.go":               "nvim",
		"/opt/homebrew/bin/nvim":     "nvim",
		"npm run dev":                "npm",
		"npm run build":              "",
		"git push":                   "",
		"make":                       "",
		"tail -f log/dev.log":        "tail",
		"tail":                       "",
		"":                           "",
	} {
		if got := stays(builtin, in, []string{"tail -f"}); got != want {
			t.Errorf("stays(%q) = %q, want %q", in, got, want)
		}
	}
}
