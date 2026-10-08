package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/run"
)

func TestListDiscoversTheBaseOnceAndRefreshesItOnTheNextSurvey(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	path := repo.Worktree("feature", "feature")
	if _, err := a.Adopt(path); err != nil {
		t.Fatal(err)
	}
	first := repo.Git("rev-parse", "main")
	repo.Git("checkout", "-qb", "second-base")
	repo.Write("base.txt", "next base\n")
	repo.Commit("next base")
	second := repo.Git("rev-parse", "HEAD")
	repo.Git("checkout", "-q", "main")
	repo.Git("update-ref", "refs/remotes/origin/first", first)
	repo.Git("update-ref", "refs/remotes/origin/second", second)
	repo.Git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/first")
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "git-calls")
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MIA_LIST_GIT_CALLS\"\nexec " + run.ShellJoin([]string{realGit}) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIA_LIST_GIT_CALLS", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, check := range []struct {
		ref    string
		behind int
	}{
		{"first", 0},
		{"second", 1},
	} {
		repo.Git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+check.ref)
		if err := os.WriteFile(log, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		listings, err := a.List()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, listing := range listings {
			if listing.Path == path {
				found = true
				if listing.Behind != check.behind {
					t.Errorf("base %s: behind = %d, want %d", check.ref, listing.Behind, check.behind)
				}
			}
		}
		if !found {
			t.Fatal("the adopted worktree was not listed")
		}
		calls, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		if count := strings.Count(string(calls), "symbolic-ref --short refs/remotes/origin/HEAD\n"); count != 1 {
			t.Errorf("base %s: discovered the base %d times, want once", check.ref, count)
		}
		if a.Config.Base != "" {
			t.Fatalf("listing rewrote the configured base to %q", a.Config.Base)
		}
	}
	if got := a.BaseBranch(path); got != "origin/second" {
		t.Fatalf("the survey retained the previous base: %q", got)
	}
}
