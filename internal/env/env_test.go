package env_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

func TestAnyProjectGetsAnImage(t *testing.T) {
	for _, c := range []struct {
		what   string
		files  map[string]string
		config string
		want   env.Source
	}{
		{"nothing at all", nil, "", env.FromDefault},
		{"a go project", map[string]string{"go.mod": "module x\n"}, "", env.FromStack},
		{"a node project", map[string]string{"package-lock.json": "{}"}, "", env.FromStack},
		{"a python project", map[string]string{"pyproject.toml": "[project]\n"}, "", env.FromStack},
		{
			"a repository that describes itself",
			map[string]string{".devcontainer/devcontainer.json": `{"image": "ghcr.io/acme/dev:1"}`},
			"", env.FromDevcontainer,
		},
		{
			"a project that names its own",
			map[string]string{"go.mod": "module x\n"},
			"ghcr.io/acme/chosen:2", env.FromConfig,
		},
	} {
		dir := t.TempDir()
		for name, content := range c.files {
			path := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		image := env.DiscoverImage(dir, c.config)
		if image.Source != c.want {
			t.Errorf("%s: image came from %q, want %q (%s)", c.what, image.Source, c.want, image.Ref)
		}
		if image.Ref == "" {
			t.Errorf("%s: no image at all", c.what)
		}
	}
}

func TestConfigBeatsDevcontainerBeatsGuess(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".devcontainer", "devcontainer.json"),
		[]byte(`{"image":"ghcr.io/acme/described:1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := env.DiscoverImage(dir, ""); got.Ref != "ghcr.io/acme/described:1" {
		t.Errorf("devcontainer.json lost to the stack guess: %+v", got)
	}
	if got := env.DiscoverImage(dir, "ghcr.io/acme/chosen:2"); got.Ref != "ghcr.io/acme/chosen:2" {
		t.Errorf("the project's own config lost: %+v", got)
	}
}

func TestADevcontainerWithCommentsIsStillRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0o755); err != nil {
		t.Fatal(err)
	}
	jsonc := `{
  // the base everybody on this project uses
  "image": "ghcr.io/acme/dev:3",
  "features": {},
}`
	if err := os.WriteFile(filepath.Join(dir, ".devcontainer", "devcontainer.json"), []byte(jsonc), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := env.DiscoverImage(dir, ""); got.Ref != "ghcr.io/acme/dev:3" {
		t.Errorf("a devcontainer with comments and a trailing comma was not read: %+v", got)
	}
}

func TestListeningPortsSurviveAnImageWithNoTools(t *testing.T) {
	procNetTCP := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid
   0: 00000000:1F40 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000
   1: 00000000:1435 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000
   2: 0100007F:1F40 0100007F:C1B4 01 00000000:00000000 00:00000000 00000000  1000`
	got := container.ParseListening(procNetTCP)
	if len(got) != 2 || got[0] != 5173 || got[1] != 8000 {
		t.Errorf("/proc/net/tcp gave %v, want [5173 8000] — 0x1435 and 0x1F40, listeners only", got)
	}

	ss := `LISTEN 0 511 0.0.0.0:5173 0.0.0.0:*
LISTEN 0 2048 [::]:8000 [::]:*`
	got = container.ParseListening(ss)
	if len(got) != 2 || got[0] != 5173 || got[1] != 8000 {
		t.Errorf("ss output gave %v, want [5173 8000] — including the IPv6 row", got)
	}

	if got := container.ParseListening(""); len(got) != 0 {
		t.Errorf("an empty probe reported %v listening", got)
	}
}

func TestTheContainerNameIsDerived(t *testing.T) {
	if got := env.ContainerName("monduli"); got != "mia-monduli" {
		t.Errorf("ContainerName = %q", got)
	}
	if got := env.Sanitise("odd/name here"); strings.ContainsAny(got, "/ ") {
		t.Errorf("Sanitise left something a container name cannot hold: %q", got)
	}
}

func environmentProbe(t *testing.T, script string) (env.Manager, model.Record, string) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "engine")
	log := filepath.Join(dir, "calls")
	t.Setenv("MIA_ENV_CALLS", log)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MIA_ENV_CALLS\"\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return env.Manager{Engine: container.Engine{Binary: binary}}, model.Record{Path: dir, Name: "monduli"}, log
}

