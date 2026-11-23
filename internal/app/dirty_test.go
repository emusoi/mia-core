package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestUntrackedFilesStayDirtyBetweenWalks(t *testing.T) {
	repo := gittest.New(t)
	a := &App{}
	if err := os.WriteFile(filepath.Join(repo.Root, "notes.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !a.dirtyOf(git.Worktree{Path: repo.Root}, dirtyActive) {
		t.Fatal("an untracked file is not dirty")
	}
	got, _ := a.dirtiness.Load(repo.Root)
	d := got.(dirtiness)
	d.At = time.Now().Add(-time.Minute)
	a.dirtiness.Store(repo.Root, d)
	if !a.dirtyOf(git.Worktree{Path: repo.Root}, dirtyActive) {
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
	if !first.dirtyOf(git.Worktree{Path: repo.Root}, dirtySession) {
		t.Fatal("an untracked file is not dirty")
	}
	first.saveDirtiness()
	if err := os.Remove(notes); err != nil {
		t.Fatal(err)
	}
	// Another process, inside the time a quiet worktree is trusted: no git status, the kept answer.
	if !(&App{MiaDir: dir}).dirtyOf(git.Worktree{Path: repo.Root}, dirtySession) {
		t.Error("the next process asked git again rather than using what was kept")
	}
	// gc forgets on purpose: it looks again, and reads nothing back from disk.
	forgets := &App{MiaDir: dir}
	forgets.ForgetDirtiness()
	if forgets.dirtyOf(git.Worktree{Path: repo.Root}, dirtySession) {
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
		if (&App{MiaDir: a.MiaDir}).dirtyOf(git.Worktree{Path: path}, dirtySession) {
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
	if !(&App{MiaDir: a.MiaDir}).dirtyOf(git.Worktree{Path: second.Path}, dirtySession) {
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
	if !(&App{MiaDir: a.MiaDir}).dirtyOf(git.Worktree{Path: back.Path}, dirtySession) {
		t.Error("a worktree brought back with its work is listed clean")
	}
}

// A worktree with no session is asked git status every five minutes, not
// every half minute as one with a session is, but at once when its index or
// HEAD is written: work staged or committed outside mia is listed as it is.
func TestAWorktreeWithNoSessionIsAskedLessOftenButAtOnceWhenItsGitIsWritten(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := a.New("quiet")
	if err != nil {
		t.Fatal(err)
	}
	dirty := func() bool {
		t.Helper()
		listings, err := a.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range listings {
			if l.Name == quiet.Name {
				return l.Dirty
			}
		}
		t.Fatalf("%s is not listed: %+v", quiet.Name, listings)
		return false
	}
	aged := func(by time.Duration) {
		t.Helper()
		got, ok := a.dirtiness.Load(quiet.Path)
		if !ok {
			t.Fatal("nothing was kept of it")
		}
		d := got.(dirtiness)
		d.At, d.Untracked = d.At.Add(-by), d.Untracked.Add(-by)
		a.dirtiness.Store(quiet.Path, d)
	}
	if dirty() {
		t.Fatal("a worktree just made is dirty")
	}
	if err := os.WriteFile(filepath.Join(quiet.Path, "draft.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	aged(31 * time.Second)
	if dirty() {
		t.Error("a worktree with no session was asked git status again after 31 s")
	}
	if got, _ := a.dirtiness.Load(quiet.Path); got.(dirtiness).Git == "" {
		t.Error("its folder of git's was not known: nothing would say it was written")
	}
	if !a.dirtyOf(git.Worktree{Path: quiet.Path}, dirtySession) {
		t.Error("one with a session is asked again after 31 s")
	}
	aged(31 * time.Second)
	aged(5 * time.Minute)
	if !dirty() {
		t.Error("a worktree with no session was not asked git status again after five minutes")
	}
	if err := os.Remove(filepath.Join(quiet.Path, "draft.txt")); err != nil {
		t.Fatal(err)
	}
	aged(time.Second) // within any time it is trusted
	if !dirty() {
		t.Fatal("asked again within the time it is trusted, its index and HEAD as they were")
	}
	repo.GitIn(quiet.Path, "commit", "--allow-empty", "-q", "-m", "outside mia")
	if dirty() {
		t.Error("a commit made outside mia, its HEAD and index written, was not answered from at once")
	}
}
