package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestALayerMoveRefusesToCarryUncommittedWork(t *testing.T) {
	repo := gittest.New(t)
	file := filepath.Join(repo.Root, "notes.txt")
	if err := os.WriteFile(file, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo.Git("add", "notes.txt")
	repo.Git("commit", "-q", "-m", "notes")
	repo.Git("branch", "upper")
	if err := os.WriteFile(file, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := toLayer(repo.Root, "main", "upper"); err == nil {
		t.Error("moved to upper with notes.txt changed")
	}
	if got := git.CurrentBranch(repo.Root); got != "main" {
		t.Errorf("on %s after a refused move", got)
	}
}
