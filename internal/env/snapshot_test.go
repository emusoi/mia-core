package env_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

func TestSnapshotSharesProbesAndPreservesIndependentFactsAndErrors(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		calls int
	}{
		{"running", 7}, {"stopped", 1}, {"absent", 2}, {"unavailable", 2},
		{"ports-failed", 7}, {"process-failed", 6}, {"unhealthy", 7}, {"interrupted", 7},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Setenv("MIA_SNAPSHOT_SCENARIO", scenario.name)
			manager, record, log := environmentProbe(t, `case "$1" in
inspect)
  case "$MIA_SNAPSHOT_SCENARIO" in
  stopped) echo exited ;;
  absent|unavailable) exit 1 ;;
  *) echo running ;;
  esac ;;
info)
  if [ "$MIA_SNAPSHOT_SCENARIO" = unavailable ]; then echo 'engine unavailable' >&2; exit 2; fi ;;
exec)
  case "$*" in
  *ss\ -ltnH*)
    if [ "$MIA_SNAPSHOT_SCENARIO" = ports-failed ]; then echo 'ports unavailable' >&2; exit 2; fi
    printf 'LISTEN 0 511 0.0.0.0:3000 0.0.0.0:*\nLISTEN 0 511 0.0.0.0:9090 0.0.0.0:*\n' ;;
  *zeta.pid*)
    if [ "$MIA_SNAPSHOT_SCENARIO" = process-failed ]; then echo 'process unavailable' >&2; exit 2; fi
    echo yes ;;
  *.pid*) echo yes ;;
  *health-zeta*)
    case "$MIA_SNAPSHOT_SCENARIO" in
    unhealthy) exit 3 ;;
    interrupted) echo 'health interrupted' >&2; kill -TERM $$ ;;
    esac ;;
  esac ;;
*) exit 99 ;;
esac
`)
			manager.Settings = env.Settings{Image: "fixture/image:1", Ports: []int{3000, 4000}, Page: 3000, HostPorts: []int{9090}}
			manager.Services = []env.Service{
				{ID: "zeta", Run: []string{"serve app", "--value=$10; 'quoted'"}, Health: []string{"health-zeta"}},
				{ID: "alpha", Run: []string{"serve", "4000"}, Health: []string{"health-alpha"}},
			}
			got := manager.Snapshot(record)
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if count := strings.Count(string(calls), "\n"); count != scenario.calls || strings.Count(string(calls), "inspect ") != 1 || strings.Count(string(calls), "ss -ltnH") > 1 {
				t.Fatalf("snapshot repeated a probe: got %d, want %d calls: %s", count, scenario.calls, calls)
			}
			if got.State == "running" && got.PortsError == "" && (!slices.Equal(got.Listening, []int{3000}) || !slices.Equal(got.Ports.Listening, []int{3000})) {
				t.Fatalf("forwarded ports were not excluded: %+v", got)
			}
			if scenario.name == "process-failed" && strings.Contains(string(calls), "health-zeta") {
				t.Fatalf("queried health after a process failure: %s", calls)
			}
			ports, portsErr := manager.PortsOf(record)
			services, servicesErr := manager.ServiceStatus(record)
			want := env.Snapshot{Environment: manager.Describe(record), Ports: ports, Services: services}
			if portsErr != nil {
				want.PortsError = portsErr.Error()
			}
			if servicesErr != nil {
				want.ServicesError = servicesErr.Error()
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("snapshot changed independent results: got %+v, want %+v", got, want)
			}
		})
	}
}

