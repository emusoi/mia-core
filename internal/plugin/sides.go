package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Places is where a side can sit around the dash, in drawing order.
var Places = []string{"top", "left", "right", "bottom"}

// Ats is where a corner can sit; the top two belong to mia itself.
var Ats = []string{"bl", "br"}

type Side struct {
	ID    string `json:"id"`
	Where string `json:"side"`
	Label string `json:"label"`
	Rank  int    `json:"rank"`
	Schedule
}

type Corner struct {
	ID string `json:"id"`
	At string `json:"at"`
	Schedule
}

type Cell struct {
	Text  string `json:"text"`
	Width int    `json:"width,omitempty"`
	Align string `json:"align,omitempty"`
	Style string `json:"style,omitempty"`
}

type Line []Cell

// Item is one line of a side: its cells, and what ⏎ does on it. A plugin
// may answer a bare array of cells (nothing to open) or {"cells", "open"}.
type Item struct {
	Cells Line  `json:"cells"`
	Open  *Open `json:"open,omitempty"`
}

// Open is what ⏎ on an item does: exactly one of a command in a tmux
// popup, the plugin's own verb, or landing in a worktree[:window].
type Open struct {
	Popup       []string `json:"popup,omitempty"`
	Title       string   `json:"title,omitempty"`
	Interactive bool     `json:"interactive,omitempty"`
	Verb        string   `json:"verb,omitempty"`
	Args        []string `json:"args,omitempty"`
	Report      bool     `json:"report,omitempty"`
	Lands       string   `json:"lands,omitempty"`
}

func (it *Item) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &it.Cells); err == nil {
		it.Open = nil
		return nil
	}
	var object struct {
		Cells Line  `json:"cells"`
		Open  *Open `json:"open"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	if o := object.Open; o != nil {
		ways := 0
		for _, set := range []bool{len(o.Popup) > 0, o.Verb != "", o.Lands != ""} {
			if set {
				ways++
			}
		}
		if ways != 1 {
			return fmt.Errorf("open needs exactly one of popup, verb or lands")
		}
	}
	it.Cells, it.Open = object.Cells, object.Open
	return nil
}

type Segment struct {
	Text  string `json:"text"`
	Style string `json:"style,omitempty"`
}

type SideAnswer struct {
	Count string `json:"count"`
	Lines []Item `json:"lines"`
}

type CornerAnswer struct {
	Segments []Segment `json:"segments"`
}

// SideTab is one tab of a side as the dash draws it: what the plugin
// declared and what it last answered, or why it did not.
type SideTab struct {
	Plugin string
	Side
	SideAnswer
	Problem string
}

type CornerText struct {
	Plugin string
	Corner
	CornerAnswer
	Problem string
}

type idParams struct {
	ID string `json:"id"`
}

type cachedAnswer[T any] struct {
	At     time.Time `json:"at"`
	Answer T         `json:"answer"`
}

// fetch asks `mia-<name> <kind> <id>` (or the serve method <kind>), keeping
// the answer for every so the dash does not run the plugin on each redraw.
func fetch[T any](p Plugin, c Context, kind, id string, every time.Duration) (T, error) {
	path := filepath.Join(p.DataDir(c), kind+"-"+id+".json")
	var last cachedAnswer[T]
	if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &last) == nil && p.Server == nil && time.Since(last.At) < every {
		return last.Answer, nil
	}
	var answer T
	out, err := p.ask(c, []string{kind, id}, kind, idParams{ID: id}, nil)
	if err != nil {
		return answer, err
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		return answer, fmt.Errorf("%s %s is not JSON: %w", kind, id, err)
	}
	if data, err := json.Marshal(cachedAnswer[T]{At: time.Now(), Answer: answer}); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
	return answer, nil
}

// AllSides asks every plugin for its sides and corners at once. Tabs come
// back grouped by place and ordered by rank, so a place's first tab is its default.
func AllSides(plugins []Plugin, contextOf func(Plugin) Context) ([]SideTab, []CornerText) {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		tabs    []SideTab
		corners []CornerText
	)
	for _, p := range plugins {
		c := contextOf(p)
		for _, s := range p.Manifest.Sides {
			wg.Go(func() {
				every, _ := s.Interval()
				tab := SideTab{Plugin: p.Name, Side: s}
				answer, err := fetch[SideAnswer](p, c, "side", s.ID, every)
				if err != nil {
					tab.Problem = firstLine(err.Error())
				} else {
					tab.SideAnswer = answer
				}
				mu.Lock()
				tabs = append(tabs, tab)
				mu.Unlock()
			})
		}
		for _, k := range p.Manifest.Corners {
			wg.Go(func() {
				every, _ := k.Interval()
				corner := CornerText{Plugin: p.Name, Corner: k}
				answer, err := fetch[CornerAnswer](p, c, "corner", k.ID, every)
				if err != nil {
					corner.Problem = firstLine(err.Error())
				} else {
					corner.CornerAnswer = answer
				}
				mu.Lock()
				corners = append(corners, corner)
				mu.Unlock()
			})
		}
	}
	wg.Wait()
	sort.Slice(tabs, func(i, j int) bool {
		a, b := tabs[i], tabs[j]
		if a.Where != b.Where {
			return a.Where < b.Where
		}
		if a.Rank != b.Rank {
			return a.Rank < b.Rank
		}
		return a.Plugin+"/"+a.ID < b.Plugin+"/"+b.ID
	})
	sort.Slice(corners, func(i, j int) bool { return corners[i].Plugin < corners[j].Plugin })
	return tabs, corners
}
