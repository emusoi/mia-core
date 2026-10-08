package panel

import "strings"

const Version = 2

type Panel struct {
	Version  int                 `json:"version"`
	ID       string              `json:"id"`
	Title    string              `json:"title"`
	Sections []Section           `json:"sections"`
	Actions  map[string]Action   `json:"actions"`
	Hints    []string            `json:"hints,omitempty"`
	Empty    string              `json:"empty,omitempty"`
	Order    []string            `json:"order,omitempty"`
	Columns  []Column            `json:"columns,omitempty"`
	Keys     map[string][]string `json:"keys,omitempty"`
}

func (p *Panel) Bind(keys map[string][]string) {
	for name, ks := range keys {
		if len(ks) == 0 {
			continue
		}
		if action, ok := p.Actions[name]; ok {
			action.Key = ks[0]
			p.Actions[name] = action
			continue
		}
		bound := false
		for i := range p.Sections {
			if p.Sections[i].ID == name {
				p.Sections[i].ToggleKey = ks[0]
				bound = true
			}
		}
		if !bound {
			if p.Keys == nil {
				p.Keys = map[string][]string{}
			}
			p.Keys[name] = ks
		}
	}
}

func (p Panel) Rank(id string) int {
	for i, o := range p.Order {
		if o == id {
			return i
		}
	}
	return len(p.Order) + 1
}

type Column struct {
	Name string
	Kind string
}

type Section struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Rows      []Row  `json:"rows"`
	Collapsed bool   `json:"collapsed,omitempty"`
	ToggleKey string `json:"toggle_key,omitempty"`
}

type Row struct {
	ID       string              `json:"id"`
	Glyph    string              `json:"glyph,omitempty"`
	Cells    []string            `json:"cells"`
	Note     string              `json:"note,omitempty"`
	Actions  []string            `json:"actions,omitempty"`
	Preview  []string            `json:"preview,omitempty"`
	Tabs     []Tab               `json:"tabs,omitempty"`
	Facts    []string            `json:"facts,omitempty"`
	Blocks   []string            `json:"blocks,omitempty"`
	Children []Row               `json:"children,omitempty"`
	Choices  map[string][]string `json:"choices,omitempty"`
	Target   string              `json:"target,omitempty"`
	Dim      bool                `json:"dim,omitempty"`
	Options  []string            `json:"options,omitempty"`
	Depth    int                 `json:"-"`
}

func (r Row) TargetID() string {
	if r.Target != "" {
		return r.Target
	}
	return r.ID
}

type Tab struct {
	Name     string   `json:"name"`
	Worktree string   `json:"worktree,omitempty"`
	Note     string   `json:"note,omitempty"`
	Preview  []string `json:"preview,omitempty"`
}

func (a Tab) Label() string {
	if a.Worktree != "" {
		return a.Name + " (" + a.Worktree + ")"
	}
	return a.Name
}

func (a Action) Matches(key string) bool {
	if a.Key == key {
		return true
	}
	for _, spelling := range keySpellings[a.Key] {
		if spelling == key {
			return true
		}
	}
	return false
}

var keySpellings = map[string][]string{
	"⏎": {"enter", "<CR>", "\r", "\n"},
	"⎋": {"esc", "escape", "<Esc>"},
	"⇥": {"tab", "<Tab>"},
}

type Action struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Verb    string   `json:"verb"`
	Args    []string `json:"args,omitempty"`
	Confirm string   `json:"confirm,omitempty"`
	Retry   []Action `json:"retry,omitempty"`
	When    string   `json:"when,omitempty"`
	Input   string   `json:"input,omitempty"`
	Choices []string `json:"choices,omitempty"`
	Panel   string   `json:"panel,omitempty"`
	Report  bool     `json:"report,omitempty"`
	Lands   bool     `json:"lands,omitempty"`
	Popup   bool     `json:"popup,omitempty"`
	Prefill bool     `json:"prefill,omitempty"`
	Pick    []string `json:"pick,omitempty"`
	Lines   bool     `json:"lines,omitempty"`
	Global  bool     `json:"global,omitempty"`
	Help    string   `json:"help,omitempty"`
}

func (a Action) NeedsRow() bool {
	if a.Global {
		return false
	}
	for _, arg := range a.Args {
		if arg == "{row}" {
			return true
		}
	}
	return a.Panel != ""
}

func (a Action) RetryFor(refusal string) (Action, bool) {
	for _, retry := range a.Retry {
		if retry.When == "" || strings.Contains(refusal, retry.When) {
			return retry, true
		}
	}
	return Action{}, false
}

const (
	GlyphWaiting  = "●"
	GlyphWorking  = "◐"
	GlyphDone     = "✓"
	GlyphDirty    = "±"
	GlyphDetached = "○"
	GlyphHere     = "▸"
	GlyphUnseen   = "◆"
)

func (p Panel) Rows() []Row {
	var rows []Row
	for _, section := range p.Sections {
		for _, row := range section.Rows {
			rows = append(rows, row)
			rows = append(rows, row.Children...)
		}
	}
	return rows
}

func (a Action) Command(rowID string) []string {
	return a.CommandWith(rowID, "")
}

func (a Action) CommandWith(rowID, input string) []string {
	argv := []string{a.Verb}
	for _, arg := range a.Args {
		switch arg {
		case "{row}":
			arg = rowID
		case "{input}":
			arg = input
		}
		argv = append(argv, arg)
	}
	return argv
}

// Argv is the mia command for the action: the row's target, the typed input,
// the side tab showing and the picked choice, each left out when empty the
// way the dashboard leaves them out.
func (a Action) Argv(target, input, tab, choice string) []string {
	argv := a.CommandWith(target, input)
	kept := argv[:0]
	for _, arg := range argv {
		switch arg {
		case "{tab}":
			if tab == "" {
				continue
			}
			arg = tab
		case "{choice}":
			if choice == "" {
				continue
			}
			arg = choice
		}
		kept = append(kept, arg)
	}
	return kept
}

func (a Action) ConfirmFor(rowID string) string {
	return substitute(a.Confirm, rowID)
}

func substitute(text, rowID string) string {
	out := ""
	for i := 0; i < len(text); i++ {
		if i+5 <= len(text) && text[i:i+5] == "{row}" {
			out += rowID
			i += 4
			continue
		}
		out += string(text[i])
	}
	return out
}

func (a Action) CLI() string {
	if a.Panel != "" {
		cli := "mia dash " + strings.TrimSuffix(a.Panel, "-panel")
		if a.NeedsRow() {
			cli += " <worktree>"
		}
		return cli
	}
	parts := []string{"mia", a.Verb}
	for _, arg := range a.Args {
		switch arg {
		case "{row}":
			arg = "<worktree>"
		case "{input}":
			arg = "<" + a.Input + ">"
		case "{tab}":
			arg = "[tab]"
		}
		parts = append(parts, arg)
	}
	return strings.Join(parts, " ")
}

var Moves = map[string][]string{
	"next": {" "}, "details": {"i"}, "scrollup": {"K"}, "scrolldown": {"J"},
	"down": {"j", "down"}, "up": {"k", "up"}, "first": {"g", "home"}, "last": {"G", "end"},
	"pagedown": {"pgdown", "ctrl+d", "ctrl+f"}, "pageup": {"pgup", "ctrl+u", "ctrl+b"},
	"filter": {"/"}, "help": {"?"}, "popup": {"tab"}, "refresh": {"r"},
	"tabnext": {"]"}, "tabprev": {"["}, "unfold": {"l", "right"}, "fold": {"h", "left"},
	"back": {"q", "esc"}, "quit": {"ctrl+c"},
}
