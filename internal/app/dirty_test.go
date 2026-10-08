package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/gittest"
)

func TestUntrackedFilesStayDirtyBetweenWalks(t *testing.T) {
	repo := gittest.New(t)
	a := &App{}
	if err := os.WriteFile(filepath.Join(repo.Root, "notes.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !a.dirtyOf(repo.Root, true) {
		t.Fatal("an untracked file is not dirty")
	}
	got, _ := a.dirtiness.Load(repo.Root)
	d := got.(dirtiness)
	d.at = time.Now().Add(-time.Minute)
	a.dirtiness.Store(repo.Root, d)
	if !a.dirtyOf(repo.Root, true) {
		t.Error("the untracked file was forgotten before the next walk")
	}
}
