package cli

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/stack"
)

// `mia api stack` and `mia stack show --json` are one answer: each layer with
// the worktree it is checked out in.
func TestStackAPISaysWhatStackShowSays(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	path := repo.Worktree("longido", "lower")
	if err := a.Store.Put(model.Record{Name: "longido", Path: path}); err != nil {
		t.Fatal(err)
	}
	repo.Git("branch", "upper", "lower")
	for branch, parent := range map[string]string{"lower": "main", "upper": "lower"} {
		if err := a.Stacks().Record(branch, parent); err != nil {
			t.Fatal(err)
		}
	}
	layersOf := func(run func() int) []stack.Layer {
		t.Helper()
		output, err := os.CreateTemp(t.TempDir(), "output")
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		previous := os.Stdout
		os.Stdout = output
		code := run()
		os.Stdout = previous
		if code != exitOK {
			t.Fatalf("exited %d", code)
		}
		if _, err := output.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		var layers []stack.Layer
		if err := json.NewDecoder(output).Decode(&layers); err != nil {
			t.Fatal(err)
		}
		return layers
	}
	api := layersOf(func() int { return cmdAPI(atTerminal(), a, []string{"stack", "longido"}) })
	show := layersOf(func() int { return cmdStack(a, []string{"--in", "longido", "show"}, true) })
	if !reflect.DeepEqual(api, show) {
		t.Errorf("mia api stack says\n%+v\nmia stack show --json says\n%+v", api, show)
	}
	if len(api) != 2 || api[0].Worktree != "longido" || api[1].Worktree != "" {
		t.Errorf("the layer checked out in a worktree names it, the other names none: %+v", api)
	}
}
