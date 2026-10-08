package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/emusoi/mia-core/internal/panel"
)

func queuePanel() panel.Panel {
	row := func(id, glyph string) panel.Row {
		return panel.Row{ID: id, Glyph: glyph, Cells: []string{id}, Actions: []string{"reply"}}
	}
	return panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{"reply": {Key: "m", Label: "reply", Verb: "chat", Args: []string{"send", "{row}", "{input}"}, Input: "reply"}},
		Sections: []panel.Section{
			{ID: "starred", Rows: []panel.Row{row("arusha", "")}},
			{ID: "blocked", Rows: []panel.Row{row("longido", panel.GlyphWaiting)}},
			{ID: "working", Rows: []panel.Row{row("nkasi", panel.GlyphWorking)}},
			{ID: "unseen", Rows: []panel.Row{row("kisongo", panel.GlyphUnseen)}},
		}}
}

func TestTheListOpensOnWhatNeedsYouAndSpaceWalksIt(t *testing.T) {
	m := newModel(panel.Panel{}).replace(queuePanel())
	if got := m.rows[m.cursor].ID; got != "longido" {
		t.Fatalf("opened on %s, want the waiting row", got)
	}
	var walked []string
	for range 3 {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
		m = next.(model)
		walked = append(walked, m.rows[m.cursor].ID)
	}
	if strings.Join(walked, " ") != "kisongo longido kisongo" {
		t.Errorf("space walked %v, want only rows that need you, wrapping", walked)
	}
}

func TestAnActionSaysItWentUntilTheNextKey(t *testing.T) {
	m := newModel(panel.Panel{}).replace(queuePanel())
	p := queuePanel()
	next, _ := m.Update(performed{chosen: Chosen{Action: p.Actions["reply"], Argv: []string{"chat", "send", "longido", "yes"}, RowID: "longido"}, outcome: Outcome{Output: "sent to helper in longido\n"}})
	m = next.(model)
	if !strings.Contains(m.footer(), "✓ sent to helper in longido") {
		t.Errorf("footer = %q, want the notice", m.footer())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if strings.Contains(next.(model).footer(), "✓") {
		t.Error("the notice outlived the next key")
	}
}

func TestAReplyMovesOnOnlyToAnotherTab(t *testing.T) {
	p := queuePanel()
	p.Sections[3].Rows[0].Glyph = panel.GlyphDirty
	m := newModel(panel.Panel{}).replace(p)
	next, _ := m.Update(performed{chosen: Chosen{Action: p.Actions["reply"], Argv: []string{"chat", "send", "longido", "yes"}, RowID: "longido"}, outcome: Outcome{Output: "sent to helper in longido\n"}})
	if got := next.(model).rows[next.(model).cursor].ID; got != "longido" {
		t.Errorf("a reply moved to %s, uncommitted work, instead of staying", got)
	}
}

func TestDetailsFillTheNarrowScreen(t *testing.T) {
	p := queuePanel()
	p.Sections[1].Rows[0].Facts = []string{"branch  fix/help-flag"}
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 80, 24
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(model)
	if v := m.View(); !strings.Contains(v, "fix/help-flag") || strings.Contains(v, "arusha") {
		t.Errorf("i did not show the row's details in place of the list:\n%s", v)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(model).details {
		t.Error("esc did not leave the details")
	}
}

func TestHelpNamesSpaceAndDetails(t *testing.T) {
	help := strings.Join(newModel(queuePanel()).help(), "\n")
	for _, want := range []string{"space", "next", "details"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}

func TestTheSwitcherStartsFilteringAndEnterOpensTheRow(t *testing.T) {
	p := queuePanel()
	p.Actions["open"] = panel.Action{Key: "⏎", Label: "attach", Verb: "shell", Args: []string{"{row}"}, Lands: true}
	for i := range p.Sections {
		for j := range p.Sections[i].Rows {
			p.Sections[i].Rows[j].Actions = []string{"open"}
		}
	}
	m := newModel(panel.Panel{})
	m.hooks.Switch = true
	m.mode, m.label = working, loadingLabel
	next, _ := m.Update(reloaded{panel: p, gen: m.gen})
	m = next.(model)
	if m.mode != filtering || m.side() {
		t.Fatalf("the switcher should open on the filter with no side pane, mode %v", m.mode)
	}
	m = press(m, runes("kis"))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if cmd == nil || m.chosen.RowID != "kisongo" || m.chosen.Argv[0] != "shell" {
		t.Fatalf("enter chose %+v", m.chosen)
	}
	m = newModel(p)
	m.hooks.Switch, m.mode = true, filtering
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc}); cmd == nil {
		t.Error("esc in the switcher should leave")
	}
}

func TestAOneShotPopupOpensOnItsActionShowsTheResultAndCloses(t *testing.T) {
	p := replyPanel()
	p.Sections[0].Rows = append(p.Sections[0].Rows, panel.Row{ID: "sinza", Cells: []string{"sinza"}, Actions: []string{"reply"}})
	m := newModel(panel.Panel{})
	m.hooks.Do, m.hooks.Here = "reply", "sinza"
	m.mode, m.label = working, loadingLabel
	next, _ := m.Update(reloaded{panel: p, gen: m.gen})
	m = next.(model)
	if m.mode != typing || m.pending.row.ID != "sinza" || m.side() {
		t.Fatalf("the popup should open on a reply to sinza, mode %v", m.mode)
	}
	m = press(m, runes("go"))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("ctrl+s did not send")
	}
	m = press2(next.(model), performed{chosen: Chosen{Action: p.Actions["reply"], RowID: "sinza"}, outcome: Outcome{Output: "sent to helper in sinza"}})
	if m.mode != reporting || !strings.Contains(m.View(), "✓ sent to helper in sinza") {
		t.Fatalf("the result was not shown:\n%s", m.View())
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Error("a key after the result should close the popup")
	}
}

