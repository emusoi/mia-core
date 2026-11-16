package cli

import (
	"encoding/json"
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

// mia account add makes an account's folder and opens a window running the
// agent on it, for the person to sign in to; a window started on it says
// so, and it cannot be removed while one runs. The agent is a plugin's,
// run from the lab: no real agent can start.
func TestAnAccountIsAddedSignedInToInAWindowAndUsed(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-account-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	bin := filepath.Join(socket, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	if err := os.Symlink(tmux, filepath.Join(bin, "tmux")); err != nil {
		t.Fatal(err)
	}
	agent := "#!/bin/sh\nprintf '\\033]2;echo-agent %s\\033\\\\' \"$*\"; echo \"ran $0 on $ECHO_HOME\"; exec cat\n"
	if err := os.WriteFile(filepath.Join(bin, "echo-agent"), []byte(agent), 0o755); err != nil {
		t.Fatal(err)
	}
	// only the lab, git and the system's own: what a window's login shell finds is the lab's
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	if found, err := exec.Command("/bin/sh", "-lc", "command -v echo-agent").Output(); err != nil || strings.TrimSpace(string(found)) != filepath.Join(bin, "echo-agent") {
		t.Fatalf("a window's shell finds echo-agent at %q", found)
	}
	manifest := `#!/bin/sh
case "$1" in
manifest) echo '{"protocol":1,"agent":{"name":"echo","commands":["echo-agent"],"title":"^echo-agent","account_env":"ECHO_HOME"}}' ;;
agent) echo '[]' ;;
esac
`
	plugins := plugin.Host{Dirs: []string{t.TempDir()}, StatePath: filepath.Join(t.TempDir(), "plugins.json")}
	if err := os.WriteFile(filepath.Join(plugins.Dirs[0], plugin.Prefix+"echo"), []byte(manifest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := plugins.Enable("echo"); err != nil {
		t.Fatal(err)
	}
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = plugins
	record, err := a.New("goba")
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "accounts", "echo", "work")
	screen := func(worktree, window, want string) {
		t.Helper()
		var shown string
		for range 250 {
			if shown, _ = session.Here().Capture(worktree, window, false); strings.Contains(shown, want) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s's window %s never showed %q: %q", worktree, window, want, shown)
	}

	opened, err := a.AddAccount("echo", "work", "")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Session != "mia-accounts" || opened.Window != "echo-work" || opened.Command != "ECHO_HOME='"+work+"' echo-agent" {
		t.Errorf("opened = %+v", opened)
	}
	if info, err := os.Stat(work); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("its folder: %v %v", info, err)
	}
	screen("accounts", "echo-work", "ran "+filepath.Join(bin, "echo-agent")+" on "+work)
	if _, err := a.AddAccount("echo", "work", ""); err == nil {
		t.Error("an account added twice")
	}
	if _, err := a.AddAccount("echo", "default", ""); err == nil {
		t.Error("the agent's own folder added as an account")
	}
	if _, err := a.AddAccount("opencode", "work", ""); err == nil {
		t.Error("an account of an agent mia keeps none of")
	}

	// a window started on it, this once
	if code := cmdWindow(a, []string{"new", record.Name, "echo", "--account", "work"}, false); code != exitOK {
		t.Fatalf("started on an account: exit %d", code)
	}
	screen(record.Name, "echo", "on "+work)
	if code := cmdWindow(a, []string{"new", record.Name, "echo", "--account", "nobody"}, false); code != exitFailed {
		t.Errorf("an account it has not: exit %d", code)
	}
	if err := a.AccountOfWindow(record, "echo", "default"); err == nil || !strings.Contains(err.Error(), "takes its account when it starts") {
		t.Errorf("a window put on another account: %v", err)
	}
	var listed []app.Account
	echo := func(name string) app.Account {
		for _, one := range listed {
			if one.Agent == "echo" && one.Name == name {
				return one
			}
		}
		return app.Account{}
	}
	for range 100 { // the window's title is the agent's once it has started
		if listed = a.Accounts(); len(echo("work").Sessions) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if work := echo("work"); strings.Join(work.Sessions, " ") != "accounts "+record.Name || work.LoggedIn != "no" || work.Default {
		t.Fatalf("echo's work = %+v", work)
	}
	if own := echo("default"); own.Home != "" || own.LoggedIn != "unknown" || len(own.Sessions) != 0 || !own.Default {
		t.Fatalf("echo's own = %+v", own)
	}
	if _, err := a.RemoveAccount("echo", "work", true); err == nil || !strings.Contains(err.Error(), "stop it there first") {
		t.Errorf("an account in use removed: %v", err)
	}
	data, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("mia api accounts: %s", data)
}
