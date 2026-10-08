package panel

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/stack"
)

func Dashboard(listings []app.Listing, prefix string, extras []plugin.Rows) Panel {
	title := "Worktrees"
	for _, listing := range listings {
		if listing.Main {
			title += " / " + filepath.Base(listing.Path)
			break
		}
	}
	p := Panel{
		Columns: []Column{{Name: "name"}, {Name: "branch", Kind: "ref"}, {Name: "status"}, {Name: "age", Kind: "time"}},
		Version: Version,
		ID:      "dashboard",
		Title:   title,
		Actions: dashboardActions(),
		Hints:   []string{"⏎ open", "a adopt", "n new", "E env", "S stack", "D delete", "e quiet"},
		Empty:   "No worktrees yet. `n` here, or `mia new <branch>`.",
		Order: []string{
			"open", "shell", "editor", "blocks",
			"env", "dev", "land", "star",
			"stack", "begin", "layer", "apart", "claim", "worktree", "merge", "name", "restack", "forget",
			"adopt", "delete", "abandoned", "tidy", "new", "branch", "pr", "machines", "config",
		},
	}

	listings = append([]app.Listing(nil), listings...)
	sort.SliceStable(listings, func(i, j int) bool {
		if listings[i].Main != listings[j].Main {
			return listings[i].Main
		}
		return listings[i].LastWork.After(listings[j].LastWork)
	})
	ranks := map[string]int{"starred": 10, "active": 30, "quiet": 80, "unadopted": 90}
	declared := map[string]plugin.Section{}
	for _, extra := range extras {
		for _, section := range extra.Sections {
			if _, core := ranks[section.ID]; !core && declared[section.ID].ID == "" && section.ID != "" {
				declared[section.ID] = section
				ranks[section.ID] = section.Rank
			}
		}
	}
	claims := func(name string) []claim {
		var found []claim
		for _, extra := range extras {
			if info, ok := extra.Rows[name]; ok {
				rank, known := ranks[info.Section]
				found = append(found, claim{plugin: extra.Plugin, info: info, rank: rank, places: known && info.Section != "", tabs: extra.Tabs})
			}
		}
		return found
	}
	buckets := map[string][]Row{}
	grouped := map[string]bool{}
	byName := map[string]app.Listing{}
	for _, listing := range listings {
		byName[listing.Name] = listing
	}
	mainOnTop := false
	place := func(row Row, listing app.Listing, names []string) {
		id := "quiet"
		switch {
		case !listing.Adopted && !listing.Main:
			id = "unadopted"
		case listing.Main:
			buckets["starred"] = append([]Row{row}, buckets["starred"]...)
			mainOnTop = !listing.Starred
			return
		case listing.Starred:
			id = "starred"
		case listing.Dirty || listing.Ahead > 0:
			id = "active"
		}
		var all []claim
		for _, name := range names {
			all = append(all, claims(name)...)
		}
		if won, ok := winner(all, ranks[id]); ok && id != "starred" && id != "unadopted" {
			id = won.info.Section
			if won.info.Status != "" {
				row.Cells[2] = won.info.Status
			}
		}
		buckets[id] = append(buckets[id], row)
	}
	decorate := func(row *Row, ownsStatus bool) {
		all := claims(row.ID)
		won, placed := winner(all, int(^uint(0)>>1))
		placed = placed && ownsStatus
		for _, c := range all {
			row.Facts = append(row.Facts, c.info.Facts...)
			for _, tab := range c.tabs {
				if lines := c.info.Tabs[tab.ID]; len(lines) > 0 {
					row.Tabs = append(row.Tabs, Tab{Name: tab.Label, Preview: lines})
				}
			}
			if c.info.Note != "" && row.Note == "" {
				row.Note = c.info.Note
			}
			switch {
			case c.info.Status == "":
			case placed && c.plugin == won.plugin:
				row.Cells[2] = c.info.Status
			default:
				row.Facts = append(row.Facts, c.plugin+"  "+c.info.Status)
			}
		}
	}

	shown := map[string]bool{}
	for _, listing := range listings {
		if len(listing.Layers) == 0 {
			row := dashboardRow(listing, prefix)
			decorate(&row, false)
			place(row, listing, []string{listing.Name})
			continue
		}
		bottom := listing.Layers[0].Branch
		if grouped[bottom] {
			continue
		}
		grouped[bottom] = true
		row, summary := stackRow(listing, byName, prefix)
		var members []string
		for i := range row.Children {
			shown[row.Children[i].ID] = true
			if _, ok := byName[row.Children[i].ID]; ok {
				decorate(&row.Children[i], true)
				members = append(members, row.Children[i].ID)
			}
		}
		place(row, summary, members)
	}
	for _, listing := range listings {
		if len(listing.Layers) > 0 && !shown[listing.Name] {
			row := dashboardRow(listing, prefix)
			row.Facts = append(row.Facts, "stack  a fork off "+listing.Layers[0].Branch+" — not on the stack's main line")
			decorate(&row, false)
			place(row, listing, []string{listing.Name})
		}
	}
	onlyQuiet := len(buckets["starred"])+len(buckets["active"])+len(buckets["unadopted"]) == 0
	for id, section := range declared {
		if !section.Collapsed {
			onlyQuiet = onlyQuiet && len(buckets[id]) == 0
		}
	}
	sections := []Section{
		{ID: "starred", Label: starredLabel(mainOnTop, len(buckets["starred"]))},
		{ID: "active", Label: "in progress"},
		{ID: "quiet", Label: "quiet", Collapsed: !onlyQuiet, ToggleKey: "e"},
		{ID: "unadopted", Label: "not mia's"},
	}
	for _, section := range declared {
		folded := Section{ID: section.ID, Label: section.Label, Collapsed: section.Collapsed}
		if folded.Collapsed {
			folded.ToggleKey = "e"
		}
		sections = append(sections, folded)
	}
	sort.SliceStable(sections, func(i, j int) bool {
		if ranks[sections[i].ID] != ranks[sections[j].ID] {
			return ranks[sections[i].ID] < ranks[sections[j].ID]
		}
		return sections[i].ID < sections[j].ID
	})
	for _, section := range sections {
		if section.Rows = buckets[section.ID]; len(section.Rows) > 0 {
			p.Sections = append(p.Sections, section)
		}
	}
	var problems []string
	for _, extra := range extras {
		if extra.Problem != "" {
			problems = append(problems, "plugin  "+extra.Plugin+": "+extra.Problem)
		}
	}
	problems = append(problems, addKeys(&p, extras, byName)...)
	if len(p.Sections) > 0 && len(p.Sections[0].Rows) > 0 {
		first := &p.Sections[0].Rows[0]
		first.Facts = append(first.Facts, problems...)
	}
	return p
}

