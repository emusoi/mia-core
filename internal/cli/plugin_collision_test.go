package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/plugin"
)

func TestDuplicatePluginVerbIsRefused(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ran := filepath.Join(t.TempDir(), "ran")
	for _, name := range []string{"one", "two"} {
		body := `case "$1" in
manifest) echo '{"protocol":1,"verbs":[{"name":"hello"}]}' ;;
hello) touch ` + ran + ` ;;
esac
`
		if err := os.WriteFile(filepath.Join(dir, "mia-"+name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a.Config.Plugins = []string{"one", "two"}
	if code, owned := runPluginVerb(a, "hello", nil); !owned || code != exitFailed {
		t.Fatalf("duplicate verb: owned=%t code=%d", owned, code)
	}
	if _, err := os.Stat(ran); !os.IsNotExist(err) {
		t.Fatal("a plugin ran despite the collision")
	}
}

func TestCoreVerbShadowingIsReported(t *testing.T) {
	plugins := []plugin.Plugin{{
		Name: "custom", Enabled: true,
		Manifest: plugin.Manifest{Verbs: []plugin.Verb{{Name: "new"}}},
	}}
	conflicts := verbConflicts(plugins)
	if len(conflicts) != 1 || conflicts[0].Verb != "new" || len(conflicts[0].Plugins) != 2 {
		t.Fatalf("core shadowing was not reported: %+v", conflicts)
	}
}
