package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestBringHomeTakesTheMachinesBranchOnlyForward(t *testing.T) {
	here := gittest.New(t)
	there := filepath.Join(t.TempDir(), "there")
	here.GitIn(filepath.Dir(there), "clone", "-q", here.Root, there)
	here.GitIn(there, "config", "user.email", "test@example.com")
	here.GitIn(there, "config", "user.name", "test")
	commitThere := func(message string) {
		if err := os.WriteFile(filepath.Join(there, message), []byte(message), 0o644); err != nil {
			t.Fatal(err)
		}
		here.GitIn(there, "add", "-A")
		here.GitIn(there, "commit", "-qm", message)
	}
	here.GitIn(there, "checkout", "-qb", "fix")
	commitThere("one")
	here.Git("remote", "add", "fedora", there)
	a := &App{Root: here.Root}

	head, err := a.BringHome("fedora", "fix")
	if err != nil || head != git.RevParse(there, "HEAD") || git.RevParse(here.Root, "refs/heads/fix") != head {
		t.Fatalf("a branch new here is made at the machine's head: %s %v", head, err)
	}
	commitThere("two")
	if head, err := a.BringHome("fedora", "fix"); err != nil || git.RevParse(here.Root, "refs/heads/fix") != head || head != git.RevParse(there, "HEAD") {
		t.Fatalf("a branch behind here moves forward: %v", err)
	}
	here.Git("checkout", "-q", "fix")
	here.Write("mine", "mine\n")
	here.Commit("mine")
	ahead := git.RevParse(here.Root, "HEAD")
	if head, err := a.BringHome("fedora", "fix"); err != nil || head != ahead {
		t.Fatalf("a branch ahead here is left alone: %s %v", head, err)
	}
	commitThere("three")
	if _, err := a.BringHome("fedora", "fix"); err == nil || !strings.Contains(err.Error(), "merge fedora/fix") {
		t.Fatalf("branches moved on both sides are not overwritten: %v", err)
	}
	if git.RevParse(here.Root, "refs/heads/fix") != ahead {
		t.Error("the diverged branch here was touched")
	}
	if _, err := a.BringHome("fedora", "nothing"); err == nil {
		t.Error("a branch the machine does not have is an error")
	}
}
