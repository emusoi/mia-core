package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

func TestMergedFindsOnlyCleanUnstarredAncestorBranches(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	merged, err := a.New("merged")
	if err != nil {
		t.Fatal(err)
	}
	unmerged, err := a.New("unmerged")
	if err != nil {
		t.Fatal(err)
	}
	commit := func(path, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(path, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		repo.GitIn(path, "add", "-A")
		repo.GitIn(path, "commit", "-qm", name)
	}
	commit(merged.Path, "merged.txt")
	commit(unmerged.Path, "unmerged.txt")
	repo.Git("merge", "--ff-only", "merged")

	found, err := a.Merged()
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Branch != "merged" || !found[0].Merged {
		t.Fatalf("merged candidates = %+v", found)
	}
	if _, err := a.Star(merged.Name); err != nil {
		t.Fatal(err)
	}
	found, err = a.Merged()
	if err != nil || len(found) != 0 {
		t.Fatalf("a starred worktree was offered: %+v, %v", found, err)
	}
	if _, err := a.Star(merged.Name); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(merged.Path, "README.md")
	if err := os.WriteFile(readme, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err = a.Merged()
	if err != nil || len(found) != 0 {
		t.Fatalf("a dirty worktree was offered: %+v, %v", found, err)
	}
	repo.GitIn(merged.Path, "checkout", "--", ".")
	found, err = a.Merged()
	if err != nil || len(found) != 1 {
		t.Fatalf("clean merged worktree did not return: %+v, %v", found, err)
	}
}
