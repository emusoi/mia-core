package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/panel"
)

func TestEnvironmentAPIAndPanelShareOneStateAndPortRead(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "longido", Path: repo.Root}); err != nil {
		t.Fatal(err)
	}
	a.Config.Env = env.Settings{Image: "fixture/image:1", Ports: []int{3000}, Page: 3000}
	a.Config.Services = []env.Service{{ID: "web", Run: []string{"serve", "3000"}, Health: []string{"health"}}}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte(`#!/bin/sh
printf '%s\n' "$*" >> "$MIA_ENVIRONMENT_API_CALLS"
case "$1" in
inspect) echo running ;;
exec)
  case "$*" in
  *ss\ -ltnH*) printf 'LISTEN 0 511 0.0.0.0:3000 0.0.0.0:*\n' ;;
  *.pid*) echo yes ;;
  esac ;;
*) exit 99 ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIA_ENVIRONMENT_API_CALLS", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, query := range []string{"env", "env-panel"} {
		t.Run(query, func(t *testing.T) {
			if err := os.WriteFile(log, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := os.CreateTemp(t.TempDir(), "output")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			previous := os.Stdout
			os.Stdout = output
			t.Cleanup(func() { os.Stdout = previous })
			if code := cmdAPI(a, []string{query, "longido"}); code != exitOK {
				t.Fatalf("mia api %s exited %d", query, code)
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(calls), "\n") != 4 || strings.Count(string(calls), "inspect ") != 1 || strings.Count(string(calls), "ss -ltnH") != 1 {
				t.Fatalf("%s repeated a state or port probe: %s", query, calls)
			}
			if _, err := output.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			if query == "env" {
				var snapshot env.Snapshot
				if err := json.NewDecoder(output).Decode(&snapshot); err != nil {
					t.Fatal(err)
				}
				if snapshot.Name != "longido" || snapshot.Worktree != repo.Root || snapshot.State != "running" || snapshot.Ports.Page != 3000 || len(snapshot.Services) != 1 || !snapshot.Services[0].Running || snapshot.Services[0].Healthy == nil || !*snapshot.Services[0].Healthy {
					t.Fatalf("environment facts were lost: %+v", snapshot)
				}
			} else {
				var view panel.Panel
				if err := json.NewDecoder(output).Decode(&view); err != nil {
					t.Fatal(err)
				}
				if view.ID != "env" || view.Title != "Environment / longido" || len(view.Sections) != 3 || view.Sections[1].Rows[0].ID != "port-3000" || view.Sections[2].Rows[0].Cells[1] != "running · healthy" {
					t.Fatalf("environment panel facts were lost: %+v", view)
				}
			}
		})
	}
}