func stackRow(first app.Listing, byName map[string]app.Listing, prefix string) (Row, app.Listing) {
	var children []Row
	var members []app.Listing
	summary := app.Listing{Adopted: true}
	landed, restack := 0, ""
	below := func(of stack.Layer) []string {
		var names []string
		for j := len(first.Layers) - 1; j >= 0; j-- {
			if first.Layers[j].Branch == of.Branch {
				for k := j - 1; k >= 0; k-- {
					names = append(names, first.Layers[k].Branch)
				}
			}
		}
		return names
	}
	var bare []stack.Layer
	flush := func() {
		if len(bare) == 0 {
			return
		}
		if len(bare) == 1 {
			layer := bare[0]
			children = append(children, Row{
				ID:      layer.Branch,
				Cells:   []string{strings.TrimPrefix(layer.Branch, prefix), layerState(layer), "", ""},
				Note:    "no worktree · ⏎ or W",
				Dim:     true,
				Actions: []string{"worktree", "claim"},
				Facts:   []string{"layer  " + layer.Branch, "on  " + layer.Parent, "state  " + layerState(layer)},
			})
		} else {
			var facts []string
			var rows []Row
			for _, layer := range bare {
				facts = append(facts, "layer  "+layer.Branch+"  "+layerState(layer))
				rows = append(rows, Row{
					ID:      layer.Branch,
					Cells:   []string{strings.TrimPrefix(layer.Branch, prefix), layerState(layer), "", ""},
					Note:    "no worktree · W",
					Dim:     true,
					Actions: []string{"worktree"},
					Facts:   []string{"layer  " + layer.Branch, "on  " + layer.Parent, "state  " + layerState(layer)},
				})
			}
			children = append(children, Row{
				ID:       "layers:" + bare[0].Branch,
				Target:   first.Name,
				Cells:    []string{fmt.Sprintf("%d layers", len(bare)), strings.TrimPrefix(bare[0].Branch, prefix) + " … " + strings.TrimPrefix(bare[len(bare)-1].Branch, prefix), "", ""},
				Note:     "none checked out · ⏎ unfolds",
				Dim:      true,
				Actions:  []string{"stack"},
				Facts:    facts,
				Children: rows,
			})
		}
		bare = nil
	}
	for i := len(first.Layers) - 1; i >= 0; i-- {
		layer := first.Layers[i]
		if layer.Landed {
			landed++
		}
		if layer.NeedsRestack && restack == "" {
			restack = layer.Branch
		}
		member, ok := byName[layer.Worktree]
		if !ok || layer.Worktree == "" {
			bare = append(bare, layer)
			continue
		}
		flush()
		members = append(members, member)
		child := dashboardRow(member, prefix)
		child.Cells[2] = fmt.Sprintf("+%d", layer.Ahead)
		if layer.Landed {
			child.Cells[2] = "landed"
		}
		if layer.NeedsRestack && child.Note == "" {
			child.Note = "behind " + layer.Parent + " · R"
		}
		if i > 0 {
			child.Actions = append(child.Actions, "merge")
			child.Choices = map[string][]string{"merge": below(layer)}
		}
		children = append(children, child)
		summary.Starred = summary.Starred || member.Starred
		summary.Dirty = summary.Dirty || member.Dirty
		if member.Seen.After(summary.Seen) {
			summary.Seen = member.Seen
		}
		if member.LastWork.After(summary.LastWork) {
			summary.LastWork = member.LastWork
		}
	}
	flush()
	top := first
	if len(members) > 0 {
		top = members[0]
	}
	summary.Ahead, summary.Behind = top.Ahead, top.Behind
	name := strings.TrimPrefix(first.Stack, prefix)
	if name == "" {
		name = strings.TrimPrefix(first.Layers[0].Branch, prefix)
	}
	glyph := ""
	if summary.Dirty {
		glyph = GlyphDirty
	}
	shape := fmt.Sprintf("%d layers · %d landed", len(first.Layers), landed)
	facts := []string{"stack  " + name, "layers  " + shape, "top  " + first.Layers[len(first.Layers)-1].Branch}
	if summary.Starred {
		facts = append(facts, "starred  yes — * on the stack row unstars its top worktree")
	}
	if restack != "" {
		facts = append(facts, "restack  "+restack+" is behind its parent")
	}
	actions := []string{"star", "stack", "layer", "apart", "name", "restack", "forget"}
	return Row{
		ID:       "stack:" + first.Layers[0].Branch,
		Target:   top.Name,
		Glyph:    glyph,
		Cells:    []string{name, shape, strings.TrimSpace(stackStatus(summary, top)), ageCell(summary.LastWork)},
		Actions:  actions,
		Facts:    facts,
		Children: children,
	}, summary
}

