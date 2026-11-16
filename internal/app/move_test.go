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
// What git ignores and is too much to take does not cross, and is named.
func TestUncommittedWorkCrossesWithItsIndex(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("kept.txt", "one\n")
	repo.Write("gone.txt", "gone\n")
	repo.Write("staged-gone.txt", "gone\n")
	repo.Write("tool.sh", "#!/bin/sh\n")
	repo.Write(".gitignore", "logs/\n")
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
	if err := os.Mkdir(filepath.Join(from, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("logs/debug.log", "ignored\n")

	local := side{}
	work, err := local.uncommitted(from)
	if err != nil {
		t.Fatal(err)
	}
	if said := work.staysBehind(); said != "not crossing — git ignores them, and each is a folder or over 1 MiB: logs/" {
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

// A small file git ignores, a .env, crosses with the move, its mode kept: it is
// lost otherwise when the worktree it left is removed. A folder git ignores and
// a file over 1 MiB stay, and only they are named as staying.
func TestSmallIgnoredFilesCross(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write(".gitignore", "*.env\nbuild/\n*.bin\nlinked\n")
	repo.Commit("files")
	from, to := repo.Worktree("from", "work"), repo.Detached("to")
	write := func(dir, name string, content []byte, mode os.FileMode) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, mode); err != nil {
			t.Fatal(err)
		}
	}
	write(from, ".env", []byte("KEY=here\n"), 0o600)
	write(from, "conf/local é.env", []byte("KEY=deeper\n"), 0o644)
	write(from, "edge.bin", make([]byte, ignoredLimit), 0o644)
	write(from, "large.bin", make([]byte, ignoredLimit+1), 0o644)
	write(from, "build/out.o", []byte("built\n"), 0o644)
	write(from, "conf/large.bin", make([]byte, ignoredLimit+1), 0o644)
	if err := os.Symlink(".env", filepath.Join(from, "linked")); err != nil {
		t.Fatal(err)
	}

	local := side{}
	work, err := local.uncommitted(from)
	if err != nil {
		t.Fatal(err)
	}
	if work.none() {
		t.Fatal("ignored files that cross are work to put on the other side")
	}
	if said := work.summary(); said != "with 0 file(s) staged, 0 changed and not staged, 0 new, 3 ignored file(s) carried" {
		t.Errorf("the work was summed up as %q", said)
	}
	if said := work.staysBehind(); said != "not crossing — git ignores them, and each is a folder or over 1 MiB: build/, conf/large.bin, large.bin, linked" {
		t.Errorf("what stays behind was said as %q", said)
	}
	if err := local.putUncommitted(to, work); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", "conf/local é.env", "edge.bin"} {
		was, _ := os.ReadFile(filepath.Join(from, name))
		is, err := os.ReadFile(filepath.Join(to, name))
		if err != nil || string(was) != string(is) {
			t.Errorf("%s did not cross as it was: %v", name, err)
		}
	}
	if info, err := os.Stat(filepath.Join(to, ".env")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf(".env there should be readable by its owner only: %v, %v", info, err)
	}
	for _, name := range []string{"large.bin", "build", "linked"} {
		if _, err := os.Lstat(filepath.Join(to, name)); err == nil {
			t.Errorf("%s crossed, and should have stayed", name)
		}
	}
	// the same file there already is no reason to refuse: nothing would change
	if err := local.putUncommitted(to, work); err != nil {
		t.Errorf("putting the same ignored files where they are: %v", err)
	}
}

// An ignored file the other side already has, and has differently, is not
// written over: the move is refused before anything is put there.
func TestAnIgnoredFileThereIsNotWrittenOver(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("kept.txt", "one\n")
	repo.Write(".gitignore", ".env\n")
	repo.Commit("files")
	from, to := repo.Worktree("from", "work"), repo.Detached("to")
	for dir, content := range map[string]string{from: "KEY=here\n", to: "KEY=there\n"} {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(from, "kept.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	local := side{}
	work, err := local.uncommitted(from)
	if err != nil {
		t.Fatal(err)
	}
	err = local.putUncommitted(to, work)
	if err == nil || !strings.Contains(err.Error(), "has a .env of its own that differs") {
		t.Fatalf("putting .env over another: %v", err)
	}
	if is, _ := os.ReadFile(filepath.Join(to, ".env")); string(is) != "KEY=there\n" {
		t.Errorf(".env there was written over: %q", is)
	}
	if status := repo.GitIn(to, "status", "--porcelain"); status != "" {
		t.Errorf("the refusal should leave the worktree there as it was:\n%s", status)
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
