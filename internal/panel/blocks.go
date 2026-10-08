package panel

import (
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/session"
)

func Blocks(record model.Record, windows []session.Window, launches []string) Panel {
	name := record.Name
	p := Panel{
		Columns: []Column{{Name: "block"}, {Name: "what"}, {Name: "quiet", Kind: "time"}},
		Version: Version,
		ID:      "blocks",
		Title:   "Session / " + name,
		Actions: blockActions(name, launches),
		Hints:   []string{"⏎ open", "n shell", "e editor", "x close", "q back"},
		Empty:   "no session yet — ⏎ on the worktree opens one",
		Order:   []string{"open", "new", "editor", "close"},
	}
	var rows []Row
	for _, w := range windows {
		rows = append(rows, Row{
			ID:      strconv.Itoa(w.Index),
			Cells:   []string{w.Name, w.Command},
			Note:    "quiet " + since(w.Quiet),
			Actions: []string{"open", "close"},
			Preview: LastLines(w.Screen, 8),
			Facts:   []string{"window  " + w.Name, "running  " + w.Command, "quiet  " + since(w.Quiet)},
		})
	}
	if len(rows) > 0 {
		p.Sections = []Section{{ID: "windows", Label: "windows", Rows: rows}}
	}
	return p
}

func LastLines(screen string, limit int) []string {
	var lines []string
	for _, line := range strings.Split(screen, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimRight(line, " \t")+"\x1b[0m")
		}
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

func blockActions(name string, launches []string) map[string]Action {
	help := "a shell in a new window of this session"
	if len(launches) > 0 {
		help = "a new window: a name for a shell, or one of the config's [launch] commands"
	}
	return map[string]Action{
		"open":   {Lands: true, Key: "⏎", Label: "open", Verb: "window", Args: []string{"open", name, "{row}"}, Help: "land in this window"},
		"new":    {Key: "n", Label: "new window", Verb: "window", Args: []string{"new", name, "{input}"}, Input: "window", Choices: launches, Help: help},
		"editor": {Key: "e", Label: "editor", Verb: "window", Args: []string{"new", name, "--editor"}, Help: "your editor on the worktree, in a new window of this session"},
		"close":  {Key: "x", Label: "close", Verb: "window", Args: []string{"close", name, "{row}"}, Confirm: "Close window {row}? Whatever runs in it ends.", Help: "kill the window"},
	}
}