func layerState(layer stack.Layer) string {
	state := fmt.Sprintf("%d commit(s)", layer.Ahead)
	switch {
	case layer.Landed:
		state = "landed"
	case layer.Ahead == 0:
		state = "not started"
	}
	if layer.NeedsRestack {
		state += " · needs restack"
	}
	return state
}

func driftCell(ahead, behind int) string {
	if ahead == 0 && behind == 0 {
		return "        "
	}
	return fmt.Sprintf("%8s", fmt.Sprintf("+%d −%d", ahead, behind))
}

func ageCell(t time.Time) string {
	if t.IsZero() {
		return "    "
	}
	return fmt.Sprintf("%4s", since(time.Since(t)))
}

func dashboardRow(listing app.Listing, prefix string) Row {
	branch := strings.TrimPrefix(listing.Branch, prefix)
	glyph := ""
	if listing.Dirty {
		glyph = GlyphDirty
	}
	if branch == "" {
		if glyph == "" {
			glyph = GlyphDetached
		}
		branch = "(detached)"
	}

	facts := []string{"branch  " + listing.Branch}
	if listing.Branch == "" {
		facts[0] = "branch  (detached)"
	}
	if listing.Main {
		facts = append(facts, "main  the repository itself — always first")
	}
	if listing.Ahead > 0 || listing.Behind > 0 {
		facts = append(facts, fmt.Sprintf("drift   +%d ahead, −%d behind", listing.Ahead, listing.Behind))
	}
	if listing.Files > 0 {
		facts = append(facts, fmt.Sprintf("changes  %d file(s) · +%d −%d", listing.Files, listing.Added, listing.Deleted))
	}
	if listing.LastSubject != "" {
		facts = append(facts, "last    "+listing.LastSubject+"  ·  "+since(time.Since(listing.LastWork))+" ago")
	}
	if listing.Where != "" && listing.Where != "local" {
		facts = append(facts, "runs    "+listing.Where)
	}
	if listing.Dirty {
		facts = append(facts, "tree    uncommitted changes")
	}

	if listing.Session {
		facts = append(facts, "session  open")
	}
	if listing.Dev != "" {
		facts = append(facts, "dev     "+listing.Dev)
	}

	actions := []string{"open", "shell"}
	switch {
	case listing.Main && !listing.Adopted:
		actions = []string{"adopt"}
	case listing.Main:
		actions = []string{"open", "shell", "editor", "env", "stack"}
	case !listing.Adopted:
		actions = []string{"adopt"}
	default:
		actions = append(actions, "editor", "star", "env", "land", "stack", "delete")
	}
	if listing.Adopted && listing.Branch != "" && len(listing.Layers) == 0 {
		actions = append(actions, "begin")
	}
	if (listing.Adopted && !listing.Main) || (listing.Main && listing.Dev != "") {
		actions = append(actions, "dev")
	}
	if listing.Branch == "" {
		actions = without(actions, "stack", "land")
	}
	if listing.Session {
		actions = append(actions, "blocks")
	}
	return Row{
		ID:      listing.Name,
		Glyph:   glyph,
		Cells:   []string{listing.Name, branch, status(listing), ageCell(listing.LastWork)},
		Actions: actions,
		Facts:   facts,
		Blocks:  blocksOf(listing.Windows),
	}
}

