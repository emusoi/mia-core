package env_test

import (
	"os"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
)

const composeFixture = `{"services":{"app":{"image":"example/app","container_name":"custom-app"},"db":{"image":"postgres"}}}`

func composeProbe(t *testing.T, config string) (env.Manager, model.Record, string) {
	t.Helper()
	return environmentProbe(t, `case "$1" in
info) exit 0 ;;
inspect)
  case "$3" in *State.Status*) cat "$MIA_COMPOSE_STATE" 2>/dev/null ;; esac ;;
compose)
  case "$*" in
  *" config "*) printf '%s\n' '`+config+`' ;;
  *" ps -q "*) [ -f "$MIA_COMPOSE_UP" ] && echo abc123; exit 0 ;;
  *" up "*) touch "$MIA_COMPOSE_UP"; echo running > "$MIA_COMPOSE_STATE" ;;
  *" stop "*) echo exited > "$MIA_COMPOSE_STATE" ;;
  *" down "*) rm -f "$MIA_COMPOSE_UP"; : > "$MIA_COMPOSE_STATE" ;;
  esac ;;
exec) exit 0 ;;
esac
`)
}

func TestComposeLifecycleUsesProjectAndActualContainerID(t *testing.T) {
	manager, record, log := composeProbe(t, composeFixture)
	manager.Settings.Compose = env.ComposeSettings{File: "compose.yaml", Service: "app"}
	t.Setenv("MIA_COMPOSE_STATE", record.Path+"/state")
	t.Setenv("MIA_COMPOSE_UP", record.Path+"/up")
	if err := manager.CheckCompose(record); err != nil {
		t.Fatal(err)
	}

	environment, err := manager.Up(record, os.Stderr, false)
	if err != nil {
		calls, _ := os.ReadFile(log)
		t.Fatalf("up: %v\ncalls:\n%s", err, calls)
	}
	if environment.Container != "abc123" || environment.Image.Ref != "compose.yaml" || environment.Image.Source != env.FromCompose {
		t.Fatalf("compose environment = %+v", environment)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"compose -p mia-monduli", "config --format json", "up -d --remove-orphans", "ps -q app"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("missing %q: %s", want, calls)
		}
	}
	if strings.Contains(string(calls), "run -d --name") {
		t.Fatalf("compose used the single-container path: %s", calls)
	}
	if err := manager.Down(record); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(record); err != nil {
		t.Fatal(err)
	}
	calls, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "down --remove-orphans") || strings.Contains(string(calls), "volume rm") {
		t.Fatalf("compose removal was wrong: %s", calls)
	}
}

func TestComposeRejectsAnyPublishedService(t *testing.T) {
	manager, record, log := composeProbe(t, `{"services":{"app":{"image":"example/app"},"db":{"image":"postgres","ports":[{"target":5432,"published":"5432"}]}}}`)
	manager.Settings.Compose = env.ComposeSettings{File: "compose.yaml", Service: "app"}
	t.Setenv("MIA_COMPOSE_STATE", record.Path+"/state")
	t.Setenv("MIA_COMPOSE_UP", record.Path+"/up")
	if _, err := manager.Up(record, os.Stderr, false); err == nil || !strings.Contains(err.Error(), "db publishes host ports") {
		t.Fatalf("published dependency was accepted: %v", err)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), " up -d ") {
		t.Fatalf("rejected compose project was started: %s", calls)
	}
}

func TestComposeContainerNameUsesSavedID(t *testing.T) {
	settings := env.Settings{Compose: env.ComposeSettings{File: "compose.yaml", Service: "app"}}
	record := model.Record{Name: "monduli"}
	if got := env.ContainerNameFor(record, settings); got != "mia-monduli-app-1" {
		t.Fatalf("default compose name = %q", got)
	}
	record.Env = &model.Environment{Container: "abc123"}
	if got := env.ContainerNameFor(record, settings); got != "abc123" {
		t.Fatalf("saved compose ID = %q", got)
	}
}