func TestUnknownRuntimeRetainsEnvironmentFacts(t *testing.T) {
	dir := t.TempDir()
	placement, err := model.OnRuntime("missing", "/home/person/.mia/app/monduli")
	if err != nil {
		t.Fatal(err)
	}
	record := model.Record{Path: dir, Name: "monduli", Env: &model.Environment{Placement: placement}}
	manager := env.Manager{
		Runtimes: runtime.Store{Path: filepath.Join(dir, "runtimes.json")},
		Settings: env.Settings{Image: "example/image:1", Ports: []int{3000}, Page: 3000},
	}
	description := manager.Describe(record)
	if description.Machine != "missing" || description.Worktree != record.Path || description.Image.Ref != "example/image:1" {
		t.Fatalf("configured environment facts were lost: %+v", description)
	}
	if !strings.Contains(description.Error, "missing") || description.Mirror != description.Error || description.State != "" {
		t.Fatalf("unknown runtime became a successful environment: %+v", description)
	}
	ports, err := manager.PortsOf(record)
	if err == nil || !slices.Equal(ports.Declared, []int{3000}) || ports.Page != 3000 {
		t.Fatalf("unknown runtime lost declared ports or its error: %+v, %v", ports, err)
	}
	if statuses, err := manager.ServiceStatus(record); err == nil || len(statuses) != 0 {
		t.Fatalf("unknown runtime reported services: %+v, %v", statuses, err)
	}
}

func TestDescriptionDistinguishesAbsentContainerFromUnavailableEngine(t *testing.T) {
	for _, tc := range []struct {
		name        string
		state       string
		info        string
		unavailable bool
	}{
		{name: "running", state: "running", info: "echo 'engine unavailable' >&2; exit 2"},
		{name: "stopped", state: "exited", info: "echo 'engine unavailable' >&2; exit 2"},
		{name: "absent", info: "exit 0"},
		{name: "unavailable", info: "echo 'engine unavailable' >&2; exit 2", unavailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, record, log := environmentProbe(t, `case "$1" in
inspect) echo `+tc.state+` ;;
info) `+tc.info+` ;;
exec) exit 0 ;;
*) exit 99 ;;
esac
`)
			description := manager.Describe(record)
			if description.State != tc.state || (description.Error != "") != tc.unavailable {
				t.Fatalf("container state confused with engine availability: %+v", description)
			}
			if tc.unavailable && !strings.Contains(description.Error, "engine unavailable") {
				t.Fatalf("engine failure detail was lost: %+v", description)
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(calls), "info --format") != (tc.state == "") {
				t.Fatalf("engine availability probed for the wrong state: %s", calls)
			}
		})
	}
}

func TestPortProbeFailureRetainsIndependentServiceFacts(t *testing.T) {
	manager, record, _ := environmentProbe(t, `case "$1" in
inspect) echo running ;;
exec)
  case "$*" in
  *ss\ -ltnH*) echo 'port probe unavailable' >&2; exit 2 ;;
  *.pid*) echo yes ;;
  *) exit 0 ;;
  esac ;;
*) exit 99 ;;
esac
`)
	manager.Settings = env.Settings{Ports: []int{3000, 8000}, Page: 3000}
	manager.Services = []env.Service{{ID: "web", Run: []string{"serve", "--port", "3000"}, Health: []string{"health"}}}
	ports, err := manager.PortsOf(record)
	if err == nil || !strings.Contains(err.Error(), "port probe unavailable") || !slices.Equal(ports.Declared, manager.Settings.Ports) || ports.Page != 3000 || len(ports.Listening) != 0 {
		t.Fatalf("failed listening probe lost declared facts: %+v, %v", ports, err)
	}
	statuses, err := manager.ServiceStatus(record)
	if err != nil || len(statuses) != 1 || !statuses[0].Running || statuses[0].Healthy == nil || !*statuses[0].Healthy || statuses[0].Error != "" {
		t.Fatalf("port failure erased a healthy service: %+v, %v", statuses, err)
	}
}

