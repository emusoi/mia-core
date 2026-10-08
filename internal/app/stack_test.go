package app_test

import (
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/stack"
)

func TestRestackMovesALayerOntoItsParentsNewHead(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	base := git.CurrentBranch(repo.Root)
	if err := git.CreateBranch(repo.Root, "layer"); err != nil {
		t.Fatal(err)
	}
	if err := a.Stacks().Record("layer", base); err != nil {
		t.Fatal(err)
	}
	repo.Write("layer.txt", "layer work\n")
	repo.Commit("layer work")
	if err := git.Checkout(repo.Root, base); err != nil {
		t.Fatal(err)
	}
	repo.Write("base.txt", "base moved\n")
	repo.Commit("base moved")
	if err := git.Checkout(repo.Root, "layer"); err != nil {
		t.Fatal(err)
	}
	if git.IsAncestor(repo.Root, base, "layer") {
		t.Fatal("the layer already sits on the new base")
	}
	var moved []string
	if err := a.Restack(repo.Root, "layer", base, func(l stack.Layer) { moved = append(moved, l.Branch) }); err != nil {
		t.Fatal(err)
	}
	if !git.IsAncestor(repo.Root, base, "layer") {
		t.Error("the layer was not moved onto the base's new head")
	}
	if len(moved) != 1 || moved[0] != "layer" {
		t.Errorf("restacked %v", moved)
	}
	if git.CurrentBranch(repo.Root) != "layer" {
		t.Errorf("left on %s, not where it started", git.CurrentBranch(repo.Root))
	}
}

func TestRestackAfterAmendingALowerLayerKeepsEachLayersOwnCommits(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	base := git.CurrentBranch(repo.Root)
	parent := base
	for _, name := range []string{"s1", "s2", "s3"} {
		if err := git.CreateBranch(repo.Root, name); err != nil {
			t.Fatal(err)
		}
		if err := a.Stacks().Record(name, parent); err != nil {
			t.Fatal(err)
		}
		repo.Write(name+".txt", name+"\n")
		repo.Commit(name + " work")
		parent = name
	}
	if err := git.Checkout(repo.Root, "s1"); err != nil {
		t.Fatal(err)
	}
	repo.Write("s1.txt", "s1, amended\n")
	repo.Git("commit", "-q", "-a", "--amend", "-m", "s1 work, amended")
	if err := a.Restack(repo.Root, "s1", base, nil); err != nil {
		t.Fatal(err)
	}
	for branch, want := range map[string]string{
		"s1": "s1 work, amended",
		"s2": "s2 work|s1 work, amended",
		"s3": "s3 work|s2 work|s1 work, amended",
	} {
		got := strings.ReplaceAll(strings.TrimSpace(repo.Git("log", "--format=%s", base+".."+branch)), "\n", "|")
		if got != want {
			t.Errorf("%s holds %q, want %q", branch, got, want)
		}
	}
}

func TestForgettingAStackKeepsALayersUnmergedWorkUnlessForced(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	base := git.CurrentBranch(repo.Root)
	if err := git.CreateBranch(repo.Root, "layer"); err != nil {
		t.Fatal(err)
	}
	record := func() {
		if err := a.Stacks().Record("layer", base); err != nil {
			t.Fatal(err)
		}
	}
	record()
	repo.Write("layer.txt", "only here\n")
	repo.Commit("only here")
	if err := git.Checkout(repo.Root, base); err != nil {
		t.Fatal(err)
	}

	_, err := a.RemoveStack(repo.Root, "layer", false, true, false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("an unmerged layer was deleted, or the refusal gave no way past: %v", err)
	}
	if !git.HasBranch(repo.Root, "layer") {
		t.Fatal("the layer's branch is gone")
	}
	record()
	if _, err := a.RemoveStack(repo.Root, "layer", false, true, true); err != nil {
		t.Fatal(err)
	}
	if git.HasBranch(repo.Root, "layer") {
		t.Error("--force did not delete the branch")
	}
}
