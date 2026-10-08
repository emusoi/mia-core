package panel_test

import (
	"slices"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/stack"
)

func sectionOf(p panel.Panel, id string) string {
	for _, section := range p.Sections {
		for _, row := range section.Rows {
			if row.ID == id {
				return section.ID
			}
		}
	}
	return ""
}

func rowOf(p panel.Panel, id string) panel.Row {
	for _, row := range p.Rows() {
		if row.ID == id {
			return row
		}
		for _, child := range row.Children {
			if child.ID == id {
				return child
			}
		}
	}
	return panel.Row{}
}

func ids(p panel.Panel) []string {
	var out []string
	for _, section := range p.Sections {
		out = append(out, section.ID)
	}
	return out
}

var fleet = []app.Listing{
	{Name: "app", Branch: "main", Main: true, Adopted: true},
	{Name: "quiet-one", Branch: "a", Adopted: true},
	{Name: "busy", Branch: "b", Adopted: true, Dirty: true},
	{Name: "fave", Branch: "c", Adopted: true, Starred: true},
}

func TestAPluginSectionSitsByRankAndTakesTheStatus(t *testing.T) {
	waiting := plugin.Section{ID: "waiting", Label: "waiting on you", Rank: 20}
	extras := []plugin.Rows{
		{Plugin: "helper", Sections: []plugin.Section{waiting}, Rows: map[string]plugin.RowInfo{
			"quiet-one": {Section: "waiting", Status: "asks a question", Facts: []string{"helper  waiting 3m"}},
			"fave":      {Section: "waiting", Status: "asks too"},
		}},
		{Plugin: "reviewer", Sections: []plugin.Section{{ID: "waiting", Label: "ignored, helper said it first", Rank: 99}}, Rows: map[string]plugin.RowInfo{
			"busy": {Section: "waiting", Status: "reviewer asks"},
		}},
	}
	p := panel.Dashboard(fleet, "", extras)

	if got := ids(p); !slices.Equal(got, []string{"starred", "waiting"}) {
		t.Fatalf("sections %v", got)
	}
	if p.Sections[1].Label != "waiting on you" || len(p.Sections[1].Rows) != 2 {
		t.Errorf("one shared section, first label wins: %+v", p.Sections[1])
	}
	row := rowOf(p, "quiet-one")
	if row.Cells[2] != "asks a question" || !slices.Contains(row.Facts, "helper  waiting 3m") {
		t.Errorf("the placing plugin's status and facts: %v %v", row.Cells, row.Facts)
	}
	if sectionOf(p, "fave") != "starred" {
		t.Error("a starred worktree is the person's choice; a plugin does not move it")
	}
	if !slices.Contains(rowOf(p, "fave").Facts, "helper  asks too") {
		t.Errorf("a status that did not place the row is still told: %v", rowOf(p, "fave").Facts)
	}
}

func TestAClaimRankedBelowCoreDoesNotMoveTheRow(t *testing.T) {
	extras := []plugin.Rows{{Plugin: "ci", Sections: []plugin.Section{{ID: "green", Label: "green", Rank: 60}}, Rows: map[string]plugin.RowInfo{
		"busy":      {Section: "green", Status: "ci passed"},
		"quiet-one": {Section: "nowhere", Status: "undeclared"},
	}}}
	p := panel.Dashboard(fleet, "", extras)
	if sectionOf(p, "busy") != "active" || rowOf(p, "busy").Cells[2] != "uncommitted" {
		t.Errorf("in progress outranks a rank-60 section: %s %v", sectionOf(p, "busy"), rowOf(p, "busy").Cells)
	}
	if sectionOf(p, "quiet-one") != "quiet" {
		t.Error("a section nobody declared places nothing")
	}
}

func TestAPluginProblemIsAFactNeverAFailure(t *testing.T) {
	p := panel.Dashboard(fleet, "", []plugin.Rows{{Plugin: "slow", Problem: "rows took longer than 2s"}})
	if !slices.Contains(rowOf(p, "app").Facts, "plugin  slow: rows took longer than 2s") {
		t.Errorf("facts %v", rowOf(p, "app").Facts)
	}
}

