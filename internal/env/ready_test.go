package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/model"
)

func TestWaitBlocksForServicesAndPortsThenReportsWhatIsUnready(t *testing.T) {
	dir := t.TempDir()
	up := filepath.Join(dir, "up")
	t.Setenv("MIA_WAIT_UP", up)
	engine := filepath.Join(dir, "engine")
	script := `case "$1" in
inspect) echo running ;;
exec)
  case "$*" in
  *ss\ -ltnH*) [ "$(cat "$MIA_WAIT_UP")" = yes ] && printf 'LISTEN 0 511 0.0.0.0:3000 0.0.0.0:*\n'; exit 0 ;;
  *health*) [ "$(cat "$MIA_WAIT_UP")" = yes ] ;;
  *.pid*) [ "$(cat "$MIA_WAIT_UP")" = yes ] && echo yes || echo no ;;
  esac ;;
esac
`
	if err := os.WriteFile(engine, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := Manager{
		Engine:   container.Engine{Binary: engine},
		Settings: Settings{Ports: []int{3000}},
		Services: []Service{{ID: "web", Run: []string{"serve"}, Health: []string{"health"}, Autostart: true}},
	}
	record := model.Record{Name: "monduli", Path: dir}
	oldEvery, oldTimeout := waitEvery, waitTimeout
	waitEvery = 5 * time.Millisecond
	t.Cleanup(func() { waitEvery, waitTimeout = oldEvery, oldTimeout })

	waitTimeout = 200 * time.Millisecond
	timer := time.AfterFunc(20*time.Millisecond, func() { _ = os.WriteFile(up, []byte("yes"), 0o600) })
	defer timer.Stop()
	started := time.Now()
	if err := manager.Wait(record); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 15*time.Millisecond {
		t.Fatal("wait returned before the service and port became ready")
	}

	if err := os.WriteFile(up, []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitTimeout = 30 * time.Millisecond
	err := manager.Wait(record)
	if err == nil || !strings.Contains(err.Error(), "web") || !strings.Contains(err.Error(), "port 3000") {
		t.Fatalf("timeout did not name the unready service and port: %v", err)
	}
}
