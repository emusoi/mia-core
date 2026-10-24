package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestDevLendAndOffReturnJSONAndRestoreMain(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	feature := repo.Worktree("feature", "feature")
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "feature", Path: feature}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo.Root)

	run := func(args ...string) (devState, int) {
		t.Helper()
		out, err := os.CreateTemp(t.TempDir(), "dev-json")
		if err != nil {
			t.Fatal(err)
		}
		stdout := os.Stdout
		os.Stdout = out
		code := Run(args)
		os.Stdout = stdout
		if err := out.Close(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(out.Name())
		if err != nil {
			t.Fatal(err)
		}
		var state devState
		if err := json.Unmarshal(data, &state); err != nil {
			t.Fatalf("%v was not JSON: %v\n%s", args, err, data)
		}
		return state, code
	}

	state, code := run("dev", "feature", "--json")
	if code != exitOK || !state.Lent || state.Worktree != "feature" || state.Path != feature {
		t.Fatalf("lend state = %+v, exit %d", state, code)
	}
	state, code = run("dev", "off", "--json")
	if code != exitOK || state.Lent {
		t.Fatalf("off state = %+v, exit %d", state, code)
	}
	if dirty := repo.Git("status", "--porcelain"); dirty != "" {
		t.Fatalf("main was not restored: %s", dirty)
	}
}
