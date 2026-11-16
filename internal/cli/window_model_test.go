package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/session"
)

// mia window new <worktree> <agent> --model starts the agent the window is
// named for with it, and mia window model switches it while it runs: an
// agent a plugin adds, which says it is in its pane's title.
func TestAnAgentsWindowIsStartedWithAModelAndSwitchedByItsVerbs(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-model-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	agent := "#!/bin/sh\nprintf '\\033]2;echo-agent %s\\033\\\\' \"$*\"; echo \"echo-agent $*\"; exec cat\n"
	if err := os.WriteFile(filepath.Join(socket, "echo-agent"), []byte(agent), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", socket+string(os.PathListSeparator)+os.Getenv("PATH"))
	manifest, err := os.ReadFile("../app/testdata/mia-echo")
	if err != nil {
		t.Fatal(err)
	}
	plugins := plugin.Host{Dirs: []string{t.TempDir()}, StatePath: filepath.Join(t.TempDir(), "plugins.json")}
	if err := os.WriteFile(filepath.Join(plugins.Dirs[0], plugin.Prefix+"echo"), manifest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := plugins.Enable("echo"); err != nil {
		t.Fatal(err)
	}
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = plugins
	record, err := a.New("chosen")
	if err != nil {
		t.Fatal(err)
	}
	screen := func(want string) {
		t.Helper()
		var shown string
		for range 250 {
			if shown, _ = session.Here().Capture(record.Name, "echo", false); strings.Contains(shown, want) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("the window never showed %q: %q", want, shown)
	}

	if code := cmdWindow(a, []string{"new", record.Name, "echo", "--model", "shout"}, false); code != exitFailed || session.Here().HasWindow(record.Name, "echo") {
		t.Fatalf("a model it has not: exit %d", code)
	}
	if code := cmdWindow(a, []string{"new", record.Name, "notes", "--model", "loud"}, false); code != exitFailed {
		t.Errorf("a model for a window of no agent: exit %d", code)
	}
	if code := cmdWindow(a, []string{"read", record.Name, "echo", "--effort", "low"}, false); code != exitUsage {
		t.Errorf("an effort for a verb that starts nothing: exit %d", code)
	}
	if code := cmdWindow(a, []string{"new", record.Name, "echo", "--model", "loud"}, false); code != exitOK {
		t.Fatalf("started with a model: exit %d", code)
	}
	screen("echo-agent --model loud")
	if code := cmdWindow(a, []string{"model", record.Name, "echo", "soft", "--effort", "high"}, false); code != exitOK {
		t.Fatalf("switched: exit %d", code)
	}
	screen("/effort high\n/effort high")
	if code := cmdWindow(a, []string{"model", record.Name, "echo"}, false); code != exitUsage {
		t.Errorf("switched to no model: exit %d", code)
	}
}
