package panel

import (
	"fmt"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/stack"
)

func Stack(record model.Record, rows []stack.Layer) Panel {
	name := record.Name
	p := Panel{
		Columns: []Column{{Name: "layer", Kind: "ref"}, {Name: "state"}, {Name: "where"}},
		Version: Version,
		ID:      "stack",
		Title:   "Stack / " + name,
		Actions: stackActions(name),
		Hints:   []string{"⏎ go", "n layer", "N layer apart", "W worktree", "M merge down", "= name", "R restack", "D forget", "q back"},
		Empty:   "not a stack — `n` adds a layer on top of this branch",
		Order:   []string{"go", "worktree", "merge", "layer", "apart", "name", "restack", "forget"},
	}
	var layers []Row
	for i, r := range rows {
		glyph := ""
		if r.Current {
			glyph = GlyphHere
		}
		state := fmt.Sprintf("%d commit(s)", r.Ahead)
		switch {
		case r.Landed:
			state = "landed"
		case r.Ahead == 0:
			state = "not started"
		}
		if r.NeedsRestack {
			state += " · needs restack"
		}
		where := ""
		actions := []string{"go", "worktree"}
		facts := []string{"layer  " + r.Branch, "on  " + r.Parent, "state  " + state}
		if r.Worktree != "" {
			where = "in " + r.Worktree
			actions = []string{"go"}
			facts = append(facts, "worktree  "+r.Worktree)
		}
		var lower []string
		for j := i - 1; j >= 0; j-- {
			lower = append(lower, rows[j].Branch)
		}
		var choices map[string][]string
		if len(lower) > 0 {
			actions = append(actions, "merge")
			choices = map[string][]string{"merge": lower}
		}
		layers = append(layers, Row{
			ID:      r.Branch,
			Glyph:   glyph,
			Cells:   []string{r.Branch, state, where},
			Actions: actions,
			Facts:   facts,
			Choices: choices,
		})
	}
	if len(layers) > 0 {
		p.Sections = []Section{{ID: "layers", Label: "layers, bottom first", Rows: layers}}
	}
	return p
}

func stackActions(name string) map[string]Action {
	return map[string]Action{
		"go":       {Key: "⏎", Label: "go to this layer", Verb: "stack", Args: []string{"go", "--in", name, "{row}"}, Help: "check the layer out in place; session and environment stay"},
		"layer":    {Key: "n", Label: "new layer on top", Verb: "new", Args: []string{"--stack", "--in", name, "{input}"}, Input: "layer name", Help: "a branch on top of the current layer, sharing this worktree"},
		"apart":    {Key: "N", Label: "new layer, own worktree", Verb: "new", Args: []string{"--stack", "--worktree", "--in", name, "{input}"}, Input: "layer name", Report: true, Help: "a branch on top of the current layer, in a worktree of its own"},
		"worktree": {Key: "W", Label: "a worktree for this layer", Verb: "new", Args: []string{"{row}"}, Report: true, Help: "check the layer out in a worktree of its own, so layers run and test side by side"},
		"merge":    {Key: "M", Label: "merge down", Verb: "stack", Args: []string{"merge", "{row}", "onto", "{input}"}, Input: "onto", Prefill: true, Report: true, Confirm: "Merge down onto {input}? Every layer above it, up to this one, folds into it and disappears.", Help: "fast-forward a lower layer to this one; the layers in between fold into it — never into the base branch"},
		"forget":   {Key: "D", Label: "forget the stack", Verb: "stack", Args: []string{"rm", "--in", name}, Report: true, Global: true, Confirm: "Forget this stack? Its layers stay as ordinary branches and worktrees.", Help: "drop the stack's layer edges and name; nothing in git is touched"},
		"name":     {Key: "=", Label: "name the stack", Verb: "stack", Args: []string{"name", "--in", name, "{input}"}, Input: "stack name", Report: true, Help: "give the stack a name for the list and for `mia switch <name>`"},
		"restack":  {Key: "R", Label: "restack", Verb: "stack", Args: []string{"restack", "--in", name}, Report: true, Help: "rebase every layer onto the one below it, bottom up"},
	}
}
