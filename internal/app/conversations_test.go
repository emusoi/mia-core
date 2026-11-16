package app_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/session"
)

// quoted is text as a JSON string.
func quoted(text string) string {
	data, _ := json.Marshal(text)
	return string(data)
}

func put(t *testing.T, file string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		t.Fatal(err)
	}
}

// agentHomes points each agent at an empty home, and says where claude
// keeps a folder's conversations there.
func agentHomes(t *testing.T) (claudeDir func(folder string) string) {
	t.Helper()
	claude := filepath.Join(os.Getenv("HOME"), ".claude") // where a carrier looks
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", "")
	return func(folder string) string {
		return filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(folder, "-"))
	}
}

// claudeSays is a line of a claude transcript, made in a folder.
func claudeSays(cwd, kind, content, more string) string {
	return fmt.Sprintf(`{"parentUuid":"p","type":%q,"cwd":%s,"timestamp":"2026-11-02T10:00:00Z","message":{"content":%s}%s}`, kind, quoted(cwd), content, more)
}

func kinds(turns []app.Turn) string {
	var all []string
	for _, turn := range turns {
		all = append(all, fmt.Sprintf("%d:%s", turn.N, turn.Kind))
	}
	return strings.Join(all, " ")
}

func TestAClaudeConversationReadsAsTurns(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	claudeDir := agentHomes(t)
	file := filepath.Join(claudeDir(repo.Root), "c1.jsonl")
	put(t, file,
		`{"type":"ai-title","aiTitle":"The list","sessionId":"c1"}`,
		claudeSays(repo.Root, "user", `"make the list   faster\nplease"`, ""),
		claudeSays(repo.Root, "user", `"<system-reminder>be brief</system-reminder>"`, `,"isMeta":true`),
		claudeSays(repo.Root, "assistant", `[{"type":"thinking","thinking":"where is the time going"}]`, ""),
		claudeSays(repo.Root, "assistant", `[{"type":"text","text":"I will **look**."}]`, ""),
		claudeSays(repo.Root, "assistant", `[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/w/a.go"}},{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/w/b.go"}}]`, ""),
		claudeSays(repo.Root, "user", `[{"type":"tool_result","tool_use_id":"t1","content":"package a"}]`, `,"toolUseResult":{"file":"a.go"}`),
		claudeSays(repo.Root, "assistant", `[{"type":"tool_use","id":"t3","name":"Edit","input":{"file_path":"/w/view.lua","old_string":"a\nb\nc","new_string":"a\nB\nB2\nc"}}]`, ""),
		claudeSays(repo.Root, "assistant", `[{"type":"tool_use","id":"t4","name":"Bash","input":{"command":"go test ./..."}}]`, ""),
		claudeSays(repo.Root, "user", `[{"type":"tool_result","tool_use_id":"t4","content":"[main abc1234def] The list is faster\n 1 file changed"}]`, `,"toolUseResult":{"stdout":"x"}`),
		claudeSays(repo.Root, "assistant", `[{"type":"tool_use","id":"q1","name":"AskUserQuestion","input":{"questions":[{"question":"Which one?","options":[{"label":"Fast"},{"label":"Small"}]}]}}]`, ""),
		claudeSays(repo.Root, "user", `[{"type":"tool_result","tool_use_id":"q1","content":"answered"}]`, `,"toolUseResult":{"questions":[{"question":"Which one?"}],"answers":{"Which one?":"Fast"}}`),
		claudeSays(repo.Root, "assistant", `[{"type":"text","text":"a subagent's own words"}]`, `,"isSidechain":true`),
		claudeSays(repo.Root, "assistant", `[{"type":"text","text":"Done."}]`, ""),
	)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	turns, err := a.ConversationTurns("c1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(turns), "1:asked 2:said 3:tool 4:tool 5:tool 6:commit 7:question 8:answer 9:said"; got != want {
		t.Fatalf("turns = %s, want %s", got, want)
	}
	if turns[0].Text != "make the list   faster\nplease" || turns[1].Text != "I will **look**." {
		t.Errorf("what was asked and said, as written: %+v", turns[:2])
	}
	if turns[2].Text != "Read 2 files" || turns[3].Text != "Edited view.lua +2 −1" || turns[3].File != "/w/view.lua" || turns[4].Text != "Ran go test ./..." || turns[4].Name != "Bash" {
		t.Errorf("each tool in a line: %+v", turns[2:5])
	}
	if turns[5].Hash != "abc1234" || turns[5].Subject != "The list is faster" {
		t.Errorf("the commit made: %+v", turns[5])
	}
	if q := turns[6]; q.Text != "Which one?" || strings.Join(q.Options, ",") != "Fast,Small" || q.Answer != "Fast" || turns[7].Question != 7 || turns[7].Text != "Fast" {
		t.Errorf("the question, its options and its answer: %+v %+v", q, turns[7])
	}

	// a view that polls asks for what came after the last it has
	if later, err := a.ConversationTurns("c1", 9); err != nil || later == nil || len(later) != 0 {
		t.Errorf("nothing new: %+v %v", later, err)
	}
	put(t, file, claudeSays(repo.Root, "user", `"thanks"`, ""), `{"parentUuid":"p","type":"assistant","cwd":"`)
	later, err := a.ConversationTurns("c1", 9)
	if err != nil || kinds(later) != "10:asked" {
		t.Errorf("only what is new, and not a line still being written: %s %v", kinds(later), err)
	}
	// and one that fell behind what was last read gets the same numbers
	if back, _ := a.ConversationTurns("c1", 6); kinds(back) != "7:question 8:answer 9:said 10:asked" || back[0].Answer != "Fast" {
		t.Errorf("from further back: %s", kinds(back))
	}
	if _, err := a.ConversationTurns("nobody", 0); err == nil {
		t.Error("a conversation there is not is said to be none")
	}
}

