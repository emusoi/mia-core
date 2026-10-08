package panel_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/cli"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/stack"
)

func everyPanel() []panel.Panel {
	return []panel.Panel{
		panel.Dashboard(sample(), "", nil),
		panel.Dashboard(nil, "", nil),
		panel.Env(model.Record{Name: "monduli"}, env.Environment{Name: "monduli", State: "running", Machine: "local"},
			env.Ports{Listening: []int{5173}}, []env.Status{{ID: "web", Running: true}}),
		panel.Stack(model.Record{Name: "monduli"}, []stack.Layer{{Branch: "schema", Current: true, Ahead: 2}}),
		panel.Stack(model.Record{Name: "monduli"}, nil),
		panel.Blocks(model.Record{Name: "monduli"}, []session.Window{{Name: "helper", Command: "helper"}, {Name: "zsh", Command: "zsh"}}, []string{"test"}),
		panel.Blocks(model.Record{Name: "monduli"}, nil, nil),
		panel.Machines([]panel.MachineRow{{Machine: runtime.Machine{Name: "fedora", SSH: "fedora", Engine: "podman"}, Reachable: true, Hosting: []string{"iringa"}}}),
	}
}

func TestBlocksKeepSameNamedWindowsDistinct(t *testing.T) {
	p := panel.Blocks(model.Record{Name: "monduli"}, []session.Window{
		{Index: 1, Name: "zsh", Screen: "first"},
		{Index: 3, Name: "zsh", Screen: "second"},
		{Index: 4, Name: "helper"},
	}, nil)
	rows := p.Rows()
	if len(rows) != 3 || rows[0].ID != "1" || rows[1].ID != "3" || rows[2].ID != "4" {
		t.Fatalf("window row IDs = %+v", rows)
	}
	if rows[0].Cells[0] != "zsh" || rows[1].Cells[0] != "zsh" {
		t.Errorf("window labels = %q, %q", rows[0].Cells[0], rows[1].Cells[0])
	}
	if got := p.Actions["close"].Command(rows[1].TargetID()); !slices.Equal(got, []string{"window", "close", "monduli", "3"}) {
		t.Errorf("second zsh close command = %v", got)
	}
	if got := p.Actions["open"].Command(rows[1].TargetID()); !slices.Equal(got, []string{"window", "open", "monduli", "3"}) {
		t.Errorf("second zsh open command = %v", got)
	}
}

var servedPanels = map[string]bool{"dashboard": true, "env-panel": true, "stack-panel": true, "blocks-panel": true, "machines-panel": true}

func sample() []app.Listing {
	return []app.Listing{
		{Name: "app", Path: "/w/app", Branch: "main", Main: true},
		{Name: "monduli", Path: "/w/app.monduli", Branch: "due-dates", Adopted: true},
		{Name: "longido", Path: "/w/app.longido", Branch: "", Adopted: true, Dirty: true},
		{Name: "app.byhand", Path: "/w/app.byhand", Branch: "theirs"},
	}
}

func TestEveryPanelActionNamesARealVerb(t *testing.T) {
	for _, p := range everyPanel() {
		for id, action := range p.Actions {
			if action.Panel != "" {
				if !servedPanels[action.Panel] {
					t.Errorf("%s: action %q opens %q, which no query serves", p.ID, id, action.Panel)
				}
				continue
			}
			if action.Verb == "" {
				t.Errorf("%s: action %q names no verb", p.ID, id)
				continue
			}
			if !slices.ContainsFunc(cli.Verbs(), func(v cli.Verb) bool { return v.Name == action.Verb }) {
				t.Errorf("%s: action %q runs %q, which is not an mia command", p.ID, id, action.Verb)
			}
			for _, retry := range action.Retry {
				if !slices.ContainsFunc(cli.Verbs(), func(v cli.Verb) bool { return v.Name == retry.Verb }) {
					t.Errorf("%s: retry of %q runs %q, which is not an mia command", p.ID, id, retry.Verb)
				}
			}
		}
	}
}

func TestRowsOnlyOfferWhatWouldWork(t *testing.T) {
	p := panel.Dashboard(sample(), "", nil)
	for _, row := range p.Rows() {
		for _, id := range row.Actions {
			if _, ok := p.Actions[id]; !ok {
				t.Errorf("row %q offers action %q which the panel does not define", row.ID, id)
			}
		}
	}

	byID := map[string][]string{}
	for _, row := range p.Rows() {
		byID[row.ID] = row.Actions
	}
	for _, offered := range byID["app"] {
		if offered == "delete" {
			t.Error("the main checkout offers delete")
		}
		if offered == "open" || offered == "shell" {
			t.Error("an unadopted main checkout offers a shell it cannot open")
		}
	}
	if !slices.Contains(byID["app"], "adopt") {
		t.Error("an unadopted main checkout does not offer adoption")
	}
	adoptedMain := panel.Dashboard([]app.Listing{{Name: "app", Path: "/w/app", Branch: "main", Main: true, Adopted: true}}, "", nil)
	if !slices.Contains(adoptedMain.Rows()[0].Actions, "open") {
		t.Error("an adopted main checkout cannot be opened")
	}
	var adoptable, deletable bool
	for _, offered := range byID["app.byhand"] {
		adoptable = adoptable || offered == "adopt"
		deletable = deletable || offered == "delete"
	}
	if !adoptable {
		t.Error("an unadopted worktree does not offer adoption")
	}
	if deletable {
		t.Error("mia offers to delete a worktree it does not own")
	}
}

