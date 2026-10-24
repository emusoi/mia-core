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

func TestEnvHostQueryHasJSON(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo.Root)
	if err := a.Store.Put(model.Record{Name: "main", Path: repo.Root}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := os.CreateTemp(t.TempDir(), "host-json")
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = out
	code := cmdEnv(a, []string{"host", "--json"}, false)
	os.Stdout = stdout
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if code != exitOK {
		t.Fatalf("env host exited %d", code)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	var got hostReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Worktree != "main" || got.Machine != "local" || got.Staging != "" {
		t.Fatalf("host report = %+v", got)
	}
}
