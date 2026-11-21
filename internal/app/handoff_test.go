package app_test

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	would, err := a.ContinueConversation(app.Continue{ID: "c1", With: "codex", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "claude-c1.md")
	if !would.DryRun || would.Handoff != file || would.From != (app.ContinuedFrom{Agent: "claude", ID: "c1", Worktree: record.Name}) || would.To.Agent != "codex" ||
		would.To.Window.Name != "codex" || would.To.ID != "" || would.Command != "codex" || would.Turns != 9 || would.LeftOut != 0 || would.Full != "" || would.Sent || !strings.Contains(would.Instruction, file) {
		t.Fatalf("would = %+v", would)
	}
	if _, err := os.Stat(filepath.Join(repo.Root, ".git", "mia", "handoffs")); err == nil {
		t.Error("a dry run wrote the handoff")
	}
	if session.Here().HasWindow(record.Name, "codex") {
		t.Error("a dry run opened a window")
	}
	// pi's, by its id, and in the worktree it is named in
	if by, err := a.ContinueConversation(app.Continue{ID: "p1", Worktree: record.Name, With: "claude", DryRun: true}); err != nil || by.From.Agent != "pi" ||
		by.Handoff != filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "pi-p1.md") {
		t.Errorf("by its id: %+v %v", by, err)
	}
	for _, refused := range []struct {
		want app.Continue
		says string
	}{
		{app.Continue{ID: "nobody", With: "pi", DryRun: true}, "no conversation nobody here"},
		{app.Continue{ID: "c1", Worktree: filepath.Base(repo.Root), With: "pi", DryRun: true}, "no conversation c1 in " + filepath.Base(repo.Root)},
		{app.Continue{ID: "c1", With: "nobody", DryRun: true}, "knows no agent nobody"},
		{app.Continue{ID: "c1", With: "codex", Model: "no-such-model", DryRun: true}, "no-such-model"},
		// no window has one open: a window names none, and none is taken for it
		{app.Continue{Worktree: record.Name, Window: "claude", With: "codex", DryRun: true}, "no window of " + record.Name + " has a conversation open"},
		// with the agent it is with there is nothing to hand over: that agent's own resume loses nothing
		{app.Continue{ID: "c1", With: "claude", DryRun: true}, "is claude's own, and claude goes on with it as it is, with nothing lost: `mia conversation resume c1` takes it up again (claude --resume c1)"},
		{app.Continue{ID: "p1", With: "pi", Model: "opus", DryRun: true}, "`mia conversation resume p1 --model opus`"},
	} {
		if _, err := a.ContinueConversation(refused.want); err == nil || !strings.Contains(err.Error(), refused.says) {
			t.Errorf("%+v: %v", refused.want, err)
		}
	}
}

// Back to the agent a conversation went on from: that agent takes its own
// up again, by its own id, and the handoff is only what was said since it
// was handed on. One that went on from no conversation of that agent's is
// handed over whole, to a new one.
func TestGoingBackToTheAgentAConversationCameFromTakesItsOwnUpAgain(t *testing.T) {
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
	whole, err := a.ContinueConversation(app.Continue{ID: "p1", Worktree: record.Name, With: "claude", DryRun: true})
	if err != nil || whole.Resumes != "" || whole.To.ID != "" || whole.Command != "claude" || whole.Turns < 2 {
		t.Fatalf("with no handoff before it: %+v %v", whole, err)
	}
	handed := func(at string) {
		t.Helper()
		os.Remove(filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "claude-c1.1.json")) // put adds to a file
		put(t, filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "claude-c1.1.json"),
			`{"from":{"agent":"claude","id":"c1","worktree":`+quoted(record.Name)+`},"to":{"agent":"pi","window":{"index":2,"name":"pi"},"id":"p1"},"handoff":"/h/claude-c1.md","at":"`+at+`"}`)
	}
	handed("2026-11-02T09:59:00Z") // before anything pi was said
	back, err := a.ContinueConversation(app.Continue{ID: "p1", Worktree: record.Name, With: "claude", Model: "opus", DryRun: true})
	if err != nil || back.Resumes != "c1" || back.To.ID != "c1" || back.Command != "claude '--model' 'opus' --resume c1" || back.Turns != whole.Turns || back.From.Agent != "pi" {
		t.Fatalf("back = %+v %v", back, err)
	}
	handed("2026-11-02T10:00:03.500Z") // after pi's first two turns: only what was said since
	if since, err := a.ContinueConversation(app.Continue{ID: "p1", Worktree: record.Name, With: "claude", DryRun: true}); err != nil || since.Resumes != "c1" || since.Turns == 0 || since.Turns >= whole.Turns {
		t.Fatalf("since = %+v %v (whole had %d turns)", since, err, whole.Turns)
	}
	// to an agent it did not come from: whole, and a conversation of its own
	if other, err := a.ContinueConversation(app.Continue{ID: "p1", Worktree: record.Name, With: "codex", DryRun: true}); err != nil || other.Resumes != "" || other.Command != "codex" || other.Turns != whole.Turns {
		t.Fatalf("other = %+v %v", other, err)
	}
}