func TestDestructiveActionsAskAndOfferAWayPast(t *testing.T) {
	p := panel.Dashboard(sample(), "", nil)
	del, ok := p.Actions["delete"]
	if !ok {
		t.Fatal("the dashboard has no delete action")
	}
	if del.Confirm == "" {
		t.Error("delete does not ask before destroying something")
	}
	if len(del.Retry) == 0 {
		t.Fatal("delete offers no way past its own refusal")
	}
	if del.Retry[0].Confirm == "" {
		t.Error("forcing past a refusal does not ask, and it is the more destructive of the two")
	}

	if got := del.ConfirmFor("monduli"); !strings.Contains(got, "monduli") {
		t.Errorf("the question does not name what it would destroy: %q", got)
	}
	argv := del.Command("monduli")
	if len(argv) != 2 || argv[0] != "rm" || argv[1] != "monduli" {
		t.Errorf("delete runs %v, want [rm monduli]", argv)
	}
}

func TestGlyphsAreStateNotDecoration(t *testing.T) {
	p := panel.Dashboard(sample(), "", nil)
	byID := map[string]panel.Row{}
	for _, row := range p.Rows() {
		byID[row.ID] = row
	}
	if byID["longido"].Glyph != panel.GlyphDirty {
		t.Errorf("an uncommitted worktree is marked %q", byID["longido"].Glyph)
	}
	if byID["monduli"].Glyph != "" {
		t.Errorf("a clean worktree carries a glyph %q for no state", byID["monduli"].Glyph)
	}
}

func TestADetachedWorktreeIsNamedNotBlank(t *testing.T) {
	p := panel.Dashboard(sample(), "", nil)
	for _, row := range p.Rows() {
		if row.ID != "longido" {
			continue
		}
		if row.Cells[1] != "(detached)" {
			t.Errorf("a detached worktree shows branch %q", row.Cells[1])
		}
		return
	}
	t.Fatal("the detached worktree is missing from the panel")
}

func TestAnEmptyPanelSaysWhatToDo(t *testing.T) {
	p := panel.Dashboard(nil, "", nil)
	if len(p.Sections) != 0 {
		t.Errorf("an empty dashboard produced %d sections", len(p.Sections))
	}
	if p.Empty == "" {
		t.Fatal("an empty dashboard says nothing")
	}
	if !strings.Contains(p.Empty, "mia new") {
		t.Errorf("the empty state does not say what to do: %q", p.Empty)
	}
}

func TestDashboardNamesTheRepoAndCollapsesQuietWork(t *testing.T) {
	p := panel.Dashboard(sample(), "", nil)
	if p.Title != "Worktrees / app" {
		t.Errorf("dashboard title = %q, want repository context", p.Title)
	}
	for _, section := range p.Sections {
		if section.ID != "quiet" {
			continue
		}
		if !section.Collapsed || section.ToggleKey != "e" {
			t.Fatalf("quiet section is not folded behind e: %+v", section)
		}
		return
	}
	t.Fatal("dashboard has no quiet section")
}

func TestThePanelSurvivesJSON(t *testing.T) {
	original := panel.Dashboard(sample(), "", nil)
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var back panel.Panel
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Version != panel.Version {
		t.Errorf("version %d survived as %d", panel.Version, back.Version)
	}
	if len(back.Rows()) != len(original.Rows()) {
		t.Errorf("%d rows became %d", len(original.Rows()), len(back.Rows()))
	}
	if back.Actions["delete"].Confirm == "" {
		t.Error("the confirmation did not survive the wire")
	}
}

func TestSectionOrderIsFixed(t *testing.T) {
	p := panel.Dashboard(sample(), "", nil)
	var order []string
	for _, section := range p.Sections {
		order = append(order, section.ID)
	}
	want := []string{"starred", "active", "quiet", "unadopted"}
	if len(order) != len(want) {
		t.Fatalf("sections = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("sections = %v, want %v", order, want)
		}
	}
}

