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
	"github.com/emusoi/mia-core/internal/session"
)

// talked puts a conversation with claude and one with pi in a worktree, as
// each agent writes its own.
func talked(t *testing.T, claudeDir func(string) string, cwd string) {
	t.Helper()
	t.Setenv("PI_CODING_AGENT_DIR", "")
	put(t, filepath.Join(claudeDir(cwd), "c1.jsonl"),
		claudeSays(cwd, "user", `"make the list faster"`, ""),
		claudeSays(cwd, "assistant", `[{"type":"thinking","thinking":"where is the time going"}]`, ""),
		claudeSays(cwd, "assistant", `[{"type":"text","text":"I will **look**."}]`, ""),
		claudeSays(cwd, "assistant", `[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/w/a.go"}},{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/w/b.go"}}]`, ""),
		claudeSays(cwd, "user", `[{"type":"tool_result","tool_use_id":"t1","content":"package a // what the file had"}]`, `,"toolUseResult":{"file":"a.go"}`),
		claudeSays(cwd, "assistant", `[{"type":"tool_use","id":"t3","name":"Edit","input":{"file_path":"/w/view.lua","old_string":"a\nb\nc","new_string":"a\nB\nB2\nc"}}]`, ""),
		claudeSays(cwd, "assistant", `[{"type":"tool_use","id":"t4","name":"Bash","input":{"command":"go test ./..."}}]`, ""),
		claudeSays(cwd, "user", `[{"type":"tool_result","tool_use_id":"t4","content":"[main abc1234def] The list is faster\n 1 file changed"}]`, `,"toolUseResult":{"stdout":"x"}`),
		claudeSays(cwd, "assistant", `[{"type":"tool_use","id":"q1","name":"AskUserQuestion","input":{"questions":[{"question":"Which one?","options":[{"label":"Fast"},{"label":"Small"}]}]}}]`, ""),
		claudeSays(cwd, "user", `[{"type":"tool_result","tool_use_id":"q1","content":"answered"}]`, `,"toolUseResult":{"questions":[{"question":"Which one?"}],"answers":{"Which one?":"Fast"}}`),
		claudeSays(cwd, "assistant", `[{"type":"text","text":`+quoted("Done. "+strings.Repeat("é", 7000))+`}]`, ""),
	)
	put(t, filepath.Join(os.Getenv("HOME"), ".pi", "agent", "sessions", "-"+strings.ReplaceAll(cwd, "/", "-")+"--", "2026-11-02T10-00-00-000Z_p1.jsonl"),
		`{"type":"session","version":3,"id":"p1","timestamp":"2026-11-02T10:00:00.000Z","cwd":`+quoted(cwd)+`}`,
		`{"type":"message","timestamp":"2026-11-02T10:00:02.000Z","message":{"role":"user","content":[{"type":"text","text":"tidy the docs"}]}}`,
		`{"type":"message","timestamp":"2026-11-02T10:00:03.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"which"},{"type":"text","text":"Reading them."},{"type":"toolCall","id":"a","name":"read","arguments":{"path":"docs/a.md"}},{"type":"toolCall","id":"b","name":"edit","arguments":{"path":"docs/a.md","edits":[{"oldText":"teh","newText":"the"}]}},{"type":"toolCall","id":"c","name":"bash","arguments":{"command":"git commit -am tidy","timeout":10}}]}}`,
		`{"type":"message","timestamp":"2026-11-02T10:00:04.000Z","message":{"role":"toolResult","toolName":"bash","content":[{"type":"text","text":"[main 0123abc] Tidy the docs"}]}}`)
}

