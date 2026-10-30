package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emusoi/mia-core/internal/panel"
)

func homePanel() panel.Panel {
	return panel.Panel{ID: "dashboard", Room: &panel.Room{Repo: "lido-app"}, Sections: []panel.Section{
		{ID: "needs", Label: "needs you", Rows: []panel.Row{
			{ID: "docs-base", Title: "Docs base stack fell behind its parent", Cells: []string{"docs-base", "147 layers", "behind", "3d"},
				Facts: []string{"restack  docs-shared is behind its parent"}},
		}},
		{ID: "active", Label: "in progress", Rows: []panel.Row{
			{ID: "temeke", Title: "Add separate Filevine OAuth credentials", Cells: []string{"temeke", "filevine_auth", "uncommitted", "20d"}},
		}},
		{ID: "quiet", Label: "resting", Collapsed: true, ToggleKey: "e", Rows: []panel.Row{
			{ID: "kijenge", Cells: []string{"kijenge", "nsa", "", "44d"}},
		}},
	}}
}

func TestHomeSetsTheSelectedWorktreeAsAHeadword(t *testing.T) {
	m := newModel(homePanel())
	m.width, m.height = 180, 45
	view := m.View()
	for _, want := range []string{"mia", "lido-app", "Docs base stack fell behind its parent", "docs-shared is behind its parent",
		"needs you", "in progress", "resting", "e show", "▔ docs-base", "Add separate Filevine OAuth credentials"} {
		if !strings.Contains(view, want) {
			t.Errorf("home lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "╭") || strings.Contains(view, "kijenge") {
		t.Errorf("home has no box, and a folded section lists nobody:\n%s", view)
	}
	if lines := strings.Split(view, "\n"); len(lines) != 45 {
		t.Errorf("home fills the terminal's height: %d lines", len(lines))
	}
}

func TestJMovesTheHeadword(t *testing.T) {
	m := newModel(homePanel())
	m.width, m.height = 180, 45
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	view := next.(model).View()
	if !strings.Contains(view, "▔ temeke") {
		t.Errorf("j makes the next worktree the headword:\n%s", view)
	}
}

func TestVSwitchesToTheRoomAndBack(t *testing.T) {
	m := newModel(homePanel())
	m.width, m.height = 212, 48
	press := func(m model, key string) model {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		return next.(model)
	}
	m = press(m, "v")
	if !m.classic || !strings.Contains(m.View(), "╭") {
		t.Fatal("v shows the room, the box in it")
	}
	m = press(press(m, "v"), "v")
	if m.classic || strings.Contains(m.View(), "╭") {
		t.Error("v twice in the room, through its legend, comes back home")
	}
}

func TestHomeFitsEightyColumns(t *testing.T) {
	m := newModel(homePanel())
	m.width, m.height = 80, 24
	for i, line := range strings.Split(m.View(), "\n") {
		if w := len([]rune(stripANSI(line))); w > 80 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
}

func TestHomeSaysItIsReadingBeforeThePanelArrives(t *testing.T) {
	m := newModel(panel.Panel{ID: "dashboard", Room: &panel.Room{Repo: "lido-app"}})
	m.width, m.height = 180, 45
	if !strings.Contains(m.View(), "Reading the worktrees") {
		t.Errorf("no panel yet:\n%s", m.View())
	}
	m.panel.Empty = "No worktrees yet."
	if !strings.Contains(m.View(), "No worktrees yet.") {
		t.Errorf("an empty repository says so:\n%s", m.View())
	}
}
