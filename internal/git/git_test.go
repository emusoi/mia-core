package git_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestADetachedWorktreeHasNoBranchAndThatIsFine(t *testing.T) {
	repo := gittest.New(t)
	attached := repo.Worktree("monduli", "feature/due-dates")
	detached := repo.Detached("lane2")

	if got := git.CurrentBranch(attached); got != "feature/due-dates" {
		t.Errorf("CurrentBranch(attached) = %q", got)
	}
	if got := git.CurrentBranch(detached); got != "" {
		t.Errorf("CurrentBranch(detached) = %q, want empty — absence is the answer", got)
	}
	trees, err := git.Worktrees(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tree := range trees {
		if tree.Path == detached {
			found, _ = true, tree
			if tree.Branch != "" {
				t.Errorf("the detached worktree reported branch %q", tree.Branch)
			}
		}
	}
	if !found {
		t.Error("the detached worktree is missing from the list")
	}
}

func TestEveryWorktreeSharesOneCommonDir(t *testing.T) {
	repo := gittest.New(t)
	first := repo.Worktree("monduli", "one")
	second := repo.Worktree("longido", "two")

	fromMain, err := git.CommonDir(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{first, second} {
		common, err := git.CommonDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if common != fromMain {
			t.Errorf("CommonDir(%s) = %s, want %s — each worktree would keep a private world", dir, common, fromMain)
		}
	}
}

func TestPathsAreResolvedToOneForm(t *testing.T) {
	repo := gittest.New(t)
	tree := repo.Worktree("monduli", "one")

	top, err := git.Toplevel(tree)
	if err != nil {
		t.Fatal(err)
	}
	if top != tree {
		t.Errorf("Toplevel(%s) = %s; a worktree must resolve to itself", tree, top)
	}
	nested := filepath.Join(tree, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	fromInside, err := git.Toplevel(nested)
	if err != nil {
		t.Fatal(err)
	}
	if fromInside != tree {
		t.Errorf("Toplevel from inside %s = %s, want %s", nested, fromInside, tree)
	}
}

func TestWorktreesListsEveryCheckout(t *testing.T) {
	repo := gittest.New(t)
	repo.Worktree("monduli", "feature/due-dates")
	repo.Detached("lane2")

	trees, err := git.Worktrees(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(trees) != 3 {
		t.Fatalf("got %d worktrees, want 3 (main + two linked)", len(trees))
	}
	if !trees[0].Main || trees[0].Path != repo.Root {
		t.Errorf("the first entry should be the main checkout, got %+v", trees[0])
	}
	for _, tree := range trees[1:] {
		if tree.Main {
			t.Errorf("%s is marked as the main checkout", tree.Path)
		}
	}
	var slashed bool
	for _, tree := range trees {
		if tree.Branch == "feature/due-dates" {
			slashed = true
		}
	}
	if !slashed {
		t.Error("a branch name containing a slash was not parsed back")
	}
}

func TestAddAndRemove(t *testing.T) {
	repo := gittest.New(t)
	path := repo.Root + ".created"

	if err := git.AddFrom(repo.Root, path, "new-branch", true, ""); err != nil {
		t.Fatal(err)
	}
	if !git.BranchExists(repo.Root, "new-branch") {
		t.Error("the branch was not created")
	}
	if got := git.CurrentBranch(git.Resolve(path)); got != "new-branch" {
		t.Errorf("the new worktree is on %q", got)
	}

	if err := git.Remove(repo.Root, path, false); err != nil {
		t.Fatal(err)
	}
	trees, _ := git.Worktrees(repo.Root)
	for _, tree := range trees {
		if tree.Path == git.Resolve(path) {
			t.Error("the worktree survived removal")
		}
	}
}

func TestRemoveRefusesADirtyWorktree(t *testing.T) {
	repo := gittest.New(t)
	tree := repo.Worktree("monduli", "one")
	if err := writeFile(tree, "scratch.txt", "unsaved\n"); err != nil {
		t.Fatal(err)
	}

	dirty, err := git.Dirty(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("the fixture is not dirty")
	}
	if err := git.Remove(repo.Root, tree, false); err == nil {
		t.Error("a dirty worktree was removed without force")
	}
	if err := git.Remove(repo.Root, tree, true); err != nil {
		t.Errorf("forcing failed: %v", err)
	}
}

func TestErrorsCarryGitsOwnWords(t *testing.T) {
	repo := gittest.New(t)
	err := git.AddFrom(repo.Root, repo.Root+".x", "main", false, "")
	if err == nil {
		t.Fatal("checking out an already-checked-out branch was allowed")
	}
	if strings.Contains(err.Error(), "exit status") {
		t.Errorf("the error lost git's explanation: %v", err)
	}
}

func writeFile(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func TestDiffStatCountsTheBranchsOwnChanges(t *testing.T) {
	repo := gittest.New(t)
	repo.Git("checkout", "-qb", "feature")
	repo.Write("a.go", "one\ntwo\n")
	repo.Write("README.md", "changed\n")
	repo.Commit("work")
	files, added, deleted := git.DiffStat(repo.Root, "main", "feature")
	if files != 2 || added != 3 || deleted != 1 {
		t.Errorf("DiffStat = %d files +%d −%d, want 2 +3 −1", files, added, deleted)
	}
}

func TestChangedFiles(t *testing.T) {
	repo := gittest.New(t)
	repo.Git("checkout", "-qb", "feature")
	repo.Write("a.go", "one\n")
	repo.Commit("work")
	files := git.ChangedFiles(repo.Root, "main", "feature")
	if len(files) != 1 || files[0] != (git.FileStat{Path: "a.go", Added: 1}) {
		t.Fatalf("ChangedFiles = %+v", files)
	}
}

func TestFetchPRBringsThePullRequestHeadIntoABranch(t *testing.T) {
	repo := gittest.New(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	repo.Git("init", "-q", "--bare", origin)
	repo.Git("remote", "add", "origin", origin)
	repo.Git("checkout", "-qb", "contribution")
	repo.Write("a.go", "one\n")
	repo.Commit("their work")
	head := repo.Git("rev-parse", "HEAD")
	repo.Git("push", "-q", "origin", "contribution:refs/pull/7/head")
	repo.Git("checkout", "-q", "main")
	if err := git.FetchPR(repo.Root, 7, "pr-7"); err != nil {
		t.Fatal(err)
	}
	if got := repo.Git("rev-parse", "pr-7"); got != head {
		t.Errorf("pr-7 = %s, want %s", got, head)
	}
}
