package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/plugin"
)

// popupRoom is the design's cost side as its 64-column popup shows it: the
// spend lines say nothing to open, each worktree's bar opens its cost.
func popupRoom() *panel.Room {
	bar := func(name string, full int, cost string) plugin.Item {
		return plugin.Item{
			Cells: cells(name, 14, "", "", strings.Repeat("━", full), 0, "", "green", strings.Repeat("─", 16-full), 0, "", "dim", cost, 28, "right", ""),
			Open:  &plugin.Open{Popup: []string{"mia", "cost", name}, Title: "cost / " + name},
		}
	}
	room := designRoom()
	for i, t := range room.Sides {
		if t.ID == "cost" {
			room.Sides[i].Lines = []plugin.Item{
				{Cells: cells("spent today", 52, "", "dim", "$5.76", 6, "right", "bold green")},
				{Cells: cells("7d ▁▂▂▃▅▇█", 52, "", "green", "$18.40", 6, "right", "")},
				{},
				bar("kinondoni", 16, "$2.93"), bar("mia-plugins", 9, "$1.72"), bar("ilala", 6, "$1.11"),
			}
		}
	}
	return room
}

func sideModel(t *testing.T) model {
	t.Helper()
	m := newModel(panel.Panel{Room: popupRoom()})
	m.hooks.Only = "right"
	next, _ := m.Update(tea.WindowSizeMsg{Width: 64, Height: 18})
	return next.(model)
}

func TestASideAloneIsTheDesignsPopup(t *testing.T) {
	data, err := os.ReadFile("testdata/side-popup.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := sideModel(t)
	got, want := strings.Split(plain(m.View()), "\n"), strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("%d lines, the design has %d", len(got), len(want))
	}
	for y := range want {
		if got[y] != strings.TrimRight(want[y], " ") {
			t.Errorf("row %d\n got: %q\nwant: %q", y, got[y], want[y])
		}
	}
}

func TestASideAloneTakesItsKeys(t *testing.T) {
	m := sideModel(t)
	if m.sideSel != "right" || m.sideRow != 3 {
		t.Fatalf("the cursor starts on the first line that opens something: %q %d", m.sideSel, m.sideRow)
	}
	m = press(m, runes("j"), runes("j"), runes("j"))
	if m.sideRow != 5 {
		t.Errorf("j stops at the last item: %d", m.sideRow)
	}
	m = press(m, runes("k"))
	if m.sideRow != 4 {
		t.Errorf("k moves up: %d", m.sideRow)
	}
	var shown []plugin.Open
	m.hooks.Show = func(open plugin.Open) error { shown = append(shown, open); return nil }
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil || cmd() != nil || len(shown) != 1 || shown[0].Title != "cost / mia-plugins" {
		t.Errorf("⏎ opens the item's popup as the room does: %+v", shown)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := plain(m.View()); !strings.HasPrefix(got, "╭──tokens─") {
		t.Errorf("tab shows the next tab, titled with it:\n%s", got)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyShiftTab}, tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := plain(m.View()); !strings.HasPrefix(got, "╭──machine─") {
		t.Errorf("shift+tab goes back round:\n%s", got)
	}
	for _, key := range []tea.KeyMsg{runes("q"), {Type: tea.KeyEsc}, runes("2"), runes("v")} {
		_, cmd := m.Update(key)
		quits := cmd != nil && cmd() == tea.Quit()
		if want := key.String() == "q" || key.String() == "esc"; quits != want {
			t.Errorf("%s quits: %v, want %v", key, quits, want)
		}
	}
}

func TestASideAloneShowsEvenWhenTheRoomHasItOff(t *testing.T) {
	m := newModel(panel.Panel{Room: popupRoom()})
	m.hooks.Only, m.sides.Off = "right", []string{"right"}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 64, Height: 18})
	if got := plain(next.(model).View()); !strings.Contains(got, "kinondoni") {
		t.Errorf("opening a side explicitly always shows it:\n%s", got)
	}
}