func TestAfterMakingAWorktreeThePopupOffersToAttach(t *testing.T) {
	p := queuePanel()
	p.Actions["open"] = panel.Action{Key: "⏎", Label: "attach", Verb: "shell", Args: []string{"{row}"}, Lands: true}
	p.Actions["new"] = panel.Action{Key: "n", Label: "new", Verb: "new", Args: []string{"{input}"}, Input: "branch", Report: true}
	m := newModel(panel.Panel{})
	m.hooks.Do = "new"
	m.mode, m.label = working, loadingLabel
	next, _ := m.Update(reloaded{panel: p, gen: m.gen})
	m = next.(model)
	if !strings.Contains(m.frameTop(), "New worktree") {
		t.Errorf("title = %q", m.frameTop())
	}
	m = press(m, runes("fix-footer"))
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = press2(next.(model), performed{chosen: Chosen{Action: p.Actions["new"], Argv: []string{"new"}}, outcome: Outcome{Output: "fix-footer — fix-footer in a worktree of its own\n/w/app.fix-footer\n"}})
	if !strings.Contains(m.View(), "✓ made fix-footer") || !strings.Contains(m.footer(), "attach") {
		t.Fatalf("view:\n%s\nfooter %q", m.View(), m.footer())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := strings.Join(next.(model).chosen.Argv, " "); got != "shell fix-footer" {
		t.Errorf("enter chose %q, want to attach to the new worktree", got)
	}
}

func TestKScrollsTheTabsScreenBack(t *testing.T) {
	var screen []string
	for i := range 60 {
		screen = append(screen, fmt.Sprintf("line %02d", i))
	}
	p := panel.Panel{ID: "dashboard", Sections: []panel.Section{{Rows: []panel.Row{{ID: "longido", Glyph: panel.GlyphWaiting, Cells: []string{"longido"}, Preview: screen}}}}}
	m := newModel(p)
	m.width, m.height = 140, 30
	if v := m.View(); !strings.Contains(v, "line 59") || strings.Contains(v, "line 20") {
		t.Fatalf("the pane should end on the newest line:\n%s", v)
	}
	m = press(m, runes("K"), runes("K"))
	if v := m.View(); strings.Contains(v, "line 59") || !strings.Contains(v, "newer") {
		t.Errorf("K did not scroll back:\n%s", v)
	}
	m = press(m, runes("j"))
	if m.scroll != 0 {
		t.Error("moving the cursor did not bring the pane back to the newest line")
	}
}

func TestANewWorktreeInAFoldedSectionIsUnfoldedAndSelected(t *testing.T) {
	p := queuePanel()
	p.Sections = append(p.Sections, panel.Section{ID: "quiet", Collapsed: true, Rows: []panel.Row{{ID: "duluti", Cells: []string{"duluti"}}}})
	m := newModel(panel.Panel{}).replace(p)
	m.focus = "duluti"
	m = m.replace(p)
	if m.cursor >= len(m.rows) || m.rows[m.cursor].ID != "duluti" {
		t.Errorf("cursor on %v, want the new worktree, unfolded", m.rows)
	}
}

func TestAFailedActionStaysSaidUntilTheNextKey(t *testing.T) {
	p := queuePanel()
	m := newModel(panel.Panel{}).replace(p)
	m = press2(m, performed{chosen: Chosen{Argv: []string{"new"}}, outcome: Outcome{Err: errors.New("the task was not sent")}})
	next, _ := m.Update(reloaded{panel: p, gen: m.gen})
	if !strings.Contains(next.(model).footer(), "not sent") {
		t.Fatal("the refresh wiped the failure")
	}
	m = press(next.(model), runes("j"))
	if strings.Contains(m.footer(), "not sent") {
		t.Error("the failure outlived the next key")
	}
}

func TestALongTaskWrapsInTheComposer(t *testing.T) {
	p := replyPanel()
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 100, 30
	m = press(m, runes("m"), runes(strings.Repeat("word ", 40)+"END"))
	if v := m.View(); !strings.Contains(v, "END") || strings.Contains(v, "…") {
		t.Errorf("long text was cut instead of wrapped:\n%s", v)
	}
}

func TestKeysThatArriveTogetherAreTakenOneByOne(t *testing.T) {
	m := newModel(panel.Panel{}).replace(queuePanel())
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/kis")})
	m = next.(model)
	if m.mode != filtering || m.query != "kis" {
		t.Errorf("mode %v query %q, want filtering on kis", m.mode, m.query)
	}
}

func TestTabsInADiffDoNotSpillPastTheSidePane(t *testing.T) {
	p := panel.Panel{ID: "files", Sections: []panel.Section{{Rows: []panel.Row{
		{ID: "a.go", Cells: []string{"a.go"}, Preview: []string{"@@ -1 +1 @@", "+\t\t\tfor _, arg := range argv {"}},
	}}}}
	m := newModel(p)
	m.width, m.height = 120, 20
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "\t") || lipgloss.Width(line) > 120 {
			t.Fatalf("line spills: %q", line)
		}
	}
}

