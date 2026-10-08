package cli

import (
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
