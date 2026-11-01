package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
)

// A repository is found once however many of its worktrees are named, by its
// main checkout; one gone from disk is left out.
func TestRepositoriesFromNames(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	dir, err := os.MkdirTemp(home, ".mia-repos-test-")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(dir)
	main := filepath.Join(dir, "shop")
	wt := filepath.Join(dir, "shop.kilwa")
	for _, args := range [][]string{{"init", "-q", main}, {"-C", main, "commit", "-q", "--allow-empty", "-m", "x"}, {"-C", main, "worktree", "add", "-q", wt}} {
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	got := repositoriesOf(map[string]string{"shop": main, "kilwa": wt, "gone": filepath.Join(dir, "gone")})
	if len(got) != 1 || got[0].Name != "shop" || got[0].Path != git.Resolve(main) {
		t.Fatalf("%+v", got)
	}
}
