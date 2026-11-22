package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/session"
)

// What opencode models prints is a provider/model a line: each is a
// model once, named by what comes after its provider, and anything else
// it prints is none.
func TestOpencodesModelsAreTheLinesThatAreAProvidersModel(t *testing.T) {
	listed := opencodeListed("openai/gpt-5\n  ollama/qwen3:4b  \r\nopenrouter/meta/llama-4\n\nopenai/gpt-5\nINFO service started\nno-provider\n/nothing\nnothing/\nError: not signed in\n")
	want := []plugin.Model{{ID: "openai/gpt-5", Label: "gpt-5"}, {ID: "ollama/qwen3:4b", Label: "qwen3:4b"}, {ID: "openrouter/meta/llama-4", Label: "meta/llama-4"}}
	if !slices.Equal(listed, want) {
		t.Errorf("listed = %+v", listed)
	}
	for _, none := range []string{"", "\n\n", "Error: no providers\n"} {
		if listed := opencodeListed(none); listed == nil || len(listed) != 0 {
			t.Errorf("%q lists %+v", none, listed)
		}
	}
}

// opencode is asked for its models once, and what it said is kept for an
// hour: a list of agents does not start it each time. One that cannot say
// has none, as before it was asked, and is asked again after a minute; it
// is not asked at all where it has kept nothing.
func TestOpencodesModelsAreAskedOnceAndKept(t *testing.T) {
	gittest.Isolate(t)
	cache := t.TempDir()
	its := opencodeModels{cache: cache}
	if models := its.models(); len(models) != 0 {
		t.Fatalf("with no opencode that has kept anything: %+v", models)
	}
	if _, err := os.Stat(filepath.Join(cache, "opencode.json")); err == nil {
		t.Fatal("what was never asked was kept")
	}

	store := fakeOpencode(t)
	asked := func() int {
		data, _ := os.ReadFile(filepath.Join(store, "asked"))
		_ = os.Remove(filepath.Join(store, "asked"))
		return strings.Count(string(data), "models\n")
	}
	kept := func(age time.Duration) {
		t.Helper()
		var listing modelListing
		data, err := os.ReadFile(filepath.Join(cache, "opencode.json"))
		if err != nil || json.Unmarshal(data, &listing) != nil {
			t.Fatalf("kept: %s %v", data, err)
		}
		listing.At = listing.At.Add(-age)
		data, _ = json.Marshal(listing)
		if err := os.WriteFile(filepath.Join(cache, "opencode.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// it cannot say: not signed in
	if models := its.models(); len(models) != 0 || asked() != 1 {
		t.Fatalf("one that fails has none: %+v", models)
	}
	if models := its.models(); len(models) != 0 || asked() != 0 {
		t.Errorf("and is not asked again at once: %+v", models)
	}
	if err := os.WriteFile(filepath.Join(store, "models"), []byte("openai/gpt-5\nollama/qwen3:4b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	kept(2 * time.Minute)
	want := []plugin.Model{{ID: "openai/gpt-5", Label: "gpt-5"}, {ID: "ollama/qwen3:4b", Label: "qwen3:4b"}}
	if models := its.models(); !slices.Equal(models, want) || asked() != 1 {
		t.Fatalf("asked again after a minute: %+v", models)
	}
	if err := os.WriteFile(filepath.Join(store, "models"), []byte("openai/gpt-6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if models := its.models(); !slices.Equal(models, want) || asked() != 0 {
		t.Errorf("within the hour, what was kept: %+v", models)
	}
	kept(2 * time.Hour)
	if models := its.models(); len(models) != 1 || models[0].ID != "openai/gpt-6" || asked() != 1 {
		t.Errorf("after it, asked again: %+v", models)
	}
	// with nowhere to keep it, it is asked each time
	if models := (opencodeModels{}).models(); len(models) != 1 || asked() != 1 {
		t.Errorf("with no cache: %+v", models)
	}

	if listing := (modelListing{}); listing.fresh(time.Now()) {
		t.Error("nothing kept is fresh")
	}
}

// opencode has the models it lists and is started on one as its minimal
// interface, which has a word for one; it has no word for an effort.
// Switched, it is started so again on its conversation. gemini has a word
// for a model and lists none: it has those the config names.
func TestOpencodeAndGeminiAreStartedWithAModel(t *testing.T) {
	opencode, _ := agentNamed(builtin, "opencode")
	opencode.lists = func() []plugin.Model { return opencodeListed("openai/gpt-5\nollama/qwen3:4b\n") }
	with, chosen, err := choose(opencode, "opencode --session ses_1", "ollama/qwen3:4b", "", config.Agent{Effort: "high"})
	if err != nil || !slices.Equal(with, []string{"mini", "--model", "ollama/qwen3:4b"}) || chosen != (Chosen{Model: "ollama/qwen3:4b"}) {
		t.Errorf("started with a model: %v, %+v, %v", with, chosen, err)
	}
	if with, chosen, err := choose(opencode, "opencode", "", "", config.Agent{Effort: "high"}); err != nil || len(with) != 0 || chosen != (Chosen{}) {
		t.Errorf("with none it is started as it is, and an effort is none of its: %v, %+v, %v", with, chosen, err)
	}
	if _, _, err := choose(opencode, "opencode", "", "high"); err == nil || err.Error() != "mia knows no efforts of opencode" {
		t.Errorf("an effort: %v", err)
	}
	if _, _, err := choose(opencode, "opencode", "big/none", ""); err == nil || err.Error() != "opencode has no model big/none — it has openai/gpt-5, ollama/qwen3:4b" {
		t.Errorf("a model it does not list: %v", err)
	}
	if models, efforts := modelsOf(opencode, config.Agent{Model: "local/own", Effort: "high"}), effortsOf(opencode, config.Agent{Effort: "high"}); len(models) != 3 || models[2].ID != "local/own" || len(efforts) != 0 {
		t.Errorf("those it lists and the config's: %+v %v", models, efforts)
	}
	for id, want := range map[string]string{
		"/w/feature":     "opencode 'mini' '--model' 'openai/gpt-5' --session ses_1",
		"/w/feature/api": "cd '/w/feature/api' && opencode 'mini' '--model' 'openai/gpt-5' --session ses_1",
	} {
		if got, err := againCommand(opencode, "ses_1", id, "/w/feature", "openai/gpt-5", "", ""); err != nil || got != want {
			t.Errorf("started again in %s: %q %v", id, got, err)
		}
	}
	if to, again := opencode.to, opencode.again; to != nil || !again {
		t.Error("it is switched by being started again, not by what is typed")
	}

	many := ""
	for i := range manyModels + 1 {
		many += "openai/gpt-" + string(rune('a'+i)) + "\n"
	}
	opencode.lists = func() []plugin.Model { return opencodeListed(many) }
	if _, _, err := choose(opencode, "opencode", "big/none", ""); err == nil || err.Error() != "opencode has no model big/none — `mia api agents` lists the 13 it has" {
		t.Errorf("of many, they are not all said: %v", err)
	}
	opencode.lists = func() []plugin.Model { return nil }
	if _, _, err := choose(opencode, "opencode", "openai/gpt-5", ""); err == nil || err.Error() != "mia knows no models of opencode" {
		t.Errorf("when it cannot list them: %v", err)
	}

	gemini, _ := agentNamed(builtin, "gemini")
	if with, chosen, err := choose(gemini, "gemini", "", "", config.Agent{Model: "gemini-3-pro"}); err != nil || !slices.Equal(with, []string{"--model", "gemini-3-pro"}) || chosen != (Chosen{Model: "gemini-3-pro"}) {
		t.Errorf("gemini with the config's: %v, %+v, %v", with, chosen, err)
	}
	if _, _, err := choose(gemini, "gemini", "gemini-3-pro", "", config.Agent{Model: "gemini-3-pro"}); err != nil {
		t.Errorf("gemini with one the config names: %v", err)
	}
	if _, _, err := choose(gemini, "gemini", "gemini-3-pro", ""); err == nil || err.Error() != "mia knows no models of gemini" {
		t.Errorf("gemini with one no config names: %v", err)
	}
}

// A window mia started has, to tmux, the shell it was started by in
// front: ps says what that shell runs, and the agent behind it is the
// window's — whatever the window is called, with a conversation or none
// yet — until the agent ends and the shell is its own again. A shell mia
// did not mark as an agent's is not asked about.
func TestAWindowBehindTheShellMiaStartedItByIsItsAgents(t *testing.T) {
	for command, want := range map[string]string{
		"zsh -lc opencode; exec 'zsh' -l": "opencode",
		"/bin/zsh -lc opencode 'mini' '--model' 'openai/gpt-5' --session ses_1; exec '/bin/zsh' -l": "opencode mini --model openai/gpt-5 --session ses_1",
		"sh -lc cd '/w/feature/api' && opencode --session ses_1; exec 'sh' -l":                      "opencode --session ses_1",
		"zsh -lc CLAUDE_CONFIG_DIR='/a/work' claude '--model' 'opus'; exec 'zsh' -l":                "CLAUDE_CONFIG_DIR=/a/work claude --model opus",
		"zsh -l":                         "zsh -l",
		"-zsh":                           "-zsh",
		"node /opt/pi/cli.js --continue": "node /opt/pi/cli.js --continue",
		"":                               "",
	} {
		if got := launched(command); got != want {
			t.Errorf("launched(%q) = %q, want %q", command, got, want)
		}
	}
	for command, want := range map[string]string{
		"opencode mini --model openai/gpt-5 --session ses_1": "opencode",
		"CLAUDE_CONFIG_DIR=/a/work claude --model opus":      "claude",
		"/bin/sh /opt/bin/opencode --session ses_1":          "opencode",
		"zsh -l":     "",
		"sh -c make": "",
	} {
		if got, _ := agentOf(builtin, command); got != want {
			t.Errorf("agentOf(%q) = %q, want %q", command, got, want)
		}
	}

	panes := []session.Pane{
		{Index: 1, Window: "opencode · shop", Command: "opencode", PID: 11, Title: "OC | Greeting"},
		{Index: 2, Window: "opencode-2", Command: "zsh", PID: 12, Title: "OpenCode", Kind: session.KindAgent, Label: "opencode"},
		{Index: 3, Window: "claude-2", Command: "zsh", PID: 13, Title: "✳ Ship it", Kind: session.KindAgent, Label: "claude"},
		{Index: 4, Window: "opencode", Command: "zsh", PID: 14, Title: "host"},
		{Index: 5, Window: "opencode-3", Command: "zsh", PID: 15, Title: "OpenCode", Kind: session.KindAgent, Label: "opencode"},
		{Index: 6, Window: "notes", Command: "zsh", PID: 16, Title: "OpenCode"},
		{Index: 7, Window: "node", Command: "node", PID: 17, Title: "π - shop"},
	}
	asked := 0
	front := func() map[int]string {
		asked++
		return map[int]string{
			12: "zsh -lc opencode; exec 'zsh' -l",
			13: "zsh -lc claude '--model' 'opus'; exec 'zsh' -l",
			15: "zsh -l", // opencode ended: its shell is in front
			16: "zsh -lc opencode; exec 'zsh' -l",
			17: "node /opt/pi/cli.js",
		}
	}
	for window, want := range map[string]string{"1": "opencode", "2": "opencode", "opencode-2": "opencode", "3": "claude", "4": "opencode", "5": "", "6": "", "7": "pi", "9": ""} {
		agent, _, known := agentInWindow(builtin, panes, window, front)
		if agent.Name != want || known != (want != "") {
			t.Errorf("window %s runs %q, want %q", window, agent.Name, want)
		}
	}
	asked = 0
	for _, window := range []string{"1", "4", "6"} {
		agentInWindow(builtin, panes, window, front)
	}
	if asked != 0 {
		t.Errorf("ps was asked %d times for windows tmux names, or that are no agent's", asked)
	}
	// on a machine there is no ps to ask: only what tmux says
	if agent, _, known := agentInWindow(builtin, panes, "2", nil); known {
		t.Errorf("with nothing to ask, window 2 runs %q", agent.Name)
	}
}
