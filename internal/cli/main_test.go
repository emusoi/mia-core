package cli

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// The main agent is a person's, not one repository's: an app asks from
// wherever its server was started.
func TestTheMainAgentAnswersOutsideARepository(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())

	data, code := answered(t, "main")
	var state app.MainState
	if err := json.Unmarshal(data, &state); err != nil || code != exitOK || state.Running || state.Agent != "claude" || state.Session != "mia-main" || state.Home == "" {
		t.Fatalf("api main = %s, exit %d, %v", data, code, err)
	}
	if data, code = answered(t, "main", "turns", "--after", "3"); code != exitOK || string(data) != "[]\n" {
		t.Fatalf("no conversation is no turns: %q, exit %d", data, code)
	}
	if _, code = answered(t, "main", "turns", "--after", "soon"); code != exitUsage {
		t.Errorf("--after soon exited %d", code)
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Queries, "main") || !slices.Contains(caps.Features, "main-agent") {
		t.Errorf("capabilities = %+v", caps)
	}

	// what is said to it is its own: not read for mia's flags, and refused only because nothing runs
	if code := Run([]string{"main", "say", "what does", "--json", "-h", "mean"}); code != exitFailed {
		t.Errorf("say to a main agent that is not running exited %d", code)
	}
	if code := Run([]string{"main", "say"}); code != exitUsage {
		t.Errorf("say with nothing to say exited %d", code)
	}
	if code := Run([]string{"main", "restart"}); code != exitUsage {
		t.Errorf("a verb main has not exited %d", code)
	}
	if code := Run([]string{"main"}); code != exitOK {
		t.Errorf("mia main exited %d", code)
	}
}
