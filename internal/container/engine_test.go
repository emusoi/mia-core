package container

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerUsesDockerNativeInfoAndImageInspection(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("MIA_DOCKER_CALLS", log)
	binary := filepath.Join(dir, "docker")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MIA_DOCKER_CALLS\"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	engine := Engine{Binary: binary}
	if err := engine.Ready(); err != nil {
		t.Fatal(err)
	}
	if !engine.HasImage("example/image:1") {
		t.Fatal("image inspection failed")
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"info --format {{.Architecture}}", "image inspect example/image:1"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("missing %q: %s", want, calls)
		}
	}
	if strings.Contains(string(calls), "image exists") {
		t.Fatalf("used the Podman-only image check: %s", calls)
	}
}
