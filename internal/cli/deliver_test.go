package cli

import (
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/tui"
)

func TestAPullRequestOrALayerIsNotPickedAsABranch(t *testing.T) {
	for _, argv := range [][]string{{"new", "--shell", "--pr", "7"}, {"new", "--stack", "--in", "x", "layer"}} {
		if picks(tui.Chosen{Argv: argv}) {
			t.Errorf("%v would be handed to the editor as a branch", argv)
		}
	}
}

func TestDraftsSurviveTheDashboardClosing(t *testing.T) {
	gittest.Isolate(t)
	a, err := app.Open(gittest.New(t).Root)
	if err != nil {
		t.Fatal(err)
	}
	saveDrafts(a, map[string]string{"reply:longido/helper": "a\nb"})
	if got := loadDrafts(a)["reply:longido/helper"]; got != "a\nb" {
		t.Errorf("draft came back as %q", got)
	}
}

func TestTheSidesChosenAreKeptPerRepository(t *testing.T) {
	gittest.Isolate(t)
	a, err := app.Open(gittest.New(t).Root)
	if err != nil {
		t.Fatal(err)
	}
	saveSides(a, tui.SideState{Off: []string{"right"}, Tabs: map[string]string{"top": "agent/answered"}})
	got := loadSides(a)
	if len(got.Off) != 1 || got.Off[0] != "right" || got.Tabs["top"] != "agent/answered" {
		t.Errorf("sides came back as %+v", got)
	}
	if want := filepath.Join(a.Root, ".git", "mia", "dash-sides.json"); sidesPath(a) != want {
		t.Errorf("kept in %s, not %s", sidesPath(a), want)
	}
}
