package env_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
)

func TestComposeWithRealDocker(t *testing.T) {
	if os.Getenv("MIA_TEST_COMPOSE") == "" {
		t.Skip("set MIA_TEST_COMPOSE=1 to use Docker and the cached redis:7-alpine image")
	}
	if exec.Command("docker", "image", "inspect", "redis:7-alpine").Run() != nil {
		t.Skip("redis:7-alpine is not cached")
	}
	engine := container.Engine{Binary: "docker"}
	if err := engine.Ready(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	body := "services:\n  app:\n    image: redis:7-alpine\n    expose: [\"6379\"]\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	name := "probe-" + strconv.Itoa(os.Getpid())
	record := model.Record{Name: name, Path: dir}
	manager := env.Manager{Engine: engine, Settings: env.Settings{Compose: env.ComposeSettings{File: "compose.yaml", Service: "app"}}}
	t.Cleanup(func() { _ = manager.Remove(record) })

	environment, err := manager.Up(record, os.Stderr, false)
	if err != nil {
		t.Fatal(err)
	}
	if environment.Container == "" || environment.State != "running" {
		t.Fatalf("compose environment = %+v", environment)
	}
	out, err := engine.Exec(environment.Container, "redis-cli", "ping")
	if err != nil || strings.TrimSpace(out) != "PONG" {
		t.Fatalf("redis ping = %q, %v", out, err)
	}
	if err := manager.Down(record); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(record); err != nil {
		t.Fatal(err)
	}
}
