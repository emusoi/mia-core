package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
)

func TestNamedLaunchUsesConfiguredShellOnRemoteHost(t *testing.T) {
	repo := gittest.New(t)
	gittest.Isolate(t)
	dir := t.TempDir()
	ssh := `#!/bin/sh
for arg do command=$arg; done
printf '%s\n' "$command" >> "$MIA_TEST_SSH_LOG"
case "$command" in
  *echo*) printf '/home/koala\n' ;;
  *"command -v"*) printf '/usr/bin/zsh\n' ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SHELL", "/mac-only/zsh")
	log := filepath.Join(dir, "ssh.log")
	t.Setenv("MIA_TEST_SSH_LOG", log)

	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Config.Dotfiles.Shell = "zsh"
	a.Config.Launch = map[string]string{"logs": "printf launch-marker"}
	if err := a.Runtimes.Add(runtime.Machine{Name: "fedora", SSH: "named-launch.invalid"}); err != nil {
		t.Fatal(err)
	}
	placement, err := model.OnRuntime("fedora", "/home/koala/.mia/app/example")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Path: repo.Root, Name: "example", Env: &model.Environment{Placement: placement}}); err != nil {
		t.Fatal(err)
	}
	if got := cmdWindow(a, []string{"new", "example", "logs"}, false); got != exitOK {
		t.Fatalf("named launch exit = %d", got)
	}

	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, "'new-window'") {
			if !strings.Contains(line, "'zsh' '-lc'") || strings.Contains(line, "/mac-only/zsh") {
				t.Fatalf("remote launch = %q, want configured zsh on the target host", line)
			}
			return
		}
	}
	t.Fatalf("no remote new-window call in %q", data)
}
