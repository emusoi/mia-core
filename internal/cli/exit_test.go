package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestEnvExecPreservesTheChildExitCode(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "main", Path: repo.Root}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := `case "$1" in
inspect) echo running ;;
exec) exit 37 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if code := cmdEnv(a, []string{"exec", "main", "sh", "-c", "exit 37"}, false); code != 37 {
		t.Fatalf("env exec exited %d, want the child's 37", code)
	}
}
