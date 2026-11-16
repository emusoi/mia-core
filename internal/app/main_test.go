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

	state, err := open().StartMain()
	if err != nil {
		t.Fatal(err)
	}
	// the plugin's last conversation in the folder is e2, and its resume says how it is gone on with
	if !state.Running || state.Agent != "echo" || state.Window == nil || state.Window.Name != "echo" || state.Conversation != "e2" || state.Since.IsZero() || state.Machine != "local" {
		t.Fatalf("started: %+v", state)
	}
	notes, _ := os.ReadFile(filepath.Join(mainHome(), "AGENTS.md"))
	if !strings.Contains(string(notes), "mia-captain") || !strings.Contains(string(notes), "answers later prompts too") {
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
	if again, err := open().StartMain(); err != nil || !again.Running || !slices.Equal(session.Here().Windows("main"), windows) {
		t.Fatalf("started again: %+v, %v, windows %v then %v", again, err, windows, session.Here().Windows("main"))
	}
	if err := open().SayToMain("where do things stand --json -h"); err != nil {
		t.Fatal(err)
	}
	if screen := read("where do things stand --json -h\nwhere do things stand --json -h"); strings.Count(screen, "where do things stand") != 2 {
		t.Errorf("said once and submitted, it is typed and cat's echo: %q", screen)
	}
	if err := open().InterruptMain(); err != nil {
		t.Fatal(err)
	}
	read("^[") // Escape, as a terminal that reads lines shows it

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
	if _, err := a.StartMain(); err == nil || !strings.Contains(err.Error(), "nobody") {
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
	if got, err := a.MainTurns(0); err != nil || len(got) != 0 {
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
	turns, err := a.MainTurns(0)
	if err != nil || kinds(turns) != "1:asked 2:did 3:tool 4:said" {
		t.Fatalf("turns = %s, %v", kinds(turns), err)
	}
	if did := turns[1]; did.Verb != "window press" || did.Text != "Answered kijenge: 1" || did.Repository != filepath.Base(repo.Root) || did.Worktree != "kijenge" {
		t.Errorf("did = %+v", did)
	}
	if turns[2].Text != "Ran mia api standing" {
		t.Errorf("a verb that reads = %+v", turns[2])
	}
	if later, err := a.MainTurns(2); err != nil || kinds(later) != "3:tool 4:said" {
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
