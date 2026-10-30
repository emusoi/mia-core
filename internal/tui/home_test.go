package tui

import (
	"os/exec"
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
	tab := func(m model) model {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		return next.(model)
	}
	home := newModel(p)
	home.width, home.height = 180, 45
	if one := tab(home); !one.viewing || one.view != 0 {
		t.Error("tab from the worktrees goes to the first view")
	}
	if round := tab(tab(tab(home))); round.viewing {
		t.Error("tab past the last view comes back to the worktrees")
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

func TestTheKeysAreCentredUnderThePage(t *testing.T) {
	m := newModel(homePanel())
	m.width, m.height = 180, 45
	lines := strings.Split(strings.TrimRight(stripANSI(m.View()), "\n "), "\n")
	foot := strings.TrimRight(lines[len(lines)-1], " ")
	text := strings.TrimSpace(foot)
	left := len([]rune(foot)) - len([]rune(strings.TrimLeft(foot, " ")))
	right := 180 - left - len([]rune(text))
	if !strings.HasPrefix(text, "j k move") || left-right > 2 || right-left > 2 {
		t.Errorf("the keys sit in the middle: %d left, %d right: %q", left, right, foot)
	}
}

func TestAViewsTitleTakesHomesWidth(t *testing.T) {
	p := homePanel()
	long := "A provider emails a filing and Lido sorts it into the client's folder"
	p.Room.Sides = []plugin.SideTab{{Plugin: "issue", Side: plugin.Side{ID: "issues", Where: "left", Label: "issues"},
		SideAnswer: plugin.SideAnswer{Lines: []plugin.Item{{Cells: plugin.Line{{Text: "○", Width: 2}, {Text: "WL-55", Width: 7}, {Text: long, Width: 22, Grow: true}}}}}}}
	m := newModel(p)
	m.width, m.height = 180, 45
	m = m.showView(0)
	if !strings.Contains(stripANSI(m.View()), long) {
		t.Errorf("the title is cut to the side's 22 columns:\n%s", stripANSI(m.View()))
	}
}

func TestAPopupWithoutTmuxRunsInTheTerminal(t *testing.T) {
	p := homePanel()
	p.Room.Sides = []plugin.SideTab{{Plugin: "issue", Side: plugin.Side{ID: "issues", Where: "left", Label: "issues"},
		SideAnswer: plugin.SideAnswer{Lines: []plugin.Item{{Cells: plugin.Line{{Text: "WL-55"}}, Open: &plugin.Open{Popup: []string{"echo", "hi"}}}}}}}
	m := newModel(p)
	m.width, m.height = 180, 45
	m.hooks.ShowIn = func(open plugin.Open) *exec.Cmd {
		return exec.Command("printf", "WL-55  A provider emails a filing\n  state  todo\n")
	}
	m = m.showView(0)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || next.(model).problem != "" {
		t.Fatalf("⏎ runs the popup's command: problem %q", next.(model).problem)
	}
	shown, _ := next.(model).Update(cmd())
	view := stripANSI(shown.(model).View())
	if !strings.Contains(view, "WL-55  A provider emails a filing") || !strings.Contains(view, "q close") {
		t.Errorf("home shows what it printed as a page:\n%s", view)
	}
	closed, _ := shown.(model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if closed.(model).reading != nil {
		t.Error("q closes the page")
	}
}

func TestAViewDrawsItsHeadSectionsMetersAndChart(t *testing.T) {
	p := homePanel()
	answer := plugin.SideAnswer{Head: "$22.59 spent today", Note: "$882.63 this week", Lines: []plugin.Item{
		{Label: "this week"},
		{Chart: []plugin.Bar{{Label: "mon", Value: 52}, {Label: "today", Value: 22.59, On: true}}},
		{Label: "today by worktree", Cells: plugin.Line{{Text: "1"}}},
		{Cells: plugin.Line{{Text: "lemara", Width: 16}, {Meter: &plugin.Meter{Value: 1, Max: 2}, Width: 10}, {Text: "$22.59"}}},
	}}
	p.Room.Sides = []plugin.SideTab{{Plugin: "cost", Side: plugin.Side{ID: "cost", Where: "right", Label: "cost"}, SideAnswer: answer}}
	m := newModel(p)
	m.width, m.height = 180, 45
	view := stripANSI(m.showView(0).View())
	for _, want := range []string{"$22.59 spent today", "$882.63 this week", "this week", "████", "today", "today by worktree  1", "lemara", "━━━━━━━━━━", "$22.59"} {
		if !strings.Contains(view, want) {
			t.Errorf("the view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Add separate Filevine") {
		t.Error("the view's head takes the entry's place")
	}
}

func TestHelpAndReportsAreHomesPages(t *testing.T) {
	m := newModel(homePanel())
	m.width, m.height = 180, 45
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	view := stripANSI(next.(model).View())
	for _, want := range []string{"Keys", "the next view", "the dots", "? close"} {
		if !strings.Contains(view, want) {
			t.Errorf("help lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "╭") || strings.Contains(view, "of the room") {
		t.Errorf("help is home's page, about home:\n%s", view)
	}
	m.mode, m.report, m.reportOf = reporting, []string{"restacked 3 layers"}, "mia stack restack"
	view = stripANSI(m.View())
	if !strings.Contains(view, "mia stack restack") || !strings.Contains(view, "restacked 3 layers") || strings.Contains(view, "╭") {
		t.Errorf("a report is a page titled with its command:\n%s", view)
	}
}

func TestTheComposerAndDetailsAreHomesPages(t *testing.T) {
	m := newModel(homePanel())
	m.width, m.height = 180, 45
	m.mode = typing
	m.pending = &pending{action: panel.Action{Label: "message the agent", Lines: true, Input: "message"}, row: m.rows[0]}
	view := stripANSI(m.View())
	if !strings.Contains(view, "message the agent") || !strings.Contains(view, "^s send") || strings.Contains(view, "╭") {
		t.Errorf("the composer is a page with its keys:\n%s", view)
	}
	m.mode, m.pending, m.details = browsing, nil, true
	view = stripANSI(m.View())
	if !strings.Contains(view, "i close") || strings.Contains(view, "╭") {
		t.Errorf("details is a page:\n%s", view)
	}
}
