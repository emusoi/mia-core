package tui

import (
	"regexp"
	"strings"

	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/plugin"
)

func cells(spec ...any) plugin.Line {
	var line plugin.Line
	for i := 0; i < len(spec); i += 4 {
		line = append(line, plugin.Cell{Text: spec[i].(string), Width: spec[i+1].(int), Align: spec[i+2].(string), Style: spec[i+3].(string)})
	}
	return line
}

func tab(owner, where, id string, rank int, count string, lines ...plugin.Line) plugin.SideTab {
	var items []plugin.Item
	for _, line := range lines {
		items = append(items, plugin.Item{Cells: line})
	}
	return plugin.SideTab{Plugin: owner, Side: plugin.Side{ID: id, Where: where, Label: id, Rank: rank}, SideAnswer: plugin.SideAnswer{Count: count, Lines: items}}
}

// issuesOpen makes the design's issue lines open in a popup, as mia-issue's do.
func issuesOpen(t plugin.SideTab) plugin.SideTab {
	for i, line := range t.Lines {
		if len(line.Cells) > 1 {
			ref := line.Cells[1].Text
			t.Lines[i].Open = &plugin.Open{Popup: []string{"wazo", "--local", "issues", "show", ref}, Title: "issues / " + ref}
		}
	}
	return t
}

// designRoom is the plugin answers the design's Dash page shows.
func designRoom() *panel.Room {
	bar := func(name string, full int, cost string) plugin.Line {
		return cells(name, 12, "", "dim", strings.Repeat("━", full), 0, "", "green", strings.Repeat("─", 16-full), 0, "", "dim", cost, 8, "right", "")
	}
	return &panel.Room{
		Repo: "mia-plugins",
		Sides: []plugin.SideTab{
			tab("agent", "top", "waiting", 10, "2",
				cells("●", 3, "", "yellow", "sinza", 11, "", "bold", "claude", 8, "", "dim", "Allow Bash(go test ./...)", 50, "", "", "3m", 6, "right", "dim"),
				nil,
				cells("●", 3, "", "yellow", "osunyai", 11, "", "bold", "codex", 8, "", "dim", "Push the branch to origin?", 50, "", "", "11m", 6, "right", "red")),
			tab("agent", "top", "answered", 20, "5"),
			issuesOpen(tab("issue", "left", "issues", 10, "4",
				cells("●", 2, "", "yellow", "LB-9", 7, "", "bold", "Deleting a sheet", 22, "", "", "ilala", 6, "right", "dim"),
				nil,
				cells("○", 2, "", "dim", "LB-14", 7, "", "bold", "PlusVibe → CRM", 22, "", "dim"),
				cells("○", 2, "", "dim", "LB-13", 7, "", "bold", "Support call booking", 22, "", "dim"),
				cells("!", 2, "", "red", "LB-15", 7, "", "bold", "MyCase integration", 22, "", "dim"),
				nil,
				cells("  13 more", 0, "", "dim"))),
			tab("gh", "left", "prs", 20, "3",
				cells("#41", 5, "", "bold", "docs-plugins", 15, "", "", "●", 2, "", "yellow", "review", 10, "", "dim", "2h", 4, "right", "dim"),
				nil,
				cells("#38", 5, "", "bold", "fix-login", 15, "", "", "✓", 2, "", "green", "approved", 10, "", "dim", "5h", 4, "right", "dim"),
				cells("#37", 5, "", "bold", "cost", 15, "", "", "○", 2, "", "dim", "draft", 10, "", "dim", "1d", 4, "right", "dim")),
			tab("plan", "left", "plans", 30, "4"),
			tab("cost", "right", "cost", 10, "",
				cells("spent today", 0, "", "dim"),
				nil,
				cells("$5.76", 0, "", "bold green"),
				cells("7d ", 0, "", "dim", "▁▂▂▃▅▇█", 0, "", "green", "  $18.40", 0, "", "dim"),
				nil,
				bar("kinondoni", 16, "$2.93"), bar("mia-plugins", 9, "$1.72"), bar("ilala", 6, "$1.11")),
			tab("cost", "right", "tokens", 20, ""),
			tab("machine", "right", "machine", 30, ""),
			tab("agent", "bottom", "activity", 10, "31",
				cells("8:41", 7, "", "dim", "●", 3, "", "yellow", "osunyai", 12, "", "bold", "waiting · push the branch to origin?", 70, "", "dim"),
				cells("8:12", 7, "", "dim", "✓", 3, "", "green", "kinondoni", 12, "", "bold", "finished · mia-cost committed 53d2ba8", 70, "", "dim"),
				cells("8:03", 7, "", "dim", "✓", 3, "", "green", "ilala", 12, "", "bold", "finished · mia-issue committed c2af06f", 70, "", "dim"),
				cells("7:49", 7, "", "dim", "›", 3, "", "blue", "kinondoni", 12, "", "bold", "started · build mia-cost", 70, "", "dim")),
			tab("gh", "bottom", "commits", 20, "6"),
			tab("gh", "bottom", "checks", 30, "4"),
		},
		Corners: []plugin.CornerText{
			{Plugin: "agent", Corner: plugin.Corner{ID: "agents", At: "bl"}, CornerAnswer: plugin.CornerAnswer{Segments: []plugin.Segment{
				{Text: "⠋ 2", Style: "blue"}, {Text: "  "}, {Text: "● 2", Style: "yellow"}, {Text: "  "}, {Text: "✓ 2", Style: "green"}, {Text: "  agents", Style: "dim"}}}},
			{Plugin: "cost", Corner: plugin.Corner{ID: "today", At: "br"}, CornerAnswer: plugin.CornerAnswer{Segments: []plugin.Segment{
				{Text: "$5.76", Style: "green"}, {Text: " today", Style: "dim"}}}},
		},
	}
}

var escapes = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(screen string) string {
	lines := strings.Split(escapes.ReplaceAllString(screen, ""), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}
