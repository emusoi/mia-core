package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/paths"
	"github.com/emusoi/mia-core/internal/session"
)

// roleLab is a private tmux, an agent called echo that says what it was
// started with and then shows what it is sent, and roles of it.
func roleLab(t *testing.T) {
	t.Helper()
	gittest.Isolate(t)
	t.Setenv("MIA_WORKTREE", "")
	socket, err := os.MkdirTemp("/tmp", "mia-role-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	bin := filepath.Join(socket, "bin")
	program := "#!/bin/sh\necho \"echo-agent $*\"\nexec cat\n"
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "echo-agent"), []byte(program), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	manifest, err := os.ReadFile(filepath.Join("..", "app", "testdata", "mia-echo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.In("plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.In("plugins", "mia-echo"), manifest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := pluginHost(config.Config{}).Enable("echo"); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"reviewer": "---\nagent: echo\nmodel: loud\neffort: high\n---\nYou review.\nRead the diff first.\n",
		"quiet":    "---\nagent: echo\nmodel: soft\n---\nSay little.\n",
		"ghost":    "---\nagent: nobody\n---\nBoo.\n",
		"shouter":  "---\nagent: echo\nmodel: shout\n---\nLOUDER.\n",
		"broken":   "---\nagent: echo\nYou review.\n",
	} {
		path, err := app.RolePath(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func screenShows(t *testing.T, worktree, window string, want ...string) string {
	t.Helper()
	var screen string
	for range 250 {
		screen, _ = session.Here().Capture(worktree, window, false)
		all := true
		for _, one := range want {
			all = all && strings.Contains(screen, one)
		}
		if all {
			return screen
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("window %s of %s never showed %q: %q", window, worktree, want, screen)
	return ""
}

// mia window new --role starts the role's agent with its model and effort,
// in a window named for the agent or as it is told, and sends it the
// role's body once it is ready. What the role names that mia does not
// know is refused before any window opens.
func TestAWindowStartedAsARoleIsSentItsOpening(t *testing.T) {
	roleLab(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = pluginHost(a.Config)
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	if code := cmdWindow(a, []string{"new", record.Name, "--role", "reviewer"}, false); code != exitOK {
		t.Fatalf("mia window new --role reviewer exited %d", code)
	}
	screenShows(t, record.Name, "echo", "echo-agent --model loud --effort high\n", "You review.", "Read the diff first.")
	if code := cmdWindow(a, []string{"new", record.Name, "calm", "--role", "quiet", "--effort", "low"}, false); code != exitOK {
		t.Fatalf("mia window new calm --role quiet exited %d", code)
	}
	screenShows(t, record.Name, "calm", "echo-agent --model soft --effort low\n", "Say little.")

	windows := len(session.Here().Windows(record.Name))
	for _, c := range []struct {
		args []string
		code int
	}{
		{[]string{"new", record.Name, "--role", "ghost"}, exitFailed},
		{[]string{"new", record.Name, "--role", "shouter"}, exitFailed},
		{[]string{"new", record.Name, "--role", "broken"}, exitFailed},
		{[]string{"new", record.Name, "--role", "missing"}, exitFailed},
		{[]string{"new", record.Name, "logs", "--role", "quiet", "--", "tail", "-f", "log"}, exitUsage},
		{[]string{"send", record.Name, "echo", "--role", "quiet", "--", "hello"}, exitUsage},
	} {
		if code := cmdWindow(a, c.args, false); code != c.code {
			t.Errorf("mia window %v exited %d, want %d", c.args, code, c.code)
		}
	}
	if now := len(session.Here().Windows(record.Name)); now != windows {
		t.Errorf("a refused role opened a window: %d then %d", windows, now)
	}
}

// mia main start --role starts the main agent as the role says and sends
// it the role's body; one that runs already is not sent it unasked.
func TestTheMainAgentStartedAsARoleIsSentItsOpening(t *testing.T) {
	roleLab(t)
	t.Chdir(t.TempDir())
	for _, refused := range []string{"ghost", "shouter", "broken", "missing"} {
		if code := Run([]string{"main", "start", "--role", refused}); code != exitFailed {
			t.Errorf("mia main start --role %s exited %d", refused, code)
		}
	}
	if session.Here().Exists("main") {
		t.Fatal("a refused role started the main agent")
	}
	if code := Run([]string{"main", "start", "--role", "quiet"}); code != exitOK {
		t.Fatalf("mia main start --role quiet exited %d", code)
	}
	t.Cleanup(func() { Run([]string{"main", "stop"}) })
	// the plugin says the last conversation in its folder is e2, and goes on with it
	screenShows(t, "main", "echo", "echo-agent --model soft e2\n", "Say little.")
	if code := Run([]string{"main", "start", "--role", "reviewer"}); code != exitFailed {
		t.Errorf("a role for a main agent that runs exited %d", code)
	}
	if code := Run([]string{"main", "start", "--new", "--restart", "--role", "reviewer"}); code != exitOK {
		t.Fatalf("mia main start --new --restart --role reviewer exited %d", code)
	}
	screenShows(t, "main", "echo", "echo-agent --model loud --effort high\n", "You review.", "Read the diff first.")
}

// mia role lists, shows and finds the roles, and mia api setups lists
// them with the other setups: a role is a setup with a body.
func TestRolesAreListedShownAndAmongTheSetups(t *testing.T) {
	roleLab(t)
	t.Chdir(t.TempDir())
	data, code := printed(t, "role", "ls", "--json")
	var roles []app.Role
	if err := json.Unmarshal(data, &roles); err != nil || code != exitOK || len(roles) != 4 || roles[2].Name != "reviewer" || roles[2].Body != "You review.\nRead the diff first." {
		t.Fatalf("mia role ls --json = %s, exit %d, %v", data, code, err)
	}
	if data, code = printed(t, "role", "show", "quiet"); code != exitOK || !strings.HasPrefix(string(data), "quiet  echo · soft\n") || !strings.HasSuffix(string(data), "\n\nSay little.\n") {
		t.Errorf("mia role show quiet = %q, exit %d", data, code)
	}
	path, _ := app.RolePath("new-one")
	if data, code = printed(t, "role", "path", "new-one"); code != exitOK || string(data) != path+"\n" {
		t.Errorf("mia role path new-one = %q, exit %d", data, code)
	}
	for _, bad := range [][]string{{"role", "show", "broken"}, {"role", "show", "missing"}, {"role", "path", "a/b"}} {
		if _, code := printed(t, bad...); code != exitFailed {
			t.Errorf("mia %v exited %d", bad, code)
		}
	}
	if _, code := printed(t, "role", "edit", "quiet"); code != exitUsage {
		t.Errorf("mia role edit exited %d", code)
	}
	data, code = answered(t, "setups")
	var setups []app.Setup
	if err := json.Unmarshal(data, &setups); err != nil || code != exitOK {
		t.Fatalf("api setups = %s, exit %d, %v", data, code, err)
	}
	var listed []string
	for _, one := range setups {
		if one.Role != "" {
			listed = append(listed, one.Role+":"+one.Agent+":"+one.Model+":"+one.Body)
		}
	}
	if want := "ghost:nobody::Boo. quiet:echo:soft:Say little. reviewer:echo:loud:You review.\nRead the diff first. shouter:echo:shout:LOUDER."; strings.Join(listed, " ") != want {
		t.Errorf("roles among the setups = %q", listed)
	}
	if !slices.Contains(apiCapabilities().Features, "roles") {
		t.Error("the capabilities have no roles")
	}
}
