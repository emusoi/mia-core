package tui_test

import (
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/tui"
)

func TestTheTerminalDrawsTheSnapshotTheEditorDraws(t *testing.T) {
	p := panel.Panel{
		Version: panel.Version, ID: "dashboard", Title: "Worktrees",
		Sections: []panel.Section{{ID: "blocked", Label: "waiting on you", Rows: []panel.Row{
			{ID: "iringa", Glyph: panel.GlyphWaiting, Cells: []string{"iringa", "due-dates"},
				Note: "probe waiting 30s", Actions: []string{"open"},
				Preview: []string{"Do you want to proceed?", "❯ 1. Yes"}},
			{ID: "service", Cells: []string{"service", "main"}, Actions: []string{"open"},
				Preview: []string{"this row is not selected"}},
		}}},
		Actions: map[string]panel.Action{"open": {Key: "⏎", Label: "open", Verb: "shell", Args: []string{"{row}"}}},
	}

	view := tui.Render(p, 0)
	if !strings.Contains(view, "Do you want to proceed?") {
		t.Errorf("the selected row's snapshot is missing from the terminal view:\n%s", view)
	}
	if strings.Contains(view, "this row is not selected") {
		t.Errorf("an unselected row's snapshot was drawn:\n%s", view)
	}

	moved := tui.Render(p, 1)
	if !strings.Contains(moved, "Do you want to proceed?") {
		t.Errorf("the waiting helper's question left the screen when the cursor moved:\n%s", moved)
	}
	if strings.Contains(moved, "this row is not selected") {
		t.Errorf("a row that is not waiting was shown over the one that is:\n%s", moved)
	}
}
