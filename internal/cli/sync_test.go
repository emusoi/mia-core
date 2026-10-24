package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/mirror"
)

func TestSyncPreviewReportsSecretPolicyWithoutReadingContents(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "excluded", true: "allowed"}[allow], func(t *testing.T) {
			gittest.Isolate(t)
			repo := gittest.New(t)
			repo.Write(".env", "SECRET=do-not-print\n")
			t.Chdir(repo.Root)
			a, err := app.Open(repo.Root)
			if err != nil {
				t.Fatal(err)
			}
			a.Config.Env.SyncSecrets = allow
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

			out, err := os.CreateTemp(t.TempDir(), "sync-preview")
			if err != nil {
				t.Fatal(err)
			}
			stdout := os.Stdout
			os.Stdout = out
			code := cmdEnv(a, []string{"sync", "preview", "--json"}, false)
			os.Stdout = stdout
			if err := out.Close(); err != nil {
				t.Fatal(err)
			}
			if code != exitOK {
				t.Fatalf("sync preview exited %d", code)
			}
			data, err := os.ReadFile(out.Name())
			if err != nil {
				t.Fatal(err)
			}
			var preview mirror.Preview
			if err := json.Unmarshal(data, &preview); err != nil {
				t.Fatalf("preview was not JSON: %v\n%s", err, data)
			}
			if slices.Contains(preview.Secrets, ".env") != allow || slices.Contains(preview.Excluded, ".env") == allow {
				t.Fatalf("allow=%t preview = %+v", allow, preview)
			}
			if strings.Contains(string(data), "do-not-print") {
				t.Fatal("preview exposed secret contents")
			}
		})
	}
}
