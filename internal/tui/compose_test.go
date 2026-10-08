package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emusoi/mia-core/internal/panel"
)

func replyPanel() panel.Panel {
	return panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{"reply": {Key: "m", Label: "reply to {tab}", Verb: "chat",
			Args: []string{"send", "--when-ready", "{row}", "{tab}", "{input}"}, Input: "reply", Lines: true}},
		Sections: []panel.Section{{ID: "blocked", Rows: []panel.Row{{ID: "longido", Glyph: panel.GlyphWaiting, Cells: []string{"longido"},
			Actions: []string{"reply"}, Tabs: []panel.Tab{{Name: "helper", Preview: []string{"Every verb?"}}}}}}}}
}

func press(m model, keys ...tea.KeyMsg) model {
	for _, k := range keys {
		next, _ := m.Update(k)
		m = next.(model)
	}
	return m
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestAReplyKeepsItsLinesAndItsDraft(t *testing.T) {
	m := newModel(panel.Panel{}).replace(replyPanel())
	m.width, m.height = 120, 36
	m = press(m, runes("m"), runes("a"), tea.KeyMsg{Type: tea.KeyEnter}, runes("b"))
	if m.mode != typing || m.input != "a\nb" {
		t.Fatalf("enter should add a line, got mode %v input %q", m.mode, m.input)
	}
	if v := m.View(); !strings.Contains(v, "Every verb?") || !strings.Contains(v, "^s") {
		t.Errorf("the composer should quote the tab and say how to send:\n%s", v)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(m.View(), "✎ draft") {
		t.Error("the row does not show its kept draft")
	}
	m = press(m, runes("m"))
	if m.input != "a\nb" {
		t.Fatalf("the draft came back as %q", m.input)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(model)
	if cmd == nil || m.mode == typing {
		t.Fatal("ctrl+s did not send")
	}
	p := replyPanel()
	failed := Chosen{Action: p.Actions["reply"], RowID: "longido", Draft: "reply:longido/helper"}
	m = press2(m, performed{chosen: failed, outcome: Outcome{Err: errors.New("not delivered — helper is working, not at its prompt")}})
	if m.hooks.Drafts["reply:longido/helper"] != "a\nb" || !strings.Contains(m.footer(), "draft kept") {
		t.Errorf("a failed send lost the draft or did not say so: %q %q", m.hooks.Drafts, m.footer())
	}
	m = press2(m, performed{chosen: failed, outcome: Outcome{Output: "sent to helper in longido"}})
	if _, kept := m.hooks.Drafts["reply:longido/helper"]; kept {
		t.Error("a delivered reply left its draft behind")
	}
}

func press2(m model, msg performed) model {
	next, _ := m.Update(msg)
	return next.(model)
}

func TestATaskNamesItsChoiceAndTabChangesIt(t *testing.T) {
	p := panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{"new": {Key: "n", Label: "new task", Verb: "new",
			Args: []string{"--task", "{input}", "--with", "{choice}"}, Input: "task", Lines: true, Choices: []string{"helper", "reviewer"}}},
		Sections: []panel.Section{{Rows: []panel.Row{{ID: "longido", Cells: []string{"longido"}}}}}}
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 120, 36
	m = press(m, runes("n"), runes("add help"), tea.KeyMsg{Type: tea.KeyTab})
	if !strings.Contains(m.View(), "→  reviewer") {
		t.Errorf("tab did not move to the next helper:\n%s", m.View())
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("ctrl+s did not launch")
	}
	_ = next
	if got := strings.Join(m.argvFor(p.Actions["new"], p.Sections[0].Rows[0]), " "); got != "new --task add help --with reviewer" {
		t.Errorf("argv = %q", got)
	}
}

func TestAKeyAnotherRowWouldUseStillReachesTheGlobalAction(t *testing.T) {
	p := panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{
			"layer": {Key: "n", Verb: "new", Args: []string{"--stack", "--in", "{row}", "{input}"}, Input: "layer name"},
			"new":   {Key: "n", Verb: "new", Args: []string{"--task", "{input}"}, Input: "task", Lines: true},
		},
		Sections: []panel.Section{{Rows: []panel.Row{{ID: "longido", Cells: []string{"longido"}}}}}}
	for range 20 {
		m := press(newModel(panel.Panel{}).replace(p), runes("n"))
		if m.mode != typing || m.pending.action.Input != "task" {
			t.Fatalf("n on a plain row did not open the task composer")
		}
	}
}

func TestEscLeavesALaunchRunningAndItsResultStillArrives(t *testing.T) {
	m := newModel(panel.Panel{}).replace(replyPanel())
	m.mode, m.label = working, "mia new --task add help"
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != browsing || !strings.Contains(m.footer(), "still running") {
		t.Fatalf("esc did not hand the list back: mode %v footer %q", m.mode, m.footer())
	}
	m = press(m, runes("m"))
	if m.mode != typing {
		t.Fatal("could not start a reply while the launch runs")
	}
	m = press2(m, performed{chosen: Chosen{Argv: []string{"new"}}, outcome: Outcome{Output: "task delivered to helper in add-help"}})
	if m.mode != typing {
		t.Error("the launch finishing threw away the reply being written")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	next, _ := m.Update(performed{chosen: Chosen{Argv: []string{"new"}}, outcome: Outcome{Output: "task delivered to helper in add-help"}})
	if !strings.Contains(next.(model).footer(), "task delivered") {
		t.Errorf("footer = %q", next.(model).footer())
	}
}

func TestDigitsAreTextInATaskAndDraftsDoNotCrossActions(t *testing.T) {
	p := replyPanel()
	p.Actions["new"] = panel.Action{Key: "n", Label: "new task", Verb: "new", Args: []string{"--task", "{input}", "--with", "{choice}"}, Input: "task", Lines: true, Choices: []string{"helper"}}
	m := newModel(panel.Panel{}).replace(p)
	m = press(m, runes("n"), runes("fix bug 1"))
	if m.input != "fix bug 1" {
		t.Fatalf("a digit replaced the task: %q", m.input)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc}, runes("m"))
	if m.input != "" {
		t.Errorf("the reply opened on the task's draft: %q", m.input)
	}
}

func TestARowsOwnActionWinsItsKeyEveryTime(t *testing.T) {
	p := panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{
			"layer": {Key: "n", Verb: "new", Args: []string{"--stack", "--in", "{row}", "{input}"}, Input: "layer name"},
			"new":   {Key: "n", Verb: "new", Args: []string{"--task", "{input}"}, Input: "task", Lines: true},
		},
		Sections: []panel.Section{{Rows: []panel.Row{{ID: "stack:a", Cells: []string{"a"}, Actions: []string{"layer"}}}}}}
	for range 20 {
		m := press(newModel(panel.Panel{}).replace(p), runes("n"))
		if m.pending == nil || m.pending.action.Input != "layer name" {
			t.Fatal("n on a stack row did not always mean new layer")
		}
	}
}

