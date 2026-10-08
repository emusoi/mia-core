package app_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
)

func TestNamedListsOnlySurveyTheSelectedWorktree(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	for i := range 8 {
		name := fmt.Sprintf("named-%d", i)
		path := repo.Worktree(name, fmt.Sprintf("feature-%d", i))
		if err := a.Store.Put(model.Record{Name: name, Path: path}); err != nil {
			t.Fatal(err)
		}
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	for name, body := range map[string]string{
		"git":  "printf 'git %s\\n' \"$*\" >> \"$MIA_NAMED_LIST_CALLS\"\nexec " + run.ShellJoin([]string{realGit}) + " \"$@\"\n",
		"tmux": "printf 'tmux %s\\n' \"$1\" >> \"$MIA_NAMED_LIST_CALLS\"\nexit 1\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("MIA_NAMED_LIST_CALLS", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	all, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	var selected app.Listing
	for _, listing := range all {
		if listing.Name == "named-0" {
			selected = listing
		}
	}
	next, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	filtered, err := next.List("named-0")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(filtered)
	want, _ := json.Marshal([]app.Listing{selected})
	if string(got) != string(want) {
		t.Fatalf("the selected listing changed: got %+v, want %+v", filtered, selected)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(calls), "\n"); count != 5 {
		t.Fatalf("named listing used %d subprocesses, want 5: %s", count, calls)
	}
	if count := strings.Count(string(calls), "git status --porcelain"); count != 2 {
		t.Fatalf("named listing probed %d statuses, want the selected tree only: %s", count, calls)
	}
}

func TestNamedListsKeepWorktreeKindsAndRefreshIdentity(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	path := repo.Worktree("selected", "feature")
	stranger := repo.Worktree("stranger", "unadopted")
	detached := repo.Detached("detached")
	for _, record := range []model.Record{
		{Name: "selected", Path: path},
		{Name: "detached", Path: detached},
		{Name: "missing", Path: filepath.Join(t.TempDir(), "absent")},
	} {
		if err := a.Store.Put(record); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	missing, err := a.List("missing")
	if err != nil || missing == nil || len(missing) != 0 {
		t.Fatalf("an absent name did not produce an empty list: %+v, %v", missing, err)
	}
	records, err := a.Store.Load()
	if err != nil || len(records) != 2 {
		t.Fatalf("the missing record was not removed: %+v, %v", records, err)
	}
	if err := a.Stacks().Record("unadopted", "feature"); err != nil {
		t.Fatal(err)
	}
	all, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("the fixture lost a worktree: %+v", all)
	}
	for _, listing := range all {
		filtered, err := a.List(listing.Name)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(filtered, []app.Listing{listing}) {
			t.Fatalf("named %s differs from the full list: %+v", listing.Name, filtered)
		}
		if listing.Path == path && (len(filtered[0].Layers) != 2 || filtered[0].Layers[1].Worktree != filepath.Base(stranger)) {
			t.Fatalf("the selected stack lost another worktree: %+v", filtered)
		}
		if listing.Path == stranger && listing.Adopted || listing.Path == detached && (listing.Branch != "" || !listing.Adopted) {
			t.Fatalf("named listing changed its worktree kind: %+v", listing)
		}
	}
	repo.GitIn(path, "checkout", "-qb", "next-feature")
	moved := path + "-moved"
	repo.Git("worktree", "move", path, moved)
	if err := a.Store.Remove(path); err != nil {
		t.Fatal(err)
	}
	placement, err := model.OnRuntime("fixture", "/fixture/selected")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "selected", Path: moved, Starred: true, Env: &model.Environment{Placement: placement}}); err != nil {
		t.Fatal(err)
	}
	fresh, err := a.List("selected")
	if err != nil || len(fresh) != 1 || fresh[0].Path != moved || fresh[0].Branch != "next-feature" || fresh[0].Where != "fixture" || !fresh[0].Starred {
		t.Fatalf("named preflight kept old identity: %+v, %v", fresh, err)
	}
}
