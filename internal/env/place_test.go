package env_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
)

func TestAStagingIsNeverTheWorktree(t *testing.T) {
	worktree := "/Users/somebody/src/app.monduli"
	staging := env.Staging("/home/somebody", "app", "monduli")

	if staging == worktree {
		t.Fatal("the staging path is the worktree itself")
	}
	if !strings.HasPrefix(staging, "/home/somebody/.mia/") {
		t.Errorf("a staging path landed outside mia's own directory: %s", staging)
	}
	if filepath.Base(staging) != "monduli" {
		t.Errorf("the staging path is not named for the worktree: %s", staging)
	}
}

func TestMovingRecomputesTheStaging(t *testing.T) {
	first := env.Staging("/home/somebody", "app", "monduli")
	second := env.Staging("/var/home/core", "app", "monduli")
	if first == second {
		t.Fatalf("the same staging path came back for two different machines: %s", first)
	}
}

func TestTwoWorktreesNeverShareStaging(t *testing.T) {
	if env.Staging("/home/x", "app", "monduli") == env.Staging("/home/x", "app", "longido") {
		t.Fatal("two worktrees were given the same staging path")
	}
	if env.Staging("/home/x", "app", "monduli") == env.Staging("/home/x", "other", "monduli") {
		t.Fatal("two repositories were given the same staging path")
	}
}

func TestComingHomeClearsThePlacement(t *testing.T) {
	away, err := model.OnRuntime("fedora", env.Staging("/home/x", "app", "monduli"))
	if err != nil {
		t.Fatal(err)
	}
	if away.IsLocal() {
		t.Fatal("a placement on a box reported itself local")
	}

	home := model.Local()
	if !home.IsLocal() {
		t.Fatal("a local placement did not report itself local")
	}
	if staging, ok := home.Staging(); ok {
		t.Errorf("coming home kept a staging path: %s", staging)
	}
}

func TestALocalEnvironmentMountsTheWorktree(t *testing.T) {
	manager := env.Manager{Repo: "/Users/somebody/src/app"}
	record := model.Record{Path: "/Users/somebody/src/app.monduli", Name: "monduli"}

	where, err := manager.Where(record)
	if err != nil {
		t.Fatal(err)
	}
	if where.Remote() {
		t.Fatal("a record with no placement resolved to a box")
	}
	if where.Dir != record.Path {
		t.Errorf("a local environment mounts %s, not its worktree", where.Dir)
	}
	if where.Mirror != nil {
		t.Error("a local environment was given a mirror to keep in step with itself")
	}
}

func TestSymlinksAreFollowedOnceAndNotOutOfHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	real := filepath.Join(home, "checkout", "skills")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "craft.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "skills")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	got, err := env.ResolveOneHop("~/skills")
	if err != nil {
		t.Fatal(err)
	}
	if got != real {
		t.Errorf("a symlinked config directory resolved to %s, not what it points at", got)
	}

	outside := t.TempDir()
	escape := filepath.Join(home, "elsewhere")
	if err := os.Symlink(outside, escape); err != nil {
		t.Fatal(err)
	}
	if _, err := env.ResolveOneHop("~/elsewhere"); err == nil {
		t.Error("a link pointing outside your home was followed")
	}
}
