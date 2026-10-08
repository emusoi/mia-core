package cli

import (
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestTheWorktreeMayComeBeforeOrAfterTheSubcommand(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "kisongo", Path: repo.Worktree("kisongo", "kisongo")}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"kisongo", "edit", "first", "notes"}, {"edit", "kisongo", "first", "notes"}} {
		sub, record, rest, err := subject(a, args, "list", "list", "edit")
		if err != nil || sub != "edit" || record.Name != "kisongo" || strings.Join(rest, " ") != "first notes" {
			t.Errorf("%v read as %s on %q with %v (%v)", args, sub, record.Name, rest, err)
		}
	}
}