// A conversation over a handoff's budget has turns of its middle left out,
// and the file that has them all named; asked for whole, none is left out
// and there is no other file.
func TestAHandoffAskedForWholeLeavesNoTurnOut(t *testing.T) {
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
	var lines []string
	for range 40 {
		lines = append(lines, claudeSays(record.Path, "user", quoted(strings.Repeat("word ", 500)), ""))
	}
	put(t, filepath.Join(claudeDir(record.Path), "long.jsonl"), lines...)
	latest, err := a.ContinueConversation(app.Continue{ID: "long", With: "codex", DryRun: true})
	if err != nil || latest.Turns != 40 || latest.LeftOut == 0 || latest.Full == "" {
		t.Fatalf("its latest: %d turns, %d left out, full %q, %v", latest.Turns, latest.LeftOut, latest.Full, err)
	}
	whole, err := a.ContinueConversation(app.Continue{ID: "long", With: "codex", DryRun: true, Whole: true})
	if err != nil || whole.Turns != 40 || whole.LeftOut != 0 || whole.Full != "" || whole.Handoff != latest.Handoff {
		t.Fatalf("whole: %d turns, %d left out, full %q, %v", whole.Turns, whole.LeftOut, whole.Full, err)
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

	done, err := a.ContinueConversation(app.Continue{ID: "c1", With: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "claude-c1.md")
	if done.DryRun || done.Handoff != file || done.To.Window.Name != "echo" || done.Command != "echo-agent" || !done.Sent || done.Problem != "" ||
		done.To.Agent != "echo" || done.From.ID != "c1" || done.From.Window != nil || done.Turns != 9 || done.LeftOut != 0 || done.Full != "" {
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
		"9 turns, in order\n",
		"Nothing claude only thought is here.",
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
	from, err := a.ContinueConversation(app.Continue{ID: "p1", With: "echo", Model: "loud"})
	if err != nil {
		t.Fatal(err)
	}
	if from.From.ID != "p1" || from.To.Window.Name != "echo-2" || from.Command != "echo-agent '--model' 'loud'" || from.Model != "loud" || !from.Sent || from.Handoff != filepath.Join(filepath.Dir(file), "pi-p1.md") {
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
	ended, err := a.ContinueConversation(app.Continue{ID: "p1", With: "echo"})
	os.Remove(filepath.Join(os.Getenv("HOME"), "broken"))
	if err != nil || ended.Sent || !strings.Contains(ended.Problem, "echo ended as soon as it started") || !strings.Contains(ended.Instruction, from.Handoff) {
		t.Fatalf("an agent that ended = %+v %v", ended, err)
	}
	if screen, _ := a.ReadWindow(record, ended.To.Window.Name); strings.Contains(screen, "Read ") {
		t.Errorf("its first message was typed into the shell it left:\n%s", screen)
	}

	// a second time writes the same file again and opens another window
	again, err := a.ContinueConversation(app.Continue{ID: "c1", With: "echo"})
	if err != nil || again.Handoff != file || again.To.Window.Name != "echo-4" || !again.Sent {
		t.Errorf("again = %+v %v", again, err)
	}
	// each time is kept beside the handoff: which went on with which agent, in which window. The lab's agent writes no
	// conversation that names the file, so no list says yet which it went on as
	kept, _ := filepath.Glob(filepath.Join(filepath.Dir(file), "claude-c1.*.json"))
	if len(kept) != 2 {
		t.Fatalf("what is kept of c1's two handoffs: %v", kept)
	}
	record1, _ := os.ReadFile(kept[0])
	if want := `{"from":{"agent":"claude","id":"c1","worktree":` + quoted(record.Name) + `},"to":{"agent":"echo","window":{"index":` + strconv.Itoa(done.To.Window.Index) + `,"name":"echo"}},"handoff":` + quoted(file) + `,"at":"`; !strings.HasPrefix(string(record1), want) {
		t.Errorf("kept = %s, want %s…", record1, want)
	}
	if info, _ := os.Stat(kept[0]); info.Mode().Perm() != 0o600 {
		t.Errorf("what is kept is the person's alone to read: %v", info.Mode())
	}
	heads, _ := a.Conversations(app.Wanted{Worktree: record.Name, Agent: "claude"})
	if len(heads) != 1 || heads[0].ContinuedBy != nil || heads[0].Continues != nil {
		t.Errorf("a handoff no conversation was begun from yet: %+v", heads)
	}
	if out, err := exec.Command("git", "-C", record.Path, "status", "--porcelain").Output(); err != nil || len(out) != 0 {
		t.Errorf("the worktree is not as it was: %q %v", out, err)
	}

	// started quiet, the agent runs and is told nothing: a chat says the handoff with the first thing sent
	quiet, err := a.ContinueConversation(app.Continue{ID: "c1", With: "echo", Model: "soft", Quiet: true})
	if err != nil || !quiet.Sent || quiet.Problem != "" || quiet.Handoff != file || quiet.Model != "soft" {
		t.Fatalf("quiet = %+v %v", quiet, err)
	}
	if screen, _ := a.ReadWindow(record, quiet.To.Window.Name); !strings.Contains(screen, "ready") {
		t.Errorf("one started quiet does not run:\n%s", screen)
	}
	if told, _ := os.ReadFile(filepath.Join(os.Getenv("HOME"), "heard")); strings.Count(string(told), "Read ") != 3 {
		t.Errorf("one started quiet was told to read the handoff:\n%s", told)
	}
}

// A handoff's record says which conversation went on with which agent; the
// conversation that agent began from it is the first of its own in the
// worktree, begun no earlier, whose first message names the handoff's
// file. A list says the two as one thread, and once found it is kept in
// the record, not looked for again.
func TestAListSaysWhichConversationGoesOnFromWhich(t *testing.T) {
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
	talked(t, claudeDir, record.Path) // claude's c1 and pi's p1, begun 10:00
	file := filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "pi-p1.md")
	kept := filepath.Join(filepath.Dir(file), "pi-p1.1793613600000.json")
	put(t, kept, `{"from":{"agent":"pi","id":"p1","worktree":`+quoted(record.Name)+`,"window":{"index":1,"name":"pi"}},"to":{"agent":"claude","window":{"index":2,"name":"claude"}},"handoff":`+quoted(file)+`,"at":"2026-11-02T10:30:00.250Z"}`)
	began := func(id, at, asked string) {
		t.Helper()
		put(t, filepath.Join(claudeDir(record.Path), id+".jsonl"),
			strings.ReplaceAll(claudeSays(record.Path, "user", quoted(asked), ""), "2026-11-02T10:00:00Z", at),
			strings.ReplaceAll(claudeSays(record.Path, "assistant", `[{"type":"text","text":"I have read it."}]`, ""), "2026-11-02T10:00:00Z", at))
	}
	threads := func() map[string]string {
		t.Helper()
		heads, problems := a.Conversations(app.Wanted{})
		if len(problems) > 0 {
			t.Fatal(problems)
		}
		said := map[string]string{}
		for _, h := range heads {
			data, _ := json.Marshal(struct {
				Continues   *app.Talk `json:"continues,omitempty"`
				ContinuedBy *app.Talk `json:"continued_by,omitempty"`
			}{h.Continues, h.ContinuedBy})
			if whole, _ := json.Marshal(h); !strings.Contains(string(whole), strings.Trim(string(data), "{}")) {
				t.Errorf("%s is not listed with %s: %s", h.ID, data, whole)
			}
			said[h.ID] = string(data)
		}
		return said
	}

	// one begun before the handoff, one of another agent's and one that names another file are none of them it
	began("c0", "2026-11-02T09:00:00Z", "Read "+file+" — said before there was one")
	began("c2", "2026-11-02T10:31:00Z", "Read "+filepath.Join(filepath.Dir(file), "claude-c1.md")+" — another's")
	if said := threads(); said["p1"] != "{}" || said["c0"] != "{}" || said["c2"] != "{}" || len(said) != 4 {
		t.Fatalf("nothing goes on from p1 yet: %v", said)
	}
	// the first begun at or after it that names the file is it, a later one that does too is not
	began("c4", "2026-11-02T10:40:00Z", "Read "+file+" — once more")
	began("c3", "2026-11-02T10:30:00Z", "Read "+file+" — a conversation the person had with pi in this worktree, written out by mia. "+strings.Repeat("Go on. ", 60))
	want := map[string]string{"p1": `{"continued_by":{"agent":"claude","id":"c3"}}`, "c3": `{"continues":{"agent":"pi","id":"p1"}}`, "c0": "{}", "c1": "{}", "c2": "{}", "c4": "{}"}
	if said := threads(); !maps.Equal(said, want) {
		t.Fatalf("threads = %v, want %v", said, want)
	}
	if data, _ := os.ReadFile(kept); !strings.Contains(string(data), `"to":{"agent":"claude","window":{"index":2,"name":"claude"},"id":"c3"}`) || !strings.Contains(string(data), `"at":"2026-11-02T10:30:00.25Z"`) {
		t.Errorf("what was found is not kept: %s", data)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(file), ".handed-*")); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
	// kept, it is not looked for again: an earlier one that turns up changes nothing, and it is said of p1 with claude's not listed
	began("c5", "2026-11-02T10:30:00Z", "Read "+file)
	if said := threads(); said["p1"] != want["p1"] || said["c5"] != "{}" {
		t.Errorf("found once, looked for again: %v", said)
	}
	if heads, _ := a.Conversations(app.Wanted{Agent: "pi"}); len(heads) != 1 || heads[0].ContinuedBy == nil || *heads[0].ContinuedBy != (app.Talk{Agent: "claude", ID: "c3"}) {
		t.Errorf("pi's alone: %+v", heads)
	}
}