func TestAReportWrapsSoACommandCanBeCopiedWhole(t *testing.T) {
	m := newModel(queuePanel())
	m.width, m.height = 100, 30
	m.mode, m.report = reporting, []string{"cd '/Users/x/src/app.duluti' && gh pr create --base 'main' --head 'try/answer-flow' --title 'Add todo.md' --body-file '/Users/x/.git/mia/pr/try/answer-flow.md'"}
	if v := m.View(); !strings.Contains(v, "answer-flow.md'") || strings.Contains(v, "…") {
		t.Errorf("the command was cut:\n%s", v)
	}
}

func TestTheSidePaneStepsAsideRatherThanCutTheList(t *testing.T) {
	p := panel.Panel{ID: "dashboard", Sections: []panel.Section{{Rows: []panel.Row{
		{ID: "desktop-terminal-chrome", Cells: []string{"desktop-terminal-chrome", "6 layers · 5 landed", "uncommitted", "9m"}, Preview: []string{"x"}},
	}}}}
	m := newModel(p)
	m.width, m.height = 102, 25
	if m.side() {
		t.Error("at 102 columns the side pane would cut the status column")
	}
	m.width = 140
	if !m.side() {
		t.Error("at 140 columns there is room for both")
	}
}

func TestTheSelectedRowFitsWithoutASidePane(t *testing.T) {
	p := queuePanel()
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 80, 24
	m.hooks.Switch = true
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "longido") && strings.Contains(line, "…") {
			t.Errorf("the selected row was clipped: %q", line)
		}
	}
}

func TestTheSwitcherSwitchesToAStacksWorktreeInsteadOfFolding(t *testing.T) {
	p := panel.Panel{ID: "dashboard",
		Actions: map[string]panel.Action{"open": {Key: "⏎", Label: "attach", Verb: "shell", Args: []string{"{row}"}, Lands: true}},
		Sections: []panel.Section{{Rows: []panel.Row{{ID: "stack:chrome", Target: "mbozi", Cells: []string{"chrome"},
			Children: []panel.Row{{ID: "mbozi", Cells: []string{"mbozi"}}}}}}}}
	m := newModel(p)
	m.hooks.Switch, m.mode = true, filtering
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := strings.Join(next.(model).chosen.Argv, " "); got != "shell mbozi" {
		t.Errorf("enter chose %q, want to switch to the stack's worktree", got)
	}
}

