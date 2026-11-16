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

// The setups used are asked from anywhere: in a repository, its own; with
// --all or outside one, every repository's; with --repository, the one
// named.
func TestTheSetupsUsedAnswerFromAnywhere(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	name := filepath.Base(repo.Root)
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	history := `[{"agent":"claude","model":"opus","effort":"high","repository":"` + name + `","worktree":"goba","machine":"local","at":"2026-11-12T10:00:00Z"},
	{"agent":"codex","model":"5.1-codex","effort":"high","repository":"mia-desk","worktree":"olasiti","machine":"fedora","at":"2026-11-14T10:00:00Z"}]`
	if err := os.WriteFile(filepath.Join(dir, "setups.json"), []byte(history), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[agents.claude]\nmodel = \"sonnet\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	asked := func(args ...string) []app.Setup {
		t.Helper()
		data, code := answered(t, append([]string{"setups"}, args...)...)
		var setups []app.Setup
		if err := json.Unmarshal(data, &setups); err != nil || code != exitOK {
			t.Fatalf("api setups %v = %s, exit %d, %v", args, data, code, err)
		}
		return setups
	}
	agents := func(setups []app.Setup) []string {
		names := []string{}
		for _, s := range setups {
			names = append(names, s.Agent+" "+s.Model)
		}
		return names
	}

	t.Chdir(repo.Root)
	if got := asked(); !slices.Equal(agents(got), []string{"claude opus", "claude sonnet"}) || !got[0].Usual || got[0].Last.Worktree != "goba" || got[1].Uses != 0 || got[1].Last != nil {
		t.Errorf("in the repository = %+v", got)
	}
	if got := asked("--all"); !slices.Equal(agents(got), []string{"codex 5.1-codex", "claude opus", "claude sonnet"}) || got[0].Usual || got[0].Last.Machine != "fedora" {
		t.Errorf("--all = %+v", got)
	}
	t.Chdir(t.TempDir())
	if got := asked(); !slices.Equal(agents(got), []string{"codex 5.1-codex", "claude opus", "claude sonnet"}) {
		t.Errorf("outside one = %+v", got)
	}
	if got := asked("--repository", "mia-desk"); !slices.Equal(agents(got), []string{"codex 5.1-codex", "claude sonnet"}) || !got[0].Usual {
		t.Errorf("--repository mia-desk = %+v", got)
	}
	for _, wrong := range [][]string{{"--repository"}, {"--all", "--repository", "mia-desk"}, {"goba"}} {
		if _, code := answered(t, append([]string{"setups"}, wrong...)...); code != exitUsage {
			t.Errorf("api setups %v exited %d", wrong, code)
		}
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Queries, "setups") || !slices.Contains(caps.Features, "setups") {
		t.Errorf("capabilities = %+v", caps)
	}
}
