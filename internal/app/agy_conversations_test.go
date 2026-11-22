package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// agyHome is the folder agy keeps everything in, under the test's HOME.
func agyHome() string { return filepath.Join(os.Getenv("HOME"), ".gemini", "antigravity-cli") }

// agyLogs is where agy writes a conversation's transcripts.
func agyLogs(id string) string {
	return filepath.Join(agyHome(), "brain", id, ".system_generated", "logs")
}

// agyStep is a line of an agy transcript: its step, who wrote it, what it
// is and when, and the rest of it.
func agyStep(n int, source, kind, at, more string) string {
	return fmt.Sprintf(`{"step_index":%d,"source":%q,"type":%q,"status":"DONE","created_at":%q%s}`, n, source, kind, at, more)
}

// agyAsked is what agy writes of something the person sent: their words
// between lines of its own, and its own after them.
func agyAsked(words string) string {
	return quoted("<USER_REQUEST>\n" + words + "\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nThe current local time is: 2026-11-02T05:00:00-05:00.\n</ADDITIONAL_METADATA>\n" +
		"<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Gemini 4 Flash.\n</USER_SETTINGS_CHANGE>")
}

const (
	agyNow   = "11111111-1111-4111-8111-111111111111" // an agy of now's: both transcripts, its folder in the list agy keeps
	agyOld   = "22222222-2222-4222-8222-222222222222" // an older agy's: transcript.jsonl alone, its folder only in what was typed
	agyFirst = "55555555-5555-4555-8555-555555555555" // an older agy's, its folder only as the last begun there
)

