package dev

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	gittest.Isolate(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	for rel, body := range files {
		write(t, filepath.Join(dir, rel), body)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "first")
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		return "<missing>"
	}
	return string(body)
}

func TestLendAndGiveBack(t *testing.T) {
	main := repo(t, map[string]string{".gitignore": "node_modules\n", "app.js": "main", "old/gone.js": "x", "keep.js": "same"})
	write(t, filepath.Join(main, "node_modules", "dep.js"), "installed")
	write(t, filepath.Join(main, "app.js"), "main, edited")
	write(t, filepath.Join(main, "notes.txt"), "main's untracked")

	branch := repo(t, map[string]string{".gitignore": "node_modules\n", "app.js": "branch", "keep.js": "same", "new/feature.js": "f"})
	write(t, filepath.Join(branch, "draft.js"), "uncommitted")

	p := Main{Path: main, State: filepath.Join(t.TempDir(), "lent.json")}
	if err := p.Lend("monduli", branch); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"app.js": "branch", "new/feature.js": "f", "draft.js": "uncommitted", "keep.js": "same",
		"old/gone.js": "<missing>", "notes.txt": "<missing>", "node_modules/dep.js": "installed",
	} {
		if got := read(t, filepath.Join(main, rel)); got != want {
			t.Errorf("lent %s = %q, want %q", rel, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(main, "old")); !os.IsNotExist(err) {
		t.Errorf("empty folder old/ left behind")
	}

	write(t, filepath.Join(branch, "app.js"), "branch, again")
	if changed, err := Mirror(branch, main); err != nil || changed != 1 {
		t.Fatalf("second mirror changed %d, %v; want 1", changed, err)
	}

	if _, err := p.Off(); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"app.js": "main, edited", "notes.txt": "main's untracked", "old/gone.js": "x",
		"draft.js": "<missing>", "new/feature.js": "<missing>", "node_modules/dep.js": "installed",
	} {
		if got := read(t, filepath.Join(main, rel)); got != want {
			t.Errorf("given back %s = %q, want %q", rel, got, want)
		}
	}
	if list := gitIn(t, main, "stash", "list"); list != "" {
		t.Errorf("stash left behind: %s", list)
	}
}

func TestLendingAnotherGivesBackFirst(t *testing.T) {
	main := repo(t, map[string]string{"app.js": "main"})
	write(t, filepath.Join(main, "app.js"), "main, edited")
	a := repo(t, map[string]string{"app.js": "a", "a.js": "a"})
	b := repo(t, map[string]string{"app.js": "b"})
	p := Main{Path: main, State: filepath.Join(t.TempDir(), "lent.json")}
	if err := p.Lend("a", a); err != nil {
		t.Fatal(err)
	}
	if err := p.Lend("b", b); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(main, "a.js")); got != "<missing>" {
		t.Errorf("a.js = %q after lending b", got)
	}
	if _, err := p.Off(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(main, "app.js")); got != "main, edited" {
		t.Errorf("app.js = %q, want main's own edit back", got)
	}
}
