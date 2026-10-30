package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	// Grow lets the cell take the width a wide view has spare: a title.
	Grow bool `json:"grow,omitempty"`
	// Meter draws the cell as a bar, Value of Max, Width wide.
	Meter *Meter `json:"meter,omitempty"`
}

type Meter struct {
	Value float64 `json:"value"`
	Max   float64 `json:"max"`
}

// Filled is how many of width a meter's bar fills.
func (m Meter) Filled(width int) int {
	if m.Max <= 0 {
		return 0
	}
	return min(max(int(m.Value/m.Max*float64(width)+0.5), 0), width)
}

// Bar is one value of a chart.
type Bar struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	On    bool    `json:"on,omitempty"` // drawn in ink: today, the one that matters
}

type Line []Cell

// Item is one line of a side: its cells, and what ⏎ does on it. A plugin
// may answer a bare array of cells (nothing to open) or {"cells", "open"}.
// A Label makes the line a section's heading, its cells the count; a Chart
// makes it a small bar chart.
type Item struct {
	Cells Line   `json:"cells"`
	Open  *Open  `json:"open,omitempty"`
	Label string `json:"label,omitempty"`
	Chart []Bar  `json:"chart,omitempty"`
}

// Flat is the item as one line of plain cells, for the room's narrow sides:
// a label leads its cells, a chart is a row of block heights, a meter a bar.
func (it Item) Flat() Line {
	var line Line
	if it.Label != "" {
		line = append(line, Cell{Text: it.Label + "  ", Style: "dim"})
	}
	if len(it.Chart) > 0 {
		top := 0.0
		for _, b := range it.Chart {
			top = max(top, b.Value)
		}
		blocks := []rune("▁▂▃▄▅▆▇█")
		var spark []rune
		for _, b := range it.Chart {
			i := 0
			if top > 0 {
				i = min(int(b.Value/top*float64(len(blocks)-1)+0.5), len(blocks)-1)
			}
			spark = append(spark, blocks[i])
		}
		line = append(line, Cell{Text: string(spark)})
	}
	for _, c := range it.Cells {
		if c.Meter != nil {
			w := c.Width
			if w <= 0 {
				w = 10
			}
			n := c.Meter.Filled(w)
			c = Cell{Text: strings.Repeat("━", n) + strings.Repeat("╌", w-n), Width: w, Style: c.Style}
		}
		line = append(line, c)
	}
	return line
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
		Cells Line   `json:"cells"`
		Open  *Open  `json:"open"`
		Label string `json:"label"`
		Chart []Bar  `json:"chart"`
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
	it.Cells, it.Open, it.Label, it.Chart = object.Cells, object.Open, object.Label, object.Chart
	return nil
}

type Segment struct {
	Text  string `json:"text"`
	Style string `json:"style,omitempty"`
}

type SideAnswer struct {
	Count string `json:"count"`
	// Head and Note sit above the lines when the dash shows the side as a
	// view: the one thing it says, in bold, and a faint line under it.
	Head  string `json:"head,omitempty"`
	Note  string `json:"note,omitempty"`
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

type cachedAnswer[T any] struct {
	At      time.Time `json:"at"`
	Answer  T         `json:"answer"`
	Problem string    `json:"problem,omitempty"`
}

// fetch asks `mia-<name> <kind> <id>` (or the serve method <kind>), keeping
// the answer for every so the dash does not run the plugin on each redraw.
// A failure is kept for every as well, serve or not, so a plugin that times
// out costs one timeout per every rather than one per reload.
func fetch[T any](p Plugin, c Context, kind, id string, every time.Duration, input any) (T, error) {
	path := filepath.Join(p.DataDir(c), kind+"-"+id+".json")
	var last cachedAnswer[T]
	if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &last) == nil &&
		(p.Server == nil || last.Problem != "") && time.Since(last.At) < every {
		if last.Problem != "" {
			return last.Answer, errors.New(last.Problem)
		}
		return last.Answer, nil
	}
	var answer T
	out, err := p.ask(c, []string{kind, id}, kind, panelParams{ID: id, Input: input}, input)
	if err == nil {
		if bad := json.Unmarshal(out, &answer); bad != nil {
			err = fmt.Errorf("%s %s is not JSON: %w", kind, id, bad)
		}
	}
	if err != nil {
		if data, err := json.Marshal(cachedAnswer[T]{At: time.Now(), Problem: err.Error()}); err == nil {
			_ = os.WriteFile(path, data, 0o644)
		}
		return answer, err
	}
	if data, err := json.Marshal(cachedAnswer[T]{At: time.Now(), Answer: answer}); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
	return answer, nil
}

// AllSides asks every plugin for its sides and corners at once, each with
// input on stdin as rows gets it. Tabs come back grouped by place and ordered
// by rank, so a place's first tab is its default.
func AllSides(plugins []Plugin, contextOf func(Plugin) Context, input any) ([]SideTab, []CornerText) {
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
				answer, err := fetch[SideAnswer](p, c, "side", s.ID, every, input)
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
				answer, err := fetch[CornerAnswer](p, c, "corner", k.ID, every, input)
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
