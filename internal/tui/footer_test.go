package tui

import (
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/panel"
)

func TestFooterIsJustTheKeysHint(t *testing.T) {
	p := panel.Panel{Hints: []string{"⏎ open", "n new", "A helper", "v env", "S stack", "p plan", "D delete", "e quiet"}}
	m := newModel(p)
	m.width = 80
	footer := m.footer()
	if !strings.Contains(footer, "? keys") || strings.Contains(footer, "n new") {
		t.Errorf("footer = %q, want only the keys hint", footer)
	}
}

func TestFooterNamesWhatTheSelectedRowCanDo(t *testing.T) {
	actions := map[string]panel.Action{
		"open":  {Key: "⏎", Label: "attach", Args: []string{"{row}"}},
		"adopt": {Key: "a", Label: "adopt", Args: []string{"{row}"}},
		"new":   {Key: "n", Label: "new", Args: []string{"{input}"}},
	}
	p := panel.Panel{Actions: actions, Sections: []panel.Section{{ID: "s", Rows: []panel.Row{
		{ID: "longido", Cells: []string{"longido"}, Actions: []string{"open"}},
		{ID: "stranger", Cells: []string{"stranger"}, Actions: []string{"adopt"}},
		{ID: "stack:base", Cells: []string{"payments"}, Children: []panel.Row{{ID: "monduli", Cells: []string{"monduli"}}}},
	}}}}
	m := newModel(p)
	for _, step := range []struct{ want, not string }{
		{"⏎ attach   n new   / find   ? keys", "adopt"},
		{"a adopt   n new   / find   ? keys", "attach"},
		{"⏎ unfold   n new   / find   ? keys", "attach"},
	} {
		footer := stripANSI(m.footer())
		if !strings.Contains(footer, step.want) || strings.Contains(footer, step.not) {
			t.Errorf("on %s the footer is %q, want %q", m.rows[m.cursor].ID, footer, step.want)
		}
		m.cursor++
	}
}