func TestAPopupOpensOnItsWorktreeEvenWhenItIsFolded(t *testing.T) {
	p := queuePanel()
	p.Actions["chat"] = panel.Action{Key: "A", Label: "task here", Verb: "chat", Args: []string{"run", "--task", "{input}", "{row}"}, Input: "task", Lines: true}
	p.Sections = append(p.Sections, panel.Section{ID: "quiet", Collapsed: true, Rows: []panel.Row{{ID: "duluti", Cells: []string{"duluti"}, Actions: []string{"chat"}}}})
	m := newModel(panel.Panel{})
	m.hooks.Do, m.hooks.Here = "chat", "duluti"
	m.mode, m.label = working, loadingLabel
	next, _ := m.Update(reloaded{panel: p, gen: m.gen})
	m = next.(model)
	if m.mode != typing || m.pending.row.ID != "duluti" {
		t.Errorf("mode %v on %v, want the task composer on duluti", m.mode, m.pending)
	}
}

func TestAPopupKeepsItsTitleOnceTheResultShows(t *testing.T) {
	p := replyPanel()
	m := newModel(panel.Panel{})
	m.hooks.Do = "reply"
	m.mode, m.label = working, loadingLabel
	next, _ := m.Update(reloaded{panel: p, gen: m.gen})
	m = press(next.(model), runes("ok"), tea.KeyMsg{Type: tea.KeyCtrlS})
	m = press2(m, performed{chosen: Chosen{Action: p.Actions["reply"], RowID: "longido"}, outcome: Outcome{Output: "sent to helper in longido"}})
	if top := m.frameTop(); !strings.Contains(top, "Reply to helper · longido") {
		t.Errorf("title after the result: %q", top)
	}
}

func TestAKeyThatIsNotForThisRowSaysSo(t *testing.T) {
	p := queuePanel()
	p.Actions["env"] = panel.Action{Key: "E", Label: "env", Verb: "env", Args: []string{"status", "{row}"}}
	p.Sections[1].Rows[0].Children = []panel.Row{{ID: "ilala", Cells: []string{"ilala"}}}
	m := newModel(panel.Panel{}).replace(p)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if footer := next.(model).footer(); !strings.Contains(footer, "E env is not for longido ") {
		t.Errorf("footer = %q, want why E did nothing", footer)
	}
}

func TestARowWithoutFactsShowsItsWholeTextBeside(t *testing.T) {
	long := "none — `u` builds one from what the project already says it needs"
	p := panel.Panel{ID: "env-panel", Sections: []panel.Section{{ID: "environment", Rows: []panel.Row{{ID: "state", Cells: []string{"state", long}}}}}}
	m := newModel(panel.Panel{}).replace(p)
	pane := strings.Join(m.paneOf(0, 30, 20), " ")
	if !strings.Contains(pane, "needs") {
		t.Errorf("the side pane lacks the row's clipped text:\n%s", pane)
	}
}

func TestAProblemWidensAnEmptyBoxToBeRead(t *testing.T) {
	m := newModel(panel.Panel{ID: "stack", Title: "stack / nosuch"})
	m.width, m.height = 160, 20
	m.problem = `no worktree called "nosuch" — ` + "`mia ls`" + ` shows what exists`
	if !strings.Contains(m.View(), "shows what exists") {
		t.Errorf("the problem was clipped:\n%s", m.View())
	}
}