func TestOutsideTheDashboardTheSidePaneFollowsTheCursor(t *testing.T) {
	p := panel.Panel{ID: "files", Sections: []panel.Section{{Rows: []panel.Row{
		{ID: "a.go", Glyph: panel.GlyphWaiting, Cells: []string{"a.go"}, Preview: []string{"+a"}},
		{ID: "b.go", Cells: []string{"b.go"}, Preview: []string{"+b"}},
	}}}}
	m := newModel(p)
	m.width, m.height = 140, 30
	m = press(m, runes("j"))
	if v := m.View(); !strings.Contains(v, "+b") {
		t.Errorf("the diff did not follow the cursor to b.go:\n%s", v)
	}
}

func TestThePickerWalksItsColumns(t *testing.T) {
	p := replyPanel()
	p.Actions["lead"] = panel.Action{Key: "@", Label: "tell mia", Verb: "lead", Args: []string{"--with", "{choice}", "{input}"}, Input: "task", Lines: true, Global: true,
		Choices: []string{"helper", "helper - high", "helper opus", "helper opus high", "helper opus max", "scribe"}}
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 120, 30
	m = press(m, runes("@"), tea.KeyMsg{Type: tea.KeyTab})
	if !m.picking || !strings.Contains(m.View(), "third") {
		t.Fatalf("tab did not open the picker:\n%s", m.View())
	}
	right, down := tea.KeyMsg{Type: tea.KeyRight}, tea.KeyMsg{Type: tea.KeyDown}
	m = press(m, right, down, right, down, down, tea.KeyMsg{Type: tea.KeyEnter})
	if m.picking || m.choice != "helper opus max" {
		t.Errorf("picked %q, want helper opus max", m.choice)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyTab}, down, tea.KeyMsg{Type: tea.KeyEnter})
	if m.choice != "scribe" {
		t.Errorf("picked %q, want pi", m.choice)
	}
}

func TestOpenInRemembersWhereYouOpenedLastTime(t *testing.T) {
	p := replyPanel()
	p.Actions["editor"] = panel.Action{Key: "o", Label: "open in…", Verb: "open", Args: []string{"{row}", "--in", "{input}"}, Input: "open in", Choices: []string{"cursor", "finder"}}
	p.Sections[0].Rows[0].Actions = append(p.Sections[0].Rows[0].Actions, "editor")
	m := newModel(panel.Panel{}).replace(p)
	m = press(m, runes("o"))
	if m.input != "cursor" {
		t.Fatalf("o opened on %q, want the first opener", m.input)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter did not open")
	}
	m = next.(model)
	m.mode = browsing
	m = press(m, runes("o"))
	if m.input != "finder" {
		t.Errorf("o opened on %q, want finder, used last", m.input)
	}
}

func TestADigitAnswersAMenuAndEnterPressesIt(t *testing.T) {
	p := replyPanel()
	p.Sections[0].Rows[0].Options = []string{"1 Yes", "2 No"}
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 120, 30
	m = press(m, runes("m"))
	if !strings.Contains(m.View(), "1 Yes") {
		t.Fatalf("the menu's options are not shown:\n%s", m.View())
	}
	m = press(m, runes("2"))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || next.(model).mode == typing {
		t.Fatal("enter on an option did not send it")
	}
	if got := strings.Join(m.argvFor(p.Actions["reply"], p.Sections[0].Rows[0]), " "); !strings.HasSuffix(got, " 2") {
		t.Errorf("argv = %q, want the option key", got)
	}
}

func TestTheComposerDeletesAWordAndALine(t *testing.T) {
	key := func(k tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: k} }
	if got := edited("first line\nsecond word", key(tea.KeyCtrlW)); got != "first line\nsecond " {
		t.Errorf("ctrl+w left %q", got)
	}
	if got := edited("first line\nsecond word", key(tea.KeyCtrlU)); got != "first line\n" {
		t.Errorf("ctrl+u left %q", got)
	}
	if got := edited("one", key(tea.KeyCtrlU)); got != "" {
		t.Errorf("ctrl+u on one line left %q", got)
	}
}
