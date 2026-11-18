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

	// signed out, or run out, one it has is signed in to again: the agent on its folder once more
	again, err := a.LoginAccount("echo", "work", "")
	if err != nil {
		t.Fatal(err)
	}
	if again.Session != "mia-accounts" || again.Window != "echo-work-2" || again.Command != opened.Command || again.Account.Home != work {
		t.Errorf("signed in again = %+v", again)
	}
	screen("accounts", "echo-work-2", "on "+work)
	own, err := a.LoginAccount("echo", "default", "")
	if err != nil {
		t.Fatal(err)
	}
	if own.Window != "echo-default" || own.Command != "echo-agent" || !own.Account.Default {
		t.Errorf("its own folder signed in to again = %+v", own)
	}
	screen("accounts", "echo-default", "ran "+filepath.Join(bin, "echo-agent")+" on")
	if shown, _ := session.Here().Capture("accounts", "echo-default", false); strings.Contains(shown, work) {
		t.Errorf("its own folder was started on another: %q", shown)
	}
	if _, err := a.LoginAccount("echo", "nobody", ""); err == nil || !strings.Contains(err.Error(), "mia account add echo nobody") {
		t.Errorf("an account it has not signed in to: %v", err)
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
		if listed = a.Accounts(); len(echo("work").Sessions) == 2 && len(echo("default").Sessions) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if work := echo("work"); strings.Join(work.Sessions, " ") != "accounts "+record.Name || work.LoggedIn != "no" || work.Default {
		t.Fatalf("echo's work = %+v", work)
	}
	if own := echo("default"); own.Home != "" || own.LoggedIn != "unknown" || strings.Join(own.Sessions, " ") != "accounts" || !own.Default || !own.UserDefault {
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

// mia account use --global says, in the user-wide config, the account an
// agent's sessions start on wherever nothing else says one; mia api
// accounts says which it is, and a rename or a removal is followed there.
func TestTheUserWideAccountIsSetInTheUserConfigAndFollowed(t *testing.T) {
	gittest.Isolate(t)
	for _, name := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "PI_CODING_AGENT_DIR"} {
		t.Setenv(name, "")
	}
	accounts := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "accounts")
	if err := os.MkdirAll(filepath.Join(accounts, "claude", "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml")
	if err := os.WriteFile(user, []byte("# mine\n[agents.claude]\nmodel = \"opus\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	said := func() string {
		t.Helper()
		data, err := os.ReadFile(user)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	chosen := func(agent string) []string {
		t.Helper()
		a, err := app.Open(repo.Root)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, one := range a.Accounts() {
			if one.Agent == agent && one.UserDefault {
				names = append(names, one.Name)
			}
		}
		return names
	}
	if got := chosen("claude"); strings.Join(got, " ") != "default" {
		t.Errorf("with none said, the user-wide one is %v", got)
	}
	if code := cmdAccount([]string{"use", "claude", "nobody", "--global"}, false); code != exitFailed {
		t.Errorf("an account it has not: exit %d", code)
	}
	if code := cmdAccount([]string{"use", "claude", "work", "--global", "--repository", "x"}, false); code != exitUsage {
		t.Errorf("--global and --repository: exit %d", code)
	}
	if code := cmdAccount([]string{"use", "claude", "work", "--global"}, true); code != exitOK {
		t.Fatalf("use --global: exit %d", code)
	}
	if want := "# mine\n[agents.claude]\nmodel = \"opus\"\naccount = \"work\"\n"; said() != want {
		t.Errorf("the user config = %q", said())
	}
	if got := chosen("claude"); strings.Join(got, " ") != "work" {
		t.Errorf("the user-wide one is %v", got)
	}
	if got := chosen("codex"); strings.Join(got, " ") != "default" {
		t.Errorf("another agent's is %v", got)
	}

	if code := cmdAccount([]string{"rename", "claude", "work", "team"}, false); code != exitOK {
		t.Fatalf("rename: exit %d", code)
	}
	if !strings.Contains(said(), "account = \"team\"") {
		t.Errorf("a rename is not followed: %q", said())
	}
	if code := cmdAccount([]string{"rm", "claude", "team"}, false); code != exitOK {
		t.Fatalf("rm: exit %d", code)
	}
	if want := "# mine\n[agents.claude]\nmodel = \"opus\"\n"; said() != want {
		t.Errorf("a removal is not followed: %q", said())
	}
	if got := chosen("claude"); strings.Join(got, " ") != "default" {
		t.Errorf("after the removal the user-wide one is %v", got)
	}

	if err := os.MkdirAll(filepath.Join(accounts, "claude", "spare"), 0o700); err != nil {
		t.Fatal(err)
	}
	if code := cmdAccount([]string{"use", "claude", "spare", "--global"}, false); code != exitOK {
		t.Fatalf("use --global: exit %d", code)
	}
	if code := cmdAccount([]string{"use", "claude", "default", "--global"}, false); code != exitOK {
		t.Fatalf("use default --global: exit %d", code)
	}
	if want := "# mine\n[agents.claude]\nmodel = \"opus\"\n"; said() != want {
		t.Errorf("default takes the line out: %q", said())
	}
}

// What mia prints to paste into a shell takes the terminal to the window
// as it is pasted, in zsh too, where =word is the path of a command: no =,
// and quoted when a shell would read anything in it; switch-client from
// inside tmux.
func TestTheWayToAnAccountsWindowIsPastedAsItIs(t *testing.T) {
	for _, c := range []struct {
		session, window string
		inTmux          bool
		want            string
	}{
		{"mia-accounts", "claude-work", false, "tmux attach -t mia-accounts:claude-work"},
		{"mia-accounts", "claude-work-2", true, "tmux switch-client -t mia-accounts:claude-work-2"},
		{"mia-accounts", "it's mine", false, `tmux attach -t 'mia-accounts:it'\''s mine'`},
		{"mia-accounts", "=x", false, "tmux attach -t 'mia-accounts:=x'"},
	} {
		if got := goesThere(c.session, c.window, c.inTmux); got != c.want {
			t.Errorf("goesThere(%q, %q, %v) = %q, want %q", c.session, c.window, c.inTmux, got, c.want)
		}
	}
	// what zsh makes of it: the words as they were
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("no zsh")
	}
	for _, window := range []string{"claude-work", "it's mine"} {
		line := goesThere("mia-accounts", window, false)
		out, err := exec.Command(zsh, "-fc", "print -rl -- "+line).CombinedOutput()
		if want := "tmux\nattach\n-t\nmia-accounts:" + window + "\n"; err != nil || string(out) != want {
			t.Errorf("zsh reads %q as %q: %v", line, out, err)
		}
	}
	if out, err := exec.Command(zsh, "-fc", "print -rl -- tmux attach -t =mia-accounts").CombinedOutput(); err == nil {
		t.Errorf("zsh took =mia-accounts as it is: %q", out)
	}
}
