package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/emusoi/mia-core/internal/panel"
)

func tall(n int) panel.Panel {
	var rows []panel.Row
	for i := 0; i < n; i++ {
		rows = append(rows, panel.Row{ID: fmt.Sprintf("tree-%02d", i), Cells: []string{fmt.Sprintf("tree-%02d", i), "main"}, Actions: []string{"open"}})
	}
	return panel.Panel{
		Version: panel.Version, ID: "dashboard", Title: "Worktrees",
		Sections: []panel.Section{{ID: "quiet", Label: "quiet", Rows: rows}},
		Actions:  map[string]panel.Action{"open": {Key: "⏎", Label: "open", Verb: "shell", Args: []string{"{row}"}}},
		Hints:    []string{"⏎ open"},
	}
}

func TestTheSelectedRowAndTheFooterAreAlwaysOnScreen(t *testing.T) {
	p := tall(30)
	for _, cursor := range []int{0, 14, 29} {
		view := RenderAt(p, cursor, 12)
		lines := strings.Split(view, "\n")
		if len(lines) > 13 {
			t.Errorf("cursor %d: drew %d lines on a 12-line terminal", cursor, len(lines))
		}
		want := fmt.Sprintf("tree-%02d", cursor)
		if !strings.Contains(view, want) {
			t.Errorf("cursor %d: the selected row %s is not on screen:\n%s", cursor, want, view)
		}
		tail := strings.Join(lines[max(len(lines)-3, 0):], "\n")
		if !strings.Contains(tail, "? keys") {
			t.Errorf("cursor %d: the footer is not at the bottom:\n%s", cursor, view)
		}
	}
}

func TestNothingIsWiderThanTheTerminal(t *testing.T) {
	p := tall(3)
	p.Sections[0].Rows[0].Preview = []string{strings.Repeat("a very long line of helper output ", 6)}
	p.Sections[0].Rows[0].Note = "helper waiting 3s"
	p.Sections[0].Rows[1].Cells = []string{"tree-01", strings.Repeat("feature/", 12)}
	view := RenderAt(p, 0, 20)
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 96 {
			t.Errorf("a line is %d wide on a 96-column terminal:\n%s", w, line)
		}
	}
	if !strings.Contains(view, "…") {
		t.Error("nothing was cut, yet the input was too wide")
	}
}

func TestAWaitingRowShowsItsQuestionWithoutBeingSelected(t *testing.T) {
	p := tall(3)
	p.Sections[0].Rows[2].Glyph = panel.GlyphWaiting
	p.Sections[0].Rows[2].Preview = []string{"Do you want to proceed?", "❯ 1. Yes"}
	view := RenderAt(p, 0, 30)
	if !strings.Contains(view, "Do you want to proceed?") {
		t.Errorf("the waiting row's question is hidden until selected:\n%s", view)
	}
}

func TestPagingKeysMoveTheCursor(t *testing.T) {
	m := newModel(tall(40))
	m.width, m.height = 100, 20
	press := func(k string) {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		m = next.(model)
	}
	press("G")
	if m.cursor != 39 {
		t.Errorf("G → %d", m.cursor)
	}
	press("g")
	if m.cursor != 0 {
		t.Errorf("g → %d", m.cursor)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = next.(model)
	if m.cursor != m.bodyHeight()/2 {
		t.Errorf("ctrl+d → %d, want %d", m.cursor, m.bodyHeight()/2)
	}
}
