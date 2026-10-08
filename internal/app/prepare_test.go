package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/gittest"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestNewClonesNamedPathsFromTheMainCheckout(t *testing.T) {
	repo := gittest.New(t)
	repo.Write(".gitignore", "node_modules/\n")
	repo.Write("app/.env.local", "SECRET=1\n")
	repo.Commit("ignore")
	repo.Write("node_modules/left-pad/index.js", "module.exports = 1\n")
	repo.Write("README.md", "changed in the main checkout, not committed\n")
	a := open(t, repo)
	a.Config.Clone = []string{"node_modules", "app/.env*", "README.md", "not-there"}

	record, err := a.New("due-dates")
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(record.Path, "node_modules/left-pad/index.js")); got != "module.exports = 1\n" {
		t.Errorf("cloned node_modules holds %q", got)
	}
	if got := read(t, filepath.Join(record.Path, "README.md")); got != "fixture\n" {
		t.Errorf("clone overwrote the worktree's own README with %q", got)
	}
	// A copy, not a link: the worktree's changes stay its own.
	if err := os.WriteFile(filepath.Join(record.Path, "node_modules/left-pad/index.js"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(repo.Root, "node_modules/left-pad/index.js")); got != "module.exports = 1\n" {
		t.Errorf("writing the clone changed the main checkout: %q", got)
	}
}

func TestNewRefusesToCloneFromOutsideTheRepository(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	a.Config.Clone = []string{"../elsewhere"}
	_, err := a.New("due-dates")
	if err == nil || !strings.Contains(err.Error(), "inside the repository") {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

func TestNewGivesFilesTheirLastCommitsTime(t *testing.T) {
	repo := gittest.New(t)
	old := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	repo.Write("old.txt", "a\n")
	t.Setenv("GIT_COMMITTER_DATE", old.Format(time.RFC3339))
	repo.Commit("old")
	repo.Write("new.txt", "b\n")
	t.Setenv("GIT_COMMITTER_DATE", newer.Format(time.RFC3339))
	repo.Commit("newer")
	a := open(t, repo)
	a.Config.CommitTimes = true

	record, err := a.New("due-dates")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]time.Time{"old.txt": old, "new.txt": newer} {
		info, err := os.Stat(filepath.Join(record.Path, name))
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(want) {
			t.Errorf("%s is dated %s, want %s", name, info.ModTime().UTC(), want)
		}
	}
}
