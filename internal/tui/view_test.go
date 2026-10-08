package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emusoi/mia-core/internal/panel"
)

func popupPanel() panel.Panel {
	return panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{
			"open":  {Key: "⏎", Verb: "shell", Args: []string{"{row}"}, Lands: true},
			"watch": {Key: "w", Verb: "chat", Args: []string{"attach", "{row}", "{tab}"}, Lands: true},
		},
		Sections: []panel.Section{{Rows: []panel.Row{{ID: "iringa", Cells: []string{"iringa"}, Actions: []string{"open", "watch"},
			Tabs: []panel.Tab{{Name: "helper", Preview: []string{"❯"}}}}}}}}
}

func TestTabOpensTheTabsThingInAPopupAndReturnsWhenItCloses(t *testing.T) {
	var opened [][]string
	m := newModel(popupPanel())
	m.width, m.height = 140, 30
	m.hooks = Hooks{Popup: func(c Chosen) error { opened = append(opened, c.Argv); return nil }}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if cmd == nil || m.popup == nil {
		t.Fatal("tab did not open a popup")
	}
	msg := cmd()
	if len(opened) != 1 || strings.Join(opened[0], " ") != "shell iringa" {
		t.Fatalf("opened %v", opened)
	}
	if _, cmd = m.Update(tea.KeyMsg{Type: tea.KeyTab}); cmd != nil {
		t.Error("tab while open opened again")
	}
	next, _ = m.Update(msg)
	if next.(model).popup != nil {
		t.Error("closing did not return to the list")
	}
	m.tab = 1
	if chosen, ok := m.viewChosen(); !ok || strings.Join(chosen.Argv, " ") != "shell iringa" {
		t.Errorf("on the facts tab the popup should be the session: %v", chosen.Argv)
	}
}

func TestResizingWhileOpenReopensTheModal(t *testing.T) {
	opened, closed := 0, 0
	m := newModel(popupPanel())
	m.width, m.height = 140, 30
	m.hooks = Hooks{Popup: func(Chosen) error { opened++; return nil }, Close: func() { closed++ }}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	first := cmd()
	next, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = next.(model)
	next, cmd = m.Update(first)
	m = next.(model)
	if closed != 1 || m.popup == nil || cmd == nil {
		t.Fatalf("resize: closed=%d open=%v reopened=%v", closed, m.popup != nil, cmd != nil)
	}
	cmd()
	if opened != 2 {
		t.Errorf("opened %d times, want 2", opened)
	}
}

func TestEditorActionsOpenInAPopup(t *testing.T) {
	p := panel.Panel{ID: "dashboard",
		Actions:  map[string]panel.Action{"plan": {Key: "p", Verb: "plan", Args: []string{"edit", "{row}"}, Lands: true, Popup: true}},
		Sections: []panel.Section{{Rows: []panel.Row{{ID: "iringa", Cells: []string{"iringa"}, Actions: []string{"plan"}}}}}}
	m := newModel(p)
	var got []string
	m.hooks = Hooks{Popup: func(c Chosen) error { got = c.Argv; return nil }}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = next.(model)
	if cmd == nil || m.popup == nil {
		t.Fatal("p did not open a popup")
	}
	cmd()
	if strings.Join(got, " ") != "plan edit iringa" {
		t.Errorf("popup ran %v", got)
	}
}