func TestTheBoxKeepsItsWidthWhileFiltering(t *testing.T) {
	m := newModel(panel.Panel{}).replace(queuePanel())
	m.width, m.height = 160, 30
	before := m.boxWidth()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "zzz" {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := next.(model).boxWidth(); got != before {
		t.Errorf("the box went from %d to %d columns while filtering", before, got)
	}
}

func TestAFilteredSectionCountsWhatMatches(t *testing.T) {
	p := queuePanel()
	p.Sections[1].Label = "blocked"
	p.Sections[1].Rows = append(p.Sections[1].Rows, panel.Row{ID: "kibaha", Cells: []string{"kibaha"}})
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 160, 30
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "longido" {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if view := next.(model).View(); !strings.Contains(view, "blocked  1") {
		t.Errorf("the heading counts rows the filter hides:\n%s", view)
	}
}

func TestAFoldedStacksLayersDoNotPushTheSidePaneOut(t *testing.T) {
	p := queuePanel()
	p.Sections[1].Rows[0].Children = []panel.Row{{ID: "deep", Cells: []string{"5 layers, none checked out", strings.Repeat("x", 60)}}}
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 133, 30
	if !m.side() {
		t.Error("a folded layer's width hid the side pane")
	}
}

func TestHelpWrapsALongDescriptionInsteadOfClippingIt(t *testing.T) {
	p := queuePanel()
	reply := p.Actions["reply"]
	reply.Help = "open the worktree in an editor, IDE, Finder or a terminal; a digit picks, tab cycles, the last one used comes first"
	p.Actions["reply"] = reply
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 100, 80
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if view := next.(model).View(); !strings.Contains(view, "comes first") {
		t.Errorf("the help was clipped:\n%s", view)
	}
}

func TestALongFactWrapsUnderItsLabel(t *testing.T) {
	p := queuePanel()
	p.Sections[1].Rows[0].Facts = []string{"last    Plans open with :Mia plan, lessons with :Mia lesson"}
	m := newModel(panel.Panel{}).replace(p)
	pane := strings.Join(m.paneOf(1, 40, 30), "\n")
	if !strings.Contains(pane, "lesson") {
		t.Errorf("the fact was clipped:\n%s", pane)
	}
}

func TestEscDismissesANoticeBeforeItQuits(t *testing.T) {
	m := newModel(panel.Panel{}).replace(queuePanel())
	m.notice = "E env is not for longido"
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || next.(model).notice != "" {
		t.Fatalf("esc with a notice showing did not just dismiss it (cmd %v)", cmd)
	}
	if _, cmd = next.Update(tea.KeyMsg{Type: tea.KeyEsc}); cmd == nil {
		t.Error("a second esc did not go back")
	}
}

func TestDetailsOpenEvenWhenTheSidePaneFits(t *testing.T) {
	p := queuePanel()
	p.Sections[1].Rows[0].Facts = []string{"branch  fix/help-flag"}
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 160, 30
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if v := next.(model).View(); !strings.Contains(v, "fix/help-flag") || strings.Contains(v, "arusha") {
		t.Errorf("i on a wide screen did not give the row's details the whole box:\n%s", v)
	}
}

func TestALongProblemIsShownWholeAboveTheFooter(t *testing.T) {
	m := newModel(panel.Panel{}).replace(queuePanel())
	m.width, m.height = 90, 30
	m.problem = "podman info: unable to connect to Podman socket: connection refused — `podman machine list` says which machines are up: start that one"
	v := m.View()
	if !strings.Contains(v, "start that one") {
		t.Errorf("the end of the problem was cut:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Errorf("a line is wider than the screen: %q", line)
		}
	}
}

func TestAFailedPopupFormReopensWithWhatWasTyped(t *testing.T) {
	p := queuePanel()
	p.Actions["new"] = panel.Action{Key: "n", Label: "new", Verb: "new", Args: []string{"{input}"}, Input: "branch", Report: true}
	m := newModel(panel.Panel{})
	m.hooks.Do = "new"
	m.mode, m.label = working, loadingLabel
	next, _ := m.Update(reloaded{panel: p, gen: m.gen})
	m = press(next.(model), runes("bad..x"))
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = press2(next.(model), performed{chosen: Chosen{Action: p.Actions["new"], Argv: []string{"new"}}, outcome: Outcome{Output: "mia: \"bad..x\" is not a valid branch name\n", Err: errors.New("exit status 1")}})
	if m.mode != typing || m.input != "bad..x" {
		t.Fatalf("mode %v input %q: the form did not come back with the text", m.mode, m.input)
	}
	if v := m.View(); !strings.Contains(v, "is not a valid branch") || strings.Contains(v, "nothing resent") {
		t.Errorf("the reason is missing or wrong:\n%s", v)
	}
}

func TestAReportThatFailedIsStillShownWhole(t *testing.T) {
	p := queuePanel()
	p.Actions["check"] = panel.Action{Key: "C", Label: "check the plan", Verb: "plan", Args: []string{"check", "{row}"}, Report: true}
	m := newModel(panel.Panel{}).replace(p)
	m.width, m.height = 120, 30
	next, _ := m.Update(performed{chosen: Chosen{Action: p.Actions["check"], RowID: "longido"}, outcome: Outcome{Output: "@ok   passed\n@bad  FAILED (3)\n    boom\n\n1/2 proven · 1 open\n", Err: errors.New("exit status 1")}})
	if v := next.(model).View(); !strings.Contains(v, "@ok   passed") || !strings.Contains(v, "boom") {
		t.Errorf("a failing check's report was cut to one line:\n%s", v)
	}
}
