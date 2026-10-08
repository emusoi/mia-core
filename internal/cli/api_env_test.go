package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

type environmentSnapshot struct {
	env.Environment
	Ports         env.Ports    `json:"ports"`
	Services      []env.Status `json:"services"`
	PortsError    string       `json:"portsError"`
	ServicesError string       `json:"servicesError"`
}

func TestEnvironmentAPIKeepsFactsWhenQueriesFail(t *testing.T) {
	for _, check := range []struct {
		name          string
		state         string
		portsFail     bool
		remoteMissing bool
		engineFail    bool
		serviceFail   bool
	}{
		{"running", "running", false, false, false, false},
		{"port probe unavailable", "running", true, false, false, false},
		{"service probe unavailable", "running", false, false, false, true},
		{"stopped", "exited", false, false, false, false},
		{"not created", "", false, false, false, false},
		{"engine unavailable", "", false, false, true, false},
		{"remote placement unavailable", "", false, true, false, false},
	} {
		t.Run(check.name, func(t *testing.T) {
			gittest.Isolate(t)
			repo := gittest.New(t)
			path := repo.Worktree("longido", "environment")
			a, err := app.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			record := model.Record{Name: "longido", Path: path}
			if check.remoteMissing {
				placement, err := model.OnRuntime("missing-box", "/home/test/.mia/app/longido")
				if err != nil {
					t.Fatal(err)
				}
				record.Env = &model.Environment{Placement: placement}
			}
			if err := a.Store.Put(record); err != nil {
				t.Fatal(err)
			}
			command := []string{"npm", "run", "dev", "--", "O'Brien;$literal"}
			a.Config.Env = env.Settings{Image: "fixture/image:1", Ports: []int{3000}, Page: 3000}
			a.Config.Services = []env.Service{{ID: "web", Run: command}}
			bin := t.TempDir()
			engine := `#!/bin/sh
case "$1" in
inspect) printf '%s\n' "$MIA_ENV_API_STATE" ;;
info)
  if [ "$MIA_ENV_API_ENGINE_FAIL" = true ]; then printf 'engine unavailable\n' >&2; exit 1; fi
  printf 'arm64\n' ;;
exec)
  for arg do
    case "$arg" in
      *'ss -ltnH'*)
        if [ "$MIA_ENV_API_PORTS_FAIL" = true ]; then printf 'port probe unavailable\n' >&2; exit 1; fi
        printf 'LISTEN 0 511 0.0.0.0:3000 0.0.0.0:*\n'; exit 0 ;;
      *'kill -0'*)
        if [ "$MIA_ENV_API_SERVICE_FAIL" = true ]; then printf 'service probe unavailable\n' >&2; exit 1; fi
        printf 'yes\n'; exit 0 ;;
    esac
  done ;;
esac
`
			if err := os.WriteFile(filepath.Join(bin, "podman"), []byte(engine), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("MIA_ENV_API_STATE", check.state)
			if check.engineFail {
				t.Setenv("MIA_ENV_API_ENGINE_FAIL", "true")
			} else {
				t.Setenv("MIA_ENV_API_ENGINE_FAIL", "false")
			}
			if check.portsFail {
				t.Setenv("MIA_ENV_API_PORTS_FAIL", "true")
			} else {
				t.Setenv("MIA_ENV_API_PORTS_FAIL", "false")
			}
			if check.serviceFail {
				t.Setenv("MIA_ENV_API_SERVICE_FAIL", "true")
			} else {
				t.Setenv("MIA_ENV_API_SERVICE_FAIL", "false")
			}
			output, err := os.CreateTemp(t.TempDir(), "output")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			stdout := os.Stdout
			os.Stdout = output
			t.Cleanup(func() { os.Stdout = stdout })
			if code := cmdAPI(a, []string{"env", "longido"}); code != exitOK {
				t.Fatalf("mia api env exited %d", code)
			}
			if _, err := output.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			var snapshot environmentSnapshot
			if err := json.NewDecoder(output).Decode(&snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.Name != record.Name || snapshot.Worktree != path || snapshot.Container != "mia-longido" || snapshot.Image.Ref != "fixture/image:1" || !slices.Equal(snapshot.Ports.Declared, []int{3000}) || snapshot.Ports.Page != 3000 {
				t.Fatalf("snapshot lost known facts: %+v", snapshot)
			}
			if err := output.Truncate(0); err != nil {
				t.Fatal(err)
			}
			if _, err := output.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			if code := cmdEnv(a, []string{"status", "longido"}, false); code != exitOK {
				t.Fatalf("mia env status exited %d", code)
			}
			status, err := os.ReadFile(output.Name())
			if err != nil {
				t.Fatal(err)
			}
			want := "longido  " + check.state
			if snapshot.Error != "" {
				want = "longido  unavailable — " + snapshot.Error
				if strings.Contains(string(status), "no environment") {
					t.Fatalf("status offered to create an unavailable environment: %s", status)
				}
			} else if check.state == "" {
				want = "longido  no environment — `mia env up` starts one"
			}
			if !strings.Contains(string(status), want) {
				t.Fatalf("plain status lost the environment outcome: %s", status)
			}
			if check.remoteMissing {
				if snapshot.Machine != "missing-box" || snapshot.Error == "" || snapshot.Mirror != snapshot.Error || snapshot.PortsError == "" || snapshot.ServicesError == "" || !strings.Contains(snapshot.Error, "missing-box") {
					t.Fatalf("snapshot hid failed placement: %+v", snapshot)
				}
				return
			}
			if snapshot.Machine != "local" || snapshot.State != check.state || (snapshot.Error != "") != check.engineFail || snapshot.ServicesError != "" || (snapshot.PortsError != "") != check.portsFail {
				t.Fatalf("snapshot confused independent query outcomes: %+v", snapshot)
			}
			if check.engineFail && !strings.Contains(snapshot.Error, "engine unavailable") {
				t.Fatalf("engine availability error was lost: %+v", snapshot)
			}
			if check.portsFail && !strings.Contains(snapshot.PortsError, "port probe unavailable") {
				t.Fatalf("port probe error was lost: %+v", snapshot)
			}
			if len(snapshot.Services) != 1 || snapshot.Services[0].ID != "web" || !slices.Equal(snapshot.Services[0].Run, command) || snapshot.Services[0].Running != (check.state == "running" && !check.serviceFail) || (snapshot.Services[0].Error != "") != check.serviceFail {
				t.Fatalf("snapshot lost declared service and query result: %+v", snapshot.Services)
			}
			if err := output.Truncate(0); err != nil {
				t.Fatal(err)
			}
			if _, err := output.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			if code := cmdEnv(a, []string{"service", "longido"}, false); code != exitOK {
				t.Fatalf("mia env service exited %d", code)
			}
			serviceStatus, err := os.ReadFile(output.Name())
			if err != nil {
				t.Fatal(err)
			}
			if check.serviceFail && (!strings.Contains(string(serviceStatus), "unavailable — "+snapshot.Services[0].Error) || strings.Contains(string(serviceStatus), "stopped") || strings.Contains(string(serviceStatus), "healthy")) {
				t.Fatalf("plain service status hid the query error: %s", serviceStatus)
			}
			if check.state == "running" && !check.portsFail && !slices.Equal(snapshot.Ports.Listening, []int{3000}) {
				t.Fatalf("snapshot lost listening ports: %+v", snapshot.Ports)
			}
		})
	}
}

func TestEnvironmentCommandFlagsRemainServiceAndRuntimeOperands(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	path := repo.Worktree("longido", "environment")
	a, err := app.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "longido", Path: path}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	engine := `#!/bin/sh
case "$1" in
inspect) printf 'running\n' ;;
exec)
  printf '%s\n' "$*" >> "$MIA_ENV_OPERAND_CALLS"
  case "$*" in *'kill -0'*) printf 'no\n' ;; esac ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte(engine), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	calls := filepath.Join(bin, "calls")
	t.Setenv("MIA_ENV_OPERAND_CALLS", calls)
	for _, id := range []string{"--watch", "--share", "--once", "--credentials"} {
		a.Config.Services = append(a.Config.Services, env.Service{ID: id, Run: []string{"serve"}})
	}
	for _, service := range a.Config.Services {
		if code := cmdEnv(a, []string{"service", "start", "longido", service.ID}, false); code != exitOK {
			t.Fatalf("service %q was consumed as a flag: exit %d", service.ID, code)
		}
	}
	arguments, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range a.Config.Services {
		if !strings.Contains(string(arguments), "/"+service.ID+".pid") || !strings.Contains(string(arguments), "/"+service.ID+".log") {
			t.Fatalf("service %q did not reach the engine unchanged: %s", service.ID, arguments)
		}
	}
	if err := a.Runtimes.Add(runtime.Machine{Name: "--watch"}); err != nil {
		t.Fatal(err)
	}
	if code := cmdEnv(a, []string{"host", "longido", "--watch"}, false); code != exitOK {
		t.Fatalf("runtime operand was consumed as a flag: exit %d", code)
	}
	record, err := a.RecordOrHere("longido")
	if err != nil {
		t.Fatal(err)
	}
	if record.Env == nil || record.Env.Placement.Runtime() != "--watch" {
		t.Fatalf("runtime operand did not become the placement: %+v", record.Env)
	}
}
