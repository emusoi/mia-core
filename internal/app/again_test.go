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
		if got := again(in); got != want {
			t.Errorf("again(%q) = %q, want %q", in, got, want)
		}
	}
}
