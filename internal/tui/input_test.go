package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emusoi/mia-core/internal/panel"
)

func TestBackspaceRemovesAWholeCharacter(t *testing.T) {
	p := panel.Panel{
		Version: panel.Version,
		Sections: []panel.Section{{Rows: []panel.Row{{
			ID: "monduli", Cells: []string{"monduli", "main"}, Actions: []string{"reply"},
		}}}},
		Actions: map[string]panel.Action{
			"reply": {Key: "m", Label: "reply", Verb: "chat", Args: []string{"send", "{row}", "{input}"}, Input: "reply"},
		},
	}
	typed := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("é")}
	backspace := tea.KeyMsg{Type: tea.KeyBackspace}

	m := newModel(p)
	updated, _ := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = updated.(model)
	updated, _ = m.key(typed)
	m = updated.(model)
	updated, _ = m.key(backspace)
	m = updated.(model)
	if m.input != "" {
		t.Errorf("reply after typing and deleting é = %q, want empty", m.input)
	}

	m = newModel(p)
	updated, _ = m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(model)
	updated, _ = m.key(typed)
	m = updated.(model)
	updated, _ = m.key(backspace)
	m = updated.(model)
	if m.query != "" || len(m.rows) != 1 {
		t.Errorf("filter after typing and deleting é = %q with %d rows, want empty with one row", m.query, len(m.rows))
	}
}