func TestAStackFollowsItsLoudestLayer(t *testing.T) {
	layers := []stack.Layer{
		{Branch: "layer-a", Parent: "main", Ahead: 1, Worktree: "longido"},
		{Branch: "layer-b", Parent: "layer-a", Ahead: 1, Worktree: "monduli"},
	}
	listings := []app.Listing{
		{Name: "monduli", Branch: "layer-b", Adopted: true, Layers: layers},
		{Name: "longido", Branch: "layer-a", Adopted: true, Layers: layers},
	}
	extras := []plugin.Rows{{Plugin: "helper", Sections: []plugin.Section{{ID: "waiting", Label: "waiting", Rank: 20}}, Rows: map[string]plugin.RowInfo{
		"longido": {Section: "waiting", Status: "asks"},
	}}}
	p := panel.Dashboard(listings, "", extras)
	if sectionOf(p, "stack:layer-a") != "waiting" {
		t.Errorf("the stack row sits where its layer's claim puts it: %v", ids(p))
	}
	if rowOf(p, "longido").Cells[2] != "asks" {
		t.Errorf("the layer row carries the status: %v", rowOf(p, "longido").Cells)
	}
}

func TestPluginTabsAndKeysReachTheRows(t *testing.T) {
	extras := []plugin.Rows{{
		Plugin: "gh",
		Tabs:   []plugin.Tab{{ID: "pr", Label: "pull request"}, {ID: "ci", Label: "ci"}},
		Keys: []plugin.Key{
			{ID: "open", Key: "U", Label: "open the PR", Verb: "gh-open", Args: []string{"{row}"}},
			{ID: "prs", Key: "B", Label: "every PR", Panel: "prs", Global: true},
			{ID: "dup", Key: "D", Label: "steals delete", Verb: "gh-x"},
			{ID: "nav", Key: "J", Label: "steals a move", Verb: "gh-y"},
		},
		Rows: map[string]plugin.RowInfo{"busy": {Tabs: map[string][]string{"pr": {"#12 open", "2 approvals"}, "ci": nil}}},
	}}
	listings := append(slices.Clone(fleet), app.Listing{Name: "stranger", Branch: "x"})
	p := panel.Dashboard(listings, "", extras)

	busy := rowOf(p, "busy")
	if len(busy.Tabs) != 1 || busy.Tabs[0].Name != "pull request" || len(busy.Tabs[0].Preview) != 2 {
		t.Errorf("a tab with lines shows, an empty one does not: %+v", busy.Tabs)
	}
	if !slices.Contains(busy.Actions, "gh.open") || slices.Contains(rowOf(p, "stranger").Actions, "gh.open") {
		t.Errorf("a key is offered on mia's worktrees only: %v / %v", busy.Actions, rowOf(p, "stranger").Actions)
	}
	if slices.Contains(busy.Actions, "gh.prs") || p.Actions["gh.prs"].Panel != "plugin:gh:prs" {
		t.Errorf("a global panel key: %+v", p.Actions["gh.prs"])
	}
	if p.Actions["gh.dup"].Key != "" || p.Actions["delete"].Key != "D" {
		t.Errorf("core keeps its key: %+v", p.Actions["gh.dup"])
	}
	if !slices.Contains(rowOf(p, "app").Facts, "plugin  gh: D is already delete; bind gh.dup in [keys]") {
		t.Errorf("the clash is told: %v", rowOf(p, "app").Facts)
	}
	if p.Actions["gh.nav"].Key != "" || !slices.Contains(rowOf(p, "app").Facts, "plugin  gh: J is already scrolldown; bind gh.nav in [keys]") {
		t.Errorf("a move keeps its key: %+v", p.Actions["gh.nav"])
	}
	p.Bind(map[string][]string{"gh.dup": {"X"}})
	if p.Actions["gh.dup"].Key != "X" {
		t.Error("a clashing plugin key can be bound by name")
	}
}

func TestAStackWithWorkInItIsInProgress(t *testing.T) {
	layers := []stack.Layer{
		{Branch: "schema", Parent: "main", Ahead: 1, Worktree: "longido"},
		{Branch: "api", Parent: "schema", Ahead: 1, Worktree: "monduli"},
	}
	p := panel.Dashboard([]app.Listing{
		{Name: "monduli", Branch: "api", Adopted: true, Layers: layers, Ahead: 2},
		{Name: "longido", Branch: "schema", Adopted: true, Layers: layers, Ahead: 1},
	}, "", nil)
	if got := sectionOf(p, "stack:schema"); got != "active" {
		t.Errorf("a stack two commits ahead is in %q, want in progress", got)
	}
}
