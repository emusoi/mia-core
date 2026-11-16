package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/session"
)

// trustAgent asks as claude 2.1.325 does in a folder it has not seen: no
// numbers, the cursor on No, exit, no digit taken, and Enter there ends it.
const trustAgent = `#!/bin/sh
sel=1
draw() {
	printf '\033[2J\033[H Accessing workspace:\n\n %s\n\n' "$PWD"
	printf " Quick safety check: Is this a project you created or one you trust? (Like your own code, a well-known open source project, or work from your team). If not, take a moment to review what's in this folder first.\n\n"
	printf " Claude Code'll be able to read, edit, and execute files here.\n\n Security guide\n\n"
	if [ "$sel" = 1 ]; then printf ' ❯ No, exit\n   Yes, I trust this folder\n'; else printf '   No, exit\n ❯ Yes, I trust this folder\n'; fi
	printf '\n Enter to confirm · Esc to cancel\n'
}
key() { dd bs=1 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n'; }
stty -icanon -echo min 1
draw
while :; do
	case "$(key)" in
	0a|0d) break ;;
	1b) [ "$(key)" = 5b ] && case "$(key)" in 41) sel=1 ;; 42) sel=2 ;; esac; draw ;;
	esac
done
stty sane
printf '\033[2J\033[H' # the question goes, as claude's does once answered
if [ "$sel" = 1 ]; then echo exiting; exit 1; fi
echo trusted
exec cat
`

// permissionAgent asks as claude asks for a permission: numbered, a digit
// answers it, and so does Enter, as the first.
const permissionAgent = `#!/bin/sh
printf ' Bash command\n\n   rm -rf build\n   Remove the build folder\n\n Do you want to proceed?\n ❯ 1. Yes\n   2. No, and tell Claude what to do differently (esc)\n\n Esc to cancel · Tab to amend\n'
stty -icanon -echo min 1
k=$(dd bs=1 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n')
stty sane
printf '\033[2J\033[H'
case "$k" in 31) echo "answered 1" ;; 0a|0d) echo "answered by Enter"; exit 1 ;; *) echo "answered $k" ;; esac
exec cat
`

// fakeAgents puts each program on the PATH under its name, and a plugin
// that makes it an agent of the same name, with a model to switch to.
func fakeAgents(t *testing.T, programs map[string]string) map[string]string {
	t.Helper()
	bin := t.TempDir()
	manifests := map[string]string{}
	for name, body := range programs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		manifests[name] = "#!/bin/sh\n" + `case "$1" in manifest) echo '{"protocol":1,"agent":{"name":"` + name + `","models":[{"id":"loud","label":"Loud"}],"efforts":["low","high"],"with_model":["--model","%s"],"with_effort":["--effort","%s"],"switch_model":"/model %s","switch_effort":"/effort %s"}}' ;; *) echo '[]' ;; esac`
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return manifests
}

func ownTmux(t *testing.T) {
	t.Helper()
	socket, err := os.MkdirTemp("/tmp", "mia-asks-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
}

func shows(t *testing.T, worktree, window, want string) string {
	t.Helper()
	var screen string
	for range 250 {
		if screen, _ = session.Here().Capture(worktree, window, false); strings.Contains(screen, want) {
			return screen
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never showed %q: %q", window, want, screen)
	return ""
}

// The main agent that asks first is said to, and nothing typed into it
// answers it: only the person's own press does.
func TestTheMainAgentThatAsksFirstIsNotTypedInto(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	manifests := fakeAgents(t, map[string]string{"asker": trustAgent})
	put(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), "[main]", `agent = "asker"`)
	open := func() *app.App {
		t.Helper()
		a, err := app.AtMain()
		if err != nil {
			t.Fatal(err)
		}
		a.Plugins = plugins(t, manifests)
		return a
	}

	state, err := open().StartMain("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	trust := app.Asks{Text: "Is this a project you created or one you trust?", Options: []string{"No, exit", "Yes, I trust this folder"}, Selected: 1}
	if !state.Running || state.Working || state.Asks == nil || state.Asks.Text != trust.Text || strings.Join(state.Asks.Options, "|") != "No, exit|Yes, I trust this folder" || state.Asks.Selected != 1 {
		t.Fatalf("started, it asks: %+v %+v", state, state.Asks)
	}
	refused := "asker asks first: Is this a project you created or one you trust? — answer it: `mia main press <n>` (1 No, exit · 2 Yes, I trust this folder)"
	if err := open().SayToMain("where do things stand"); err == nil || err.Error() != refused {
		t.Errorf("said while it asks: %v", err)
	}
	if _, err := open().SwitchMain("loud", "high"); err == nil || err.Error() != refused {
		t.Errorf("switched while it asks: %v", err)
	}
	time.Sleep(300 * time.Millisecond) // an Enter, had one gone, has ended it by now
	if screen := shows(t, "main", "asker", "❯ No, exit"); strings.Contains(screen, "where do things") || strings.Contains(screen, "/model") || !open().Main().Running {
		t.Fatalf("something was typed into it: %q", screen)
	}

	// the person answers: a digit it does not take is the cursor moved, and Enter
	if err := open().PressInMain("2"); err != nil {
		t.Fatal(err)
	}
	shows(t, "main", "asker", "trusted")
	if state := open().Main(); !state.Running || state.Asks != nil {
		t.Fatalf("answered: %+v", state)
	}
	if err := open().SayToMain("where do things stand"); err != nil {
		t.Fatal(err)
	}
	shows(t, "main", "asker", "where do things stand\nwhere do things stand")
}

// A main agent that ends as it is typed into is not running, and is said
// so: not a window that could not be pasted into.
func TestAMainAgentThatEndsAsItIsSwitchedIsNotRunning(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	manifests := fakeAgents(t, map[string]string{"oneliner": "#!/bin/sh\nread line\n"})
	put(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), "[main]", `agent = "oneliner"`)
	a, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = plugins(t, manifests)
	if _, err := a.StartMain("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	// /model and Enter end it; /effort has nothing to go into
	if _, err := a.SwitchMain("loud", "high"); err == nil || err.Error() != "the main agent is not running — `mia main start`" {
		t.Errorf("switched as it ended: %v", err)
	}
}

// A worktree's agent that asks is not typed into either, and the digit
// the person presses answers it.
func TestAWindowThatAsksIsNotTypedInto(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	dir := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "permission"), []byte(permissionAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Root: dir}
	record := model.Record{Name: "talker", Path: dir}
	if _, err := a.NewWindow(record, "agent", []string{filepath.Join(bin, "permission")}); err != nil {
		t.Fatal(err)
	}
	shows(t, "talker", "agent", "Esc to cancel")
	refused := "window agent asks first: Do you want to proceed? — answer it: `mia window press talker agent -- <n>` (1 Yes · 2 No, and tell Claude what to do differently (esc))"
	if err := a.SendToWindow(record, "agent", "go on"); err == nil || err.Error() != refused {
		t.Fatalf("sent while it asks: %v", err)
	}
	if err := a.PressInWindow(record, "agent", "1"); err != nil {
		t.Fatal(err)
	}
	shows(t, "talker", "agent", "answered 1")
	if err := a.SendToWindow(record, "agent", "go on"); err != nil {
		t.Fatal(err)
	}
	shows(t, "talker", "agent", "go on\ngo on")
}
