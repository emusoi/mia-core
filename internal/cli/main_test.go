package cli

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// The main agent and where every agent stands are a person's, not one
// repository's: an app asks from wherever its server was started.
func TestTheMainAgentAndStandingAnswerOutsideARepository(t *testing.T) {
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
	data, code = answered(t, "standing")
	var standing app.Standing
	if err := json.Unmarshal(data, &standing); err != nil || code != exitOK || standing.Waiting == nil || standing.Working == nil || standing.DoneUnread == nil || len(standing.Machines) != 1 {
		t.Fatalf("api standing = %s, exit %d, %v", data, code, err)
	}
	if _, code = answered(t, "standing", "--seen", "nowhere/kilwa"); code != exitFailed {
		t.Errorf("--seen of a repository there is not exited %d", code)
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Queries, "main") || !slices.Contains(caps.Queries, "standing") || !slices.Contains(caps.Features, "main-agent") {
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

// What the main agent is told to run has to be what this build runs: its
// notes name a verb, a query or a window verb only when there is one.
func TestTheMainAgentsNotesNameOnlyVerbsThereAre(t *testing.T) {
	notes, err := os.ReadFile("../app/main.md")
	if err != nil {
		t.Fatal(err)
	}
	known := func(name string) (Verb, bool) {
		at := slices.IndexFunc(verbs, func(v Verb) bool { return v.Name == name })
		if at < 0 {
			return Verb{}, false
		}
		return verbs[at], true
	}
	window, _ := known("window")
	named := regexp.MustCompile("(?m)(?:^    |`)mia ([a-z-]+)(?: ([a-z-]+))?").FindAllStringSubmatch(string(notes), -1)
	if len(named) < 8 {
		t.Fatalf("the notes name %d verbs: %v", len(named), named)
	}
	for _, m := range named {
		verb, sub := m[1], m[2]
		if _, ok := known(verb); !ok {
			t.Errorf("the notes say `mia %s`, and there is no such verb", verb)
		}
		if verb == "api" && !slices.Contains(apiQueryList, sub) {
			t.Errorf("the notes say `mia api %s`, and there is no such query", sub)
		}
		if verb == "window" && !regexp.MustCompile(`[\[|]`+regexp.QuoteMeta(sub)+`[|\]]`).MatchString(window.Usage) {
			t.Errorf("the notes say `mia window %s`, and window has no such verb: %s", sub, window.Usage)
		}
	}
	if strings.Contains(string(notes), "skill") {
		t.Error("the notes send it to a skill, which may name verbs this build has not")
	}
}
