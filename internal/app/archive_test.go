package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestAnArchivedWorktreeComesBackAsItWas(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("keep.txt", "one\n")
	repo.Write("gone.txt", "bye\n")
	repo.Commit("two files")
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("work")
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(record.Path, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("keep.txt", "one\ntwo\n")
	write("new.txt", "fresh\n")
	os.Remove(filepath.Join(record.Path, "gone.txt"))
	stash := repo.Git("stash", "list")

	archived, err := a.Archive(record.Name, false)
	if err != nil {
		t.Fatal(err)
	}
	if !archived.Dirty {
		t.Error("it kept uncommitted work")
	}
	if _, err := os.Stat(record.Path); !os.IsNotExist(err) {
		t.Fatal("the worktree is gone")
	}
	if repo.Git("stash", "list") != stash {
		t.Error("the stash is left alone")
	}
	if all, _ := a.Archives(); len(all) != 1 || all[0].Name != record.Name || !strings.HasSuffix(all[0].Branch, "work") {
		t.Errorf("one archive, named and on its branch: %+v", all)
	}

	back, err := a.Unarchive(record.Name)
	if err != nil {
		t.Fatal(err)
	}
	if back.Name != record.Name {
		t.Errorf("its old name, free again: %s", back.Name)
	}
	read := func(name string) string {
		data, _ := os.ReadFile(filepath.Join(back.Path, name))
		return string(data)
	}
	if read("keep.txt") != "one\ntwo\n" || read("new.txt") != "fresh\n" {
		t.Errorf("its changes and its new file: %q %q", read("keep.txt"), read("new.txt"))
	}
	if _, err := os.Stat(filepath.Join(back.Path, "gone.txt")); !os.IsNotExist(err) {
		t.Error("a file it had deleted stays deleted")
	}
	if staged := repo.GitIn(back.Path, "diff", "--cached", "--name-only"); staged != "" {
		t.Errorf("uncommitted again, nothing staged: %q", staged)
	}
	if untracked := repo.GitIn(back.Path, "ls-files", "--others", "--exclude-standard"); untracked != "new.txt" {
		t.Errorf("its new file untracked again: %q", untracked)
	}
	if all, _ := a.Archives(); len(all) != 0 {
		t.Errorf("unarchived, the archive goes: %+v", all)
	}

	repo.GitIn(back.Path, "add", "-A")
	repo.GitIn(back.Path, "commit", "-qm", "the work")
	clean, err := a.Archive(back.Name, false)
	if err != nil || clean.Dirty {
		t.Fatalf("with its work committed, the archive is the branch's own commit: %+v %v", clean, err)
	}
	if again, err := a.Unarchive(back.Name); err != nil || read("new.txt") != "fresh\n" {
		t.Errorf("a clean one comes back too: %+v %v", again, err)
	}
}

func TestAForkStartsWhereTheWorktreeIsWithItsWork(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("a.txt", "one\n")
	repo.Commit("one")
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.New("first")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first.Path, "a.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo.GitIn(first.Path, "commit", "-qam", "two")
	if err := os.WriteFile(filepath.Join(first.Path, "b.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := a.Fork(first.Name, "second")
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.GitIn(second.Path, "log", "-1", "--format=%s"); got != "two" {
		t.Errorf("it starts at the worktree's commit, not base: %q", got)
	}
	if data, _ := os.ReadFile(filepath.Join(second.Path, "b.txt")); string(data) != "draft\n" {
		t.Errorf("the uncommitted work comes along: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(first.Path, "b.txt")); string(data) != "draft\n" {
		t.Error("the first keeps its work")
	}
	if _, err := a.Fork(first.Name, "second"); err == nil {
		t.Error("a fork onto a branch that exists is refused")
	}
}

// What was staged is staged again when the worktree comes back, and in a fork
// of it: the index is part of how it was.
func TestArchiveAndForkKeepWhatWasStaged(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("keep.txt", "one\n")
	repo.Write("gone.txt", "bye\n")
	repo.Commit("two files")
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("work")
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(record.Path, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("keep.txt", "one\nstaged\n")
	write("added.txt", "added\n")
	repo.GitIn(record.Path, "add", "keep.txt", "added.txt")
	repo.GitIn(record.Path, "rm", "-q", "gone.txt")
	write("keep.txt", "one\nstaged\nnot staged\n")
	write("new.txt", "fresh\n")
	status := func(dir string) string { return repo.GitIn(dir, "status", "--porcelain") }
	was := status(record.Path)

	fork, err := a.Fork(record.Name, "second-try")
	if err != nil {
		t.Fatal(err)
	}
	if is := status(fork.Path); is != was {
		t.Errorf("the fork's status:\n%s\nwanted, as where it came from:\n%s", is, was)
	}
	if is := status(record.Path); is != was {
		t.Errorf("forking changed the first:\n%s\nwas:\n%s", is, was)
	}

	if _, err := a.Archive(record.Name, false); err != nil {
		t.Fatal(err)
	}
	back, err := a.Unarchive(record.Name)
	if err != nil {
		t.Fatal(err)
	}
	if is := status(back.Path); is != was {
		t.Errorf("back from the archive its status is:\n%s\nwanted, as it was:\n%s", is, was)
	}
	if data, _ := os.ReadFile(filepath.Join(back.Path, "keep.txt")); string(data) != "one\nstaged\nnot staged\n" {
		t.Errorf("keep.txt came back as %q", data)
	}
}
