package tui

import (
	"maps"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emusoi/mia-core/internal/plugin"
)

var clock = time.Now

// SideState is what a person chose for the plugins' sides in one
// repository: the tab showing on each side (plugin/id).
type SideState struct {
	Off  []string          `json:"off,omitempty"` // kept so an older file still reads
	Tabs map[string]string `json:"tabs,omitempty"`
}

// SideKeys name the sides of plugin.Places, in the same order, for --side.
var SideKeys = []string{"1", "2", "3", "4"}

func tabKey(tab plugin.SideTab) string { return tab.Plugin + "/" + tab.ID }

func (m model) tabsAt(place string) []plugin.SideTab {
	room := m.dashRoom()
	if room == nil {
		return nil
	}
	var tabs []plugin.SideTab
	for _, tab := range room.Sides {
		if tab.Where == place {
			tabs = append(tabs, tab)
		}
	}
	return tabs
}

func (m model) activeAt(place string, tabs []plugin.SideTab) int {
	for i, tab := range tabs {
		if tabKey(tab) == m.sides.Tabs[place] {
			return i
		}
	}
	return 0
}

// items is the active tab's lines of the selected side.
func (m model) items() []plugin.Item {
	tabs := m.tabsAt(m.sideSel)
	if len(tabs) == 0 {
		return nil
	}
	return tabs[m.activeAt(m.sideSel, tabs)].Lines
}

// nextItem is the line after (step 1) or before (step -1) from that opens
// something or has keys, or from itself when there is none; other lines
// cannot take the cursor.
func (m model) nextItem(from, step int) int {
	items := m.items()
	for i := from + step; i >= 0 && i < len(items); i += step {
		if items[i].Open != nil || len(items[i].Keys) > 0 {
			return i
		}
	}
	return from
}

// openItem is what ⏎ does on the line under the side's cursor.
func (m model) openItem() *plugin.Open {
	if items := m.items(); m.sideRow >= 0 && m.sideRow < len(items) {
		return items[m.sideRow].Open
	}
	return nil
}

// cycleSide shows the selected side's next (step 1) or previous tab.
func (m model) cycleSide(step int) model {
	tabs := m.tabsAt(m.sideSel)
	if len(tabs) == 0 {
		return m
	}
	m.sides.Tabs = maps.Clone(m.sides.Tabs)
	if m.sides.Tabs == nil {
		m.sides.Tabs = map[string]string{}
	}
	m.sides.Tabs[m.sideSel] = tabKey(tabs[(m.activeAt(m.sideSel, tabs)+step+len(tabs))%len(tabs)])
	m.sideRow = m.nextItem(-1, 1)
	m.save()
	return m
}

func (m model) save() {
	if m.hooks.Save != nil {
		m.hooks.Save(m.sides)
	}
}

// viewAt is the index in the room's sides of place's active tab, or -1.
func (m model) viewAt(place string) int {
	tabs := m.tabsAt(place)
	if len(tabs) == 0 {
		return -1
	}
	want := tabKey(tabs[m.activeAt(place, tabs)])
	for i, tab := range m.dashRoom().Sides {
		if tabKey(tab) == want {
			return i
		}
	}
	return -1
}

// widget is one side alone, filling the popup `mia side` opens it in: the
// side's view as home draws it, its tab's name over it, the keys under it.
func (m model) widget() string {
	width, height := max(m.width-4, 10), max(m.height, 4)
	place := m.hooks.Only
	title, body := place, []string{faint.Render("no plugin fills the " + place + " side")}
	switch {
	case m.loading():
		body = []string{faint.Render("Reading the worktrees…")}
	case m.mode == working:
		body = []string{faint.Render(m.label + " …")}
	case m.mode == reporting:
		body = m.report
	case m.mode == confirming:
		body = []string{ink.Render(m.pending.action.ConfirmFor(nameOf(m.pending.row))) + "    " + homeKey("y", "yes") + "    " + homeKey("n", "no")}
	default:
		if i := m.viewAt(place); i >= 0 {
			m.view = i
			title = m.dashRoom().Sides[i].Label
			body = m.viewList(width, height-5)
		}
	}
	lines := []string{"", muted.Bold(true).Render(title), ""}
	lines = append(lines, body...)
	for len(lines) < height-2 {
		lines = append(lines, "")
	}
	lines = lines[:height-2]
	foot := homeKey("j k", "move") + "    " + homeKey("⏎", "open") + "    " + homeKey("tab", "tabs") + "    " + homeKey("esc", "close")
	if m.problem != "" {
		foot = danger.Render(m.problem)
	}
	lines = append(lines, centred(foot, width), "")
	for i, line := range lines {
		lines[i] = "  " + fit(line, width)
	}
	return strings.Join(lines, "\n")
}

// widgetKey takes the keys of `mia side`: j k move, ⏎ opens, tab and
// shift+tab switch tabs, esc or q closes.
func (m model) widgetKey(key string) (tea.Model, tea.Cmd) {
	switch {
	case key == "esc" || key == "q" || key == "ctrl+c":
		return m, tea.Quit
	case key == "enter":
		return m.openSide()
	case key == "tab":
		return m.cycleSide(1), nil
	case key == "shift+tab":
		return m.cycleSide(-1), nil
	case m.nav(key) == "down":
		m.sideRow = m.nextItem(m.sideRow, 1)
	case m.nav(key) == "up":
		m.sideRow = m.nextItem(m.sideRow, -1)
	}
	return m, nil
}
