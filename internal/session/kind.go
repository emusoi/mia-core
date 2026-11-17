package session

import (
	"fmt"
	"slices"
	"strings"
)

// Kind is what a pane is for. mia keeps it on the pane as tmux's pane
// option @mia_kind, so that tmux itself, mia and the apps all read it the
// same; a pane started by hand has none.
type Kind string

const (
	KindAgent  Kind = "agent"
	KindShell  Kind = "shell"
	KindEditor Kind = "editor"
	KindServer Kind = "server"
	KindTests  Kind = "tests"
	KindLogs   Kind = "logs"
)

// Kinds is every kind there is.
var Kinds = []Kind{KindAgent, KindShell, KindEditor, KindServer, KindTests, KindLogs}

// ParseKind is the kind a word names; a word that names none is refused.
func ParseKind(word string) (Kind, error) {
	if kind := Kind(word); slices.Contains(Kinds, kind) {
		return kind, nil
	}
	var all []string
	for _, kind := range Kinds {
		all = append(all, string(kind))
	}
	return "", fmt.Errorf("no kind %q: a pane is one of %s", word, strings.Join(all, ", "))
}

// What is what mia knows of a pane it starts: its kind. The zero What is
// a pane mia knows nothing of, and marks nothing on.
type What struct {
	Kind Kind
}

// Mark keeps what a pane is on it, as its pane options.
func (h Host) Mark(pane string, what What) error {
	if what.Kind == "" {
		return nil
	}
	if out, err := h.run("set-option", "-p", "-t", pane, "@mia_kind", string(what.Kind)); err != nil {
		return fmt.Errorf("mark pane %s %s: %s", pane, what.Kind, strings.TrimSpace(out))
	}
	return nil
}
