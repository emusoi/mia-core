package app

import "testing"

func TestAnAgentContinuesItsConversation(t *testing.T) {
	for in, want := range map[string]string{
		"claude":              "claude --continue",
		"claude --model opus": "claude --model opus --continue",
		"claude --resume abc": "claude --resume abc",
		"codex":               "codex resume --last",
		"npm run dev":         "npm run dev",
		"":                    "",
	} {
		if got := again(in, ""); got != want {
			t.Errorf("again(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"claude":                        "claude --resume 23f98c02",
		"claude --model opus -c":        "claude --model opus --resume 23f98c02",
		"claude --resume old --verbose": "claude --verbose --resume 23f98c02",
	} {
		if got := again(in, "23f98c02"); got != want {
			t.Errorf("again(%q, conversation) = %q, want %q", in, got, want)
		}
	}
}