func TestSnapshotRefreshesStatePathAndPlacementOnEveryRead(t *testing.T) {
	manager, record, _ := environmentProbe(t, `case "$1" in
inspect) cat "$MIA_SNAPSHOT_STATE" ;;
exec) exit 0 ;;
*) exit 99 ;;
esac
`)
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("MIA_SNAPSHOT_STATE", state)
	if err := os.WriteFile(state, []byte("running\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if snapshot := manager.Snapshot(record); snapshot.State != "running" || snapshot.Machine != "local" || snapshot.Worktree != record.Path {
		t.Fatalf("first snapshot lost identity: %+v", snapshot)
	}
	record.Path = t.TempDir()
	if err := os.WriteFile(state, []byte("exited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if snapshot := manager.Snapshot(record); snapshot.State != "exited" || snapshot.Machine != "local" || snapshot.Worktree != record.Path {
		t.Fatalf("second snapshot reused an earlier read: %+v", snapshot)
	}
	placement, err := model.OnRuntime("missing", "/fixture/monduli")
	if err != nil {
		t.Fatal(err)
	}
	record.Env = &model.Environment{Placement: placement}
	manager.Runtimes = runtime.Store{Path: filepath.Join(t.TempDir(), "runtimes.json")}
	manager.Settings = env.Settings{Image: "next/image:2", Ports: []int{8000}, Page: 8000}
	snapshot := manager.Snapshot(record)
	if snapshot.Machine != "missing" || snapshot.Placement != placement || snapshot.Worktree != record.Path || snapshot.Image.Ref != "next/image:2" || !slices.Equal(snapshot.Ports.Declared, []int{8000}) || snapshot.Ports.Page != 8000 {
		t.Fatalf("failed runtime lookup lost fresh configured facts: %+v", snapshot)
	}
	if snapshot.Error == "" || snapshot.Mirror != snapshot.Error || snapshot.PortsError != snapshot.Error || snapshot.ServicesError != snapshot.Error || snapshot.Services != nil || snapshot.State != "" {
		t.Fatalf("failed runtime lookup lost error attribution: %+v", snapshot)
	}
}

func TestSnapshotKeepsStandaloneReadersNarrow(t *testing.T) {
	manager, record, log := environmentProbe(t, `case "$1" in
inspect) echo running ;;
exec)
  case "$*" in
  *ss\ -ltnH*) printf 'LISTEN 0 511 0.0.0.0:3000 0.0.0.0:*\n' ;;
  *.pid*) echo yes ;;
  esac ;;
*) exit 99 ;;
esac
`)
	manager.Services = []env.Service{{ID: "web", Health: []string{"health"}}}
	for _, reader := range []struct {
		name  string
		read  func()
		calls int
	}{
		{"description", func() { manager.Describe(record) }, 2},
		{"ports", func() { manager.PortsOf(record) }, 2},
		{"services", func() { manager.ServiceStatus(record) }, 3},
	} {
		t.Run(reader.name, func(t *testing.T) {
			if err := os.WriteFile(log, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			reader.read()
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if count := strings.Count(string(calls), "\n"); count != reader.calls {
				t.Fatalf("%s widened to %d probes, want %d: %s", reader.name, count, reader.calls, calls)
			}
		})
	}
}

func TestRemoteSnapshotRetainsPlacementMirrorAndStaging(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls")
	t.Setenv("MIA_SNAPSHOT_REMOTE_CALLS", log)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for name, body := range map[string]string{
		"ssh": `printf 'ssh %s\n' "$*" >> "$MIA_SNAPSHOT_REMOTE_CALLS"
case "$*" in
*inspect*) echo running ;;
*ss\ -ltnH*) printf 'LISTEN 0 511 0.0.0.0:3000 0.0.0.0:*\n' ;;
*.pid*) echo yes ;;
*) echo /srv/person ;;
esac
`,
		"mutagen": `printf 'mutagen %s\n' "$*" >> "$MIA_SNAPSHOT_REMOTE_CALLS"
printf '[{"name":"mia-monduli","status":"watching","alpha":{"connected":true},"beta":{"connected":true}}]\n'
`,
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	placement, err := model.OnRuntime("fixture", "/original/staging")
	if err != nil {
		t.Fatal(err)
	}
	record := model.Record{Name: "monduli", Path: dir, Env: &model.Environment{Placement: placement}}
	manager := env.Manager{Repo: "/fixture/repo", Runtimes: runtime.Store{Path: filepath.Join(dir, "runtimes.json")}, Settings: env.Settings{Image: "fixture/image:1"}, Services: []env.Service{{ID: "web", Health: []string{"health"}}}}
	if err := manager.Runtimes.Add(runtime.Machine{Name: "fixture", SSH: "fixture-" + filepath.Base(dir), Engine: "podman"}); err != nil {
		t.Fatal(err)
	}
	snapshot := manager.Snapshot(record)
	if snapshot.Machine != "fixture" || snapshot.Placement != placement || snapshot.Mirror != "watching" || snapshot.State != "running" || snapshot.Worktree != dir || snapshot.Error != "" || len(snapshot.Services) != 1 || !snapshot.Services[0].Running {
		t.Fatalf("remote snapshot lost facts: %+v", snapshot)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "inspect") != 1 || strings.Count(string(calls), "ss -ltnH") != 1 || strings.Count(string(calls), "mutagen sync list") != 1 || !strings.Contains(string(calls), "/srv/person/.mia/repo/monduli") {
		t.Fatalf("remote snapshot repeated probes or used the local path: %s", calls)
	}
}