// agyTalked writes two conversations of agy's as agy writes them, one in
// a worktree and one in the main checkout, one of another project's and
// one whose folder agy keeps nowhere; carried is the second thing said in
// the first, pasted.
func agyTalked(t *testing.T, worktree, root, carried string) {
	t.Helper()
	put(t, filepath.Join(agyLogs(agyNow), "transcript_full.jsonl"),
		agyStep(0, "USER_EXPLICIT", "USER_INPUT", "2026-11-02T10:00:00Z", `,"content":`+agyAsked("make the list   faster")),
		agyStep(1, "MODEL", "PLANNER_RESPONSE", "2026-11-02T10:00:01Z", `,"model":"gemini-4-flash","thinking":"where is the time going","tool_calls":[`+
			`{"name":"view_file","args":{"AbsolutePath":"/w/list/a.go","StartLine":1,"EndLine":50,"toolSummary":"View a.go"}},`+
			`{"name":"read_file","args":{"Path":"/w/list/b.go"}}]`),
		agyStep(2, "MODEL", "GENERIC", "2026-11-02T10:00:02Z", `,"content":"Created At: 2026-11-02T05:00:02-05:00\nShowing lines 1-1 of 1\n1: package list"`),
		agyStep(3, "MODEL", "GENERIC", "2026-11-02T10:00:02Z", `,"content":"Showing lines 1-1 of 1\n1: package list"`),
		agyStep(4, "MODEL", "PLANNER_RESPONSE", "2026-11-02T10:00:03Z", `,"content":"I will **look**.","tool_calls":[`+
			`{"name":"replace_file_content","args":{"TargetFile":"/w/list/view.lua","TargetContent":"a\nb\nc","ReplacementContent":"a\nB\nB2\nc","StartLine":1,"EndLine":3}}]`),
		agyStep(5, "MODEL", "GENERIC", "2026-11-02T10:00:04Z", `,"content":"The following changes were made"`),
		agyStep(6, "SYSTEM", "SYSTEM_MESSAGE", "2026-11-02T10:00:05Z", `,"content":"The following is a <SYSTEM_MESSAGE> not actually sent by the user.\n\n<SYSTEM_MESSAGE>\nwork efficiently\n</SYSTEM_MESSAGE>"`),
		agyStep(7, "MODEL", "PLANNER_RESPONSE", "2026-11-02T10:00:06Z", `,"tool_calls":[`+
			`{"name":"run_command","args":{"CommandLine":"git commit -am faster","Cwd":`+quoted(worktree)+`,"WaitMsBeforeAsync":5000,"BypassSandbox":false}},`+
			`{"name":"multi_replace_file_content","args":{"TargetFile":"/w/list/c.go","ReplacementChunks":[{"TargetContent":"x","ReplacementContent":"y\nz"},{"TargetContent":"p\nq","ReplacementContent":"q"}]}},`+
			`{"name":"write_to_file","args":{"TargetFile":"/w/list/list.json","CodeContent":"{\n  \"a\": 1\n}\n","Overwrite":true}},`+
			`{"name":"grep_search","args":{"Query":"slowSort","SearchPath":"/w","MatchPerLine":true}},`+
			`{"name":"find_by_name","args":{"Pattern":"*_test.go","SearchDirectory":"/w","MaxDepth":3}},`+
			`{"name":"list_dir","args":{"DirectoryPath":"/w"}}]`),
		agyStep(8, "MODEL", "GENERIC", "2026-11-02T10:00:07Z", `,"content":"Command exited with code 0.\n\nCommand output:\n[feature abc1234] The list is faster\n 1 file changed\r\n"`),
		agyStep(9, "USER_EXPLICIT", "USER_INPUT", "2026-11-02T10:00:08Z", `,"content":`+agyAsked(carried)),
		agyStep(10, "SYSTEM", "ERROR_MESSAGE", "2026-11-02T10:00:09Z", `,"error":"There was a problem parsing the tool call.","content":"Error invalid tool call"`),
		agyStep(11, "SYSTEM", "CHECKPOINT", "2026-11-02T10:00:09Z", `,"content":"{{ CHECKPOINT 0 }}\nThe earlier parts of this conversation have been truncated"`),
		agyStep(12, "MODEL", "PLANNER_RESPONSE", "2026-11-02T10:00:10Z", `,"model":"gemini-4-flash","content":"Done."`),
	)
	// what agy writes beside it, with long fields cut short: not read where the whole is
	put(t, filepath.Join(agyLogs(agyNow), "transcript.jsonl"),
		agyStep(0, "USER_EXPLICIT", "USER_INPUT", "2026-11-02T10:00:00Z", `,"content":`+agyAsked("make the<truncated 9 bytes>")))
	put(t, filepath.Join(agyLogs(agyOld), "transcript.jsonl"),
		agyStep(0, "USER_EXPLICIT", "USER_INPUT", "2026-11-01T09:00:00Z", `,"content":`+agyAsked("why is this wrong:\n  def f(x):\n      return x\n")),
		agyStep(1, "SYSTEM", "CONVERSATION_HISTORY", "2026-11-01T09:00:00Z", ""),
		agyStep(2, "MODEL", "PLANNER_RESPONSE", "2026-11-01T09:00:01Z", `,"tool_calls":[`+
			`{"name":"run_command","args":{"CommandLine":"\"go test ./...\"","WaitMsBeforeAsync":"5000","toolSummary":"\"Run the tests\""}},`+
			`{"name":"view_file","args":{"AbsolutePath":"\"/w/f.py\"","StartLine":"1"}}]`),
		agyStep(3, "MODEL", "RUN_COMMAND", "2026-11-01T09:00:02Z", `,"content":"The command completed successfully."`),
		agyStep(4, "MODEL", "PLANNER_RESPONSE", "2026-11-01T09:00:03Z", `,"content":"It returns x: nothing is wrong."`))
	// the older agy's was last written to first
	if err := os.Chtimes(filepath.Join(agyLogs(agyOld), "transcript.jsonl"), time.Time{}, time.Date(2026, 11, 1, 9, 0, 3, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(agyLogs(agyFirst), "transcript.jsonl"),
		agyStep(0, "USER_EXPLICIT", "USER_INPUT", "2026-10-01T09:00:00Z", `,"content":`+agyAsked("hello")),
		agyStep(1, "MODEL", "PLANNER_RESPONSE", "2026-10-01T09:00:01Z", `,"content":"Hello!"`))
	if err := os.Chtimes(filepath.Join(agyLogs(agyFirst), "transcript.jsonl"), time.Time{}, time.Date(2026, 10, 1, 9, 0, 1, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	other, nowhere := "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"
	for _, id := range []string{other, nowhere} {
		put(t, filepath.Join(agyLogs(id), "transcript_full.jsonl"),
			agyStep(0, "USER_EXPLICIT", "USER_INPUT", "2026-11-02T11:00:00Z", `,"content":`+agyAsked("another project's list")),
			agyStep(1, "MODEL", "PLANNER_RESPONSE", "2026-11-02T11:00:01Z", `,"content":"Theirs."`))
	}
	// the folders agy keeps: what was typed, a line each, and the conversations it listed last
	put(t, filepath.Join(agyHome(), "history.jsonl"),
		`{"display":"why is this wrong:","timestamp":1793523600000,"workspace":`+quoted(root)+`}`,
		`{"display":"why is this wrong:","timestamp":1793523600001,"workspace":`+quoted(root)+`,"conversationId":"`+agyOld+`"}`,
		`{"display":"make the list faster","timestamp":1793613600000,"workspace":`+quoted(worktree)+`,"conversationId":"`+agyNow+`"}`,
		`{"display":"/model","timestamp":1793613600001,"workspace":`+quoted(worktree)+`,"type":"slash_command"}`)
	// an older agy's first message has no conversation; the last begun in a folder is kept apart
	put(t, filepath.Join(agyHome(), "cache", "last_conversations.json"), `{`+quoted(root)+`:"`+agyFirst+`"}`)
	put(t, filepath.Join(agyHome(), "cache", "conversation_metadata.json"),
		`{"conversations":{`+
			`"`+agyNow+`":{"summary":{"ID":"`+agyNow+`","Title":"The list","NumSteps":13,"WorkspaceURIs":["file://`+worktree+`"],"AppDataDir":"antigravity-cli"},"is_internal":false},`+
			`"`+agyOld+`":{"summary":{"ID":"`+agyOld+`","WorkspaceURIs":null,"AppDataDir":"antigravity-cli"}},`+
			`"`+other+`":{"summary":{"ID":"`+other+`","WorkspaceURIs":["file:///somewhere/else"],"AppDataDir":"antigravity-cli"}}}}`)
}

// agy's conversations are listed by the folder agy keeps each was made
// in, as its own transcript does not say it, and read from the whole of
// what it writes: what the person sent, without agy's own lines around
// it, what it said, and each tool it used in a line. A paste is in what
// was sent as it was pasted, so the first thing sent to an agy a
// conversation was handed to, which mia pastes with that conversation in
// front, is a turn of the person's.
func TestAgysConversationsAreListedReadAndRecalled(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	carried := app.CarriedOpens("claude") + "\n\n# Handoff\n\n### 1 · user\n\nwhich word?\n\n" + app.CarriedRule + "\n\nwhat did I first ask you?"
	agyTalked(t, record.Path, repo.Root, carried)

	for range 2 { // read, then from what was kept of it
		heads, problems := a.Conversations(app.Wanted{Agent: "agy"})
		if len(heads) != 3 || len(problems) != 0 {
			t.Fatalf("the worktree's and the main checkout's two, not another project's nor one of no folder: %+v %v", heads, problems)
		}
		if h := heads[2]; h.ID != agyFirst || h.Cwd != repo.Root || h.Asked != "hello" {
			t.Errorf("the one last begun in the main checkout: %+v", h)
		}
		if h := heads[0]; h.ID != agyNow || h.Agent != "agy" || h.Account != "" || h.Cwd != record.Path || h.Worktree != record.Name || h.Gone ||
			h.Asked != "make the list faster" || h.Said != "Done." || h.Turns != 2 || h.Running || !h.Started.Equal(time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)) {
			t.Errorf("the worktree's: %+v", h)
		}
		if h := heads[1]; h.ID != agyOld || h.Cwd != repo.Root || h.Worktree != filepath.Base(repo.Root) || h.Asked != "why is this wrong: def f(x): return x" ||
			h.Said != "It returns x: nothing is wrong." || h.Turns != 1 {
			t.Errorf("the older agy's, by what was typed: %+v", h)
		}
	}

	turns, err := a.ConversationTurns(agyNow, 0)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, turn := range turns {
		lines = append(lines, fmt.Sprintf("%d %s %s%s", turn.N, turn.Kind, turn.Text, turn.Hash))
	}
	want := []string{"1 asked make the list   faster", "2 tool Read 2 files", "3 said I will **look**.", "4 tool Edited view.lua +2 −1",
		"5 tool Ran git commit -am faster", "6 tool Edited c.go +2 −2", "7 tool Wrote list.json +3", "8 tool Searched for slowSort",
		"9 tool Looked for *_test.go", "10 tool Used list_dir", "11 commit abc1234", "12 asked " + carried, "13 said Done."}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("its turns, what it thought and agy's own steps left out:\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if app.OwnWords(turns[11].Text) != "what did I first ask you?" {
		t.Errorf("the person's own words, after the conversation pasted in front of them: %q", app.OwnWords(turns[11].Text))
	}
	if turns[3].File != "/w/list/view.lua" || turns[3].Name != "edit" || turns[6].File != "/w/list/list.json" || turns[10].Subject != "The list is faster" ||
		!turns[0].At.Equal(time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("each tool's file, the commit made, and when: %+v", turns)
	}

	old, err := a.ConversationTurns(agyOld, 0)
	if err != nil {
		t.Fatal(err)
	}
	if kinds(old) != "1:asked 2:tool 3:tool 4:said" || old[0].Text != "why is this wrong:\n  def f(x):\n      return x" ||
		old[1].Text != "Ran go test ./..." || old[2].Text != "Read f.py" || old[2].File != "/w/f.py" {
		t.Errorf("the older agy's, a pasted message as it was pasted and each argument JSON in a string: %+v", old)
	}
	if later, err := a.ConversationIn(agyNow, record.Name, 12); err != nil || len(later) != 1 || later[0].Text != "Done." {
		t.Errorf("by its worktree, those after the twelfth: %+v %v", later, err)
	}
	if _, err := a.ConversationTurns("33333333-3333-4333-8333-333333333333", 0); err == nil {
		t.Error("another project's is read")
	}

	found := a.Recall("make the LIST", 10)
	if len(found) != 1 || found[0].Agent != "agy" || found[0].Session != agyNow || !found[0].Yours || found[0].N != 1 || found[0].Worktree != record.Name || found[0].Account != "" {
		t.Errorf("recalled by your own words: %+v", found)
	}
	if found := a.Recall("nothing is wrong", 10); len(found) != 1 || found[0].Session != agyOld || found[0].Yours || found[0].Cwd != repo.Root {
		t.Errorf("recalled by agy's: %+v", found)
	}
}

// An agy conversation is followed as agy writes it: what it has, then each
// step written after.
func TestAnAgyConversationIsFollowedAsItIsWritten(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	agyTalked(t, record.Path, repo.Root, "go on")
	said := make(chan []app.Turn, 4)
	stop := make(chan struct{})
	defer close(stop)
	go a.FollowConversation(agyNow, "", 12, stop, func(turns []app.Turn) error {
		said <- turns
		return nil
	})
	next := func() []app.Turn {
		t.Helper()
		select {
		case turns := <-said:
			return turns
		case <-time.After(5 * time.Second):
			t.Fatal("nothing was said")
			return nil
		}
	}
	if first := next(); len(first) != 1 || first[0].Text != "Done." {
		t.Fatalf("what it has after the twelfth: %+v", first)
	}
	put(t, filepath.Join(agyLogs(agyNow), "transcript_full.jsonl"),
		agyStep(13, "USER_EXPLICIT", "USER_INPUT", "2026-11-02T10:01:00Z", `,"content":`+agyAsked("and the map?")),
		agyStep(14, "MODEL", "PLANNER_RESPONSE", "2026-11-02T10:01:01Z", `,"content":"The map too."`))
	if then := next(); kinds(then) != "14:asked 15:said" || then[0].Text != "and the map?" {
		t.Errorf("what was written since: %+v", then)
	}
}
