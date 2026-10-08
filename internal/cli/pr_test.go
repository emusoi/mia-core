package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestPRCommandPreviewsAndSavesWithExplicitStdin(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	path := repo.Worktree("monduli", "land-task")
	t.Chdir(path)
	a, err := app.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "monduli", Path: path}); err != nil {
		t.Fatal(err)
	}
	input, err := os.CreateTemp(t.TempDir(), "body")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	body := "## Human-edited body\n\nKeep this exact.\n"
	if _, err := input.WriteString(body); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	stdin, stdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = input, output
	t.Cleanup(func() { os.Stdin, os.Stdout = stdin, stdout })
	for _, check := range []struct {
		args  []string
		saved bool
		body  string
	}{
		{[]string{"pr", "monduli", "show", "--json"}, false, "## Changes"},
		{[]string{"pr", "monduli", "draft", "--json"}, true, "## Changes"},
		{[]string{"pr", "monduli", "draft", "--branch", "land-task", "--title", "Land this task", "--body-stdin", "--remove-after-merge", "true", "--json"}, true, body},
		{[]string{"pr", "show", "monduli", "--json"}, true, body},
	} {
		if err := output.Truncate(0); err != nil {
			t.Fatal(err)
		}
		if _, err := output.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		if code := Run(check.args); code != exitOK {
			t.Fatalf("mia %q exited %d", check.args, code)
		}
		if _, err := output.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		var draft app.PRDraft
		if err := json.NewDecoder(output).Decode(&draft); err != nil {
			t.Fatal(err)
		}
		if draft.Saved != check.saved || !strings.Contains(draft.Body, check.body) {
			t.Fatalf("mia %q returned %+v", check.args, draft)
		}
	}
}
