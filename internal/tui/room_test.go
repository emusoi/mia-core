package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	return plugin.SideTab{Plugin: owner, Side: plugin.Side{ID: id, Where: where, Label: id, Rank: rank}, SideAnswer: plugin.SideAnswer{Count: count, Lines: lines}}
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
			tab("issue", "left", "issues", 10, "4",
				cells("●", 2, "", "yellow", "LB-9", 7, "", "bold", "Deleting a sheet", 22, "", "", "ilala", 6, "right", "dim"),
				nil,
				cells("○", 2, "", "dim", "LB-14", 7, "", "bold", "PlusVibe → CRM", 22, "", "dim"),
				cells("○", 2, "", "dim", "LB-13", 7, "", "bold", "Support call booking", 22, "", "dim"),
				cells("!", 2, "", "red", "LB-15", 7, "", "bold", "MyCase integration", 22, "", "dim"),
				nil,
				cells("  13 in backlog", 0, "", "dim")),
			tab("gh", "left", "prs", 20, "3"),
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

// designBox cuts the dash's box out of a design shot, so the room can be
// drawn around exactly the box the design has.
func designBox(t *testing.T, shot string) string {
	t.Helper()
	var box []string
	for _, line := range strings.Split(shot, "\n")[14:34] {
		runes := []rune(line + strings.Repeat(" ", 212))
		box = append(box, string(runes[46:166]))
	}
	return strings.Join(box, "\n")
}

func designModel(t *testing.T) model {
	t.Helper()
	was := clock
	clock = func() time.Time { return time.Date(2025, 10, 9, 8, 41, 0, 0, time.Local) }
	t.Cleanup(func() { clock = was })
	m := newModel(panel.Panel{Room: designRoom()})
	m.width, m.height = 212, 48
	return m
}

func matchesShot(t *testing.T, m model, file string) {
	t.Helper()
	data, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSuffix(string(data), "\n")
	got, ok := m.room(designBox(t, want))
	if !ok {
		t.Fatal("a 212×48 terminal has room for every side")
	}
	gotLines, wantLines := strings.Split(plain(got), "\n"), strings.Split(want, "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("%d lines, the design has %d", len(gotLines), len(wantLines))
	}
	for y := range wantLines {
		if gotLines[y] != wantLines[y] {
			t.Errorf("row %d\n got: %q\nwant: %q", y, gotLines[y], wantLines[y])
		}
	}
}

func TestTheRoomIsTheDesignsDashPage(t *testing.T) {
	matchesShot(t, designModel(t), "room.txt")
}

func TestTooSmallForAnySideIsTodaysDash(t *testing.T) {
	m := designModel(t)
	m.panel.Sections = []panel.Section{{Rows: []panel.Row{{ID: "iringa", Cells: []string{"iringa"}}}}}
	m.rows = m.visibleRows()
	m.width, m.height = 80, 24
	roomed := m.View()
	m.panel.Room = nil
	if plainDash := m.View(); roomed != plainDash {
		t.Errorf("80×24 should be exactly today's dash:\n%s\n---\n%s", plain(roomed), plain(plainDash))
	}
	m.panel.Room = designRoom()
	m.width, m.height = 212, 48
	if got := plain(m.View()); !strings.Contains(got, "v sides") || !strings.Contains(got, "waiting 2") || !strings.Contains(got, "iringa") {
		t.Errorf("212×48 draws the room around the box:\n%s", got)
	}
}

func TestASideIsDrawnOnlyWhereItFits(t *testing.T) {
	b := box{width: 212, height: 48, left: 29, top: 5, right: 180, bottom: 41}
	if b.fits("left") || b.fits("top") || !b.fits("right") || !b.fits("bottom") {
		t.Errorf("left needs 30 free columns and top 6 free rows: %+v", b)
	}
	if (box{width: 81, height: 48, top: 14, bottom: 33}).fits("top") {
		t.Error("the top side needs its 78 columns and the dots' margin")
	}
}

func TestTheLegendAndSidesOffAreTheDesignsPages(t *testing.T) {
	m := designModel(t)
	m.legend, m.sides.Off = true, []string{"right"}
	matchesShot(t, m, "room-legend.txt")

	m = designModel(t)
	m.sides.Off = []string{"top", "left", "right", "bottom"}
	matchesShot(t, m, "room-off.txt")
}

func TestTheLegendTogglesSidesAndTabsAndSavesThem(t *testing.T) {
	var saved []SideState
	m := designModel(t)
	m.hooks.Save = func(s SideState) { saved = append(saved, s) }

	if m = press(m, runes("w"), runes("c")); len(saved) != 0 || len(m.sides.Off) != 0 {
		t.Fatalf("outside the legend the sides take no keys: %+v", m.sides)
	}
	m = press(m, runes("v"))
	if !m.legend || !strings.Contains(plain(m.View()), "sides   w ● waiting") {
		t.Fatalf("v opens the legend:\n%s", plain(m.View()))
	}
	m = press(m, runes("]"))
	if m.sides.Tabs["top"] != "agent/answered" {
		t.Errorf("] with nothing toggled switches the top side's tab: %+v", m.sides)
	}
	m = press(m, runes("c"), runes("]"), runes("]"), runes("]"), runes("["))
	if strings.Join(m.sides.Off, " ") != "right" || m.sides.Tabs["right"] != "machine/machine" {
		t.Errorf("c turns the right side off and [ ] switch its tabs: %+v", m.sides)
	}
	if got := plain(m.View()); !strings.Contains(got, "c ○ machine") || strings.Contains(got, "spent today") {
		t.Errorf("a side turned off is not drawn and the legend says so:\n%s", got)
	}
	m = press(m, runes("c"), tea.KeyMsg{Type: tea.KeyEsc})
	if m.legend || len(m.sides.Off) != 0 || !strings.Contains(plain(m.View()), "v sides") {
		t.Errorf("c turns it back on and esc closes: %+v legend %v", m.sides, m.legend)
	}
	if last := saved[len(saved)-1]; len(saved) != 7 || len(last.Off) != 0 || last.Tabs["top"] != "agent/answered" {
		t.Errorf("every change is saved: %d %+v", len(saved), last)
	}
	m.width, m.height = 80, 24
	if m = press(m, runes("v")); m.legend {
		t.Error("with no room drawn v opens nothing")
	}
}

func TestWithTheBottomOffTheSpareRowGoesToTheTop(t *testing.T) {
	m := designModel(t)
	data, err := os.ReadFile("testdata/room.txt")
	if err != nil {
		t.Fatal(err)
	}
	m.sides.Off = []string{"bottom"}
	// 11 rows spare around the 20-row box: centered, the top gets 5 and the
	// bottom, which is off, 6 — a room with not one side in it.
	m.width, m.height = 140, 31
	got, ok := m.room(designBox(t, string(data)))
	if !ok || !strings.Contains(plain(got), "waiting 2") {
		t.Errorf("the top side takes the spare row the bottom does not use:\n%s", plain(got))
	}
}
