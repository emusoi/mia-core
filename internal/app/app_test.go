package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/names"
)

func open(t *testing.T, repo *gittest.Repo) *app.App {
	t.Helper()
	gittest.Isolate(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Names = &names.Store{Path: filepath.Join(t.TempDir(), "names.json")}
	return a
}

func TestNewNamesAndRecordsAWorktree(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)

	record, err := a.New("due-dates")
	if err != nil {
		t.Fatal(err)
	}
	if record.Name == "" {
		t.Fatal("the worktree was not named")
	}
	if filepath.Base(record.Path) != filepath.Base(repo.Root)+"."+record.Name {
		t.Errorf("directory %s does not carry the name %q", record.Path, record.Name)
	}
	if got := git.CurrentBranch(record.Path); got != "due-dates" {
		t.Errorf("worktree is on %q, want due-dates", got)
	}
	records, err := a.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Path != record.Path {
		t.Errorf("records = %+v", records)
	}
}

func TestNewIsIdempotent(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)

	first, err := a.New("due-dates")
	if err != nil {
		t.Fatal(err)
	}
	again, err := a.New("due-dates")
	if err != nil {
		t.Fatal(err)
	}
	if again.Path != first.Path || again.Name != first.Name {
		t.Errorf("second new gave %+v, want %+v", again, first)
	}
	records, _ := a.Store.Load()
	if len(records) != 1 {
		t.Errorf("%d records after creating the same worktree twice", len(records))
	}
}

func TestListShowsUnadoptedWorktreesToo(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	if _, err := a.New("mine"); err != nil {
		t.Fatal(err)
	}
	byHand := repo.Worktree("stranger", "theirs")

	listings, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	var sawAdopted, sawStranger bool
	for _, l := range listings {
		if l.Path == byHand {
			sawStranger = true
			if l.Adopted {
				t.Error("a worktree created by hand is reported as adopted")
			}
		}
		if l.Adopted && l.Branch == "mine" {
			sawAdopted = true
		}
	}
	if !sawAdopted || !sawStranger {
		t.Errorf("list missed something: adopted=%v stranger=%v", sawAdopted, sawStranger)
	}
}