func TestLayersInWorktreesBecomeOneStackRow(t *testing.T) {
	layers := []stack.Layer{
		{Branch: "base", Parent: "main", Landed: true, Worktree: ""},
		{Branch: "layer-a", Parent: "base", Ahead: 1, Worktree: "longido"},
		{Branch: "layer-b", Parent: "layer-a", Ahead: 1, Worktree: "monduli", Current: true},
	}
	listings := []app.Listing{
		{Name: "monduli", Branch: "layer-b", Adopted: true, Layers: layers, Stack: "payments"},
		{Name: "longido", Branch: "layer-a", Adopted: true, Layers: layers, Stack: "payments", Dirty: true},
		{Name: "solo", Branch: "other", Adopted: true},
	}
	p := panel.Dashboard(listings, "", nil)
	var stackRow panel.Row
	for _, row := range p.Rows() {
		if row.ID == "stack:base" {
			stackRow = row
		}
	}
	if stackRow.ID == "" {
		t.Fatalf("no stack row: %v", p.Rows())
	}
	if stackRow.Cells[0] != "payments" || stackRow.Cells[1] != "3 layers · 1 landed" || stackRow.Target != "monduli" {
		t.Errorf("stack row = %v target %q", stackRow.Cells, stackRow.Target)
	}
	if len(stackRow.Children) != 3 || stackRow.Children[0].ID != "monduli" || stackRow.Children[1].ID != "longido" || stackRow.Children[2].ID != "base" || !stackRow.Children[2].Dim {
		t.Errorf("children top first, the bare layer dim: %+v", stackRow.Children)
	}
	count := 0
	for _, section := range p.Sections {
		for _, row := range section.Rows {
			if row.ID == "monduli" || row.ID == "longido" {
				count++
			}
		}
	}
	if count != 0 {
		t.Errorf("layer worktrees must not also be top-level rows")
	}
}

func TestFactsSayWhatChanged(t *testing.T) {
	p := panel.Dashboard([]app.Listing{{Name: "longido", Branch: "fix", Adopted: true, Ahead: 2, Files: 3, Added: 7}}, "", nil)
	facts := strings.Join(p.Sections[0].Rows[0].Facts, "\n")
	for _, want := range []string{"changes  3 file(s) · +7 −0"} {
		if !strings.Contains(facts, want) {
			t.Errorf("facts lack %q:\n%s", want, facts)
		}
	}
}

func TestCommitsThatHaveNotLandedAreNotQuiet(t *testing.T) {
	p := panel.Dashboard([]app.Listing{{Name: "duluti", Branch: "fix", Adopted: true, Ahead: 1}}, "", nil)
	if p.Sections[0].ID != "active" {
		t.Errorf("a branch one commit ahead is in %q, want in progress", p.Sections[0].ID)
	}
}

func TestTheMainCheckoutIsNotCalledStarredUnlessItIs(t *testing.T) {
	label := func(listings []app.Listing) string {
		for _, section := range panel.Dashboard(listings, "", nil).Sections {
			if section.ID == "starred" {
				return section.Label
			}
		}
		return ""
	}
	main := app.Listing{Name: "cfgrepo", Path: "/r", Branch: "main", Main: true, Adopted: true}
	star := app.Listing{Name: "kisongo", Path: "/r.k", Branch: "k", Adopted: true, Starred: true}
	if got := label([]app.Listing{main}); got != "main" {
		t.Errorf("an unstarred main checkout alone sits under %q", got)
	}
	if got := label([]app.Listing{main, star}); got != "main · starred" {
		t.Errorf("main beside a starred worktree sits under %q", got)
	}
	main.Starred = true
	if got := label([]app.Listing{main}); got != "starred" {
		t.Errorf("a starred main checkout sits under %q", got)
	}
}

func TestALentMainCheckoutSaysWhoseFilesItHasAndGivesThemBack(t *testing.T) {
	p := panel.Dashboard([]app.Listing{
		{Name: "app", Path: "/w/app", Branch: "main", Main: true, Dev: "has longido's files"},
		{Name: "longido", Path: "/w/app.longido", Branch: "fix", Adopted: true, Dev: "on main's dev server"},
		{Name: "monduli", Path: "/w/app.monduli", Branch: "other", Adopted: true},
	}, "", nil)
	rows := map[string]panel.Row{}
	for _, row := range p.Rows() {
		rows[row.ID] = row
	}
	if !slices.Contains(rows["app"].Facts, "dev     has longido's files") || !slices.Contains(rows["app"].Actions, "dev") {
		t.Errorf("main = %+v, want its dev fact and v to give it back", rows["app"])
	}
	if !slices.Contains(rows["longido"].Facts, "dev     on main's dev server") {
		t.Errorf("longido facts = %v", rows["longido"].Facts)
	}
	if !slices.Contains(rows["monduli"].Actions, "dev") {
		t.Error("an adopted worktree cannot be lent to main's dev server")
	}
	if p.Actions["dev"].Key != "v" {
		t.Errorf("dev key = %q", p.Actions["dev"].Key)
	}
	if plain := panel.Dashboard([]app.Listing{{Name: "app", Path: "/w/app", Branch: "main", Main: true, Adopted: true}}, "", nil); slices.Contains(plain.Rows()[0].Actions, "dev") {
		t.Error("main offers v when nothing is lent")
	}
}
