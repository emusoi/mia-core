package mirror

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPatternsExcludeSecretsUnlessExplicitlyAllowed(t *testing.T) {
	patterns := Patterns([]string{"*.local"}, false)
	for _, rel := range []string{".env", "nested/id_rsa", "config/key.pem", "settings.local"} {
		if !ignored(rel, patterns) {
			t.Errorf("%s was not ignored", rel)
		}
	}
	if ignored("src/app.go", patterns) {
		t.Error("source was ignored")
	}
	allowed := Patterns(nil, true)
	if ignored(".env", allowed) {
		t.Error("sync_secrets did not allow .env")
	}
	if !ignored(".git/config", allowed) {
		t.Error("sync_secrets also enabled Git metadata")
	}
}

func TestPreviewShowsExcludedAndAllowedSecretPaths(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".env", ".env.example", "nested/id_rsa", "nested/file.txt", "node_modules/pkg/.env"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := BuildPreview(root, Patterns(nil, false))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", ".env.example", "nested/id_rsa"} {
		if !slices.Contains(preview.Excluded, name) {
			t.Errorf("%s not reported excluded: %+v", name, preview)
		}
	}
	if len(preview.Secrets) != 0 {
		t.Fatalf("secrets would sync with default rules: %+v", preview.Secrets)
	}

	preview, err = BuildPreview(root, Patterns(nil, true))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", ".env.example", "nested/id_rsa"} {
		if !slices.Contains(preview.Secrets, name) {
			t.Errorf("%s not reported allowed: %+v", name, preview)
		}
	}
	if slices.Contains(preview.Secrets, "node_modules/pkg/.env") {
		t.Fatal("preview descended into an ignored dependency directory")
	}
}

func TestSessionParseKeepsConflictPathsAndConfiguration(t *testing.T) {
	data := []byte(`[{"name":"mia-monduli","status":"watching","alpha":{"protocol":"local","path":"/local","connected":true},"beta":{"protocol":"ssh","host":"box","path":"/remote","connected":true},"labels":{"dev.mia.sync":"abc"},"conflicts":[{"path":"a.go"},{"path":"b.go"}]}]`)
	session, found, err := parse(data, "mia-monduli")
	if err != nil || !found {
		t.Fatalf("parse: found=%t err=%v", found, err)
	}
	if session.Conflicts != 2 || len(session.ConflictPaths) != 2 || session.Configuration != "abc" {
		t.Fatalf("session lost conflict or label facts: %+v", session)
	}
	if session.Healthy() {
		t.Fatal("a session with conflicts was healthy")
	}
	if !strings.Contains(session.Describe(), "mia env sync monduli conflicts") {
		t.Fatalf("conflict guidance did not use mia: %s", session.Describe())
	}
}

func TestEnsureRejectsOldRulesAndRestartAppliesNewRules(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present")
	log := filepath.Join(dir, "calls")
	t.Setenv("MIA_MIRROR_PRESENT", present)
	t.Setenv("MIA_MIRROR_LOG", log)
	binary := filepath.Join(dir, "mutagen")
	script := `printf '%s\n' "$*" >> "$MIA_MIRROR_LOG"
if [ "$1 $2" = "sync list" ]; then
  if [ -f "$MIA_MIRROR_PRESENT" ]; then
    printf '[{"name":"mia-monduli","status":"watching","alpha":{"protocol":"local","path":"/local","connected":true},"beta":{"protocol":"ssh","host":"box","path":"/remote","connected":true},"labels":{"dev.mia.sync":"old"}}]\n'
  else
    printf '[]\n'
  fi
  exit 0
fi
if [ "$1 $2" = "sync terminate" ]; then rm -f "$MIA_MIRROR_PRESENT"; exit 0; fi
if [ "$1 $2" = "sync create" ]; then touch "$MIA_MIRROR_PRESENT"; exit 0; fi
exit 1
`
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(present, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	mirror := Mirror{Binary: binary}
	if err := mirror.Ensure("monduli", "/local", "box", "/remote", Patterns(nil, false)); !errors.Is(err, ErrConfigurationChanged) {
		t.Fatalf("old rules were accepted: %v", err)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "sync create") {
		t.Fatalf("old session was replaced without an explicit restart: %s", calls)
	}
	if err := mirror.Restart("monduli", "/local", "box", "/remote", Patterns(nil, false)); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sync terminate mia-monduli", "sync create", "--label=dev.mia.sync=", "--ignore=.env"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("restart did not include %q: %s", want, calls)
		}
	}
}