func TestStoppedEnvironmentNeedsNoServiceOrPortProbes(t *testing.T) {
	manager, record, log := environmentProbe(t, `case "$1" in
inspect) echo exited ;;
*) echo 'unexpected probe' >&2; exit 99 ;;
esac
`)
	manager.Settings = env.Settings{Ports: []int{3000}, Page: 3000}
	manager.Services = []env.Service{{ID: "web", Run: []string{"serve", "3000"}, Health: []string{"health"}}}
	ports, err := manager.PortsOf(record)
	if err != nil || !slices.Equal(ports.Declared, []int{3000}) || ports.Page != 3000 || len(ports.Listening) != 0 {
		t.Fatalf("stopped environment is a port failure: %+v, %v", ports, err)
	}
	statuses, err := manager.ServiceStatus(record)
	if err != nil || len(statuses) != 1 || statuses[0].Running || statuses[0].Healthy != nil || statuses[0].Error != "" || !slices.Equal(statuses[0].Run, manager.Services[0].Run) {
		t.Fatalf("stopped environment is a service failure: %+v, %v", statuses, err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(calls), "exec") {
		t.Fatalf("probed a stopped environment: %s", calls)
	}
}

func TestFailedServiceProbePreservesLaterServicesAndLiteralRun(t *testing.T) {
	manager, record, log := environmentProbe(t, `case "$1" in
inspect) echo running ;;
exec)
  case "$*" in
  *zeta.pid*) echo 'process query unavailable' >&2; exit 2 ;;
  *.pid*) echo yes ;;
  *health-zeta*) echo 'failed service health was queried' >&2; exit 99 ;;
  *) exit 0 ;;
  esac ;;
*) exit 99 ;;
esac
`)
	manager.Services = []env.Service{
		{ID: "zeta", Run: []string{"serve app", "--value=$10; 'quoted'"}, Health: []string{"health-zeta"}},
		{ID: "alpha", Run: []string{"serve", "8000"}, Health: []string{"health-alpha"}},
	}
	statuses, err := manager.ServiceStatus(record)
	if err != nil || len(statuses) != 2 {
		t.Fatalf("failed service probe lost the partial result: %+v, %v", statuses, err)
	}
	if statuses[0].ID != "alpha" || !statuses[0].Running || statuses[0].Healthy == nil || !*statuses[0].Healthy || statuses[0].Error != "" || !slices.Equal(statuses[0].Run, manager.Services[1].Run) {
		t.Fatalf("later healthy service was lost or unsorted: %+v", statuses[0])
	}
	if statuses[1].ID != "zeta" || statuses[1].Running || statuses[1].Healthy != nil || !strings.Contains(statuses[1].Error, "process query unavailable") || !slices.Equal(statuses[1].Run, manager.Services[0].Run) {
		t.Fatalf("failed service has false health or altered argv: %+v", statuses[1])
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(calls), "health-zeta") {
		t.Fatalf("queried health after a failed process probe: %s", calls)
	}
}

func TestServiceHealthFailureDiffersFromInterruptedProbe(t *testing.T) {
	for _, tc := range []struct {
		name        string
		probe       string
		interrupted bool
	}{
		{name: "unhealthy", probe: "exit 3"},
		{name: "interrupted", probe: "echo 'health interrupted' >&2; kill -TERM $$", interrupted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, record, _ := environmentProbe(t, `case "$1" in
inspect) echo running ;;
exec)
  case "$*" in
  *.pid*) echo yes ;;
  *) `+tc.probe+` ;;
  esac ;;
*) exit 99 ;;
esac
`)
			manager.Services = []env.Service{{ID: "web", Run: []string{"serve"}, Health: []string{"health"}}}
			statuses, err := manager.ServiceStatus(record)
			if len(statuses) != 1 || !statuses[0].Running {
				t.Fatalf("health probe lost process status: %+v, %v", statuses, err)
			}
			status := statuses[0]
			if tc.interrupted {
				if err != nil || status.Error == "" || status.Healthy != nil {
					t.Fatalf("interrupted health probe became unhealthy: %+v, %v", status, err)
				}
			} else if err != nil || status.Error != "" || status.Healthy == nil || *status.Healthy {
				t.Fatalf("valid failing health probe became a query error: %+v, %v", status, err)
			}
		})
	}
}
