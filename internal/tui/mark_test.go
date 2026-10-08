package tui

import (
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/panel"
)

func litAt(m model) []int {
	var lit []int
	for row, line := range m.loadingMark()[1:11] {
		for col, dot := range strings.Fields(stripANSI(line)) {
			if dot == "●" {
				lit = append(lit, row*10+col)
			}
		}
	}
	return lit
}

func stripANSI(s string) string {
	var b strings.Builder
	skip := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			skip = true
		case skip && r == 'm':
			skip = false
		case !skip:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestLoadingDrawsTheMarkAndItMoves(t *testing.T) {
	m := newModel(panel.Panel{Version: panel.Version, Title: "Worktrees / shop"})
	m.mode, m.label = working, loadingLabel
	m.width, m.height = 100, 30

	first := litAt(m)
	if want := []int{3, 14, 27, 31, 46, 52, 68, 75, 81, 99}; !equal(first, want) {
		t.Fatalf("the first frame is not the still mark: lit %v, want %v", first, want)
	}
	if !strings.Contains(stripANSI(m.View()), "loading") || m.footer() != "" {
		t.Errorf("loading shows the mark with its word and no spinner in the footer:\n%s", m.View())
	}

	if bottom := stripANSI(m.frameBottom()); !strings.HasPrefix(bottom, "╰──") || strings.Contains(bottom, "  ") {
		t.Errorf("the bottom border has a gap where the footer would be: %q", bottom)
	}

	m.spin = 50
	if equal(litAt(m), first) {
		t.Error("the mark does not move while loading")
	}

	m.mode, m.label = browsing, ""
	if strings.Contains(stripANSI(m.View()), "● ·") {
		t.Error("the mark stays after loading")
	}
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUnfoldingNeverMovesTheLayout(t *testing.T) {
	p := panel.Panel{ID: "dashboard", Sections: []panel.Section{
		{ID: "active", Label: "in progress", Rows: []panel.Row{
			{ID: "kijenge", Cells: []string{"kijenge", "due-dates", "uncommitted", "1m"}},
			{ID: "stack:billing", Cells: []string{"billing", "3 layers · 1 landed", "+2 −0", "1m"}, Children: []panel.Row{
				{ID: "olmatejo", Cells: []string{"olmatejo", "api", "+1", "1m"}},
				{ID: "layers:schema", Cells: []string{"2 layers, none checked out", "schema … billing", "", ""}},
			}},
		}},
		{ID: "quiet", Label: "quiet", Collapsed: true, ToggleKey: "e", Rows: []panel.Row{
			{ID: "ngarenaro", Cells: []string{"ngarenaro-with-a-long-name", "old-idea", "", "9d"}},
		}},
	}}
	m := newModel(p)
	m.width, m.height = 120, 30
	before, side := m.boxWidth(), m.side()
	m.cursor = 1
	m = press(m, runes("l"), runes("e"))
	if m.boxWidth() != before || m.side() != side {
		t.Errorf("unfolding moved the layout: width %d → %d, side pane %v → %v", before, m.boxWidth(), side, m.side())
	}
}

func TestAnOldRefreshCannotLandOnTheCurrentPanel(t *testing.T) {
	here := panel.Panel{ID: "stack", Sections: []panel.Section{{Rows: []panel.Row{{ID: "api", Cells: []string{"api"}}}}}}
	elsewhere := panel.Panel{ID: "dashboard", Sections: []panel.Section{{Rows: []panel.Row{{ID: "kijenge", Cells: []string{"kijenge"}}}}}}
	m := newModel(here)
	m.reload = func() (panel.Panel, error) { return here, nil }
	old := m.gen - 1

	next, cmd := m.Update(reloaded{panel: elsewhere, gen: old})
	if cmd != nil || next.(model).panel.ID != "stack" {
		t.Error("a reload from another panel's refresh replaced this one")
	}
	if _, cmd = m.Update(tick{gen: old}); cmd != nil {
		t.Error("an old refresh chain kept ticking")
	}
	if _, cmd = m.Update(tick{gen: m.gen}); cmd == nil {
		t.Error("this panel's own refresh stopped")
	}
}
