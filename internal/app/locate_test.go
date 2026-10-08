package app_test

import (
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestAStackNameLocatesTheWorktreeThatHoldsIt(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	record, err := a.New("payments-schema")
	if err != nil {
		t.Fatal(err)
	}
	if err := git.CreateBranch(record.Path, "payments-api"); err != nil {
		t.Fatal(err)
	}
	if err := a.Stacks().Record("payments-api", "payments-schema"); err != nil {
		t.Fatal(err)
	}
	if path, err := a.Locate("payments-schema"); err != nil || path != record.Path {
		t.Errorf("the bottom layer of an unnamed stack located %q %v, want %s", path, err, record.Path)
	}
	bottom, err := a.StackName(record, "payments")
	if err != nil || bottom != "payments-schema" {
		t.Fatalf("named %q %v", bottom, err)
	}
	path, err := a.Locate("payments")
	if err != nil || path != record.Path {
		t.Errorf("located %q %v, want %s", path, err, record.Path)
	}
	if _, err := a.Locate("no-such-thing"); err == nil {
		t.Error("an unknown name resolved")
	}
	if path, _ := a.Locate(""); path != a.Root {
		t.Error("no query should be the main checkout")
	}
}

func TestAbandonedIsCleanQuietAndOld(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	old, err := a.New("forgotten")
	if err != nil {
		t.Fatal(err)
	}
	busy, err := a.New("in-progress")
	if err != nil {
		t.Fatal(err)
	}
	repo.GitIn(busy.Path, "commit", "--allow-empty", "-q", "-m", "still here")
	starred, _ := a.New("mine")
	if _, err := a.Star(starred.Name); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(30 * 24 * time.Hour)
	found, err := a.Abandoned(14*24*time.Hour, future)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, one := range found {
		names[one.Record.Name] = true
	}
	if !names[old.Name] || !names[busy.Name] || names[starred.Name] {
		t.Errorf("abandoned = %v (starred must be kept)", names)
	}
	if found, _ := a.Abandoned(14*24*time.Hour, time.Now()); len(found) != 0 {
		t.Errorf("fresh worktrees reported abandoned: %v", found)
	}
	if err := a.Collect(found[0]); err != nil {
		t.Fatal(err)
	}
	after, _ := a.List()
	for _, l := range after {
		if l.Name == found[0].Record.Name {
			t.Errorf("%s is still listed after collection", l.Name)
		}
	}
	_ = model.Record{}
}
