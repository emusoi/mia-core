package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emusoi/mia-core/internal/panel"
)

func TestCollapsedSectionShowsItsCountAndToggles(t *testing.T) {
	p := panel.Panel{
		Version: panel.Version,
		Title:   "Worktrees / app",
		Sections: []panel.Section{{
			ID: "quiet", Label: "quiet", Collapsed: true, ToggleKey: "e",
			Rows: []panel.Row{{ID: "monduli", Cells: []string{"monduli", "feature"}}},
		}},
		Empty: "No worktrees.",
	}
	m := newModel(p)
	view := m.View()
	if !strings.Contains(view, "quiet  1") || strings.Contains(view, "monduli") {
		t.Fatalf("collapsed view did not preserve the count and hide the row:\n%s", view)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = updated.(model)
	if view = m.View(); !strings.Contains(view, "monduli") || len(m.rows) != 1 {
		t.Fatalf("toggle did not reveal the quiet row:\n%s", view)
	}
}
