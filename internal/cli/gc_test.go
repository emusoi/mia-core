package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestGCMergedRejectsAnIdleDaysFilter(t *testing.T) {
	gittest.Isolate(t)
	a, err := app.Open(gittest.New(t).Root)
	if err != nil {
		t.Fatal(err)
	}
	if code := cmdGC(a, []string{"--merged", "--days", "1"}, false); code != exitUsage {
		t.Fatalf("gc --merged --days exited %d, want %d", code, exitUsage)
	}
}

func TestGCMergedApplyFailsWithoutForgettingCleanupRetry(t *testing.T) {
	gittest.Isolate(t)
	a, err := app.Open(gittest.New(t).Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("merged-cleanup")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := `case "$1" in
inspect) echo running ;;
rm) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if code := cmdGC(a, []string{"--merged", "--apply"}, false); code != exitFailed {
		t.Fatalf("failed cleanup exited %d, want %d", code, exitFailed)
	}
	if _, err := a.RecordOrHere(record.Name); err != nil {
		t.Fatalf("cleanup retry record was forgotten: %v", err)
	}
}
