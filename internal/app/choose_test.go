package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/session"
)

// What starts each agent with a model, and what switches one that runs,
// is its table's: the words it takes, put after the one that runs it.
func TestAnAgentIsStartedWithWhatItHas(t *testing.T) {
	claude, _ := agentNamed(builtin, "claude")
	codex, _ := agentNamed(builtin, "codex")
	pi, _ := agentNamed(builtin, "pi")
	for _, c := range []struct {
		agent         Agent
		model, effort string
		with, to      []string
	}{
		{claude, "sonnet", "high", []string{"--model", "sonnet", "--effort", "high"}, []string{"/model sonnet", "/effort high"}},
		{codex, "gpt-5.6-luna", "xhigh", []string{"-m", "gpt-5.6-luna", "-c", "model_reasoning_effort=xhigh"}, []string{"/model gpt-5.6-luna --reasoning=xhigh"}},
		{codex, "gpt-5.6-luna", "", []string{"-m", "gpt-5.6-luna"}, []string{"/model gpt-5.6-luna"}},
		{pi, "anthropic/claude-opus-5", "max", []string{"--model", "anthropic/claude-opus-5", "--thinking", "max"}, []string{"/model anthropic/claude-opus-5", "/thinking max"}},
	} {
		with, chosen, err := choose(c.agent, c.agent.Name, c.model, c.effort)
		if err != nil || !slices.Equal(with, c.with) || chosen != (Chosen{Model: c.model, Effort: c.effort}) {
			t.Errorf("%s started with %s %s: %v, %+v, %v", c.agent.Name, c.model, c.effort, with, chosen, err)
		}
		if to := c.agent.to(c.model, c.effort); !slices.Equal(to, c.to) {
			t.Errorf("%s switched to %s %s types %q", c.agent.Name, c.model, c.effort, to)
		}
	}

	// the configured one when none is given, unless the command says one; one it has not is refused with what it has
	set := config.Agent{Model: "opus", Effort: "low"}
	if with, chosen, _ := choose(claude, "claude --effort max", "", "", set); !slices.Equal(with, []string{"--model", "opus"}) || chosen != (Chosen{Model: "opus"}) {
		t.Errorf("configured: %v, %+v", with, chosen)
	}
	if _, _, err := choose(claude, "claude", "gpt-5.6-luna", ""); err == nil || err.Error() != "claude has no model gpt-5.6-luna — it has opus, sonnet, haiku, fable" {
		t.Errorf("a model it has not: %v", err)
	}
	if _, _, err := choose(claude, "claude", "", "turbo"); err == nil || !strings.HasSuffix(err.Error(), "it has low, medium, high, xhigh, max") {
		t.Errorf("an effort it has not: %v", err)
	}
	// a model the config names is one it can be given, as it can be started with it
	if _, chosen, err := choose(claude, "claude", "", "", config.Agent{Model: "claude-opus-5"}); err != nil || chosen.Model != "claude-opus-5" {
		t.Errorf("one the config names: %+v, %v", chosen, err)
	}
	opencode, _ := agentNamed(builtin, "opencode")
	if _, _, err := choose(opencode, "opencode", "big", ""); err == nil || err.Error() != "mia knows no models of opencode" {
		t.Errorf("an agent mia knows no models of: %v", err)
	}
	if _, err := switchTo(session.Here(), "kilwa", "opencode", opencode, "big", "", Chosen{}); err == nil || !strings.Contains(err.Error(), "restart it with --model") {
		t.Errorf("one that cannot switch while it runs: %v", err)
	}

	// into a command line, after the words that run the agent
	for command, want := range map[string]string{
		"claude --resume c1":             "claude '--model' 'opus' --resume c1",
		"claude":                         "claude '--model' 'opus'",
		"node /opt/pi/cli.js --continue": "node /opt/pi/cli.js '--model' 'opus' --continue",
	} {
		if got := put(command, []string{"--model", "opus"}); got != want {
			t.Errorf("put into %q = %q", command, got)
		}
	}
}

// A window started as env with variables and then the agent runs that
// agent, given what comes after its word: so the phone's are switched,
// and --model goes after the agent's word, not env's.
func TestAnAgentBehindEnvIsThatAgent(t *testing.T) {
	for command, want := range map[string]string{
		"env A=1 B=2 claude hi":             "claude",
		"env -i -u HOME SAWA_AGENT=1 codex": "codex",
		"A=1 /usr/local/bin/claude":         "claude",
		"env":                               "",
		"env A=1":                           "",
		"env A=1 sh -c claude":              "",
	} {
		if got, _ := agentOf(builtin, command); got != want {
			t.Errorf("agentOf(%q) = %q, want %q", command, got, want)
		}
	}
	if got := put("env A=1 B=2 claude hi", []string{"--model", "sonnet"}); got != "env A=1 B=2 claude '--model' 'sonnet' hi" {
		t.Errorf("put = %q", got)
	}
	argv, chosen, err := (&App{}).ArgvWith([]string{"env", "A=1", "B=2", "claude", "hi"}, "sonnet", "", "")
	if err != nil || strings.Join(argv, " ") != "env A=1 B=2 claude --model sonnet hi" || chosen != (Chosen{Model: "sonnet"}) {
		t.Errorf("started with = %q, %+v, %v", argv, chosen, err)
	}
	if _, _, err := (&App{}).ArgvWith([]string{"env"}, "sonnet", "", ""); err == nil {
		t.Error("env alone was given a model")
	}
	if got := again(builtin, "env A=1 claude --resume old", "23f98c02"); got != "env A=1 claude --resume 23f98c02" {
		t.Errorf("again = %q", got)
	}
}
