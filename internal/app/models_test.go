package app_test

import (
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

	if _, _, err := a.ArgvWith([]string{"echo-agent"}, "shout", "", ""); err == nil || err.Error() != "echo has no model shout — it has loud, soft" {
		t.Errorf("a model it has not: %v", err)
	}
	if _, _, err := a.ArgvWith([]string{"sh", "-c", "make"}, "loud", "", ""); err == nil || !strings.Contains(err.Error(), "runs no agent") {
		t.Errorf("a model for no agent: %v", err)
	}
	if argv, chosen, err := a.ArgvWith([]string{"sh", "-c", "make"}, "", "", ""); err != nil || len(argv) != 3 || chosen != (app.Chosen{}) {
		t.Errorf("no agent, nothing chosen: %v %+v %v", argv, chosen, err)
	}
	argv, chosen, err := a.ArgvWith([]string{"echo-agent", "say it"}, "loud", "", "")
	if err != nil || !slices.Equal(argv, []string{"echo-agent", "--model", "loud", "--effort", "low", "say it"}) || chosen != (app.Chosen{Model: "loud", Effort: "low"}) {
		t.Fatalf("started with = %q, %+v, %v", argv, chosen, err)
	}
	if _, err := a.NewWindow(record, "echo", session.What{}, argv); err != nil {
		t.Fatal(err)
	}
	if err := a.KeepChosen(record, "echo", "echo", chosen); err != nil {
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

	// each start and switch is a setup used there, the later used first of a tie
	setups := a.Setups(filepath.Base(a.Root))
	if len(setups) != 4 || !setups[0].Usual || setups[0].Model != "loud" || setups[0].Effort != "high" || setups[0].Last.Worktree != record.Name || setups[0].Last.Machine != "local" ||
		setups[1].Model != "soft" || setups[2].Model != "loud" || setups[2].Effort != "low" || setups[3] != (app.Setup{Agent: "echo", Effort: "low"}) {
		t.Errorf("setups = %+v", setups)
	}

	if _, err := a.NewWindow(record, "shell", session.What{}, []string{"sh"}); err != nil {
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
	if _, err := open().StartMain("", "", "medium", ""); err == nil || err.Error() != "echo has no effort medium — it has low, high" {
		t.Errorf("an effort it has not: %v", err)
	}
	if _, err := os.Stat(mainHome()); err == nil {
		t.Error("its folder was made for a start that was refused")
	}
	state, err := open().StartMain("", "", "high", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { open().StopMain() })
	screenOf(t, "main", "echo", "echo-agent --model soft --effort high e2")
	if state.Model != "soft" || state.Effort != "high" {
		t.Errorf("started: %+v", state)
	}
	if _, err := open().StartMain("", "loud", "", ""); err == nil || !strings.Contains(err.Error(), "mia main model") {
		t.Errorf("started again with a model: %v", err)
	}
	if now, err := open().SwitchMain("loud", ""); err != nil || now != (app.Chosen{Model: "loud", Effort: "high"}) {
		t.Fatalf("switched: %+v, %v", now, err)
	}
	screenOf(t, "main", "echo", "/model loud\n/model loud")
	if state := open().Main(); state.Model != "loud" || state.Effort != "high" {
		t.Errorf("as switched: %+v", state)
	}
	// the main agent's setups are in its folder's name, main
	if setups := open().Setups("main"); len(setups) != 3 || !setups[0].Usual || setups[0].Model != "loud" || setups[0].Last.Worktree != "main" ||
		setups[1].Model != "soft" || setups[1].Effort != "high" || setups[2] != (app.Setup{Agent: "echo", Model: "soft"}) {
		t.Errorf("setups = %+v", setups)
	}
}

// The main agent is started as another agent of the table for one start:
// with that agent's own model and effort and not [main]'s, and with its
// notes. One that runs is not started as another; [main] is as it was.
// The other is a plugin's too: a login shell puts the real codex and pi
// before the test's own.
func TestTheMainAgentIsStartedAsAnotherAgent(t *testing.T) {
	lab(t, map[string]string{"echo-agent": `echo "echo-agent $*"; exec cat`, "hush-agent": `echo "hush-agent $*"; exec cat`})
	t.Setenv("MIA_WORKTREE", "")
	put(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), "[main]", `agent = "echo"`, `model = "soft"`, "[agents.hush]", `effort = "low"`)
	hush := strings.ReplaceAll(strings.ReplaceAll(echoPlugin(t), "echo-agent", "hush-agent"), `"name":"echo"`, `"name":"hush"`)
	open := func() *app.App {
		t.Helper()
		a, err := app.AtMain()
		if err != nil {
			t.Fatal(err)
		}
		a.Plugins = plugins(t, map[string]string{"echo": echoPlugin(t), "hush": hush})
		return a
	}
	if _, err := open().StartMain("nobody", "", "", ""); err == nil || !strings.Contains(err.Error(), "nobody") {
		t.Errorf("an agent there is not: %v", err)
	}
	if _, err := open().StartMain("hush", "shout", "", ""); err == nil || !strings.Contains(err.Error(), "hush has no model shout") {
		t.Errorf("a model it has not: %v", err)
	}
	state, err := open().StartMain("hush", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { open().StopMain() })
	if screen := screenOf(t, "main", "hush", "hush-agent --effort low e2"); strings.Contains(screen, "soft") {
		t.Errorf("started with [main]'s model, which is echo's: %q", screen)
	}
	if !state.Running || state.Agent != "hush" || state.Window == nil || state.Window.Name != "hush" || state.Model != "" || state.Effort != "low" {
		t.Errorf("started: %+v", state)
	}
	if _, err := os.Stat(filepath.Join(mainHome(), "AGENTS.md")); err != nil {
		t.Errorf("its notes are not where it reads them: %v", err)
	}
	if _, err := open().StartMain("echo", "", "", ""); err == nil || !strings.Contains(err.Error(), "mia main stop") {
		t.Errorf("started as another while one runs: %v", err)
	}
	if again, err := open().StartMain("", "", "", ""); err != nil || again.Agent != "hush" {
		t.Errorf("started again with nothing said, the one that runs is left as it is: %+v %v", again, err)
	}
	if setups := open().Setups("main"); len(setups) == 0 || setups[0].Agent != "hush" || setups[0].Effort != "low" || setups[0].Uses != 1 {
		t.Errorf("setups = %+v", setups)
	}
	if err := open().StopMain(); err != nil {
		t.Fatal(err)
	}
	if state := open().Main(); state.Running || state.Agent != "echo" {
		t.Errorf("stopped, the main agent is [main]'s again: %+v", state)
	}
}

// opencode is started on one of the models it lists, as its minimal
// interface, and switched by being started again in its window on the
// conversation it has open: the window stays, behind the shell mia
// started it by. A conversation says the model opencode says it runs on,
// until mia chooses one for its window.
func TestOpencodeIsStartedOnAModelItListsAndSwitchedByStartingItAgain(t *testing.T) {
	// the list, an export and its models from $TMUX_TMPDIR/store; anything else is it started: it says how, and waits
	lab(t, map[string]string{"opencode": `store="$TMUX_TMPDIR/store"
case "$1 $2" in
"api GET") exec cat "$store/sessions.json" ;;
"session export") exec cat "$store/$3.json" ;;
"models ") exec cat "$store/models" ;;
esac
echo "opencode $*"
cat`,
		// a login shell puts the machine's own programs before the lab's,
		// a real opencode among them: this one keeps the lab's first
		"sh": `[ "$1" = -lc ] && shift && exec /bin/sh -c "$@"
exec /bin/sh "$@"`})
	bin := filepath.Join(os.Getenv("TMUX_TMPDIR"), "bin")
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	if err := os.Symlink(tmux, filepath.Join(bin, "tmux")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	t.Setenv("SHELL", filepath.Join(bin, "sh"))
	if out, err := exec.Command(os.Getenv("SHELL"), "-lc", "command -v opencode").Output(); err != nil || strings.TrimSpace(string(out)) != filepath.Join(bin, "opencode") {
		t.Fatalf("a window's shell finds opencode at %q, not the lab's: no window is opened", out)
	}
	store := filepath.Join(os.Getenv("TMUX_TMPDIR"), "store")
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".local", "share", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	info := `{"id":"ses_cart","location":{"directory":"` + record.Path + `"},"time":{"created":1793613600000,"updated":1793613601000},"model":{"id":"gpt-5","providerID":"openai","variant":"max"}}`
	put(t, filepath.Join(store, "sessions.json"), `{"data":[`+info+`]}`)
	put(t, filepath.Join(store, "ses_cart.json"), `{"info":`+info+`,"messages":[{"type":"user","time":{"created":1793613601000},"text":"fix the cart total"}]}`)
	put(t, filepath.Join(store, "models"), "openai/gpt-5", "ollama/qwen3:4b")

	var its app.AgentModels
	for _, one := range a.AgentModels() {
		if one.Name == "opencode" {
			its = one
		}
	}
	if len(its.Models) != 2 || its.Models[1].ID != "ollama/qwen3:4b" || its.Models[1].Label != "qwen3:4b" || len(its.Efforts) != 0 || its.Live || !its.Again || !its.Installed {
		t.Fatalf("opencode = %+v", its)
	}
	if argv, chosen, err := a.ArgvWith([]string{"opencode"}, "openai/gpt-5", "", ""); err != nil || !slices.Equal(argv, []string{"opencode", "mini", "--model", "openai/gpt-5"}) || chosen != (app.Chosen{Model: "openai/gpt-5"}) {
		t.Errorf("started with = %q, %+v, %v", argv, chosen, err)
	}
	if _, _, err := a.ArgvWith([]string{"opencode"}, "openai/gpt-9", "", ""); err == nil || err.Error() != "opencode has no model openai/gpt-9 — it has openai/gpt-5, ollama/qwen3:4b" {
		t.Errorf("a model it does not list: %v", err)
	}
	if heads, _ := a.Conversations(app.Wanted{Agent: "opencode"}); len(heads) != 1 || heads[0].Model != "openai/gpt-5" || heads[0].Effort != "" || heads[0].Running {
		t.Fatalf("with no window, the model opencode says it runs on: %+v", heads)
	}

	done, err := a.ResumeConversation("ses_cart", "", "", "", "", "")
	if err != nil || done.Command != "opencode --session ses_cart" {
		t.Fatalf("resumed = %+v, %v", done, err)
	}
	screenOf(t, record.Name, "opencode", "opencode --session ses_cart")
	running := func() app.Head {
		t.Helper()
		heads, _ := a.Conversations(app.Wanted{Agent: "opencode"})
		if len(heads) != 1 || !heads[0].Running || heads[0].Window == nil || heads[0].Window.Index != done.Window.Index {
			t.Fatalf("the window has it open, behind its shell: %+v", heads)
		}
		return heads[0]
	}
	if head := running(); head.Model != "openai/gpt-5" {
		t.Errorf("mia chose none: opencode's own: %+v", head)
	}
	if runs := a.WindowRuns(record, "opencode"); runs.Agent != "opencode" || runs.Model != "" || !runs.Reads || runs.Live || !runs.Again {
		t.Errorf("the window runs %+v", runs)
	}

	if _, err := a.SwitchWindow(record, "opencode", "openai/gpt-9", ""); err == nil || !strings.Contains(err.Error(), "has no model openai/gpt-9") {
		t.Errorf("switched to a model it does not list: %v", err)
	}
	if _, err := a.SwitchWindow(record, "opencode", "openai/gpt-5", "high"); err == nil || !strings.Contains(err.Error(), "no efforts of opencode") {
		t.Errorf("switched to an effort: %v", err)
	}
	now, err := a.SwitchWindow(record, "opencode", "ollama/qwen3:4b", "")
	if err != nil || now != (app.Chosen{Model: "ollama/qwen3:4b"}) {
		t.Fatalf("switched: %+v, %v", now, err)
	}
	screenOf(t, record.Name, "opencode", "opencode mini --model ollama/qwen3:4b --session ses_cart")
	if head := running(); head.Model != "ollama/qwen3:4b" {
		t.Errorf("as switched, what mia chose over what opencode last said: %+v", head)
	}
	if runs := a.WindowRuns(record, "opencode"); runs.Agent != "opencode" || runs.Model != "ollama/qwen3:4b" {
		t.Errorf("the window runs %+v", runs)
	}
	windows := a.Windows(record)
	at := slices.IndexFunc(windows, func(w session.Window) bool { return w.Name == "opencode" })
	if at < 0 || windows[at].Index != done.Window.Index || windows[at].Kind != session.KindAgent || windows[at].Panes != 1 {
		t.Errorf("the window is the one it was: %+v", windows)
	}
	// again, from the model mia chose
	if now, err := a.SwitchWindow(record, "opencode", "openai/gpt-5", ""); err != nil || now != (app.Chosen{Model: "openai/gpt-5"}) {
		t.Fatalf("switched back: %+v, %v", now, err)
	}
	screenOf(t, record.Name, "opencode", "opencode mini --model openai/gpt-5 --session ses_cart")
	if setups := a.Setups(filepath.Base(a.Root)); len(setups) != 2 || setups[0].Agent != "opencode" || setups[0].Model != "openai/gpt-5" || setups[1].Model != "ollama/qwen3:4b" {
		t.Errorf("each switch is a setup used: %+v", setups)
	}
}
