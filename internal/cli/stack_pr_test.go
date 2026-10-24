package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestStackPRDraftUsesSavedBodyAndGitHubBase(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	base := git.CurrentBranch(repo.Root)
	if err := git.CreateBranch(repo.Root, "layer"); err != nil {
		t.Fatal(err)
	}
	repo.Write("layer.txt", "layer\n")
	repo.Commit("layer work")
	if err := a.Stacks().Record("layer", "origin/main"); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "main", Path: repo.Root}); err != nil {
		t.Fatal(err)
	}
	a.Config.Base = "origin/main"
	repo.Git("update-ref", "refs/remotes/origin/main", base)
	t.Chdir(repo.Root)

	read := func(args ...string) string {
		t.Helper()
		out, err := os.CreateTemp(t.TempDir(), "stack-pr")
		if err != nil {
			t.Fatal(err)
		}
		stdout := os.Stdout
		os.Stdout = out
		code := cmdStack(a, args, false)
		os.Stdout = stdout
		if err := out.Close(); err != nil {
			t.Fatal(err)
		}
		if code != exitOK {
			t.Fatalf("stack %v exited %d", args, code)
		}
		data, err := os.ReadFile(out.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	if text := read("pr"); !strings.Contains(text, "gh pr create --base 'main'") || strings.Contains(text, "--base 'origin/main'") {
		t.Fatalf("stack pr printed the wrong base: %s", text)
	}
	text := read("pr", "--draft")
	for _, want := range []string{"--base 'main'", "--title 'layer work'", "--body-file"} {
		if !strings.Contains(text, want) {
			t.Errorf("stack pr --draft missing %q: %s", want, text)
		}
	}
	if _, err := os.Stat(a.MiaDir + "/pr/layer.md"); err != nil {
		t.Fatalf("draft was not saved: %v", err)
	}
	if got := git.CurrentBranch(repo.Root); got != "layer" {
		t.Fatalf("stack pr --draft checked out %s", got)
	}
}
