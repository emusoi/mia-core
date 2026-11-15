package cli

import (
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

// `mia new --pr 7`: a worktree at the pull request's head, named for it, its branch tracking
// nothing; asked again after the pull request moved, the same worktree, brought up to it.
func TestNewFromAPullRequestIsAWorktreeAtItsHead(t *testing.T) {
	gittest.Isolate(t)
	forge, repo := gittest.New(t), gittest.New(t)
	forge.Write("a.txt", "the pull request's work\n")
	forge.Commit("the pull request's work")
	first := forge.Git("rev-parse", "HEAD")
	forge.Git("update-ref", "refs/pull/7/head", first)
	repo.Git("remote", "add", "upstream", forge.Root)
	t.Chdir(repo.Root)

	if code := Run([]string{"new", "--pr", "7"}); code != exitOK {
		t.Fatalf("mia new --pr 7 exited %d", code)
	}
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := a.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var record model.Record
	for _, one := range records {
		if one.Name == "pr-7" {
			record = one
		}
	}
	if record.Path == "" {
		t.Fatalf("no worktree called pr-7 among %v", records)
	}
	if got := git.Head(record.Path); got != first {
		t.Fatalf("pr-7 is at %s, want the pull request's head %s", got, first)
	}
	if up := git.Upstream(record.Path, git.CurrentBranch(record.Path)); up != "" {
		t.Fatalf("pr-7's branch tracks %s; it should track nothing", up)
	}

	// The author pushes again.
	forge.Write("a.txt", "more of it\n")
	forge.Commit("more of it")
	second := forge.Git("rev-parse", "HEAD")
	forge.Git("update-ref", "refs/pull/7/head", second)
	if code := Run([]string{"new", "--pr", "7"}); code != exitOK {
		t.Fatalf("mia new --pr 7, again, exited %d", code)
	}
	if got := git.Head(record.Path); got != second {
		t.Fatalf("pr-7 stayed at %s after the pull request moved to %s", got, second)
	}
	if trees, _ := git.Worktrees(repo.Root); len(trees) != 2 {
		t.Fatalf("asking again made another worktree: %d of them in %s", len(trees), filepath.Base(repo.Root))
	}

	if code := Run([]string{"new", "--pr", "8"}); code == exitOK {
		t.Fatal("pull request 8 is nowhere, yet a worktree was made for it")
	}
}
