package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// What each agent can be started with is asked from wherever an app's
// server was started, with what the config starts it with.
func TestTheAgentsAndTheirModelsAnswerFromAnywhere(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())
	config := "[agents.claude]\nmodel = \"sonnet\"\neffort = \"high\"\n"
	if err := os.MkdirAll(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	data, code := answered(t, "agents")
	var agents []app.AgentModels
	if err := json.Unmarshal(data, &agents); err != nil || code != exitOK {
		t.Fatalf("api agents = %s, exit %d, %v", data, code, err)
	}
	at := slices.IndexFunc(agents, func(a app.AgentModels) bool { return a.Name == "claude" })
	if at < 0 || agents[at].Model != "sonnet" || agents[at].Effort != "high" || !agents[at].Live || len(agents[at].Models) == 0 || agents[at].Models[0].ID != "opus" || len(agents[at].Efforts) == 0 {
		t.Errorf("claude = %+v", agents)
	}
	at = slices.IndexFunc(agents, func(a app.AgentModels) bool { return a.Name == "opencode" })
	if at < 0 || agents[at].Live || agents[at].Models == nil || len(agents[at].Models) != 0 {
		t.Errorf("an agent mia knows no models of = %+v", agents[at])
	}
	if _, code := answered(t, "agents", "claude"); code != exitUsage {
		t.Errorf("api agents with more exited %d", code)
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Queries, "agents") || !slices.Contains(caps.Features, "agent-models") {
		t.Errorf("capabilities = %+v", caps)
	}
	// a model is an agent's to switch, and the main agent's is refused only because nothing runs
	for _, wrong := range [][]string{{"main", "model"}, {"main", "model", "a", "b"}, {"main", "model", "--effort", "high"}} {
		if code := Run(wrong); code != exitUsage {
			t.Errorf("%v exited %d", wrong, code)
		}
	}
	if code := Run([]string{"main", "model", "sonnet", "--effort", "high"}); code != exitFailed {
		t.Errorf("a main agent that is not running switched: exit %d", code)
	}
}