// A dry run says where the handoff would be, in the repository's git
// folder, the window that would open and what it would be sent; it writes
// nothing and opens nothing.
func TestGoingOnWithAnotherAgentDryRunWritesAndStartsNothing(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	claudeDir := agentHomes(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	talked(t, claudeDir, record.Path)

	would, err := a.ContinueConversation(app.Continue{Worktree: record.Name, From: "claude", With: "codex", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "claude-c1.md")
	if !would.DryRun || would.Handoff != file || would.Conversation != "c1" || would.From != "claude" || would.Agent != "codex" ||
		would.Window.Name != "codex" || would.Command != "codex" || would.Turns != 9 || would.Sent || !strings.Contains(would.Instruction, file) {
		t.Fatalf("would = %+v", would)
	}
	if _, err := os.Stat(filepath.Join(repo.Root, ".git", "mia", "handoffs")); err == nil {
		t.Error("a dry run wrote the handoff")
	}
	if session.Here().HasWindow(record.Name, "codex") {
		t.Error("a dry run opened a window")
	}
	// by its id, and the one last written to when nothing names one
	if by, err := a.ContinueConversation(app.Continue{Worktree: record.Name, From: "p1", With: "claude", DryRun: true}); err != nil || by.From != "pi" ||
		by.Handoff != filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "pi-p1.md") {
		t.Errorf("by its id: %+v %v", by, err)
	}
	if last, err := a.ContinueConversation(app.Continue{Worktree: record.Name, With: "codex", DryRun: true}); err != nil || last.Conversation == "" {
		t.Errorf("the last written to: %+v %v", last, err)
	}
	for _, refused := range []struct {
		want app.Continue
		says string
	}{
		{app.Continue{Worktree: record.Name, From: "codex", With: "pi", DryRun: true}, "no conversation codex names"},
		{app.Continue{Worktree: record.Name, With: "nobody", DryRun: true}, "knows no agent nobody"},
		{app.Continue{Worktree: record.Name, With: "codex", Model: "no-such-model", DryRun: true}, "no-such-model"},
		{app.Continue{Worktree: filepath.Base(repo.Root), With: "codex", DryRun: true}, "no conversation in"},
	} {
		if _, err := a.ContinueConversation(refused.want); err == nil || !strings.Contains(err.Error(), refused.says) {
			t.Errorf("%+v: %v", refused.want, err)
		}
	}
}

// A conversation with claude, and one with pi, is written out as a handoff
// in the git folder — who said what, each thing done in a line, nothing of
// what came back — and a window opens with the other agent, told to read
// it. The worktree is as it was. The agents are the lab's: none is real.
func TestAConversationGoesOnWithAnotherAgentFromAHandoff(t *testing.T) {
	// a plugin's agent, which no computer has a real one of: it writes down what it is sent, or ends at once
	lab(t, map[string]string{"echo-agent": `[ -f "$HOME/broken" ] && echo cannot start && exit 1
echo ready; exec cat >> "$HOME/heard"`})
	labOnly(t, "echo-agent")
	repo := gittest.New(t)
	claudeDir := agentHomes(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = plugins(t, map[string]string{"echo": echoPlugin(t)})
	talked(t, claudeDir, record.Path)
	heard := func(name, text string) {
		t.Helper()
		var got []byte
		for range 250 { // 5s
			if got, _ = os.ReadFile(filepath.Join(os.Getenv("HOME"), "heard")); strings.Contains(string(got), text) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		screen, _ := a.ReadWindow(record, name)
		t.Fatalf("%s was not sent %q: %q\nits window:\n%s", name, text, got, screen)
	}

	done, err := a.ContinueConversation(app.Continue{Worktree: record.Name, From: "claude", With: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "claude-c1.md")
	if done.DryRun || done.Handoff != file || done.Window.Name != "echo" || done.Command != "echo-agent" || !done.Sent || done.Problem != "" || done.Agent != "echo" || done.Turns != 9 {
		t.Fatalf("done = %+v", done)
	}
	if !session.Here().HasWindow(record.Name, "echo") {
		t.Fatal("no window was opened for the agent")
	}
	heard("echo", "Read "+file)
	written, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
		t.Errorf("the handoff is the person's alone to read: %v", info.Mode())
	}
	text := string(written)
	for _, has := range []string{
		"# Handoff: a conversation with claude, to go on from",
		"- From: claude, conversation `c1`, on its account default",
		"- Worktree: " + record.Name + ", `" + record.Path + "`",
		"9 turns, in order",
		"### 1 · user · 2026-11-02 10:00 UTC\n\nmake the list faster\n",
		"### 2 · claude · 2026-11-02 10:00 UTC\n\nI will **look**.\n",
		"\nclaude did:\n\n- Read 2 files\n- Edited view.lua +2 −1 (`/w/view.lua`)\n- Ran go test ./...\n- Committed abc1234 The list is faster\n",
		"### 7 · claude asked · 2026-11-02 10:00 UTC\n\nWhich one?\n- Fast\n- Small\n",
		"### 8 · user answered · 2026-11-02 10:00 UTC\n\nFast\n",
		"### 9 · claude · 2026-11-02 10:00 UTC\n\nDone. é",
		"[… 1006 more characters of this message are left out]",
	} {
		if !strings.Contains(text, has) {
			t.Errorf("the handoff has no %q", has)
		}
	}
	for _, hasNot := range []string{"what the file had", "where is the time going", "1 file changed"} {
		if strings.Contains(text, hasNot) {
			t.Errorf("the handoff has %q: what came back from a tool, or what the agent thought", hasNot)
		}
	}
	if !strings.Contains(text, "### 1 ·") || strings.Index(text, "### 1 ·") > strings.Index(text, "### 2 ·") || strings.Index(text, "claude did:") > strings.Index(text, "### 7 ·") {
		t.Error("the turns are not in order")
	}

	// pi's, with another agent: its own file, a window of its own
	from, err := a.ContinueConversation(app.Continue{Worktree: record.Name, From: "pi", With: "echo", Model: "loud"})
	if err != nil {
		t.Fatal(err)
	}
	if from.Conversation != "p1" || from.Window.Name != "echo-2" || from.Command != "echo-agent '--model' 'loud'" || from.Model != "loud" || !from.Sent || from.Handoff != filepath.Join(filepath.Dir(file), "pi-p1.md") {
		t.Fatalf("from pi = %+v", from)
	}
	heard("echo-2", "Read "+from.Handoff)
	pi, err := os.ReadFile(from.Handoff)
	if err != nil {
		t.Fatal(err)
	}
	for _, has := range []string{
		"# Handoff: a conversation with pi, to go on from",
		"### 1 · user · 2026-11-02 10:00 UTC\n\ntidy the docs\n",
		"### 2 · pi · 2026-11-02 10:00 UTC\n\nReading them.\n\npi did:\n\n- Read a.md (`docs/a.md`)\n- Edited a.md +1 −1 (`docs/a.md`)\n- Ran git commit -am tidy\n- Committed 0123abc Tidy the docs\n",
	} {
		if !strings.Contains(string(pi), has) {
			t.Errorf("pi's handoff has no %q:\n%s", has, pi)
		}
	}

	// an agent that ends as it starts leaves a shell: nothing is typed into it, and the message is given to say
	if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), "broken"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ended, err := a.ContinueConversation(app.Continue{Worktree: record.Name, From: "pi", With: "echo"})
	os.Remove(filepath.Join(os.Getenv("HOME"), "broken"))
	if err != nil || ended.Sent || !strings.Contains(ended.Problem, "echo ended as soon as it started") || !strings.Contains(ended.Instruction, from.Handoff) {
		t.Fatalf("an agent that ended = %+v %v", ended, err)
	}
	if screen, _ := a.ReadWindow(record, ended.Window.Name); strings.Contains(screen, "Read ") {
		t.Errorf("its first message was typed into the shell it left:\n%s", screen)
	}

	// a second time writes the same file again and opens another window
	again, err := a.ContinueConversation(app.Continue{Worktree: record.Name, From: "c1", With: "echo"})
	if err != nil || again.Handoff != file || again.Window.Name != "echo-4" || !again.Sent {
		t.Errorf("again = %+v %v", again, err)
	}
	if out, err := exec.Command("git", "-C", record.Path, "status", "--porcelain").Output(); err != nil || len(out) != 0 {
		t.Errorf("the worktree is not as it was: %q %v", out, err)
	}
}