func TestConversationsOfEveryAgentAreListedNewestFirst(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	claudeDir := agentHomes(t)
	home := os.Getenv("HOME")

	put(t, filepath.Join(claudeDir(repo.Root), "c1.jsonl"),
		claudeSays(repo.Root, "user", `"make the list faster"`, ""),
		claudeSays(repo.Root, "assistant", `[{"type":"text","text":"It is\nfaster now."}]`, ""),
		claudeSays(repo.Root, "user", `"thanks"`, ""))
	// a worktree that was removed, and a repository beside this one whose name starts the same
	put(t, filepath.Join(claudeDir(repo.Root+".gone"), "c2.jsonl"), claudeSays(repo.Root+".gone", "user", `"try the other way"`, ""))
	put(t, filepath.Join(claudeDir(repo.Root+"endix"), "c3.jsonl"), claudeSays(repo.Root+"endix", "user", `"not this repository's"`, ""))
	put(t, filepath.Join(claudeDir(repo.Root), "c1", "subagents", "agent-1.jsonl"), claudeSays(repo.Root, "user", `"look into the flag for me"`, ""))

	codex := func(id, cwd string, lines ...string) {
		put(t, filepath.Join(home, ".codex", "sessions", "2026", "11", "02", "rollout-2026-11-02T10-00-00-"+id+".jsonl"),
			append([]string{`{"timestamp":"2026-11-02T10:00:00Z","type":"session_meta","payload":{"id":"` + id + `","cwd":` + quoted(cwd) + `}}`}, lines...)...)
	}
	item := func(payload string) string {
		return `{"timestamp":"2026-11-02T10:01:00Z","type":"response_item","payload":` + payload + `}`
	}
	codex("x1", repo.Root,
		item(`{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>a folder</environment_context>"}]}`),
		item(`{"type":"message","role":"user","content":[{"type":"input_text","text":"rename the flag"}]}`),
		item(`{"type":"reasoning","summary":[]}`),
		item(`{"type":"function_call","name":"shell","call_id":"k1","arguments":"{\"command\":[\"bash\",\"-lc\",\"go vet ./...\"]}"}`),
		item(`{"type":"custom_tool_call","name":"apply_patch","call_id":"k2","input":"*** Begin Patch\n*** Update File: cmd/flag.go\n@@\n-old\n+new\n+newer\n*** End Patch"}`),
		item(`{"type":"function_call_output","call_id":"k1","output":"{\"output\":\"[main def5678] Rename the flag\\n\"}"}`),
		`{"timestamp":"2026-11-02T10:01:00Z","type":"event_msg","payload":{"type":"agent_message","message":"Renamed."}}`,
		item(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Renamed."}]}`))
	codex("x2", "/somewhere/else", item(`{"type":"message","role":"user","content":[{"type":"input_text","text":"another project's"}]}`))

	pi := filepath.Join(home, ".pi", "agent", "sessions", "-"+strings.ReplaceAll(repo.Root, "/", "-")+"--", "2026-11-02T10-00-00-000Z_p1.jsonl")
	put(t, pi,
		`{"type":"session","version":3,"id":"p1","timestamp":"2026-11-02T10:00:00.000Z","cwd":`+quoted(repo.Root)+`}`,
		`{"type":"model_change","id":"m","timestamp":"2026-11-02T10:00:01.000Z"}`,
		`{"type":"message","timestamp":"2026-11-02T10:00:02.000Z","message":{"role":"user","content":[{"type":"text","text":"tidy the docs"}]}}`,
		`{"type":"message","timestamp":"2026-11-02T10:00:03.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"which"},{"type":"text","text":"Reading them."},{"type":"toolCall","id":"a","name":"read","arguments":{"path":"docs/a.md"}},{"type":"toolCall","id":"b","name":"edit","arguments":{"path":"docs/a.md","edits":[{"oldText":"teh","newText":"the"}]}},{"type":"toolCall","id":"c","name":"bash","arguments":{"command":"git commit -am tidy","timeout":10}}]}}`,
		`{"type":"message","timestamp":"2026-11-02T10:00:04.000Z","message":{"role":"toolResult","toolName":"bash","content":[{"type":"text","text":"[main 0123abc] Tidy the docs"}]}}`)

	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 { // read, then from what was kept of it
		heads, problems := a.Conversations(app.Wanted{})
		if len(problems) != 0 || len(heads) != 4 {
			t.Fatalf("four conversations, the other repositories' left out: %+v %v", heads, problems)
		}
		by := map[string]app.Head{}
		for _, h := range heads {
			by[h.ID] = h
		}
		main := filepath.Base(repo.Root)
		if h := by["c1"]; h.Agent != "claude" || h.Worktree != main || h.Gone || h.Asked != "make the list faster" || h.Said != "It is faster now." || h.Turns != 2 || h.Running || h.Cwd != repo.Root || h.Started.IsZero() || h.Active.IsZero() {
			t.Errorf("claude's: %+v", h)
		}
		if h := by["c2"]; h.Worktree != "gone" || !h.Gone || h.Turns != 1 {
			t.Errorf("one whose worktree is gone: %+v", h)
		}
		if h := by["x1"]; h.Agent != "codex" || h.Asked != "rename the flag" || h.Said != "Renamed." || h.Turns != 1 {
			t.Errorf("codex's: %+v", h)
		}
		if h := by["p1"]; h.Agent != "pi" || h.Asked != "tidy the docs" || h.Said != "Reading them." || h.Turns != 1 {
			t.Errorf("pi's: %+v", h)
		}
	}
	if only, _ := a.Conversations(app.Wanted{Agent: "codex"}); len(only) != 1 || only[0].ID != "x1" {
		t.Errorf("one agent's: %+v", only)
	}
	if only, _ := a.Conversations(app.Wanted{Worktree: "gone", Limit: 5}); len(only) != 1 || only[0].ID != "c2" {
		t.Errorf("one worktree's: %+v", only)
	}
	if first, _ := a.Conversations(app.Wanted{Limit: 2}); len(first) != 2 {
		t.Errorf("the first two: %+v", first)
	}

	turns, err := a.ConversationTurns("x1", 0)
	if err != nil || kinds(turns) != "1:asked 2:tool 3:tool 4:commit 5:said" {
		t.Fatalf("codex's turns: %s %v", kinds(turns), err)
	}
	if turns[1].Text != "Ran go vet ./..." || turns[2].Text != "Edited flag.go +2 −1" || turns[2].File != "cmd/flag.go" || turns[3].Hash != "def5678" || turns[3].Subject != "Rename the flag" {
		t.Errorf("codex's tools and commit: %+v", turns[1:4])
	}
	if _, err := a.ConversationTurns("x2", 0); err == nil {
		t.Error("another project's conversation is not read from here")
	}
	turns, err = a.ConversationTurns("p1", 0)
	if err != nil || kinds(turns) != "1:asked 2:said 3:tool 4:tool 5:tool 6:commit" {
		t.Fatalf("pi's turns: %s %v", kinds(turns), err)
	}
	if turns[2].Text != "Read a.md" || turns[3].Text != "Edited a.md +1 −1" || turns[4].Text != "Ran git commit -am tidy" || turns[5].Hash != "0123abc" {
		t.Errorf("pi's tools and commit: %+v", turns[2:])
	}

	// recall finds it whichever agent it was said to, and says the turn
	found := a.Recall("flag", 10)
	if len(found) != 2 {
		t.Fatalf("codex's, and claude's subagent's: %+v", found)
	}
	for _, r := range found {
		switch r.Agent {
		case "codex":
			if r.Session != "x1" || !r.Yours || r.N != 1 || r.Cwd != repo.Root {
				t.Errorf("codex's: %+v", r)
			}
		case "claude":
			if r.Session != "c1" || r.Yours || r.N != 0 {
				t.Errorf("a subagent's is its parent's conversation: %+v", r)
			}
		default:
			t.Errorf("whose? %+v", r)
		}
	}
	if found := a.Recall("tidy the DOCS", 10); len(found) != 1 || found[0].Agent != "pi" || found[0].N != 1 {
		t.Errorf("pi's: %+v", found)
	}
}

// plugins is a plugin host with the programs given, each enabled.
func plugins(t *testing.T, programs map[string]string) plugin.Host {
	t.Helper()
	dir := t.TempDir()
	host := plugin.Host{Dirs: []string{dir}, StatePath: filepath.Join(t.TempDir(), "plugins.json")}
	for name, body := range programs {
		if err := os.WriteFile(filepath.Join(dir, plugin.Prefix+name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := host.Enable(name); err != nil {
			t.Fatal(err)
		}
	}
	return host
}

func echoPlugin(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "mia-echo"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestAPluginAddsAnAgentAndABrokenOneHidesNoOther(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	claudeDir := agentHomes(t)
	put(t, filepath.Join(claudeDir(repo.Root), "c1.jsonl"), claudeSays(repo.Root, "user", `"make the list faster"`, ""))
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = plugins(t, map[string]string{
		"echo":    echoPlugin(t),
		"garbled": "#!/bin/sh\n" + `case "$1" in manifest) echo '{"protocol":1,"agent":{"name":"garbled"}}' ;; *) echo not json ;; esac`,
		"claude":  "#!/bin/sh\n" + `case "$1" in manifest) echo '{"protocol":1,"agent":{"name":"claude"}}' ;; *) echo '[]' ;; esac`,
	})
	heads, problems := a.Conversations(app.Wanted{})
	if len(heads) != 3 || heads[0].ID != "c1" || heads[1].ID != "e2" || heads[1].Agent != "echo" || heads[2].ID != "e1" || heads[2].Asked != "say it back" || heads[2].Worktree != filepath.Base(repo.Root) {
		t.Fatalf("claude's and the plugin's two, newest first: %+v", heads)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "agent garbled") {
		t.Errorf("the one that printed garbage is named once: %v", problems)
	}
	turns, err := a.ConversationTurns("e1", 1)
	if err != nil || kinds(turns) != "2:said" || turns[0].Text != "say it back" {
		t.Errorf("the plugin's turns after the first: %s %v", kinds(turns), err)
	}
	if _, err := a.ConversationTurns("e9", 0); err == nil {
		t.Error("a conversation no agent has is said to be none")
	}
	if got := a.ResumeCommand("echo", "e1"); got != "echo-agent e1" {
		t.Errorf("resume command = %q", got)
	}
}

// lab is a tmux server of the test's own, and programs on PATH for it.
func lab(t *testing.T, programs map[string]string) {
	t.Helper()
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-talk-")
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
	for name, body := range programs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestResumingAPluginAgentsConversationOpensItsWindowOnce(t *testing.T) {
	// the agent says which it is in its pane's title, as its manifest says it does
	lab(t, map[string]string{"echo-agent": `printf '\033]2;echo-agent %s\033\\' "$1"; exec sleep 300`})
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = plugins(t, map[string]string{"echo": echoPlugin(t)})
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	done, err := a.ResumeConversation("e2", "")
	if err != nil {
		t.Fatal(err)
	}
	if done.Already || done.Copied || done.Worktree != record.Name || done.Window.Name != "echo" || done.Command != "echo-agent e2" {
		t.Fatalf("resumed = %+v", done)
	}
	if !session.Here().HasWindow(record.Name, "echo") {
		t.Fatal("no window was opened for it")
	}
	var running app.Head
	for range 250 { // 5s: the agent has to start and title its pane
		heads, _ := a.Conversations(app.Wanted{Agent: "echo"})
		if running = heads[0]; running.Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if running.ID != "e2" || !running.Running || running.Window == nil || running.Window.Name != "echo" {
		t.Fatalf("the newest of the agent's there is the one its window has: %+v", running)
	}
	again, err := a.ResumeConversation("e2", "")
	if err != nil || !again.Already || again.Window.Name != "echo" {
		t.Errorf("a second time says a window has it: %+v %v", again, err)
	}
	if windows := session.Here().Windows(record.Name); len(windows) != 2 {
		t.Errorf("and opens no other: %v", windows)
	}
}

func TestResumingAClaudeConversationInAnotherWorktreeCopiesItThere(t *testing.T) {
	lab(t, map[string]string{"claude": `exec sleep 300`})
	repo := gittest.New(t)
	claudeDir := agentHomes(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	one, err := a.New("one")
	if err != nil {
		t.Fatal(err)
	}
	two, err := a.New("two")
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(claudeDir(one.Path), "c9.jsonl"), claudeSays(one.Path, "user", `"start it here"`, ""))
	put(t, filepath.Join(claudeDir(repo.Root+".gone"), "c2.jsonl"), claudeSays(repo.Root+".gone", "user", `"try the other way"`, ""))

	if _, err := a.ResumeConversation("c2", ""); err == nil || !strings.Contains(err.Error(), "--in") || !strings.Contains(err.Error(), "gone") {
		t.Errorf("one whose worktree is gone is refused, with what to do: %v", err)
	}
	done, err := a.ResumeConversation("c9", two.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !done.Copied || done.Worktree != two.Name || done.Window.Name != "claude" || done.Command != "claude --resume c9" {
		t.Fatalf("resumed = %+v", done)
	}
	copied, err := os.ReadFile(filepath.Join(claudeDir(two.Path), "c9.jsonl"))
	if err != nil || !strings.Contains(string(copied), quoted(two.Path)) || strings.Contains(string(copied), quoted(one.Path)) {
		t.Errorf("the conversation is under the other folder's name, its folder rewritten: %s %v", copied, err)
	}
	if !session.Here().HasWindow(two.Name, "claude") {
		t.Error("no window was opened in the other worktree")
	}
	// what is said there since is not written over by a second resume
	put(t, filepath.Join(claudeDir(two.Path), "c9.jsonl"), claudeSays(two.Path, "user", `"and go on here"`, ""))
	again, err := a.ResumeConversation("c9", two.Name)
	if err != nil || again.Copied {
		t.Fatalf("a second resume there: %+v %v", again, err)
	}
	if kept, _ := os.ReadFile(filepath.Join(claudeDir(two.Path), "c9.jsonl")); !strings.Contains(string(kept), "and go on here") {
		t.Error("the copy was written over")
	}
	if again.Window.Name != "claude-2" {
		t.Errorf("a window beside the first: %+v", again)
	}
	back, err := a.ResumeConversation("c9", one.Name)
	if err != nil || back.Copied || back.Worktree != one.Name {
		t.Errorf("where it was made it is as it was: %+v %v", back, err)
	}
}
