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

// What mia changes itself it does not go on believing unchanged: a fork and a
// worktree brought back from an archive are given their work after they are
// made, and a listing in between — the phone's sweep — had kept each as clean.
func TestAWorktreeMiaGaveWorkToIsNotKeptAsClean(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("a.txt", "one\n")
	repo.Commit("one")
	a, err := Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.New("first")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first.Path, "draft.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// another process had looked at the worktree while it was still clean
	keptClean := func(path string) {
		t.Helper()
		other := &App{MiaDir: a.MiaDir}
		other.dirtiness.Store(path, dirtiness{At: time.Now(), Untracked: time.Now()})
		other.dirtySeen.Store(true)
		other.saveDirtiness()
		if (&App{MiaDir: a.MiaDir}).dirtyOf(path, false) {
			t.Fatal("the answer kept as clean was not kept")
		}
	}
	// the next worktree is handed the name, and so the folder, of the one before it
	probe, err := a.New("probe")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Remove(Removal{Record: probe}, true, true); err != nil {
		t.Fatal(err)
	}
	keptClean(probe.Path)
	second, err := a.Fork(first.Name, "second")
	if err != nil {
		t.Fatal(err)
	}
	if second.Path != probe.Path {
		t.Skipf("the fork was made at %s, not %s", second.Path, probe.Path)
	}
	if !(&App{MiaDir: a.MiaDir}).dirtyOf(second.Path, false) {
		t.Error("a fork with its source's work is listed clean")
	}

	archived, err := a.Archive(second.Name, true)
	if err != nil {
		t.Fatal(err)
	}
	keptClean(second.Path)
	back, err := a.Unarchive(archived.Name)
	if err != nil {
		t.Fatal(err)
	}
	if back.Path != second.Path {
		t.Skipf("it came back at %s, not %s", back.Path, second.Path)
	}
	if !(&App{MiaDir: a.MiaDir}).dirtyOf(back.Path, false) {
		t.Error("a worktree brought back with its work is listed clean")
	}
}