func TestListDoesNotPresentTheBaseBranchAsAStack(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	if _, err := a.Adopt(repo.Root); err != nil {
		t.Fatal(err)
	}
	for _, edge := range [][2]string{{"schema", "main"}, {"api", "schema"}} {
		repo.Git("branch", edge[0])
		if err := a.Stacks().Record(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.New("api"); err != nil {
		t.Fatal(err)
	}

	listings, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, listing := range listings {
		if listing.Main && len(listing.Layers) != 0 {
			t.Errorf("base branch appears as a %d-layer stack: %+v", len(listing.Layers), listing.Layers)
		}
		if listing.Branch == "api" && len(listing.Layers) != 2 {
			t.Errorf("top layer sees %d layers, want 2", len(listing.Layers))
		}
	}
}

func TestListKeepsAnImplicitStackBottomWhenCheckedOut(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	repo.Git("branch", "bottom")
	repo.Git("branch", "top")
	if err := a.Stacks().Record("top", "bottom"); err != nil {
		t.Fatal(err)
	}
	if a.Stacks().Member("bottom") {
		t.Fatal("fixture bottom is recorded as a member")
	}
	record, err := a.New("top")
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"top", "bottom"} {
		if branch == "bottom" {
			repo.GitIn(record.Path, "checkout", branch)
		}
		listings, err := a.List()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, listing := range listings {
			if listing.Path != record.Path {
				continue
			}
			found = true
			if listing.Stack != "bottom" || len(listing.Layers) != 2 {
				t.Errorf("on %s, stack = %q, layers = %+v", branch, listing.Stack, listing.Layers)
				continue
			}
			if listing.Layers[0].Branch != "bottom" || listing.Layers[1].Branch != "top" ||
				listing.Layers[0].Current != (branch == "bottom") ||
				listing.Layers[1].Current != (branch == "top") {
				t.Errorf("on %s, wrong stack order or current layer: %+v", branch, listing.Layers)
			}
		}
		if !found {
			t.Fatalf("worktree %s missing from listings", record.Path)
		}
	}
}

func TestADetachedWorktreeIsOrdinary(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)

	detached := repo.Detached("byhand")
	record, err := a.Adopt(detached)
	if err != nil {
		t.Fatal(err)
	}

	listings, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, l := range listings {
		if l.Path == detached {
			found = true
			if l.Branch != "" {
				t.Errorf("a detached worktree reported branch %q", l.Branch)
			}
			if !l.Adopted {
				t.Error("the adopted worktree is not marked adopted")
			}
		}
	}
	if !found {
		t.Fatal("the detached worktree is missing from the list")
	}

	resolver, err := a.Resolver()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := resolver.Worktree(record.Name); err != nil || got != detached {
		t.Errorf("resolving %q gave %s (%v)", record.Name, got, err)
	}

	removal, err := a.PlanRemoval(record.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Remove(removal, false, false); err != nil {
		t.Fatalf("removing a detached worktree failed: %v", err)
	}
}

func TestRemoveTakesEverythingWithIt(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)

	record, err := a.New("due-dates")
	if err != nil {
		t.Fatal(err)
	}
	removal, err := a.PlanRemoval(record.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Remove(removal, false, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(record.Path); err == nil {
		t.Error("the directory survived removal")
	}
	records, _ := a.Store.Load()
	if len(records) != 0 {
		t.Errorf("%d records survived removal", len(records))
	}
	owners, _ := a.Names.All()
	if len(owners) != 0 {
		t.Errorf("the name was not released: %v", owners)
	}
}

func TestRemoveRefusesUncommittedWorkAndSaysHowToProceed(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)

	record, err := a.New("due-dates")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(record.Path, "scratch.txt"), []byte("unsaved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	removal, err := a.PlanRemoval(record.Name)
	if err != nil {
		t.Fatal(err)
	}
	err = a.Remove(removal, false, false)
	if err == nil {
		t.Fatal("uncommitted work was removed without asking")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal does not offer the way past it: %v", err)
	}
	if _, statErr := os.Stat(record.Path); statErr != nil {
		t.Error("the worktree was removed despite the refusal")
	}

	if err := a.Remove(removal, true, false); err != nil {
		t.Errorf("forcing failed: %v", err)
	}
}

func TestMiaWritesNothingInTheRepository(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)

	before := treeState(t, repo.Root)

	record, err := a.New("due-dates")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.List(); err != nil {
		t.Fatal(err)
	}
	resolver, err := a.Resolver()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Record(record.Name); err != nil {
		t.Fatal(err)
	}

	after := treeState(t, repo.Root)
	for path, content := range after {
		if was, existed := before[path]; !existed {
			t.Errorf("mia created %s in the repository", path)
		} else if was != content {
			t.Errorf("mia changed %s in the repository", path)
		}
	}
	for path := range before {
		if _, still := after[path]; !still {
			t.Errorf("mia removed %s from the repository", path)
		}
	}

	dirty, err := git.Dirty(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Error("the main checkout is dirty after ordinary mia use")
	}
}

func treeState(t *testing.T, root string) map[string]string {
	t.Helper()
	state := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		state[relative] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestNewRefusesABranchNameGitWouldRefuse(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	if _, err := a.New("bad..name"); err == nil || !strings.Contains(err.Error(), "not a valid branch name") || strings.Contains(err.Error(), "hint:") {
		t.Errorf("bad..name gave %v", err)
	}
	if out := repo.Git("worktree", "list"); strings.Count(out, "\n") > 0 {
		t.Errorf("a worktree was left behind:\n%s", out)
	}
}

func TestTheMainCheckoutIsMiasWithoutAnAdopt(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	t.Chdir(repo.Root)
	record, err := a.RecordOrHere("")
	if err != nil {
		t.Fatalf("the main checkout needed an adopt: %v", err)
	}
	if git.Resolve(record.Path) != git.Resolve(repo.Root) || record.Name == "" {
		t.Errorf("record = %+v", record)
	}
}

func TestNewReusesANameWhoseFolderWasDeletedByHand(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	first, err := a.New("first")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(first.Path); err != nil {
		t.Fatal(err)
	}
	if err := a.Names.Release(first.Path); err != nil {
		t.Fatal(err)
	}
	again, err := a.New("second")
	if err != nil {
		t.Fatalf("a new worktree after one was deleted by hand: %v", err)
	}
	if again.Path != first.Path {
		t.Errorf("took %s, not the freed %s, so the stale registration was never in the way", again.Path, first.Path)
	}
}

func TestANewNameCannotHoldWhatTmuxRewritesButAnOldOneStillLoads(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	record, err := a.New("dotted")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.b", "x:y"} {
		if _, err := a.Rename(record.Name, name); err == nil {
			t.Errorf("renamed to %q", name)
		}
	}
	if owner, _ := a.Names.Owner(record.Name); owner != record.Path {
		t.Error("a refused rename gave up the old name")
	}
	record.Name = "old.style"
	if err := a.Store.Put(record); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.Load(); err != nil {
		t.Errorf("a record named before the rule no longer loads: %v", err)
	}
}
