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
