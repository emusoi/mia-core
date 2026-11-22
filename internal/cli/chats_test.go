package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/runtime"
)

// The chats are the person's, as the main agent is: listed from any folder
// with the main agent first, and the chat named main is the main agent.
func TestTheChatsAnswerOutsideARepositoryAndMainIsOne(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())

	data, code := answered(t, "chats")
	var chats []app.ChatState
	if err := json.Unmarshal(data, &chats); err != nil || code != exitOK || len(chats) != 1 || chats[0].Name != "main" || !chats[0].Main || chats[0].Title != "mia" || chats[0].Agent != "claude" || chats[0].Machine != "local" {
		t.Fatalf("api chats = %s, exit %d, %v", data, code, err)
	}
	for _, same := range [][2][]string{
		{{"chat", "main"}, {"main"}},
		{{"chat", "turns", "main", "--after", "3"}, {"main", "turns", "--after", "3"}},
	} {
		chat, chatCode := answered(t, same[0]...)
		main, mainCode := answered(t, same[1]...)
		if string(chat) != string(main) || chatCode != mainCode {
			t.Errorf("api %v = %s, exit %d; api %v = %s, exit %d", same[0], chat, chatCode, same[1], main, mainCode)
		}
	}
	for _, bad := range [][]string{{"chat"}, {"chats", "x"}, {"chat", "turns"}, {"chat", "main", "--after", "1"}, {"chat", "turns", "main", "--after", "soon"}} {
		if _, code := answered(t, bad...); code != exitUsage {
			t.Errorf("api %v exited %d", bad, code)
		}
	}
	if _, code := answered(t, "chat", "nobody"); code != exitFailed {
		t.Errorf("api chat of a chat there is not exited %d", code)
	}
	for _, bad := range [][]string{{"chats", "new"}, {"chats", "new", "--agent"}, {"chats", "say", "goba"}, {"chats", "say", "goba", "--"}, {"chats", "press", "goba"}, {"chats", "stop"}, {"chats", "start", "a", "b"}, {"chats", "restart", "goba"}, {"chats", "model", "goba"}, {"chats", "switch", "goba"}, {"chats", "switch", "--agent", "codex"}, {"main", "switch"}, {"main", "switch", "codex"}} {
		if code := Run(bad); code != exitUsage {
			t.Errorf("%v exited %d", bad, code)
		}
	}
	// what is said is its own, and refused only because there is no such chat
	if code := Run([]string{"chats", "say", "goba", "--", "what does", "--json", "mean"}); code != exitFailed {
		t.Errorf("say to a chat there is not exited %d", code)
	}
	if code := Run([]string{"chats", "new", "--agent", "nobody", "--", "hello"}); code != exitFailed {
		t.Errorf("a chat with an agent there is not exited %d", code)
	}
	if code := Run([]string{"chats", "remove", "main"}); code != exitFailed {
		t.Errorf("removing the main agent exited %d", code)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "chats")); err == nil {
		t.Error("nothing made, and the chats' folder is there")
	}
}

// The chats live where the main agent does, and from another machine
// every verb about them is that machine's to do.
func TestTheChatsOnAnotherMachineAreAskedThere(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())
	t.Setenv("MIA_MAIN_HERE", "")
	bin := t.TempDir()
	log := filepath.Join(bin, "asked")
	ssh := `#!/bin/sh
for arg do command=$arg; done
printf '%s\n' "$command" >> ` + log + `
case "$command" in
*"'api' 'chats'"*) echo '[{"name":"main","main":true,"agent":"claude","model":"","effort":"","title":"mia","last":"","asks":false,"working":false,"running":true,"machine":"local"}]' ;;
*"'api' 'chat' 'goba'"*) echo '{"name":"goba","running":true,"agent":"codex","machine":"local","home":"/home/user/.config/mia/chats/goba","session":"mia-chat-goba","conversation":"x1","working":false}' ;;
*"'chats' 'new'"*) echo goba ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	machines := runtime.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "runtimes.json")}
	if err := machines.Add(runtime.Machine{Name: "fedora", SSH: "fedora.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), []byte("[main]\nmachine = \"fedora\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var chats []app.ChatState
	if data, code := answered(t, "chats"); json.Unmarshal(data, &chats) != nil || code != exitOK || len(chats) != 1 || chats[0].Machine != "fedora" {
		t.Errorf("api chats = %s, exit %d", data, code)
	}
	var state app.MainState
	if data, code := answered(t, "chat", "goba"); json.Unmarshal(data, &state) != nil || code != exitOK || state.Machine != "fedora" || state.Name != "goba" {
		t.Errorf("api chat goba = %s, exit %d", data, code)
	}
	if data, code := printed(t, "chats", "new", "--agent", "codex", "--", "look at", "--json"); code != exitOK || string(data) != "goba\n" {
		t.Errorf("chats new = %q, exit %d", data, code)
	}
	Run([]string{"chats", "say", "goba", "--", "and -h", "--json"})
	Run([]string{"chats", "switch", "goba", "--agent", "pi", "--model", "sonnet"})
	Run([]string{"main", "switch", "--agent", "codex", "--effort", "high"})
	if _, code := answered(t, "chat", "turns", "goba", "--follow"); code != exitFailed {
		t.Errorf("following a chat on another machine exited %d", code)
	}
	data, _ := os.ReadFile(log)
	for _, want := range []string{"MIA_MAIN_HERE=1 $HOME/.local/bin/mia", "'chats' 'new' '--agent' 'codex' '--' 'look at' '--json'", "'chats' 'say' 'goba' '--' 'and -h' '--json'", "'chats' 'switch' 'goba' '--agent' 'pi' '--model' 'sonnet'", "'main' 'switch' '--agent' 'codex' '--effort' 'high'"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("fedora was not asked %s:\n%s", want, data)
		}
	}
}