func status(listing app.Listing) string {
	if listing.Dirty {
		return "uncommitted"
	}
	return strings.TrimSpace(driftCell(listing.Ahead, listing.Behind))
}

func starredLabel(mainOnTop bool, rows int) string {
	switch {
	case mainOnTop && rows == 1:
		return "main"
	case mainOnTop:
		return "main · starred"
	}
	return "starred"
}

func stackStatus(summary, top app.Listing) string {
	if summary.Dirty {
		return "uncommitted"
	}
	return driftCell(top.Ahead, top.Behind)
}

func without(actions []string, drop ...string) []string {
	kept := actions[:0:0]
	for _, action := range actions {
		dropped := false
		for _, d := range drop {
			dropped = dropped || action == d
		}
		if !dropped {
			kept = append(kept, action)
		}
	}
	return kept
}

func dashboardActions() map[string]Action {
	return map[string]Action{
		"open": {
			Lands: true, Key: "⏎", Label: "attach", Verb: "shell", Args: []string{"{row}"},
			Help: "attach to the worktree's tmux session; in the editor, switch to it",
		},
		"shell": {
			Lands: true, Key: "s", Label: "shell", Verb: "shell", Args: []string{"{row}"},
			Help: "a shell in the worktree's session, even from the editor",
		},
		"land": {
			Key: "Y", Label: "land", Verb: "pr", Args: []string{"{row}"}, Report: true,
			Help: "the pull request mia would draft for this branch and the push and create commands to run yourself; mia never pushes",
		},
		"abandoned": {
			Key: "z", Label: "what is abandoned", Verb: "gc", Report: true, Global: true,
			Help: "worktrees nobody has touched in 14 days: clean, no session, not starred",
		},
		"tidy": {
			Key: "Z", Label: "remove the abandoned", Verb: "gc", Args: []string{"--apply"}, Report: true, Global: true,
			Confirm: "Remove every worktree that is clean, has no session, is not starred and has been quiet 14 days? z lists them first.",
			Help:    "mia gc --apply: what z lists, removed",
		},
		"editor": {
			Key: "o", Label: "open in…", Verb: "open", Args: []string{"{row}", "--in", "{input}"}, Input: "open in",
			Help: "open the worktree in an editor, IDE, Finder or a terminal; a digit picks, ⇥ cycles",
		},
		"star": {
			Key: "*", Label: "star / unstar", Verb: "star", Args: []string{"{row}"},
			Help: "mark the worktree you are on right now; it floats to its own section",
		},
		"adopt": {
			Key: "a", Label: "adopt", Verb: "adopt", Args: []string{"{row}"},
			Help: "take over a worktree made by hand: give it a name and a record",
		},
		"new": {
			Key: "n", Label: "new", Verb: "new", Args: []string{"{input}"}, Input: "branch", Report: true,
			Help: "a new worktree for a branch, new or existing, with its setup run; you stay on the list",
		},
		"pr": {
			Lands: true, Key: "P", Label: "open a pull request", Verb: "new", Args: []string{"--shell", "--pr", "{input}"}, Input: "pull request number", Global: true,
			Help: "fetch a pull request's head into a worktree of its own (pr-<number>) and go there",
		},
		"branch": {
			Lands: true, Key: "+", Label: "new worktree", Verb: "new", Args: []string{"--shell", "{input}"}, Input: "branch",
			Help: "create a worktree for a branch (new or existing), run setup, and go there",
		},
		"dev": {
			Key: "v", Label: "on main's dev server", Verb: "dev", Args: []string{"{row}"}, Report: true,
			Help: "copy the worktree's files over the main checkout, so its running dev server shows them; v on main gives main its own back",
		},
		"env": {
			Key: "E", Label: "environment", Panel: "env-panel",
			Help: "the container this worktree's code runs in: up, down, shell, browse, host",
		},
		"stack": {
			Key: "S", Label: "stack", Panel: "stack-panel",
			Help: "the layers of this branch's stack",
		},
		"blocks": {
			Key: "t", Label: "session windows", Panel: "blocks-panel",
			Help: "the worktree's tmux windows — open one, add a shell, close one",
		},
		"merge": {
			Key: "M", Label: "merge down", Verb: "stack", Args: []string{"merge", "{row}", "onto", "{input}"}, Input: "onto", Prefill: true, Report: true,
			Confirm: "Merge down onto {input}? Every layer above it, up to this one, folds into it and disappears.",
			Help:    "fast-forward a lower layer to this one; the layers in between fold into it — never into the base branch",
		},
		"forget": {
			Key: "D", Label: "forget the stack", Verb: "stack", Args: []string{"rm", "--in", "{row}"}, Report: true,
			Confirm: "Forget this stack? Its layers stay as ordinary branches and worktrees; `mia stack rm --worktrees --branches` takes those too.",
			Help:    "drop the stack's layer edges and name; nothing in git is touched",
		},
		"claim": {
			Key: "⏎", Label: "a worktree for this layer", Verb: "new", Args: []string{"{row}"}, Report: true,
			Help: "check the layer out in a worktree of its own, so layers run and test side by side",
		},
		"worktree": {
			Key: "W", Label: "a worktree for this layer", Verb: "new", Args: []string{"{row}"}, Report: true,
			Help: "check the layer out in a worktree of its own, so layers run and test side by side",
		},
		"layer": {
			Key: "n", Label: "new layer on top", Verb: "new", Args: []string{"--stack", "--in", "{row}", "{input}"}, Input: "layer name",
			Help: "a branch on top of the stack's top layer, checked out in that worktree",
		},
		"begin": {
			Key: "N", Label: "start a stack here", Verb: "new", Args: []string{"--stack", "--in", "{row}", "{input}"}, Input: "first layer", Report: true,
			Help: "this branch becomes the bottom; the first layer goes on top of it, checked out in this worktree",
		},
		"name": {
			Key: "=", Label: "name the stack", Verb: "stack", Args: []string{"name", "--in", "{row}", "{input}"}, Input: "stack name", Prefill: true, Report: true,
			Help: "give the stack a name for the list and for `mia switch <name>`",
		},
		"apart": {
			Key: "N", Label: "new layer, own worktree", Verb: "new", Args: []string{"--stack", "--worktree", "--in", "{row}", "{input}"}, Input: "layer name", Report: true,
			Help: "a branch on top of the stack's top layer, in a worktree of its own",
		},
		"restack": {
			Key: "R", Label: "restack", Verb: "stack", Args: []string{"restack", "--in", "{row}"}, Report: true,
			Confirm: "Restack {row}? Every layer is rebased onto the one below it.",
			Help:    "rebase every layer onto the one below it, bottom up",
		},
		"machines": {
			Key: "b", Label: "machines", Panel: "machines-panel", Global: true,
			Help: "the machines an environment can run on: add one, see who answers, shell there",
		},
		"config": {
			Popup: true, Lands: true, Key: "c", Label: "config", Verb: "config",
			Help: "this repository's .git/mia/config.toml in your editor",
		},
		"delete": {
			Key: "D", Label: "delete", Verb: "rm", Args: []string{"{row}"},
			Help:    "remove the worktree, its session and its name; refuses uncommitted work and a live session, each with its own way past",
			Confirm: "Remove {row}? Its session and its name go with it.",
			Retry: []Action{{
				When: "uncommitted", Label: "…and discard the uncommitted changes",
				Verb: "rm", Args: []string{"--force", "{row}"},
				Confirm: "Discard uncommitted changes in {row}? They are not recoverable.",
				Retry: []Action{{
					When: "running", Label: "…and stop what is running",
					Verb: "rm", Args: []string{"--force", "--stop-running", "{row}"},
					Confirm: "Stop everything running in {row} and delete it?",
				}},
			}, {
				When: "running", Label: "…and stop what is running",
				Verb: "rm", Args: []string{"--stop-running", "{row}"},
				Confirm: "Stop everything running in {row} and delete it?",
				Retry: []Action{{
					When: "uncommitted", Label: "…and discard the uncommitted changes",
					Verb: "rm", Args: []string{"--force", "--stop-running", "{row}"},
					Confirm: "Discard uncommitted changes in {row}? They are not recoverable.",
				}},
			}},
		},
	}
}

