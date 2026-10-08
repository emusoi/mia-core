package env

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/model"
)

func TestServiceScriptsKeepLiteralIDs(t *testing.T) {
	old := healthEvery
	healthEvery = 1
	t.Cleanup(func() { healthEvery = old })
	for _, restart := range []string{RestartNever, RestartOnFailure, RestartUnhealthy} {
		t.Run(restart, func(t *testing.T) {
			dir := t.TempDir()
			id := "web $(touch injected) $literal 'quoted'"
			service := Service{ID: id, Run: []string{"printf", "%s\\n", id}, Restart: restart, Health: []string{"true"}}
			command := exec.Command("sh", "-c", strings.ReplaceAll(service.Script(), runtimeDir, dir))
			command.Dir = dir
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("service shell failed: %s, %v", output, err)
			}
			if log := logOf(dir, id); log != id+"\n" {
				t.Fatalf("service did not write its literal log: %q", log)
			}
			pid, err := os.ReadFile(filepath.Join(dir, id+".pid"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := strconv.Atoi(strings.TrimSpace(string(pid))); err != nil {
				t.Fatalf("service did not write its literal PID file: %q", pid)
			}
			if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
				t.Fatalf("service ID executed a shell substitution: %v", err)
			}
		})
	}
}

func TestServiceCommandsKeepLiteralIDs(t *testing.T) {
	dir := t.TempDir()
	engine := filepath.Join(dir, "engine")
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
inspect) printf 'running\n' ;;
exec)
  while [ "$1" != sh ]; do shift; done
  shift; shift
  script=$(printf '%%s' "$1" | sed %s)
  cd %s
  exec sh -c "$script" ;;
*) exit 99 ;;
esac
`, shellJoin([]string{"s|" + runtimeDir + "|" + dir + "|g"}), shellJoin([]string{dir}))
	if err := os.WriteFile(engine, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Engine: container.Engine{Binary: engine}}
	record := model.Record{Name: "monduli", Path: dir}
	id := "web $(touch injected) $literal 'quoted'"
	output, err := manager.Logs(record, id, 40)
	if err != nil || output != "("+id+" has printed nothing)" {
		t.Fatalf("empty logs did not print the literal ID: %q, %v", output, err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".log"), []byte("literal log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err = manager.Logs(record, id, 40)
	if err != nil || output != "literal log" {
		t.Fatalf("logs did not read the literal path: %q, %v", output, err)
	}
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- child.Wait() }()
	t.Cleanup(func() { child.Process.Kill() })
	pid := filepath.Join(dir, id+".pid")
	if err := os.WriteFile(pid, []byte(strconv.Itoa(child.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	running, err := manager.serviceRunning(Location{Engine: manager.Engine}, "mia-monduli", id)
	if err != nil || !running {
		t.Fatalf("status did not read the literal PID path: %t, %v", running, err)
	}
	if err := manager.StopService(record, id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not kill the literal service PID")
	}
	if _, err := os.Stat(pid); !os.IsNotExist(err) {
		t.Fatalf("stop did not remove the literal PID path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
		t.Fatalf("service ID executed a shell substitution: %v", err)
	}
}
