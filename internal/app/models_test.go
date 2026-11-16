package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/session"
)

// screenOf waits for a window to show want, and gives what it shows.
func screenOf(t *testing.T, worktree, window, want string) string {
	t.Helper()
	var screen string
	for range 250 {
		if screen, _ = session.Here().Capture(worktree, window, false); strings.Contains(screen, want) {
			return screen
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("window %s never showed %q: %q", window, want, screen)
	return ""
}

// An agent a plugin adds is started in a window with a model, and the
// configured effort, switched to others while it runs, and the window's
// conversation says what mia last chose: nothing here knows claude.
func TestAnAgentsWindowIsStartedWithAModelAndSwitched(t *testing.T) {
	lab(t, map[string]string{"echo-agent": `printf '\033]2;echo-agent %s\033\\' "$*"; echo "echo-agent $*"; exec cat`})
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = plugins(t, map[string]string{"echo": echoPlugin(t)})
	a.Config.Agents = map[string]config.Agent{"echo": {Effort: "low"}}
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := a.ArgvWith([]string{"echo-agent"}, "shout", ""); err == nil || err.Error() != "echo has no model shout — it has loud, soft" {
		t.Errorf("a model it has not: %v", err)
	}
	if _, _, err := a.ArgvWith([]string{"sh", "-c", "make"}, "loud", ""); err == nil || !strings.Contains(err.Error(), "runs no agent") {
		t.Errorf("a model for no agent: %v", err)
	}
	if argv, chosen, err := a.ArgvWith([]string{"sh", "-c", "make"}, "", ""); err != nil || len(argv) != 3 || chosen != (app.Chosen{}) {
		t.Errorf("no agent, nothing chosen: %v %+v %v", argv, chosen, err)
	}
	argv, chosen, err := a.ArgvWith([]string{"echo-agent", "say it"}, "loud", "")
	if err != nil || !slices.Equal(argv, []string{"echo-agent", "--model", "loud", "--effort", "low", "say it"}) || chosen != (app.Chosen{Model: "loud", Effort: "low"}) {
		t.Fatalf("started with = %q, %+v, %v", argv, chosen, err)
	}
	if _, err := a.NewWindow(record, "echo", argv); err != nil {
		t.Fatal(err)
	}
	if err := a.KeepChosen(record, "echo", chosen); err != nil {
		t.Fatal(err)
	}
	screenOf(t, record.Name, "echo", "echo-agent --model loud --effort low say it")

	running := func() app.Head {
		t.Helper()
		for range 250 { // the agent has to start and title its pane
			heads, _ := a.Conversations(app.Wanted{Agent: "echo"})
			if len(heads) > 0 && heads[0].Running {
				return heads[0]
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("its window never had a conversation")
		return app.Head{}
	}
	if head := running(); head.Model != "loud" || head.Effort != "low" {
		t.Errorf("as started: %+v", head)
	}

	if _, err := a.SwitchWindow(record, "echo", "shout", ""); err == nil || !strings.Contains(err.Error(), "loud, soft") {
		t.Errorf("switched to a model it has not: %v", err)
	}
	now, err := a.SwitchWindow(record, "echo", "soft", "high")
	if err != nil || now != (app.Chosen{Model: "soft", Effort: "high"}) {
		t.Fatalf("switched: %+v, %v", now, err)
	}
	// typed and submitted, each line: cat says it back
	screenOf(t, record.Name, "echo", "/model soft\n/model soft\n/effort high\n/effort high")
	if head := running(); head.Model != "soft" || head.Effort != "high" {
		t.Errorf("as switched: %+v", head)
	}
	// a model alone keeps the effort mia last chose
	if now, err := a.SwitchWindow(record, "echo", "loud", ""); err != nil || now != (app.Chosen{Model: "loud", Effort: "high"}) {
		t.Errorf("the model alone: %+v, %v", now, err)
	}

	if _, err := a.NewWindow(record, "shell", []string{"sh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SwitchWindow(record, "shell", "loud", ""); err == nil || !strings.Contains(err.Error(), "runs no agent") {
		t.Errorf("a window of no agent was switched: %v", err)
	}
}

// The main agent is started with [main]'s model and an effort given, said
// to be on them, and switched while it runs; started again, it is told to
// switch instead.
func TestTheMainAgentIsStartedWithAModelAndSwitched(t *testing.T) {
	lab(t, map[string]string{"echo-agent": `echo "echo-agent $*"; exec cat`})
	t.Setenv("MIA_WORKTREE", "")
	put(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), "[main]", `agent = "echo"`, `model = "soft"`)
	open := func() *app.App {
		t.Helper()
		a, err := app.AtMain()
		if err != nil {
			t.Fatal(err)
		}
		a.Plugins = plugins(t, map[string]string{"echo": echoPlugin(t)})
		return a
	}
	if _, err := open().StartMain("", "medium"); err == nil || err.Error() != "echo has no effort medium — it has low, high" {
		t.Errorf("an effort it has not: %v", err)
	}
	if _, err := os.Stat(mainHome()); err == nil {
		t.Error("its folder was made for a start that was refused")
	}
	state, err := open().StartMain("", "high")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { open().StopMain() })
	screenOf(t, "main", "echo", "echo-agent --model soft --effort high e2")
	if state.Model != "soft" || state.Effort != "high" {
		t.Errorf("started: %+v", state)
	}
	if _, err := open().StartMain("loud", ""); err == nil || !strings.Contains(err.Error(), "mia main model") {
		t.Errorf("started again with a model: %v", err)
	}
	if now, err := open().SwitchMain("loud", ""); err != nil || now != (app.Chosen{Model: "loud", Effort: "high"}) {
		t.Fatalf("switched: %+v, %v", now, err)
	}
	screenOf(t, "main", "echo", "/model loud\n/model loud")
	if state := open().Main(); state.Model != "loud" || state.Effort != "high" {
		t.Errorf("as switched: %+v", state)
	}
}
