package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/plugin"
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

func TestVShowsThePluginsViewsInHome(t *testing.T) {
	p := homePanel()
	p.Room.Sides = []plugin.SideTab{
		{Plugin: "agent", Side: plugin.Side{ID: "activity", Where: "bottom", Label: "activity"},
			SideAnswer: plugin.SideAnswer{Lines: []plugin.Item{{Cells: plugin.Line{{Text: "13:24 lemara started"}}, Open: &plugin.Open{Lands: "lemara"}}}}},
		{Plugin: "cost", Side: plugin.Side{ID: "cost", Where: "right", Label: "cost"},
			SideAnswer: plugin.SideAnswer{Lines: []plugin.Item{{Cells: plugin.Line{{Text: "$43.42 today"}}}}}},
	}
	m := newModel(p)
	m.width, m.height = 180, 45
	press := func(m model, key string) model {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		return next.(model)
	}
	m = press(m, "v")
	view := m.View()
	if strings.Contains(view, "╭") || !strings.Contains(view, "13:24 lemara started") || !strings.Contains(view, "› ") {
		t.Fatalf("v shows the first view in home, its line under the cursor:\n%s", view)
	}
	if !strings.Contains(press(m, "l").View(), "$43.42 today") {
		t.Error("l shows the next view")
	}
	if back := press(m, "v"); back.viewing || !strings.Contains(back.View(), "Add separate Filevine OAuth credentials") {
		t.Error("v again comes back to the list")
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

func TestTheListStaysPutWhileTheEntryMoves(t *testing.T) {
	m := newModel(homePanel())
	m.width, m.height = 180, 45
	at := func(m model, word string) int {
		for i, line := range strings.Split(stripANSI(m.View()), "\n") {
			if strings.Contains(line, "in progress  1") {
				return i
			}
		}
		t.Fatalf("no %q", word)
		return -1
	}
	first := at(m, "in progress")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := at(next.(model), "in progress"); got != first {
		t.Errorf("an entry with fewer facts moved the list from row %d to %d", first, got)
	}
}

func TestHomeIsAPageCentredInATallTerminal(t *testing.T) {
	m := newModel(homePanel())
	m.width, m.height = 200, 80
	lines := strings.Split(stripANSI(m.View()), "\n")
	if len(lines) != 80 {
		t.Fatalf("home fills the terminal: %d lines", len(lines))
	}
	first, last := -1, -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 15 || 80-1-last < 15 {
		t.Errorf("the page sits in the middle, not stretched: rows %d to %d of 80", first, last)
	}
	if !strings.HasPrefix(lines[first], strings.Repeat(" ", 38)) {
		t.Errorf("and centred across: %q", lines[first])
	}
}
