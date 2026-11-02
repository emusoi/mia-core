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
