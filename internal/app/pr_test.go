package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestPRPreviewUsesTheLinkedWorktreesBranchWithoutWriting(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	path := repo.Worktree("monduli", "fix/foo")
	record := model.Record{Name: "monduli", Path: path}
	if err := os.WriteFile(filepath.Join(path, "change.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo.GitIn(path, "add", "change.txt")
	repo.GitIn(path, "commit", "-qm", "Handle the finished task")
	linked, err := app.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := linked.PRDraft(record)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Path != filepath.Join(repo.MiaDir(), "pr", "fix", "foo.md") || draft.Branch != "fix/foo" || draft.Base != "main" || draft.Saved {
		t.Fatalf("preview = %+v", draft)
	}
	if draft.Title != "Handle the finished task" || !strings.Contains(draft.Body, "- Handle the finished task") {
		t.Fatalf("preview misreports work or proof: %+v", draft)
	}
	if _, err := os.Stat(filepath.Join(repo.MiaDir(), "pr")); !os.IsNotExist(err) {
		t.Fatalf("show wrote a draft: %v", err)
	}
	other, err := linked.PRPath("fix-foo")
	if err != nil || other == draft.Path {
		t.Fatalf("branch paths collide: %q, %q, %v", other, draft.Path, err)
	}
}

func TestPRDraftSavesExactBodyAndOnlyPrintsSafelyQuotedCommands(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	path := repo.Worktree("monduli's files", "fix/foo")
	record := model.Record{Name: "monduli", Path: path}
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	title := "O'Brien $(touch " + marker + "); `echo injected`"
	body := "## Change\n\nKeep $shell and `code` exactly.\n"
	remove := true
	draft, err := a.DraftPR(record, app.PRDraftOptions{Title: &title, Body: &body, RemoveAfterMerge: &remove})
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, draft.Path); got != body {
		t.Fatalf("saved body = %q, want %q", got, body)
	}
	loaded, err := a.PRDraft(record)
	if err != nil || loaded.Title != title || loaded.Body != body || !loaded.Saved || !loaded.RemoveAfterMerge {
		t.Fatalf("saved draft = %+v, %v", loaded, err)
	}
	bin := t.TempDir()
	capture := filepath.Join(t.TempDir(), "argv")
	for _, name := range []string{"git", "gh", "mia"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$MIA_PR_CAPTURE\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, check := range []struct {
		command string
		args    []string
	}{
		{draft.PushCommand, []string{"-C", path, "push", "-u", "origin", "fix/foo"}},
		{draft.CreateCommand, []string{"pr", "create", "--base", "main", "--head", "fix/foo", "--title", title, "--body-file", draft.Path}},
		{draft.RemoveCommand, []string{"rm", "monduli"}},
	} {
		command := exec.Command("/bin/sh", "-c", check.command)
		command.Dir = t.TempDir()
		command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "MIA_PR_CAPTURE="+capture)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("command %q: %s, %v", check.command, out, err)
		}
		got := strings.Split(strings.TrimSuffix(read(t, capture), "\n"), "\n")
		if !reflect.DeepEqual(got, check.args) {
			t.Fatalf("command %q args = %q, want %q", check.command, got, check.args)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("title executed a shell command: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("draft removed its worktree: %v", err)
	}
	remove = false
	if _, err := a.DraftPR(record, app.PRDraftOptions{RemoveAfterMerge: &remove}); err != nil {
		t.Fatal(err)
	}
	loaded, err = a.PRDraft(record)
	if err != nil || loaded.RemoveAfterMerge || loaded.Title != title || loaded.Body != body {
		t.Fatalf("preference-only save changed the draft: %+v, %v", loaded, err)
	}
	loaded, err = a.DraftPR(record, app.PRDraftOptions{})
	if err != nil || loaded.Title != title || loaded.Body != body {
		t.Fatalf("default save changed the draft: %+v, %v", loaded, err)
	}
}

func TestPRDraftRejectsBranchPathTraversal(t *testing.T) {
	a := &app.App{MiaDir: t.TempDir()}
	for _, branch := range []string{"", "../escape", "/escape", "fix/../escape", "fix/./escape", "fix\\escape", "fix//escape"} {
		if path, err := a.PRPath(branch); err == nil {
			t.Errorf("accepted branch %q as %q", branch, path)
		}
	}
}

func TestPRDraftUsesTheLayersParent(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	repo.Git("checkout", "-qb", "lower")
	repo.Write("lower.txt", "lower\n")
	repo.Commit("Lower layer")
	path := repo.Worktree("monduli", "upper")
	if err := a.Stacks().Record("lower", "main"); err != nil {
		t.Fatal(err)
	}
	if err := a.Stacks().Record("upper", "lower"); err != nil {
		t.Fatal(err)
	}
	draft, err := a.PRDraft(model.Record{Name: "monduli", Path: path})
	if err != nil || draft.Base != "lower" || strings.Contains(draft.Body, "Lower layer") {
		t.Fatalf("layer preview = %+v, %v", draft, err)
	}
}

func TestPRDraftRejectsSavingAfterTheBranchChanges(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	path := repo.Worktree("monduli", "first")
	record := model.Record{Name: "monduli", Path: path}
	loaded, err := a.PRDraft(record)
	if err != nil {
		t.Fatal(err)
	}
	repo.GitIn(path, "checkout", "-qb", "second")
	if _, err := a.DraftPR(record, app.PRDraftOptions{Branch: &loaded.Branch}); err == nil || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("saved after a branch change: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo.MiaDir(), "pr")); !os.IsNotExist(err) {
		t.Fatalf("branch mismatch wrote a draft: %v", err)
	}
}

func TestPRDraftNeverSuggestsRemovingTheMainCheckout(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	repo.Git("checkout", "-qb", "feature")
	draft, err := a.PRDraft(model.Record{Name: "app", Path: repo.Root})
	if err != nil || draft.RemoveCommand != "" {
		t.Fatalf("main checkout removal = %q, %v", draft.RemoveCommand, err)
	}
}

func TestPRDraftFailedSavePreservesTheBodyAndMetadata(t *testing.T) {
	for _, readonly := range []string{"body", "metadata"} {
		t.Run(readonly, func(t *testing.T) {
			repo := gittest.New(t)
			a := open(t, repo)
			path := repo.Worktree("monduli", "feature")
			record := model.Record{Name: "monduli", Path: path}
			title, body := "Original title", "Original body\n"
			draft, err := a.DraftPR(record, app.PRDraftOptions{Title: &title, Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			metadataPath := strings.TrimSuffix(draft.Path, ".md") + ".json"
			metadata := read(t, metadataPath)
			readonlyPath := draft.Path
			if readonly == "metadata" {
				readonlyPath = metadataPath
			}
			if err := os.Chmod(readonlyPath, 0o444); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(readonlyPath, 0o644) })
			newTitle, newBody := "New title", "New body\n"
			if _, err := a.DraftPR(record, app.PRDraftOptions{Title: &newTitle, Body: &newBody}); err == nil {
				t.Fatal("save succeeded through a readonly file")
			}
			if read(t, draft.Path) != body || read(t, metadataPath) != metadata {
				t.Fatal("failed save changed the body or metadata")
			}
			loaded, err := a.PRDraft(record)
			if err != nil || loaded.Title != title || loaded.Body != body {
				t.Fatalf("failed save changed the loaded draft: %+v, %v", loaded, err)
			}
		})
	}
}

func TestPRDraftRefusesAMismatchedBodyAndMetadata(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	path := repo.Worktree("monduli", "feature")
	record := model.Record{Name: "monduli", Path: path}
	draft, err := a.DraftPR(record, app.PRDraftOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(draft.Path, []byte("different body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PRDraft(record); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("offered mismatched draft commands: %v", err)
	}
}

func TestPRDraftRefusesAnInterruptedFirstSave(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	path := repo.Worktree("monduli", "feature")
	record := model.Record{Name: "monduli", Path: path}
	preview, err := a.PRDraft(record)
	if err != nil || preview.Saved {
		t.Fatalf("absent body did not return a preview: %+v, %v", preview, err)
	}
	if err := os.MkdirAll(filepath.Dir(preview.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "Custom body from an interrupted first save\n"
	if err := os.WriteFile(preview.Path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	draft, err := a.PRDraft(record)
	if err == nil || !strings.Contains(err.Error(), "metadata") || !strings.Contains(err.Error(), "mia pr monduli draft") || draft.CreateCommand != "" {
		t.Fatalf("offered an orphan body or no recovery action: %+v, %v", draft, err)
	}
	if _, err := a.DraftPR(record, app.PRDraftOptions{}); err == nil {
		t.Fatal("save silently completed an orphan body with a generated title")
	}
	if read(t, preview.Path) != body {
		t.Fatal("load or save changed the interrupted body")
	}
	if _, err := os.Stat(strings.TrimSuffix(preview.Path, ".md") + ".json"); !os.IsNotExist(err) {
		t.Fatalf("load or save generated replacement metadata: %v", err)
	}
}
