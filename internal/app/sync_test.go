package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestSyncFetchesDropsLandedAndRestacksWithoutDeletingBranches(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	base := git.CurrentBranch(repo.Root)
	remote := filepath.Join(t.TempDir(), "origin.git")
	repo.Git("init", "--bare", remote)
	repo.Git("remote", "add", "origin", remote)
	repo.Git("push", "-u", "origin", base)

	if err := git.CreateBranch(repo.Root, "bottom"); err != nil {
		t.Fatal(err)
	}
	if err := a.Stacks().Record("bottom", base); err != nil {
		t.Fatal(err)
	}
	repo.Write("bottom.txt", "bottom\n")
	repo.Commit("bottom work")
	if err := git.CreateBranch(repo.Root, "top"); err != nil {
		t.Fatal(err)
	}
	if err := a.Stacks().Record("top", "bottom"); err != nil {
		t.Fatal(err)
	}
	repo.Write("top.txt", "top\n")
	repo.Commit("top work")
	if err := git.Checkout(repo.Root, base); err != nil {
		t.Fatal(err)
	}
	repo.Git("merge", "--ff-only", "bottom")
	repo.Git("push", "origin", base)
	if err := git.Checkout(repo.Root, "top"); err != nil {
		t.Fatal(err)
	}

	clone := filepath.Join(t.TempDir(), "clone")
	repo.GitIn(filepath.Dir(clone), "clone", "--branch", base, remote, clone)
	repo.GitIn(clone, "config", "user.email", "other@example.com")
	repo.GitIn(clone, "config", "user.name", "other")
	if err := os.WriteFile(filepath.Join(clone, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo.GitIn(clone, "add", "remote.txt")
	repo.GitIn(clone, "commit", "-qm", "remote base work")
	repo.GitIn(clone, "push", "origin", base)
	remoteHead := repo.GitIn(clone, "rev-parse", "HEAD")

	result, err := a.Sync(repo.Root, "top")
	if err != nil {
		t.Fatal(err)
	}
	if result.Base != base || result.Current != "top" || !slices.Equal(result.Dropped, []string{"bottom"}) || result.Restacked != 1 {
		t.Fatalf("sync result = %+v", result)
	}
	if git.RevParse(repo.Root, base) != remoteHead || !git.IsAncestor(repo.Root, remoteHead, "top") {
		t.Fatal("the top was not restacked onto the fetched base")
	}
	if a.Stacks().Member("bottom") || !a.Stacks().Member("top") {
		t.Fatal("landed layer was not forgotten correctly")
	}
	if !git.HasBranch(repo.Root, "bottom") {
		t.Fatal("sync deleted the landed branch")
	}
	if dirty := repo.Git("status", "--porcelain"); dirty != "" {
		t.Fatalf("sync left the worktree dirty: %s", dirty)
	}

	basePath := filepath.Join(t.TempDir(), "base")
	repo.Git("worktree", "add", basePath, base)
	if err := os.WriteFile(filepath.Join(basePath, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := git.Head(repo.Root)
	if _, err := a.Sync(repo.Root, "top"); err == nil || !strings.Contains(err.Error(), "base worktree") {
		t.Fatalf("sync ignored a dirty base worktree: %v", err)
	}
	if git.Head(repo.Root) != before {
		t.Fatal("a refused sync moved the current branch")
	}
	repo.Git("worktree", "remove", "--force", basePath)
}
