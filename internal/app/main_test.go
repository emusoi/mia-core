package app_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/names"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/signal"
)

// mainHome is where the main agent's folder is under the test's config home.
func mainHome() string { return filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "main") }

// The whole life of the main agent, with an agent a plugin adds as the
// main one: nothing here knows claude.
func TestTheMainAgentIsStartedOnceSpokenToAndStopped(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-main-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	bin := t.TempDir()
	put(t, filepath.Join(bin, "echo-agent"), "#!/bin/sh", `echo "echo-agent $*"`, `echo "main=$MIA_MAIN worktree=${MIA_WORKTREE-none}"`, "exec cat")
	if err := os.Chmod(filepath.Join(bin, "echo-agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MIA_WORKTREE", "kilwa") // started from a worktree's window
	put(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), "[main]", `agent = "echo"`)

	open := func() *app.App {
		t.Helper()
		a, err := app.AtMain()
		if err != nil {
			t.Fatal(err)
		}
		a.Plugins = plugins(t, map[string]string{"echo": echoPlugin(t)})
		return a
	}
	if state := open().Main(); state.Running || state.Agent != "echo" || state.Session != "mia-main" || state.Window != nil {
		t.Fatalf("before it is started: %+v", state)
	}
	if _, err := os.Stat(filepath.Join(mainHome(), "AGENTS.md")); err == nil {
		t.Fatal("asking where it stands wrote its folder's notes")
	}
	if err := open().SayToMain("anyone?"); err == nil {
		t.Error("something was said to a main agent that is not running")
	}
	if err := open().PressInMain("1"); err == nil || err.Error() != "the main agent is not running — `mia main start`" {
		t.Errorf("a key pressed in a main agent that is not running: %v", err)
	}

	state, err := open().StartMain("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// the plugin's last conversation in the folder is e2, and its resume says how it is gone on with
	if !state.Running || state.Agent != "echo" || state.Window == nil || state.Window.Name != "echo" || state.Conversation != "e2" || state.Since.IsZero() || state.Machine != "local" {
		t.Fatalf("started: %+v", state)
	}
	notes, _ := os.ReadFile(filepath.Join(mainHome(), "AGENTS.md"))
	if strings.Contains(string(notes), "skill") || !strings.Contains(string(notes), "mia api standing") || !strings.Contains(string(notes), "answers later prompts too") {
		t.Errorf("its folder tells it %q", notes)
	}
	registry := names.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "names.json")}
	if owner, held := registry.Owner("main"); !held || owner != state.Home {
		t.Errorf("the name main is %q's, %v: a worktree could take the session's name", owner, held)
	}
	read := func(want string) string {
		t.Helper()
		var screen string
		for range 250 {
			if screen, _ = session.Here().Capture("main", "echo", false); strings.Contains(screen, want) {
				return screen
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("its window never showed %q: %q", want, screen)
		return ""
	}
	read("echo-agent e2")
	read("main=1 worktree=none") // it is the main agent's window, and no worktree's

	windows := session.Here().Windows("main")
	if again, err := open().StartMain("", "", "", ""); err != nil || !again.Running || !slices.Equal(session.Here().Windows("main"), windows) {
		t.Fatalf("started again: %+v, %v, windows %v then %v", again, err, windows, session.Here().Windows("main"))
	}
	if err := open().SayToMain("where do things stand --json -h"); err != nil {
		t.Fatal(err)
	}
	if screen := read("where do things stand --json -h\nwhere do things stand --json -h"); strings.Count(screen, "where do things stand") != 2 {
		t.Errorf("said once and submitted, it is typed and cat's echo: %q", screen)
	}
	// a key is typed and nothing after it: cat, which reads lines, has not been given this one
	if err := open().PressInMain("2"); err != nil {
		t.Fatal(err)
	}
	if screen := read("--json -h\n2"); strings.Count(screen, "2") != 2 { // e2 above, and the key
		t.Errorf("pressed once with no Enter, the key is on the line and cat has not echoed it: %q", screen)
	}
	if err := open().InterruptMain(); err != nil {
		t.Fatal(err)
	}
	read("2^[") // Escape, as a terminal that reads lines shows it, on the line the key is still on

	if err := open().StopMain(); err != nil {
		t.Fatal(err)
	}
	if session.Here().Exists("main") || open().Main().Running {
		t.Error("stopped, and its session is still there")
	}
	if err := open().InterruptMain(); err == nil {
		t.Error("a main agent that is not running was interrupted")
	}
	if _, err := os.Stat(filepath.Join(mainHome(), "AGENTS.md")); err != nil {
		t.Error("stopping took its folder")
	}
}

func TestAnAgentNoOneKnowsIsNotStartedAsTheMainOne(t *testing.T) {
	gittest.Isolate(t)
	put(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), "[main]", `agent = "nobody"`)
	a, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartMain("", "", "", ""); err == nil || !strings.Contains(err.Error(), "nobody") {
		t.Fatalf("started nobody: %v", err)
	}
	if _, err := os.Stat(mainHome()); err == nil {
		t.Error("its folder was made for an agent there is not")
	}
}

