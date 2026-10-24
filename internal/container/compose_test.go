package container

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeCommandsUseOneProjectAndResolveServiceID(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("MIA_COMPOSE_CALLS", log)
	binary := filepath.Join(dir, "podman")
	script := `printf '%s\n' "$*" >> "$MIA_COMPOSE_CALLS"
case "$*" in
*config*) printf '{"services":{"app":{"container_name":"custom","image":"example/app","network_mode":"bridge","ports":[{"target":80}]}}}\n' ;;
*ps\ -q*) printf 'abc123\n' ;;
esac
`
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	engine := Engine{Binary: binary}
	config, err := engine.ComposeConfig("mia-monduli", "/work/compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	service := config.Services["app"]
	if service.ContainerName != "custom" || service.Image != "example/app" || len(service.Ports) != 1 {
		t.Fatalf("compose config = %+v", service)
	}
	id, err := engine.ComposeID("mia-monduli", "/work/compose.yaml", "app")
	if err != nil || id != "abc123" {
		t.Fatalf("compose id = %q, %v", id, err)
	}
	for _, call := range []func() error{
		func() error { return engine.ComposeUp("mia-monduli", "/work/compose.yaml") },
		func() error { return engine.ComposeStop("mia-monduli", "/work/compose.yaml") },
		func() error { return engine.ComposeRemove("mia-monduli", "/work/compose.yaml") },
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"up -d --remove-orphans", "stop", "down --remove-orphans"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("missing %q: %s", want, calls)
		}
	}
}
