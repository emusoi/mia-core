package resolve_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/resolve"
)

func TestADetachedWorktreeResolvesByEverySpelling(t *testing.T) {
	repo := gittest.New(t)
	tree := repo.Detached("monduli")

	r := resolve.Repo{
		Root:    repo.Root,
		Records: []model.Record{{Path: tree, Name: "monduli"}},
	}

	if branch := git.CurrentBranch(tree); branch != "" {
		t.Fatalf("the fixture is not detached: branch = %q", branch)
	}

	for _, spelling := range []string{
		"monduli",
		tree,
		filepath.Base(tree),
		filepath.Join(tree, "."),
	} {
		got, err := r.Worktree(spelling)
		if err != nil {
			t.Errorf("%q did not resolve: %v", spelling, err)
			continue
		}
		if got != tree {
			t.Errorf("%q resolved to %s, want %s", spelling, got, tree)
		}
	}
}

func TestTheBranchResolvesButDoesNotOutrankTheName(t *testing.T) {
	repo := gittest.New(t)
	tree := repo.Worktree("monduli", "feature/due-dates")
	other := repo.Worktree("longido", "monduli")

	r := resolve.Repo{
		Root: repo.Root,
		Records: []model.Record{
			{Path: tree, Name: "monduli"},
			{Path: other, Name: "longido"},
		},
	}

	if got, err := r.Worktree("feature/due-dates"); err != nil || got != tree {
		t.Errorf("branch spelling resolved to %s (%v), want %s", got, err, tree)
	}

	if got, err := r.Worktree("monduli"); err != nil || got != tree {
		t.Errorf("%q resolved to %s (%v); the name must win over a branch spelled the same", "monduli", got, err)
	}
}

func TestNothingFoundIsItsOwnAnswer(t *testing.T) {
	repo := gittest.New(t)
	r := resolve.Repo{Root: repo.Root}

	_, err := r.Worktree("nowhere")
	var notFound resolve.NotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("want a NotFound, got %T: %v", err, err)
	}
	if notFound.Query != "nowhere" {
		t.Errorf("the error lost the query: %+v", notFound)
	}
}

func TestAnUnadoptedWorktreeSaysSoRatherThanVanishing(t *testing.T) {
	repo := gittest.New(t)
	tree := repo.Worktree("stranger", "some-branch")
	r := resolve.Repo{Root: repo.Root}

	if got, err := r.Worktree(tree); err != nil || got != tree {
		t.Fatalf("a real worktree did not resolve by path: %s, %v", got, err)
	}
	_, err := r.Record(tree)
	if err == nil {
		t.Fatal("a worktree with no record produced one")
	}
	var notFound resolve.NotFound
	if errors.As(err, &notFound) {
		t.Errorf("an unadopted worktree reported as missing entirely: %v", err)
	}
}

func TestHereFindsTheWorktreeYouAreStandingIn(t *testing.T) {
	repo := gittest.New(t)
	tree := repo.Worktree("monduli", "one")
	r := resolve.Repo{Root: repo.Root}

	got, err := r.Here(tree)
	if err != nil {
		t.Fatal(err)
	}
	if got != tree {
		t.Errorf("Here(%s) = %s", tree, got)
	}
}
