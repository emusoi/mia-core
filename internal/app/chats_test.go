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
	"github.com/emusoi/mia-core/internal/names"
	"github.com/emusoi/mia-core/internal/session"
)

// A chat is a main agent of its own: made with a name from the registry,
// started in a folder and a session of its own with its first words said
// once it is ready, spoken to, stopped and removed, with an agent a plugin
// adds, and listed after the main agent.
func TestAChatIsMadeStartedSpokenToAndRemoved(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-chat-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	bin := t.TempDir()
	put(t, filepath.Join(bin, "echo-agent"), "#!/bin/sh", `echo "echo-agent $*"`, `echo "chat=$MIA_CHAT main=${MIA_MAIN-none} worktree=${MIA_WORKTREE-none}"`, "exec cat")
	if err := os.Chmod(filepath.Join(bin, "echo-agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MIA_WORKTREE", "kilwa") // made from a worktree's window
	echo := plugins(t, map[string]string{"echo": echoPlugin(t)})
	main, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}
	main.Plugins = echo

	if _, err := main.NewChat("nobody", "", "", ""); err == nil {
		t.Error("a chat was made with an agent there is not")
	}
	if _, err := main.NewChat("echo", "shout", "", ""); err == nil || !strings.Contains(err.Error(), "loud") {
		t.Errorf("a chat was made with a model its agent has not: %v", err)
	}
	c, err := main.NewChat("echo", "soft", "", "  say   it back\nplease ")
	if err != nil {
		t.Fatal(err)
	}
	c.Plugins = echo
	name := c.Main().Name
	home := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "chats", name)
	registry := names.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "names.json")}
	if owner, held := registry.Owner(name); name == "" || name == "main" || !held || owner != c.Root {
		t.Fatalf("the chat is %q, its name %q's, %v", name, owner, held)
	}
	if state := c.Main(); state.Running || state.Agent != "echo" || state.Session != "mia-chat-"+name {
		t.Fatalf("made, not started: %+v", state)
	}

	state, err := c.StartMain("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Running || state.Agent != "echo" || state.Window == nil || state.Window.Name != "echo" || state.Model != "soft" || state.Name != name {
		t.Fatalf("started: %+v", state)
	}
	read := func(want string) string {
		t.Helper()
		var screen string
		for range 250 {
			if screen, _ = session.Here().Capture("chat-"+name, "echo", false); strings.Contains(screen, want) {
				return screen
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("its window never showed %q: %q", want, screen)
		return ""
	}
	read("echo-agent --model soft") // with the model it was made with
	read("chat=" + name + " main=none worktree=none")
	notes, _ := os.ReadFile(filepath.Join(home, "AGENTS.md"))
	if !strings.HasPrefix(string(notes), "# mia chat "+name+"\n") || !strings.Contains(string(notes), "mia api standing") || strings.Contains(string(notes), "the main agent:") {
		t.Errorf("its folder tells it %q", notes)
	}

	if sent, err := c.SayWaiting(); err != nil || !sent {
		t.Fatalf("its first words: %v, %v", sent, err)
	}
	read("say   it back\npleasesay   it back") // as they were given, and Enter after
	if sent, err := c.SayWaiting(); err != nil || sent {
		t.Errorf("its first words are said once: %v, %v", sent, err)
	}

	chats := main.Chats()
	if len(chats) != 2 || chats[0].Name != "main" || !chats[0].Main || chats[0].Title != "mia" || chats[0].Running {
		t.Fatalf("chats = %+v", chats)
	}
	if one := chats[1]; one.Name != name || one.Main || one.Agent != "echo" || one.Model != "soft" || !one.Running || one.Title != "say it back please" || one.At.IsZero() || one.Machine != "local" {
		t.Errorf("the chat = %+v", one)
	}

	if err := c.SayToMain("and again"); err != nil {
		t.Fatal(err)
	}
	read("and again\nand again")
	if err := c.InterruptMain(); err != nil {
		t.Fatal(err)
	}
	if err := c.StopMain(); err != nil {
		t.Fatal(err)
	}
	if session.Here().Exists("chat-"+name) || c.Main().Running {
		t.Error("stopped, and its session is still there")
	}
	if err := c.SayToMain("anyone?"); err == nil || err.Error() != "chat "+name+" is not running — `mia chats start "+name+"`" {
		t.Errorf("said to a chat that is not running: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "AGENTS.md")); err != nil {
		t.Error("stopping took its folder")
	}

	if err := main.RemoveChat("main"); err == nil {
		t.Error("the main agent was removed as a chat")
	}
	if err := main.RemoveChat(name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); err == nil {
		t.Error("removed, and its folder is there")
	}
	if _, held := registry.Owner(name); held {
		t.Error("removed, and its name is still held")
	}
	if _, err := app.AtChat(name); err == nil {
		t.Error("a removed chat is opened")
	}
	if chats := main.Chats(); len(chats) != 1 {
		t.Errorf("chats once it is removed = %+v", chats)
	}
	for _, bad := range []string{"", "..", "a/b"} {
		if _, err := app.AtChat(bad); err == nil {
			t.Errorf("chat %q is opened", bad)
		}
	}
}

// A chat's first words wait while its agent asks something first, as
// claude does in a folder it has not seen, and are said once it is
// answered: what is typed then is not taken for the answer.
func TestAChatsFirstWordsWaitForWhatItAsksFirst(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	manifests := fakeAgents(t, map[string]string{"asker": trustAgent})
	main, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}
	main.Plugins = plugins(t, manifests)
	c, err := main.NewChat("asker", "", "", "what is waiting")
	if err != nil {
		t.Fatal(err)
	}
	c.Plugins = main.Plugins
	state, err := c.StartMain("", "", "", "")
	if err != nil || state.Asks == nil {
		t.Fatalf("started: %+v, %v", state, err)
	}
	if sent, err := c.SayWaiting(); err != nil || sent {
		t.Fatalf("said over its question: %v, %v", sent, err)
	}
	if err := c.SayToMain("hello"); err == nil || !strings.Contains(err.Error(), "`mia chats press "+state.Name+" <n>`") {
		t.Errorf("typed over its question: %v", err)
	}
	if err := c.PressInMain("2"); err != nil {
		t.Fatal(err)
	}
	var screen string
	for range 250 {
		if screen, _ = session.Here().Capture("chat-"+state.Name, "asker", false); strings.Contains(screen, "what is waiting\nwhat is waiting") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(screen, "trusted") || !strings.Contains(screen, "what is waiting\nwhat is waiting") {
		t.Errorf("answered, its first words are said: %q", screen)
	}
	if sent, err := c.SayWaiting(); err != nil || sent {
		t.Errorf("its first words are said once: %v, %v", sent, err)
	}
}

// A chat switched to another agent goes on with its conversation as a
// window's does: written out, the other agent started in its place and
// sent it in front of the first thing said, its turns those it went on
// from, a switch, and its own; switched back, the agent it was with takes
// its own conversation up again and is handed only what was said since.
func TestAChatSwitchedToAnotherAgentCarriesItsConversationOver(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	claudeDir := agentHomes(t)
	bin := t.TempDir()
	for _, agent := range []string{"echo-agent", "claude"} {
		put(t, filepath.Join(bin, agent), "#!/bin/sh", `echo "`+agent+` $*"`, "exec cat")
		if err := os.Chmod(filepath.Join(bin, agent), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	main, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}
	main.Plugins = plugins(t, map[string]string{"echo": echoPlugin(t)})
	c, err := main.NewChat("echo", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c.Plugins = main.Plugins
	state, err := c.StartMain("", "", "", "")
	if err != nil || state.Conversation != "e2" {
		t.Fatalf("started: %+v, %v", state, err)
	}
	name := state.Name
	read := func(window, want string) {
		t.Helper()
		var screen string
		for range 250 {
			if screen, _ = session.Here().Capture("chat-"+name, window, false); strings.Contains(screen, want) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("its window never showed %q: %q", want, screen)
	}
	if _, err := c.SwitchChat("echo", "", ""); err == nil {
		t.Error("switched to the agent it runs as")
	}
	if _, err := c.SwitchChat("claude", "", ""); err == nil || !strings.Contains(err.Error(), "is at work") {
		t.Errorf("switched while it writes: %v", err)
	}
	time.Sleep(3200 * time.Millisecond) // what it wrote as it started is no work

	done, err := c.SwitchChat("claude", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if done.From != "echo" || done.To != "claude" || done.Turns != 2 || done.Resumes != "" || !done.State.Running || done.State.Agent != "claude" {
		t.Fatalf("switched: %+v", done)
	}
	handoff, err := os.ReadFile(done.Handoff)
	if err != nil || !strings.Contains(string(handoff), "say it back") {
		t.Fatalf("its handoff: %q, %v", handoff, err)
	}
	read("claude", "claude") // anew, going on with nothing of its own
	if again, err := app.AtChat(name); err != nil || again.Main().Agent != "claude" {
		t.Errorf("stopped and started again it would be %+v, %v", again.Main(), err)
	}
	turns, err := c.MainTurns("", 0)
	if err != nil || kinds(turns) != "1:asked 2:said 3:switch" || turns[2].From != "echo" || turns[2].To != "claude" || turns[2].Note != app.CarriedWaits || turns[2].Tone != "warn" {
		t.Fatalf("turns before anything is said to it = %s %+v, %v", kinds(turns), turns, err)
	}

	if err := c.SayToMain("go on"); err != nil {
		t.Fatal(err)
	}
	read("claude", app.CarriedOpens("echo"))
	read("claude", "go on")
	now := time.Now().UTC()
	line := func(kind, content string, at time.Time) string {
		return `{"parentUuid":"p","type":"` + kind + `","cwd":` + quoted(c.Root) + `,"timestamp":"` + at.Format(time.RFC3339Nano) + `","message":{"content":` + content + `}}`
	}
	put(t, filepath.Join(claudeDir(c.Root), "c9.jsonl"),
		line("user", quoted(app.CarriedText("echo", string(handoff), done.Handoff, "go on", app.CarriedMost)), now),
		line("assistant", `[{"type":"text","text":"on it"}]`, now.Add(time.Second)))
	turns, err = c.MainTurns("", 0)
	if err != nil || kinds(turns) != "1:asked 2:said 3:switch 4:asked 5:said" || turns[2].Note != "context handed over · 2 turns" || turns[3].Text != "go on" || turns[4].Text != "on it" {
		t.Fatalf("turns once it is said to = %s %+v, %v", kinds(turns), turns, err)
	}
	if later, err := c.MainTurns("", 3); err != nil || kinds(later) != "4:asked 5:said" {
		t.Errorf("after 3 = %s, %v", kinds(later), err)
	}

	time.Sleep(3200 * time.Millisecond)
	back, err := c.SwitchChat("echo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if back.From != "claude" || back.To != "echo" || back.Resumes != "e2" || back.Turns != 2 {
		t.Fatalf("switched back: %+v", back)
	}
	read("echo", "echo-agent e2")
	turns, err = c.MainTurns("", 0)
	if err != nil || kinds(turns) != "1:asked 2:said 3:switch 4:asked 5:said 6:switch" || turns[5].From != "claude" || turns[5].To != "echo" {
		t.Fatalf("turns switched back = %s %+v, %v", kinds(turns), turns, err)
	}
	stop := make(chan struct{})
	var first []app.Turn
	err = c.FollowMain("", 4, stop, func(got []app.Turn) error {
		if first == nil {
			first = got
			close(stop)
		}
		return nil
	})
	if err != nil || kinds(first) != "5:said 6:switch" {
		t.Errorf("followed after 4 = %s, %v", kinds(first), err)
	}
}

// The main agent is switched as a chat is, and stays with the agent it
// was switched to when it is started again, until it is switched back to
// the one [main] names.
func TestTheMainAgentSwitchedStaysWithTheOtherAgent(t *testing.T) {
	gittest.Isolate(t)
	ownTmux(t)
	agentHomes(t)
	bin := t.TempDir()
	for _, agent := range []string{"echo-agent", "claude"} {
		put(t, filepath.Join(bin, agent), "#!/bin/sh", `echo "`+agent+` $*"`, "exec cat")
		if err := os.Chmod(filepath.Join(bin, agent), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	put(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), "[main]", `agent = "echo"`)
	echo := plugins(t, map[string]string{"echo": echoPlugin(t)})
	open := func() *app.App {
		t.Helper()
		a, err := app.AtMain()
		if err != nil {
			t.Fatal(err)
		}
		a.Plugins = echo
		return a
	}
	if _, err := open().StartMain("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3200 * time.Millisecond)
	if done, err := open().SwitchChat("claude", "", ""); err != nil || done.Turns != 2 || done.State.Name != "main" || done.State.Agent != "claude" {
		t.Fatalf("switched: %+v, %v", done, err)
	}
	if err := open().StopMain(); err != nil {
		t.Fatal(err)
	}
	if state := open().Main(); state.Running || state.Agent != "claude" {
		t.Fatalf("stopped, it would start as %+v", state)
	}
	if state, err := open().StartMain("", "", "", ""); err != nil || state.Agent != "claude" {
		t.Fatalf("started again: %+v, %v", state, err)
	}
	time.Sleep(3200 * time.Millisecond)
	if _, err := open().SwitchChat("echo", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := open().StopMain(); err != nil {
		t.Fatal(err)
	}
	if state := open().Main(); state.Agent != "echo" {
		t.Errorf("switched back, it would start as %+v", state)
	}
}
