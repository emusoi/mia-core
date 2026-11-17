package session

import (
	"fmt"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/run"
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

// What is what mia knows of a pane it starts: its kind, and its label,
// the title tmux shows for it — "claude", "nvim · main.go", "api :8080",
// "tests" — or "" when its command and folder say it as they go. The zero
// What is a pane mia knows nothing of, and marks nothing on.
type What struct {
	Kind  Kind
	Label string
}

// Mark keeps what a pane is on it, as its pane options @mia_kind and
// @mia_label.
func (h Host) Mark(pane string, what What) error {
	if what.Kind == "" {
		return nil
	}
	args := []string{"set-option", "-p", "-t", pane, "@mia_kind", string(what.Kind)}
	if what.Label != "" {
		args = append(args, ";", "set-option", "-p", "-t", pane, "@mia_label", what.Label)
	}
	// the persistent control client quotes tmux's command separator too
	if out, err := (run.Tool{Binary: "tmux", SSH: h.SSH}).Combined(args...); err != nil {
		return fmt.Errorf("mark pane %s %s: %s", pane, what.Kind, strings.TrimSpace(out))
	}
	return nil
}

// Title is what tmux calls a pane, and a window it names after its active
// pane: its label; else "shell", or the command it runs, and its folder.
// Kept as a format, tmux shows it as the pane goes, to anyone attached.
const Title = "#{?@mia_label,#{@mia_label},#{?#{m/r:^-?(zsh|bash|sh|fish|dash)$,#{pane_current_command}},shell,#{pane_current_command}} · #{b:pane_current_path}}"