// What the main agent did is read from its own conversation, and only
// where the main agent's turns are asked for.
func TestTheMainAgentsTurnsSayWhatItDid(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	claudeDir := agentHomes(t)
	if err := os.MkdirAll(mainHome(), 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := a.MainTurns("", 0); err != nil || len(got) != 0 {
		t.Fatalf("with no conversation yet: %+v, %v", got, err)
	}
	ran := func(command string) string {
		return claudeSays(a.Root, "assistant", `[{"type":"tool_use","id":"t","name":"Bash","input":{"command":`+quoted(command)+`}}]`, "")
	}
	put(t, filepath.Join(claudeDir(a.Root), "c1.jsonl"),
		claudeSays(a.Root, "user", `"answer kijenge, then say where things stand"`, ""),
		ran("cd "+repo.Root+" && mia window press kijenge claude -- 1"),
		ran("mia api standing"),
		claudeSays(a.Root, "assistant", `[{"type":"text","text":"kijenge goes on."}]`, ""))

	if state := a.Main(); state.Running || state.Conversation != "c1" || state.Agent != "claude" {
		t.Fatalf("not running, with a conversation to go on with: %+v", state)
	}
	turns, err := a.MainTurns("", 0)
	if err != nil || kinds(turns) != "1:asked 2:did 3:tool 4:said" {
		t.Fatalf("turns = %s, %v", kinds(turns), err)
	}
	if did := turns[1]; did.Verb != "window press" || did.Text != "Answered kijenge: 1" || did.Repository != filepath.Base(repo.Root) || did.Worktree != "kijenge" {
		t.Errorf("did = %+v", did)
	}
	if turns[2].Text != "Ran mia api standing" {
		t.Errorf("a verb that reads = %+v", turns[2])
	}
	if later, err := a.MainTurns("", 2); err != nil || kinds(later) != "3:tool 4:said" {
		t.Errorf("after 2 = %s, %v", kinds(later), err)
	}
	// the conversation read as any other is as it was
	if plain, err := a.ConversationTurns("c1", 0); err != nil || kinds(plain) != "1:asked 2:tool 3:tool 4:said" || plain[1].Verb != "" {
		t.Errorf("as a conversation = %s, %v", kinds(plain), err)
	}
}

func TestStandingIsEveryRepositorysAgentsAndEachMachineAskedForItsOwn(t *testing.T) {
	// a repository in a temporary folder is not one mia lists: under the real home, as its own test has it
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	dir, err := os.MkdirTemp(home, ".mia-standing-test-")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	gittest.Isolate(t)
	root := filepath.Join(dir, "shop")
	for _, args := range [][]string{{"init", "-q", "-b", "main", root}, {"-C", root, "-c", "user.name=user", "-c", "user.email=user@example.com", "commit", "-q", "--allow-empty", "-m", "first"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	shop, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kilwa", "duluti", "themi", "sekei"} {
		if _, err := shop.NewNamed("task-"+name, "", name); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	for _, one := range []signal.Signal{
		{Worktree: "kilwa", Kind: "asks", Text: "wants to use Bash", Rank: 10, From: "claude", Yes: "1", At: at},
		{Worktree: "kilwa", Kind: "busy", Rank: 60, From: "tests", At: at},
		{Worktree: "duluti", Kind: "done", Rank: 30, From: "claude", UntilSeen: true, At: at},
		{Worktree: "themi", Kind: "busy", Rank: 60, From: "codex", At: at},
		{Worktree: "sekei", Kind: "note", Text: "tests green", Rank: 60, From: "note", At: at},
	} {
		if err := shop.Signals.Set(one); err != nil {
			t.Fatal(err)
		}
	}

	// two machines: one whose mia answers, one that is off
	bin := t.TempDir()
	asked := filepath.Join(bin, "asked")
	put(t, filepath.Join(bin, "ssh"), "#!/bin/sh", `echo asked >> `+asked, `case " $* " in`,
		`*" fedora.invalid "*) echo '{"waiting":[{"repository":"api","root":"/home/user/api","worktree":"sakina","machine":"local","agent":"codex","kind":"asks","line":"which one?","options":["Fast","Small"],"since":"2026-11-02T10:00:00Z"}],"working":[],"done_unread":[],"counts":{"waiting":1,"working":0,"done_unread":0},"machines":[{"name":"local","answers":true}]}' ;;`,
		`*) echo "ssh: connect to host box.invalid port 22: Operation timed out" >&2; exit 255 ;;`, "esac")
	if err := os.Chmod(filepath.Join(bin, "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	machines := runtime.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "runtimes.json")}
	for _, name := range []string{"box", "fedora"} {
		if err := machines.Add(runtime.Machine{Name: name, SSH: name + ".invalid"}); err != nil {
			t.Fatal(err)
		}
	}
	a, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}

	here, err := a.Standing(false)
	if err != nil {
		t.Fatal(err)
	}
	if here.Counts.Waiting != 1 || here.Counts.Working != 1 || here.Counts.DoneUnread != 1 || len(here.Machines) != 1 {
		t.Fatalf("here = %+v", here)
	}
	waiting := here.Waiting[0]
	if waiting.Repository != "shop" || waiting.Root != shop.Root || waiting.Worktree != "kilwa" || waiting.Machine != "local" || waiting.Agent != "claude" ||
		waiting.Kind != "asks" || waiting.Line != "wants to use Bash" || waiting.Yes != "1" || !waiting.Since.Equal(at) {
		t.Errorf("waiting = %+v", waiting)
	}
	if here.Working[0].Worktree != "themi" || here.Working[0].Agent != "codex" || here.DoneUnread[0].Worktree != "duluti" {
		t.Errorf("working = %+v, done and unread = %+v", here.Working, here.DoneUnread)
	}
	if _, err := os.Stat(asked); err == nil {
		t.Error("a machine was asked when only this one was wanted")
	}

	all, err := a.Standing(true)
	if err != nil {
		t.Fatal(err)
	}
	// the longest wait first, each with its machine
	if all.Counts.Waiting != 2 || all.Waiting[0].Worktree != "sakina" || all.Waiting[0].Machine != "fedora" || len(all.Waiting[0].Options) != 2 || all.Waiting[1].Machine != "local" {
		t.Fatalf("waiting everywhere = %+v", all.Waiting)
	}
	if len(all.Machines) != 3 || all.Machines[0] != (app.Answered{Name: "local", Answers: true}) ||
		all.Machines[1].Name != "box" || all.Machines[1].Answers || !strings.Contains(all.Machines[1].Detail, "timed out") ||
		all.Machines[2] != (app.Answered{Name: "fedora", Answers: true}) {
		t.Fatalf("machines = %+v", all.Machines)
	}
	// one that did not answer is not asked again at once, and is still named
	if again, err := a.Standing(true); err != nil || len(again.Machines) != 3 || again.Machines[1].Answers || again.Machines[1].Detail == "" {
		t.Fatalf("asked again = %+v, %v", again.Machines, err)
	}
	if log, _ := os.ReadFile(asked); strings.Count(string(log), "asked") != 3 {
		t.Errorf("ssh ran %d times, want 3: the machine that is off once", strings.Count(string(log), "asked"))
	}

	if err := a.SeeStanding("shop/duluti"); err != nil {
		t.Fatal(err)
	}
	if seen, _ := a.Standing(false); seen.Counts.DoneUnread != 0 || seen.Counts.Waiting != 1 {
		t.Errorf("seen, and still unread: %+v", seen.Counts)
	}
	if err := a.SeeStanding(shop.Root + "/kilwa"); err != nil {
		t.Errorf("a repository by its path: %v", err)
	}
	for _, bad := range []string{"nowhere/kilwa", "shop/nobody", "kilwa"} {
		if err := a.SeeStanding(bad); err == nil {
			t.Errorf("--seen %s was taken", bad)
		}
	}
}

// agentPID is the process of the main agent's one window, named for it.
func agentPID(t *testing.T, agent string) int {
	t.Helper()
	var found []int
	for _, pane := range session.Here().Panes("main") {
		if pane.Window == agent {
			found = append(found, pane.PID)
		}
	}
	if len(found) != 1 {
		t.Fatalf("the main agent's session has %d windows named %s, want one", len(found), agent)
	}
	return found[0]
}

// The main agent is started with a new conversation: it is told to go on
// with none, one that runs is not ended unasked, and one ended in so many
// words is started once, as it was. Started plainly, it goes on as ever.
func TestTheMainAgentIsStartedWithANewConversation(t *testing.T) {
	lab(t, map[string]string{"echo-agent": `echo "echo-agent $* on=${ECHO_HOME##*/}."; exec cat`})
	t.Setenv("MIA_WORKTREE", "")
	put(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), "[main]", `agent = "echo"`)
	if err := os.MkdirAll(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "accounts", "echo", "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	withAccounts := strings.Replace(echoPlugin(t), `"name":"echo",`, `"name":"echo","account_env":"ECHO_HOME",`, 1)
	open := func() *app.App {
		t.Helper()
		a, err := app.AtMain()
		if err != nil {
			t.Fatal(err)
		}
		a.Plugins = plugins(t, map[string]string{"echo": withAccounts})
		return a
	}
	t.Cleanup(func() { open().StopMain() })
	pid := func() int {
		t.Helper()
		return agentPID(t, "echo")
	}
	// working still, as one that has just written is: what the line ends with either way
	const refused = "its conversation and starts a fresh one"

	// each start in turn: what it is given, and what the agent was run with, once
	first := 0
	for _, start := range []struct {
		name                   string
		model, effort, account string
		restart                bool
		ran, refused           string
	}{
		{name: "not running, it is started with no conversation", model: "loud", effort: "high", account: "work", ran: "echo-agent --model loud --effort high on=work."},
		{name: "running, it is not ended unasked", refused: refused},
		{name: "nor with something to start it with", model: "soft", refused: refused},
		{name: "ended in so many words, it is started as it was", restart: true, ran: "echo-agent --model loud --effort high on=work."},
		{name: "a model it has not leaves it running", model: "shout", restart: true, refused: "echo has no model shout"},
		{name: "and with what is said, where something is", model: "soft", restart: true, ran: "echo-agent --model soft --effort high on=work."},
	} {
		was := 0
		if first != 0 {
			was = pid()
		}
		state, err := open().StartMainNew("", start.model, start.effort, start.account, start.restart)
		if start.refused != "" {
			if err == nil || !strings.Contains(err.Error(), start.refused) || pid() != was {
				t.Fatalf("%s: %v, its process %d then %d", start.name, err, was, pid())
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", start.name, err)
		}
		screen := screenOf(t, "main", "echo", start.ran)
		if first = pid(); first == was || strings.Count(screen, "echo-agent") != 1 || strings.Contains(screen, "e2") {
			t.Fatalf("%s: process %d then %d, and its window shows %q", start.name, was, first, screen)
		}
		// e1 and e2 are the ones it left: neither is the one it has now
		if !state.Running || state.Conversation != "" || state.Account != "work" || state.Effort != "high" || !strings.HasSuffix(start.ran, "--model "+state.Model+" --effort high on=work.") {
			t.Fatalf("%s: %+v", start.name, state)
		}
	}
	if turns, err := open().MainTurns("", 0); err != nil || len(turns) != 0 {
		t.Errorf("a conversation with nothing said yet has turns: %+v, %v", turns, err)
	}

	// the line says what ending it would cut short, once it has stopped writing
	for range 250 {
		if !open().Main().Working {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := open().StartMainNew("", "", "", "", false); err == nil || err.Error() != "the main agent is running — `mia main start --new --restart` ends "+refused {
		t.Errorf("idle, and asked for a new conversation: %v", err)
	}

	// stopped and started with nothing said, it goes on with its last
	if err := open().StopMain(); err != nil {
		t.Fatal(err)
	}
	state, err := open().StartMain("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	screenOf(t, "main", "echo", "echo-agent e2 on=.")
	if state.Conversation != "e2" {
		t.Errorf("started plainly: %+v", state)
	}
}

// One that asks something is not ended unasked either, and the line says
// what it asks; ended in so many words, it is.
func TestAMainAgentThatAsksIsGivenANewConversationOnlyWhenToldTo(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	manifests := fakeAgents(t, map[string]string{"asker": permissionAgent})
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
	if _, err := open().StartMain("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { open().StopMain() })
	was := agentPID(t, "asker")
	want := "the main agent asks something: Do you want to proceed? — `mia main start --new --restart` leaves it unanswered, ends its conversation and starts a fresh one"
	if _, err := open().StartMainNew("", "", "", "", false); err == nil || err.Error() != want || agentPID(t, "asker") != was {
		t.Fatalf("asked for a new conversation while it asks: %v", err)
	}
	state, err := open().StartMainNew("", "", "", "", true)
	if err != nil || !state.Running || agentPID(t, "asker") == was {
		t.Fatalf("ended in so many words: %+v, %v", state, err)
	}
}

// The main agent's earlier conversations are those made in its folder,
// with every agent it was run as: listed with the one it has open marked,
// read by id, and gone on with by id — as the agent that had each, and
// never unasked while it runs. One made in a repository is none of its.
func TestTheMainAgentsEarlierConversationsAreListedReadAndGoneOnWith(t *testing.T) {
	lab(t, map[string]string{
		"echo-agent": `echo "echo-agent $*."; exec cat`,
		"claude":     `echo "claude-agent $*."; exec cat`,
	})
	t.Setenv("MIA_WORKTREE", "")
	repo := gittest.New(t)
	claudeDir := agentHomes(t)
	put(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), "[main]", `agent = "echo"`)
	if err := os.MkdirAll(mainHome(), 0o755); err != nil {
		t.Fatal(err)
	}
	open := func() *app.App {
		t.Helper()
		a, err := app.AtMain()
		if err != nil {
			t.Fatal(err)
		}
		a.Plugins = plugins(t, map[string]string{"echo": echoPlugin(t)})
		return a
	}
	t.Cleanup(func() { open().StopMain() })
	home := open().Root
	put(t, filepath.Join(claudeDir(home), "c1.jsonl"),
		claudeSays(home, "user", `"where do things stand"`, ""),
		claudeSays(home, "assistant", `[{"type":"text","text":"All quiet."}]`, ""))
	put(t, filepath.Join(claudeDir(repo.Root), "c2.jsonl"), claudeSays(repo.Root, "user", `"make the list faster"`, ""))
	listed := func() string {
		t.Helper()
		heads, problems := open().MainConversations()
		if len(problems) > 0 {
			t.Fatalf("problems: %v", problems)
		}
		var all []string
		for _, head := range heads {
			one := head.Agent + ":" + head.ID
			if head.Running {
				one += "*"
			}
			all = append(all, one)
		}
		return strings.Join(all, " ")
	}

	// not running: its folder's, of both agents, the last written to first
	if got := listed(); got != "claude:c1 echo:e2 echo:e1" {
		t.Fatalf("with none open: %s", got)
	}
	if turns, err := open().MainTurns("c1", 0); err != nil || kinds(turns) != "1:asked 2:said" {
		t.Errorf("an earlier one's turns = %s, %v", kinds(turns), err)
	}
	if later, err := open().MainTurns("c1", 1); err != nil || kinds(later) != "2:said" {
		t.Errorf("after 1 = %s, %v", kinds(later), err)
	}
	const none = "no conversation c2 of the main agent's — `mia api main conversations` lists them"
	if _, err := open().MainTurns("c2", 0); err == nil || err.Error() != none {
		t.Errorf("a repository's conversation was read as the main agent's: %v", err)
	}

	// started plainly it goes on with its agent's last, which is marked
	if _, err := open().StartMain("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	screenOf(t, "main", "echo", "echo-agent e2.")
	if got := listed(); got != "claude:c1 echo:e2* echo:e1" {
		t.Fatalf("with e2 open: %s", got)
	}
	for range 250 {
		if !open().Main().Working {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// while it runs nothing is ended unasked, and an id has to be its own
	was := agentPID(t, "echo")
	for _, refused := range []struct{ id, agent, account, why string }{
		{id: "e1", why: "the main agent is running — `mia main start --resume e1 --restart` ends its conversation and goes on with that one"},
		{id: "c2", why: none},
		{id: "c1", agent: "echo", why: "conversation c1 is claude's, not echo's — it is gone on with as claude"},
		{id: "c1", account: "work", why: "conversation c1 is kept on claude's account default, not work — it is gone on with there"},
	} {
		if _, err := open().StartMainResuming(refused.id, refused.agent, "", "", refused.account, refused.agent != "" || refused.account != ""); err == nil || err.Error() != refused.why {
			t.Errorf("resuming %s: %v", refused.id, err)
		}
		if agentPID(t, "echo") != was {
			t.Fatalf("resuming %s, refused, ended the agent", refused.id)
		}
	}

	// ended in so many words, it goes on with the one named, once
	state, err := open().StartMainResuming("e1", "", "", "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	screen := screenOf(t, "main", "echo", "echo-agent e1.")
	if agentPID(t, "echo") == was || strings.Count(screen, "echo-agent") != 1 || state.Conversation != "e1" || state.Agent != "echo" {
		t.Fatalf("resumed e1: %+v, its window shows %q", state, screen)
	}
	if got := listed(); got != "claude:c1 echo:e2 echo:e1*" {
		t.Errorf("with e1 open: %s", got)
	}

	// one another agent had is gone on with as that agent
	if state, err = open().StartMainResuming("c1", "", "", "", "", true); err != nil {
		t.Fatal(err)
	}
	screenOf(t, "main", "claude", "claude-agent --resume c1.")
	if state.Agent != "claude" || state.Conversation != "c1" || agentPID(t, "claude") == 0 ||
		slices.ContainsFunc(session.Here().Panes("main"), func(p session.Pane) bool { return p.Window == "echo" }) {
		t.Fatalf("resumed c1: %+v", state)
	}
	if got := listed(); got != "claude:c1* echo:e2 echo:e1" {
		t.Errorf("with c1 open: %s", got)
	}
	if turns, err := open().MainTurns("", 0); err != nil || kinds(turns) != "1:asked 2:said" {
		t.Errorf("the open one's turns = %s, %v", kinds(turns), err)
	}

	// not running, it needs no --restart
	if err := open().StopMain(); err != nil {
		t.Fatal(err)
	}
	if state, err = open().StartMainResuming("e1", "", "", "", "", false); err != nil || state.Conversation != "e1" {
		t.Fatalf("resumed while stopped: %+v, %v", state, err)
	}
	screenOf(t, "main", "echo", "echo-agent e1.")
}

// A main agent that says which conversation it has open and has written
// nothing of it yet — just started with none, or cleared — has that one
// listed, as running, and read as no turns: not as no conversation.
func TestTheMainAgentsConversationNothingIsSaidInYetIsListedAndRead(t *testing.T) {
	lab(t, map[string]string{"claude": `echo "claude-agent $*."; exec cat`})
	t.Setenv("MIA_WORKTREE", "")
	claudeDir := agentHomes(t)
	open := func() *app.App {
		t.Helper()
		a, err := app.AtMain()
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	t.Cleanup(func() { open().StopMain() })
	if _, err := open().StartMain("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	screenOf(t, "main", "claude", "claude-agent .")
	home := open().Root
	listed := func() ([]app.Head, string) {
		t.Helper()
		heads, problems := open().MainConversations()
		if len(problems) > 0 {
			t.Fatalf("problems: %v", problems)
		}
		var all []string
		for _, head := range heads {
			one := fmt.Sprintf("%s:%d", head.ID, head.Turns)
			if head.Running && head.Window != nil {
				one += "*" + head.Window.Name
			}
			all = append(all, one)
		}
		return heads, strings.Join(all, " ")
	}

	// it has had none, and does not say: nothing is listed, and nothing made up
	if _, got := listed(); got != "" {
		t.Fatalf("before it says: %s", got)
	}
	if turns, err := open().MainTurns("", 0); err != nil || len(turns) != 0 {
		t.Errorf("with none open: %v, %v", turns, err)
	}

	saysItHolds(t, "main", "claude", "cat")(`{"sessionId":"m1"}`)
	if state := open().Main(); state.Conversation != "m1" {
		t.Fatalf("it says it has m1: %+v", state)
	}
	heads, got := listed()
	if got != "m1:0*claude" {
		t.Fatalf("holding one nothing is said in: %s", got)
	}
	if fresh := heads[0]; fresh.Agent != "claude" || fresh.Account != app.DefaultAccount || fresh.Cwd != home || fresh.Worktree != filepath.Base(home) ||
		fresh.Asked != "" || fresh.Said != "" || fresh.Started.IsZero() {
		t.Errorf("its head = %+v", fresh)
	}
	for _, id := range []string{"", "m1"} {
		if turns, err := open().MainTurns(id, 0); err != nil || turns == nil || len(turns) != 0 {
			t.Errorf("turns of %q = %v, %v: want none, and no error", id, turns, err)
		}
	}
	if _, err := open().MainTurns("m9", 0); err == nil {
		t.Error("one it does not have and nothing is written of was read")
	}

	// the first thing is said: the same conversation, listed once
	put(t, filepath.Join(claudeDir(home), "m1.jsonl"), claudeSays(home, "user", `"where do things stand"`, ""))
	if _, got := listed(); got != "m1:1*claude" {
		t.Errorf("once something is said: %s", got)
	}
	if turns, err := open().MainTurns("", 0); err != nil || kinds(turns) != "1:asked" {
		t.Errorf("its turns = %s, %v", kinds(turns), err)
	}

	// cleared: the new one is first, and the one it left a past one
	saysItHolds(t, "main", "claude", "cat")(`{"sessionId":"m2"}`)
	if _, got := listed(); got != "m2:0*claude m1:1" {
		t.Errorf("after it is cleared: %s", got)
	}
}
