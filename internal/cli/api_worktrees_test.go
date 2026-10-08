package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestWorktreesAPIFiltersExactNamesAndKeepsAnEmptyArray(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	path := repo.Worktree("longido", "feature")
	if err := a.Store.Put(model.Record{Name: "longido", Path: path}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"longido", "long"} {
		t.Run(name, func(t *testing.T) {
			output, err := os.CreateTemp(t.TempDir(), "output")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			previous := os.Stdout
			os.Stdout = output
			t.Cleanup(func() { os.Stdout = previous })
			if code := cmdAPI(a, []string{"worktrees", name}); code != exitOK {
				t.Fatalf("mia api worktrees exited %d", code)
			}
			if _, err := output.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			var listings []app.Listing
			if err := json.NewDecoder(output).Decode(&listings); err != nil {
				t.Fatal(err)
			}
			if name == "longido" {
				if len(listings) != 1 || listings[0].Name != name || listings[0].Path != path || listings[0].Branch != "feature" || !listings[0].Adopted {
					t.Fatalf("the exact name lost its listing: %+v", listings)
				}
			} else if listings == nil || len(listings) != 0 {
				t.Fatalf("a partial name must return []: %+v", listings)
			}
		})
	}
}
