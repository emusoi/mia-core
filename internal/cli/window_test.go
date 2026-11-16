package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
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

// mia window takes <worktree>@<machine> as mia run and mia shell do: the
// machine's own mia answers. It used to read the whole word as a window's name
// in the worktree it was run from, and `new` opened one called that.
func TestWindowVerbsOfAWorktreeOnAMachineGoToItsMia(t *testing.T) {
	repo := gittest.New(t)
	gittest.Isolate(t)
	t.Setenv("HOME", filepath.Dir(repo.Root)) // the repository has a place under the home there
	dir := t.TempDir()
	ssh := "#!/bin/sh\nfor arg do command=$arg; done\nprintf '%s\\n' \"$command\" >> \"$MIA_TEST_SSH_LOG\"\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	log := filepath.Join(dir, "ssh.log")
	t.Setenv("MIA_TEST_SSH_LOG", log)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Runtimes.Add(runtime.Machine{Name: "fedora", SSH: "koala@fedora", Engine: "/usr/bin/podman"}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo.Root)
	if code := Run([]string{"window", "send", "monduli@fedora", "agent", "--", "go on"}); code != exitOK {
		t.Fatalf("mia window send monduli@fedora exited %d", code)
	}
	sent, _ := os.ReadFile(log)
	if !strings.Contains(string(sent), `mia window "$@"`) || !strings.Contains(string(sent), "'send' 'monduli' 'agent' '--' 'go on'") {
		t.Fatalf("the machine's mia was asked: %s", sent)
	}
	if code := Run([]string{"window", "new", "monduli@nowhere"}); code != exitFailed {
		t.Fatalf("a machine there is not: exited %d", code)
	}
	if session.Here().Exists(filepath.Base(repo.Root)) {
		t.Fatal("a window was opened here for a worktree named on a machine")
	}
}

// A window is its command: one whose command has finished is gone, and its
// output with it. mia says so, rather than only that it opened.
func TestAWindowWhoseCommandEndedIsSaidToHaveEnded(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-window-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("quick")
	if err != nil {
		t.Fatal(err)
	}
	said := func(args ...string) string {
		t.Helper()
		output, err := os.CreateTemp(t.TempDir(), "output")
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		previous := os.Stdout
		os.Stdout = output
		code := cmdWindow(a, args, false)
		os.Stdout = previous
		if code != exitOK {
			t.Fatalf("mia window %v exited %d", args, code)
		}
		data, err := os.ReadFile(output.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if got := said("new", record.Name, "quick", "--", "true"); !strings.Contains(got, "window quick has ended already") {
		t.Errorf("a window whose command finished at once:\n%s", got)
	}
	if got := said("new", record.Name, "stays", "--", "sleep", "300"); strings.Contains(got, "ended already") {
		t.Errorf("a window whose command still runs:\n%s", got)
	}
}
