package panel_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/panel"
)

func TestARetryIsPickedByTheRefusalItAnswers(t *testing.T) {
	del := panel.Dashboard(sample(), "", nil).Actions["delete"]

	retry, ok := del.RetryFor("monduli has uncommitted changes — commit them")
	if !ok || !reflect.DeepEqual(retry.Command("monduli"), []string{"rm", "--force", "monduli"}) {
		t.Errorf("uncommitted → %v %v, want rm --force", retry.Command("monduli"), ok)
	}
	retry, ok = del.RetryFor("monduli has a session running — `mia shell monduli`")
	if !ok || !reflect.DeepEqual(retry.Command("monduli"), []string{"rm", "--stop-running", "monduli"}) {
		t.Errorf("running → %v %v, want rm --stop-running", retry.Command("monduli"), ok)
	}
	second, ok := retry.RetryFor("monduli has uncommitted changes")
	if !ok || !reflect.DeepEqual(second.Command("monduli"), []string{"rm", "--force", "--stop-running", "monduli"}) {
		t.Errorf("running then uncommitted → %v, want both flags", second.Command("monduli"))
	}
	if _, ok := del.RetryFor("some other failure entirely"); ok {
		t.Error("a retry was offered for a refusal it does not answer")
	}
}

func TestInputAndRowlessActions(t *testing.T) {
	p := panel.Dashboard(sample(), "", nil)
	create := p.Actions["branch"]
	if create.Input == "" {
		t.Fatal("new worktree does not ask for a branch")
	}
	if got := create.CommandWith("", "due-dates"); !reflect.DeepEqual(got, []string{"new", "--shell", "due-dates"}) {
		t.Errorf("new worktree with input → %v", got)
	}
	if got := p.Actions["new"].CommandWith("", "due-dates"); !reflect.DeepEqual(got, []string{"new", "due-dates"}) {
		t.Errorf("new with input → %v", got)
	}
	if create.NeedsRow() {
		t.Error("new needs a row, but a new worktree comes from nowhere")
	}
	if !p.Actions["delete"].NeedsRow() {
		t.Error("delete does not need a row")
	}
	if !p.Actions["env"].NeedsRow() || p.Actions["env"].Panel != "env-panel" {
		t.Errorf("env should open env-panel for a row: %+v", p.Actions["env"])
	}
}

func TestAPanelActionNamesAWorktreeOnlyWhenItNeedsOne(t *testing.T) {
	p := panel.Dashboard(sample(), "", nil)
	if got := p.Actions["machines"].CLI(); got != "mia dash machines" {
		t.Errorf("machines → %q, want mia dash machines", got)
	}
	if got := p.Actions["env"].CLI(); got != "mia dash env <worktree>" {
		t.Errorf("env → %q, want mia dash env <worktree>", got)
	}
}

func TestStarredWorktreesGetTheirOwnSection(t *testing.T) {
	listings := []app.Listing{
		{Name: "app", Path: "/w/app", Branch: "main", Main: true, Adopted: true},
		{Name: "monduli", Path: "/w/app.monduli", Branch: "due-dates", Adopted: true, Starred: true, Dirty: true},
		{Name: "iringa", Path: "/w/app.iringa", Branch: "billing", Adopted: true, Starred: true},
	}
	p := panel.Dashboard(listings, "", nil)
	var order []string
	byID := map[string]string{}
	for _, section := range p.Sections {
		order = append(order, section.ID)
		for _, row := range section.Rows {
			byID[row.ID] = section.ID
		}
	}
	if byID["iringa"] != "starred" {
		t.Errorf("a starred worktree is in %q, want starred", byID["iringa"])
	}
	if byID["monduli"] != "starred" {
		t.Errorf("a starred dirty worktree is in %q, want starred", byID["monduli"])
	}
	if !reflect.DeepEqual(order, []string{"starred"}) {
		t.Errorf("section order %v", order)
	}
	if byID["app"] != "starred" || p.Sections[0].Rows[0].ID != "app" {
		t.Errorf("the main worktree is not first under starred: %v", byID["app"])
	}
	for _, row := range p.Rows() {
		switch row.ID {
		case "monduli":
			if !slices.Contains(row.Actions, "star") || p.Actions["star"].Key != "*" {
				t.Error("rows do not offer * to star")
			}
		case "app":
			if slices.Contains(row.Actions, "star") {
				t.Error("the main worktree is always starred; it must not offer * ")
			}
		}
	}
}

func TestRowsCarryDriftAgeAndFactsAndSortByRecency(t *testing.T) {
	now := time.Now()
	listings := []app.Listing{
		{Name: "app", Path: "/w/app", Branch: "main", Main: true, Adopted: true, LastWork: now.Add(-48 * time.Hour)},
		{Name: "old", Path: "/w/app.old", Branch: "you/old-thing", Adopted: true, Ahead: 2, Behind: 17, LastWork: now.Add(-72 * time.Hour), LastSubject: "tidy"},
		{Name: "fresh", Path: "/w/app.fresh", Branch: "you/fresh-thing", Adopted: true, LastWork: now.Add(-time.Hour), LastSubject: "wip", Ahead: 1},
	}
	p := panel.Dashboard(listings, "you/", nil)
	rows := p.Rows()
	if rows[0].ID != "app" || rows[1].ID != "fresh" || rows[2].ID != "old" {
		t.Errorf("order %s %s %s — want main, then most recent first", rows[0].ID, rows[1].ID, rows[2].ID)
	}
	if rows[2].Cells[1] != "old-thing" || !strings.Contains(rows[2].Cells[2], "+2 −17") {
		t.Errorf("old row cells %q", rows[2].Cells)
	}
	if !strings.Contains(strings.Join(rows[1].Facts, "\n"), "last    wip") {
		t.Errorf("facts %v", rows[1].Facts)
	}
}
