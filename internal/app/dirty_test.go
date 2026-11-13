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
	d.At = time.Now().Add(-time.Minute)
	a.dirtiness.Store(repo.Root, d)
	if !a.dirtyOf(repo.Root, true) {
		t.Error("the untracked file was forgotten before the next walk")
	}
}

func TestWhatGitStatusSaidIsKeptBetweenProcesses(t *testing.T) {
	repo := gittest.New(t)
	dir := t.TempDir()
	notes := filepath.Join(repo.Root, "notes.md")
	if err := os.WriteFile(notes, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := &App{MiaDir: dir}
	if !first.dirtyOf(repo.Root, false) {
		t.Fatal("an untracked file is not dirty")
	}
	first.saveDirtiness()
	if err := os.Remove(notes); err != nil {
		t.Fatal(err)
	}
	// Another process, inside the time a quiet worktree is trusted: no git status, the kept answer.
	if !(&App{MiaDir: dir}).dirtyOf(repo.Root, false) {
		t.Error("the next process asked git again rather than using what was kept")
	}
	// gc forgets on purpose: it looks again, and reads nothing back from disk.
	forgets := &App{MiaDir: dir}
	forgets.ForgetDirtiness()
	if forgets.dirtyOf(repo.Root, false) {
		t.Error("after ForgetDirtiness the kept answer came back")
	}
}
