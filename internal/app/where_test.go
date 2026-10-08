package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

func TestASessionFollowsItsEnvironment(t *testing.T) {
	repo := gittest.New(t)
	gittest.Isolate(t)
	config := os.Getenv("XDG_CONFIG_HOME")

	machines := []runtime.Machine{{Name: "fedora", SSH: "fedora.invalid", Engine: "/usr/bin/podman"}}
	data, err := json.Marshal(machines)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config, "mia"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "mia", "runtimes.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Config.Dotfiles.Shell = "zsh"

	here := model.Record{Path: repo.Root, Name: "monduli"}
	host, err := a.SessionHostOf(here)
	if err != nil {
		t.Fatal(err)
	}
	if host.SSH != "" {
		t.Errorf("a local environment's session was placed on %q", host.SSH)
	}
	if host.Shell != "zsh" {
		t.Errorf("local session shell = %q, want zsh", host.Shell)
	}

	placement, err := model.OnRuntime("fedora", "/home/x/.mia/app/longido")
	if err != nil {
		t.Fatal(err)
	}
	away := model.Record{Path: repo.Root, Name: "longido", Env: &model.Environment{Placement: placement}}
	host, err = a.SessionHostOf(away)
	if err != nil {
		t.Fatal(err)
	}
	if host.SSH != "fedora.invalid" {
		t.Errorf("an environment on a box got the session host %q — `mia shell` would open an empty local one", host.SSH)
	}
	if host.Shell != "zsh" {
		t.Errorf("remote session shell = %q, want zsh", host.Shell)
	}
}

func TestFindingTheSessionHostNeverReachesForTheBox(t *testing.T) {
	repo := gittest.New(t)
	gittest.Isolate(t)
	config := os.Getenv("XDG_CONFIG_HOME")

	machines := []runtime.Machine{{Name: "fedora", SSH: "no-such-host.invalid", Engine: "/usr/bin/podman"}}
	data, _ := json.Marshal(machines)
	if err := os.MkdirAll(filepath.Join(config, "mia"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "mia", "runtimes.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	placement, err := model.OnRuntime("fedora", "/home/x/.mia/app/longido")
	if err != nil {
		t.Fatal(err)
	}
	record := model.Record{Path: repo.Root, Name: "longido", Env: &model.Environment{Placement: placement}}

	if _, err := a.SessionHostOf(record); err != nil {
		t.Fatalf("finding the session host reached for the machine: %v", err)
	}
}
