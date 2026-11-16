package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

// What is not committed crosses as it stands: what was staged is staged on the
// other side, what was only changed is only changed, a new file is new.
// What git ignores does not cross, and is named.
func TestUncommittedWorkCrossesWithItsIndex(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("kept.txt", "one\n")
	repo.Write("gone.txt", "gone\n")
	repo.Write("staged-gone.txt", "gone\n")
	repo.Write("tool.sh", "#!/bin/sh\n")
	repo.Write(".gitignore", "*.log\n")
	repo.Commit("files")
	from, to := repo.Worktree("from", "work"), repo.Detached("to")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(from, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("kept.txt", "one\nstaged\n")
	write("staged-new.txt", "new\n")
	repo.GitIn(from, "add", "kept.txt", "staged-new.txt")
	write("kept.txt", "one\nstaged\nnot staged\n")
	repo.GitIn(from, "rm", "-q", "staged-gone.txt")
	if err := os.Remove(filepath.Join(from, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(from, "tool.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("a new file é.txt", "untracked\n")
	write("debug.log", "ignored\n")

	local := side{}
	work, err := local.uncommitted(from)
	if err != nil {
		t.Fatal(err)
	}
	if said := work.staysBehind(); said != "not crossing, since git ignores them: debug.log" {
		t.Errorf("what stays behind was said as %q", said)
	}
	if said := work.summary(); said != "with 3 file(s) staged, 3 changed and not staged, 1 new" {
		t.Errorf("the work was summed up as %q", said)
	}
	if err := local.putUncommitted(to, work); err != nil {
		t.Fatal(err)
	}
	// a worktree with work of its own is not one to put more into: a move that
	// found the branch checked out there would overwrite its new files
	if err := local.putUncommitted(to, work); err == nil || !strings.Contains(err.Error(), "uncommitted work of its own") {
		t.Errorf("putting work into a worktree that has some: %v", err)
	}
	status := func(dir string) string {
		return repo.GitIn(dir, "-c", "core.quotepath=off", "status", "--porcelain")
	}
	if was, is := status(from), status(to); was != is {
		t.Errorf("status there:\n%s\nwanted, as here:\n%s", is, was)
	}
	for _, name := range []string{"kept.txt", "staged-new.txt", "a new file é.txt"} {
		was, _ := os.ReadFile(filepath.Join(from, name))
		is, err := os.ReadFile(filepath.Join(to, name))
		if err != nil || string(was) != string(is) {
			t.Errorf("%s there is %q (%v), here %q", name, is, err, was)
		}
	}
	if staged := repo.GitIn(to, "diff", "--cached", "--", "kept.txt"); staged == "" || repo.GitIn(to, "diff", "--", "kept.txt") == "" {
		t.Errorf("kept.txt should be staged and changed beyond that there")
	}
}

// A move to where the worktree already is says so, not that it is two moves, and what to name instead.
func TestMoveToWhereItIsSaysSo(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Move("monduli", "local", "local", false, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "monduli would move from local to where it is") {
		t.Fatalf("moving to where it is: %v", err)
	}
}

// What a command here said when it failed is the error, with no machine's name before it.
func TestAFailureHereNamesNoMachine(t *testing.T) {
	_, err := side{}.sh("echo no such thing >&2; exit 1")
	if err == nil || err.Error() != "no such thing" {
		t.Fatalf("error = %v", err)
	}
}