func TestSideTabsSwitchBetweenTabFactsAndBlocks(t *testing.T) {
	p := panel.Panel{ID: "dashboard", Sections: []panel.Section{{Rows: []panel.Row{{ID: "iringa", Cells: []string{"iringa"},
		Preview: []string{"❯ working"}, Facts: []string{"branch  due-dates"}, Blocks: []string{"web-server  helper  quiet 2s", "zsh  zsh  quiet 1h"}}}}}}
	m := newModel(p)
	m.width, m.height = 140, 30
	if got := m.tabs(); len(got) != 3 || got[0] != "preview" || got[2] != "blocks" {
		t.Fatalf("tabs %v", got)
	}
	if v := m.View(); !strings.Contains(v, "❯ working") || strings.Contains(v, "due-dates") {
		t.Errorf("first tab is not the tab:\n%s", v)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	m = next.(model)
	if v := m.View(); !strings.Contains(v, "due-dates") {
		t.Errorf("] did not move to facts:\n%s", v)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(model)
	if v := m.View(); !strings.Contains(v, "web-server") {
		t.Errorf("3 did not move to blocks:\n%s", v)
	}
}

func TestEachTabIsATabAndActionsNameTheSelectedOne(t *testing.T) {
	p := panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{"reply": {Key: "m", Verb: "chat", Args: []string{"send", "{row}", "{tab}", "{input}"}, Input: "reply"}},
		Sections: []panel.Section{{Rows: []panel.Row{{ID: "iringa", Cells: []string{"iringa"}, Actions: []string{"reply"},
			Preview: []string{"helper says"}, Facts: []string{"branch  x"},
			Tabs: []panel.Tab{{Name: "helper", Note: "helper working", Preview: []string{"helper says"}}, {Name: "reviewer", Note: "reviewer waiting 3s", Preview: []string{"reviewer asks"}}}}}}}}
	m := newModel(p)
	m.width, m.height = 140, 30
	if got := m.tabs(); len(got) != 3 || got[0] != "helper" || got[1] != "reviewer" {
		t.Fatalf("tabs %v", got)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	m = next.(model)
	if v := m.View(); !strings.Contains(v, "reviewer asks") || !strings.Contains(v, "reviewer waiting 3s") {
		t.Errorf("second tab is not reviewer:\n%s", v)
	}
	m.input = "yes"
	argv := m.argvFor(p.Actions["reply"], m.rows[0])
	if strings.Join(argv, " ") != "chat send iringa reviewer yes" {
		t.Errorf("argv = %v", argv)
	}
	m.tab = 2
	if argv := m.argvFor(p.Actions["reply"], m.rows[0]); strings.Join(argv, " ") != "chat send iringa yes" {
		t.Errorf("facts tab should leave the tab to the CLI: %v", argv)
	}
}

func TestActionsRunInPlaceAndReportsShowInTheFrame(t *testing.T) {
	p := panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{
			"why":  {Key: "d", Verb: "chat", Args: []string{"explain", "{row}"}, Report: true},
			"open": {Key: "s", Verb: "shell", Args: []string{"{row}"}, Lands: true},
		},
		Sections: []panel.Section{{Rows: []panel.Row{{ID: "iringa", Cells: []string{"iringa"}, Actions: []string{"why", "open"}}}}}}
	m := newModel(p)
	m.width, m.height = 100, 20
	m.reload = func() (panel.Panel, error) { return p, nil }
	m.hooks.Perform = func(c Chosen) Outcome { return Outcome{Output: "helper is waiting\nbecause it asked\n"} }
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(model)
	if m.mode != working || cmd == nil {
		t.Fatalf("d did not start work in place: mode %v", m.mode)
	}
	next, _ = m.Update(performed{chosen: Chosen{Action: p.Actions["why"], RowID: "iringa"}, outcome: Outcome{Output: "helper is waiting\nbecause it asked\n"}})
	m = next.(model)
	if v := m.View(); m.mode != reporting || !strings.Contains(v, "because it asked") || !strings.Contains(v, "⏎") {
		t.Errorf("the report is not shown in the frame:\n%s", v)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.mode != browsing {
		t.Errorf("⏎ did not return to the list")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = next.(model)
	if cmd == nil || m.chosen.Argv == nil || m.chosen.Argv[0] != "shell" {
		t.Errorf("an action that lands must still leave the list: %v", m.chosen)
	}
}

func TestPanelsStackAndQGoesBack(t *testing.T) {
	inner := panel.Panel{ID: "env", Sections: []panel.Section{{Rows: []panel.Row{{ID: "up", Cells: []string{"up"}}}}}}
	outer := panel.Panel{ID: "dashboard",
		Actions:  map[string]panel.Action{"env": {Key: "v", Panel: "env-panel"}},
		Sections: []panel.Section{{Rows: []panel.Row{{ID: "iringa", Cells: []string{"iringa"}, Actions: []string{"env"}}}}}}
	m := newModel(outer)
	m.width, m.height = 100, 20
	m.reload = func() (panel.Panel, error) { return outer, nil }
	m.hooks.Load = func(name, target string) (panel.Panel, Reload, Hooks, error) {
		return inner, func() (panel.Panel, error) { return inner, nil }, Hooks{}, nil
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m = next.(model)
	next, _ = m.Update(loaded{name: "env-panel", panel: inner, reload: m.reload, hooks: Hooks{}})
	m = next.(model)
	if m.panel.ID != "env" || len(m.stack) != 1 || !strings.Contains(m.View(), "q back") {
		t.Fatalf("the env panel did not open over the dashboard: %s", m.panel.ID)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = next.(model)
	if m.panel.ID != "dashboard" || len(m.stack) != 0 {
		t.Errorf("q did not go back to the dashboard: %s", m.panel.ID)
	}
}

func TestAStackRowFoldsItsLayersAndActsOnTheTabsWorktree(t *testing.T) {
	p := panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{
			"why":      {Key: "d", Verb: "chat", Args: []string{"explain", "{row}", "{tab}"}, Report: true},
			"stack":    {Key: "S", Panel: "stack-panel"},
			"worktree": {Key: "W", Verb: "new", Args: []string{"{row}"}, Report: true},
		},
		Sections: []panel.Section{{ID: "quiet", Rows: []panel.Row{{ID: "stack:base", Target: "monduli", Cells: []string{"payments", "3 layers · 1 landed"},
			Actions: []string{"why", "stack"},
			Tabs:    []panel.Tab{{Name: "helper", Worktree: "longido", Note: "helper waiting 3s", Preview: []string{"?"}}},
			Children: []panel.Row{
				{ID: "monduli", Cells: []string{"monduli", "layer-b"}, Actions: []string{"stack"}},
				{ID: "longido", Cells: []string{"longido", "layer-a"}},
				{ID: "layer:base", Target: "base", Cells: []string{"base", "landed"}, Dim: true, Actions: []string{"worktree"}},
			}}}}}}
	m := newModel(p)
	m.width, m.height = 140, 30
	if len(m.rows) != 1 || !strings.HasPrefix(m.rows[0].Cells[0], "▸ ") {
		t.Fatalf("a stack starts folded with a marker: %v", m.rows)
	}
	if got := m.tabs(); got[0] != "helper (longido)" {
		t.Fatalf("the tab names its worktree: %v", got)
	}
	if argv := m.argvFor(p.Actions["why"], m.rows[0]); strings.Join(argv, " ") != "chat explain longido helper" {
		t.Errorf("helper actions go to the tab's worktree: %v", argv)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if len(m.rows) != 4 || !strings.HasPrefix(m.rows[0].Cells[0], "▾ ") || !strings.Contains(m.rows[1].Cells[0], "monduli") {
		t.Fatalf("⏎ did not unfold the layers: %v", m.rows)
	}
	m.cursor = 3
	if argv := m.argvFor(p.Actions["worktree"], m.rows[3]); strings.Join(argv, " ") != "new base" {
		t.Errorf("W on a layer without a worktree makes one for that branch: %v", argv)
	}
	m.cursor = 0
	m.hooks.Load = nil
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	m = next.(model)
	if m.chosen.Panel != "stack-panel" || m.chosen.RowID != "monduli" {
		t.Errorf("S on the stack row opens the stack panel on its top worktree: %+v", m.chosen)
	}
	m.query = "longido"
	m.expanded = map[string]bool{}
	m.rows = m.visibleRows()
	if len(m.rows) != 2 || !strings.Contains(m.rows[1].Cells[0], "longido") {
		t.Errorf("a filter reaches into a folded stack: %v", m.rows)
	}
}

func TestEveryFrameLineIsTheSameWidthWithAndWithoutTheSide(t *testing.T) {
	var rows []panel.Row
	for i := 0; i < 40; i++ {
		rows = append(rows, panel.Row{ID: fmt.Sprintf("w%02d", i), Cells: []string{fmt.Sprintf("w%02d", i), "branch"}})
	}
	p := panel.Panel{ID: "dashboard", Sections: []panel.Section{{ID: "quiet", Rows: rows}}}
	for _, width := range []int{80, 150} {
		m := newModel(p)
		m.width, m.height = width, 40
		widths := map[int]bool{}
		var lines []string
		for _, line := range strings.Split(m.View(), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			widths[lipgloss.Width(strings.TrimRight(line, " "))] = true
			lines = append(lines, line)
		}
		if len(widths) != 1 {
			t.Errorf("width %d: frame lines have %d different widths:\n%s", width, len(widths), strings.Join(lines[:4], "\n"))
		}
		if !strings.Contains(m.View(), "↓") || !strings.Contains(m.View(), "┃") {
			t.Errorf("width %d: a 40-row list shows no scrollbar or more-cue", width)
		}
	}
}

func TestWideCharactersKeepTheColumnsInLine(t *testing.T) {
	row := func(id, branch string) panel.Row {
		return panel.Row{ID: id, Cells: []string{id, branch, "0m"}}
	}
	p := panel.Panel{ID: "dashboard", Sections: []panel.Section{{ID: "quiet", Rows: []panel.Row{
		row("duluti", "日本語のタスク"), row("kiranyi", "🚀🔥"), row("monduli", "fix-the-login-bug"),
	}}}}
	m := newModel(p)
	m.width, m.height = 80, 20
	var ends []int
	for _, line := range strings.Split(m.View(), "\n") {
		if i := strings.Index(line, "0m"); i >= 0 {
			ends = append(ends, lipgloss.Width(line[:i]))
		}
	}
	if len(ends) != 3 || ends[0] != ends[1] || ends[1] != ends[2] {
		t.Errorf("the age column starts at %v, want one column for all three rows", ends)
	}
}

func TestWrapMeasuresScreenCells(t *testing.T) {
	for _, part := range wrap("日本語のタスクを直す日本語のタスクを直す", 10) {
		if lipgloss.Width(part) > 10 {
			t.Errorf("a wrapped line is %d cells wide: %q", lipgloss.Width(part), part)
		}
	}
	if got := wrap("the quick brown fox", 10); strings.Join(got, "|") != "the quick |brown fox" {
		t.Errorf("wrap broke words: %q", got)
	}
}

func TestANudgeRedrawsOnceWithoutAnotherTick(t *testing.T) {
	before := panel.Panel{ID: "dashboard", Sections: []panel.Section{{Rows: []panel.Row{{ID: "iringa", Cells: []string{"iringa", "", "old"}}}}}}
	after := panel.Panel{ID: "dashboard", Sections: []panel.Section{{Rows: []panel.Row{{ID: "iringa", Cells: []string{"iringa", "", "pushed"}}}}}}
	m := newModel(before)
	m.width, m.height = 100, 20
	m.reload = func() (panel.Panel, error) { return after, nil }
	forgot := false
	m.hooks.Refresh = func() { forgot = true }

	_, cmd := m.Update(nudged{})
	if cmd == nil {
		t.Fatal("a nudge did not reload")
	}
	if !forgot {
		t.Error("a nudge reloads from a cached listing; it must forget it so new sessions show")
	}
	msg := cmd()
	got, ok := msg.(reloaded)
	if !ok || !got.once {
		t.Fatalf("a nudge reloads once: %#v", msg)
	}
	next, again := m.Update(got)
	if again != nil {
		t.Error("a nudged reload must not start another refresh tick")
	}
	if !strings.Contains(next.(model).View(), "pushed") {
		t.Error("the pushed rows are not drawn")
	}
}