func since(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func blocksOf(windows []session.Window) []string {
	var blocks []string
	for _, w := range windows {
		blocks = append(blocks, w.Name+"  "+w.Command+"  quiet "+since(w.Quiet))
	}
	return blocks
}

type claim struct {
	plugin string
	info   plugin.RowInfo
	rank   int
	places bool
	tabs   []plugin.Tab
}

func addKeys(p *Panel, extras []plugin.Rows, worktrees map[string]app.Listing) []string {
	taken := map[string]string{}
	for _, move := range slices.Sorted(maps.Keys(Moves)) {
		for _, key := range Moves[move] {
			taken[key] = move
		}
	}
	for _, name := range slices.Sorted(maps.Keys(p.Actions)) {
		if key := p.Actions[name].Key; key != "" && taken[key] == "" {
			taken[key] = name
		}
	}
	var problems []string
	for _, extra := range extras {
		for _, key := range extra.Keys {
			name := extra.Plugin + "." + key.ID
			action := Action{
				Key: key.Key, Label: key.Label, Help: key.Help, Verb: key.Verb, Args: key.Args,
				Input: key.Input, Confirm: key.Confirm, Report: key.Report, Lands: key.Lands, Popup: key.Popup, Global: key.Global,
			}
			if key.Panel != "" {
				action.Panel = PluginPanel(extra.Plugin, key.Panel)
			}
			if owner, clash := taken[key.Key]; clash {
				problems = append(problems, "plugin  "+extra.Plugin+": "+key.Key+" is already "+owner+"; bind "+name+" in [keys]")
				action.Key = ""
			} else if key.Key != "" {
				taken[key.Key] = name
			}
			p.Actions[name] = action
			p.Order = append(p.Order, name)
			if key.Global {
				continue
			}
			for i := range p.Sections {
				offer(p.Sections[i].Rows, name, worktrees)
			}
		}
	}
	return problems
}

func offer(rows []Row, action string, worktrees map[string]app.Listing) {
	for i := range rows {
		if listing, ok := worktrees[rows[i].ID]; ok && listing.Adopted {
			rows[i].Actions = append(rows[i].Actions, action)
		}
		offer(rows[i].Children, action, worktrees)
	}
}

func PluginPanel(plugin, id string) string {
	return "plugin:" + plugin + ":" + id
}

func winner(claims []claim, below int) (claim, bool) {
	best, found := claim{}, false
	for _, c := range claims {
		if c.places && c.rank < below && (!found || c.rank < best.rank) {
			best, found = c, true
		}
	}
	return best, found
}
