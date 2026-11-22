package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/gittest"
)

// fakeOpencode puts an opencode on PATH that answers from a folder as the
// real one does from its server — sessions.json for the list, <id>.json
// for an export, a failure for one it has not, models for its models and
// a failure with none — and writes each thing it
// was asked to asked. It makes the data folder that says opencode has
// kept something, and returns the store.
func fakeOpencode(t *testing.T) (store string) {
	t.Helper()
	store, bin := t.TempDir(), t.TempDir()
	script := `#!/bin/sh
echo "$*" >> "` + store + `/asked"
case "$1 $2" in
"api GET") case "$3" in /api/session\?order=desc\&limit=1000) exec cat "` + store + `/sessions.json" ;; esac ;;
"session export") [ -f "` + store + `/$3.json" ] && exec cat "` + store + `/$3.json"; echo "Session not found: $3" >&2; exit 1 ;;
"models ") [ -f "` + store + `/models" ] && exec cat "` + store + `/models"; echo "Error: not signed in" >&2; exit 1 ;;
esac
echo "not asked as mia asks: $*" >&2
exit 2
`
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".local", "share", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	return store
}

// opencodeSaid is a session as opencode lists it, and with messages as it
// exports it.
func opencodeSaid(id, dir, more string, messages ...string) (listed, exported string) {
	info := fmt.Sprintf(`{"id":%q,"projectID":"prj_1","cost":0.25,"title":"A title","location":{"directory":%q}%s}`, id, dir, more)
	return info, `{"info":` + info + `,"messages":[` + strings.Join(messages, ",") + `]}`
}

func TestOpencodesConversationsAreListedReadAndTakenUpAgain(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	if heads, problems := a.Conversations(Wanted{Agent: "opencode"}); len(heads) != 0 || len(problems) != 0 {
		t.Fatalf("with no opencode that has kept anything, none and no complaint: %+v %v", heads, problems)
	}

	store := fakeOpencode(t)
	asked := func() string {
		data, _ := os.ReadFile(filepath.Join(store, "asked"))
		_ = os.Remove(filepath.Join(store, "asked"))
		return string(data)
	}
	tool := func(id, name, input, more string) string {
		return fmt.Sprintf(`{"type":"tool","id":%q,"name":%q,"state":{"status":"completed","input":%s%s},"time":{"created":1793613604000}}`, id, name, input, more)
	}
	var list []string
	session := func(id, dir, more string, messages ...string) {
		listed, exported := opencodeSaid(id, dir, more, messages...)
		list = append(list, listed)
		if err := os.WriteFile(filepath.Join(store, id+".json"), []byte(exported), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	session("ses_cart", record.Path, `,"time":{"created":1793613600000,"updated":1793613601000,"busy":1793613602000,"idle":1793613609000}`,
		`{"type":"system","id":"m0","time":{"created":1793613600000},"text":"You are in a folder."}`,
		`{"type":"user","id":"m1","time":{"created":1793613601000},"text":"fix the cart total","files":[],"metadata":{"by":"testuser1"}}`,
		`{"type":"assistant","id":"m2","time":{"created":1793613603000},"agent":"build","cost":0,"content":[`+
			`{"type":"reasoning","text":"where is the total added up","time":{"created":1793613603000,"completed":1793613605600}},`+
			`{"type":"reasoning","text":"","native":{"encrypted":"gAAAAencrypted"}},`+
			`{"type":"text","text":"Looking at\nthe cart."},`+
			tool("t1", "read", `{"path":"cart/total.go"}`, `,"content":[{"type":"text","text":"package cart"}]`)+`,`+
			tool("t2", "read", `{"filePath":"/w/cart/total_test.go","limit":20}`, "")+`,`+
			tool("t3", "edit", `{"path":"cart/total.go","oldString":"a\nb\nc","newString":"a\nB\nB2\nc"}`, "")+`,`+
			tool("t4", "patch", `{"patchText":"*** Begin Patch\n*** Update File: cart/tax.go\n@@\n-old\n+new\n*** End Patch"}`, "")+`,`+
			tool("t5", "shell", `{"command":"git commit -am cart"}`, `,"content":[{"type":"text","text":"[feature 0123abc] The cart total adds up\n 1 file changed"}]`)+`,`+
			tool("t6", "subagent", `{"agent":"explore","description":"find the tax rules","prompt":"look"}`, "")+`,`+
			tool("t7", "question", `{"questions":[{"question":"Round up?","header":"Rounding","options":[{"label":"Up","description":"x"},{"label":"Down","description":"y"}]}]}`, `,"metadata":{"answers":[["Up"]]}`)+`,`+
			tool("t8", "tracker_find", `{"id":7}`, `,"metadata":{"answers":"another tool's own"}`)+
			`],"finish":"tool-calls"}`,
		`{"type":"shell","id":"m3","time":{"created":1793613605000},"shellID":"sh1","command":"ls","status":"exited"}`,
		`{"type":"assistant","id":"m4","time":{"created":1793613606000},"agent":"build","content":[{"type":"text","text":"The total adds up now."}],"finish":"stop"}`,
		`{"type":"idle","id":"m5","time":{"created":1793613609000},"outcome":"succeeded"}`)
	session("ses_gone", repo.Root+".gone", `,"time":{"created":1793613000000,"updated":1793613000500}`,
		`{"type":"user","id":"m1","time":{"created":1793613000000},"text":"try the other way"}`)
	session("ses_work", repo.Root, `,"time":{"created":1793612000000,"updated":1793612000500,"busy":1793612001000}`,
		`{"type":"user","id":"m1","time":{"created":1793612000000},"text":"still at it"}`,
		`{"type":"assistant","id":"m2","time":{"created":1793612001000},"content":[{"type":"text","text":"Writing it down."},`+
			`{"type":"tool","id":"t1","name":"write","state":{"status":"streaming","input":""}}]}`)
	session("ses_else", "/somewhere/else", `,"time":{"created":1793613000000,"updated":1793613900000}`,
		`{"type":"user","id":"m1","time":{"created":1793613000000},"text":"another project's"}`)
	session("ses_child", record.Path, `,"parentID":"ses_cart","time":{"created":1793613000000,"updated":1793613000000}`,
		`{"type":"user","id":"m1","time":{"created":1793613000000},"text":"find the tax rules"}`)
	session("ses_empty", repo.Root, `,"time":{"created":1793613000000,"updated":1793613000000}`)
	session("ses_torn", repo.Root, `,"time":{"created":1793613000000,"updated":1793613000000}`)
	if err := os.WriteFile(filepath.Join(store, "ses_torn.json"), []byte(`{"info":{"id":"ses_to`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "sessions.json"), []byte(`{"data":[`+strings.Join(list, ",")+`],"cursor":{"next":"c1","previous":"c0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	for round := range 2 { // read, then from what was kept of it
		heads, problems := a.Conversations(Wanted{})
		if len(heads) != 3 || len(problems) != 1 || !strings.Contains(problems[0], "agent opencode: session ses_torn is not exported") {
			t.Fatalf("three, the torn one said and left out with another folder's, a subagent's and an empty one: %+v %v", heads, problems)
		}
		if h := heads[0]; h.ID != "ses_cart" || h.Agent != "opencode" || h.Cwd != record.Path || h.Worktree != record.Name || h.Gone || h.Account != "" ||
			h.Asked != "fix the cart total" || h.Said != "The total adds up now." || h.Turns != 1 || h.Running ||
			h.Started.UnixMilli() != 1793613600000 || h.Active.UnixMilli() != 1793613609000 {
			t.Errorf("the one in the worktree, last written to when it came to rest: %+v", h)
		}
		if h := heads[1]; h.ID != "ses_gone" || h.Worktree != "gone" || !h.Gone || h.Asked != "try the other way" || h.Said != "" {
			t.Errorf("one whose worktree is gone: %+v", h)
		}
		if h := heads[2]; h.ID != "ses_work" || h.Worktree != filepath.Base(repo.Root) || h.Active.UnixMilli() != 1793612001000 {
			t.Errorf("one still at work: %+v", h)
		}
		said := asked()
		if strings.Contains(said, "session list") || strings.Count(said, "api GET") != 1 {
			t.Errorf("one list, and never of a folder's project: %s", said)
		}
		for id, want := range map[string]int{"ses_cart": 1 - round, "ses_gone": 1 - round, "ses_work": 1, "ses_torn": 1, "ses_else": 0, "ses_child": 0} {
			if got := strings.Count(said, "session export "+id+"\n"); got != want {
				t.Errorf("round %d: %s exported %d times, want %d — one at rest is read once, one at work or not read each time, another's never", round, id, got, want)
			}
		}
	}
	if only, _ := a.Conversations(Wanted{Agent: "opencode", Worktree: record.Name}); len(only) != 1 || only[0].ID != "ses_cart" {
		t.Errorf("one worktree's: %+v", only)
	}

	turns, err := a.ConversationTurns("ses_cart", 0)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, turn := range turns {
		lines = append(lines, fmt.Sprintf("%d %s %s%s", turn.N, turn.Kind, turn.Text, turn.Hash))
	}
	want := []string{"1 asked fix the cart total", "2 thought where is the total added up", "3 said Looking at\nthe cart.", "4 tool Read 2 files", "5 tool Edited total.go +2 −1",
		"6 tool Edited tax.go +1 −1", "7 tool Ran git commit -am cart", "8 commit 0123abc", "9 tool Asked an agent: find the tax rules", "10 question Round up?",
		"11 answer Up", "12 tool Used tracker_find", "13 said The total adds up now."}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("its turns, what it thought but none it encrypted, and opencode's own notes left out:\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if turns[1].Seconds != 3 {
		t.Errorf("how long it thought, from when it began to when it was done: %+v", turns[1])
	}
	if q, answer := turns[9], turns[10]; q.Answer != "Up" || strings.Join(q.Options, ",") != "Up,Down" || answer.Question != 10 {
		t.Errorf("a question and what was chosen: %+v %+v", q, answer)
	}
	if turns[4].File != "cart/total.go" || turns[4].Name != "edit" || turns[5].File != "cart/tax.go" || turns[5].Name != "patch" || turns[8].Name != "subagent" ||
		turns[7].Subject != "The cart total adds up" || turns[0].At.UnixMilli() != 1793613601000 {
		t.Errorf("each tool by opencode's name for it, its file, and the commit made: %+v", turns)
	}
	if later, err := a.ConversationTurns("ses_cart", 11); err != nil || len(later) != 2 || later[0].N != 12 {
		t.Errorf("those after the eleventh: %+v %v", later, err)
	}
	if in, err := a.ConversationIn("ses_cart", record.Name, 12); err != nil || len(in) != 1 || in[0].Kind != turnSaid {
		t.Errorf("by its worktree: %+v %v", in, err)
	}
	if work, err := a.ConversationTurns("ses_work", 0); err != nil || len(work) != 3 || work[1].Text != "Writing it down." || work[2].Text != "Using write" {
		t.Errorf("what the agent is still writing, as far as it is written: %+v %v", work, err)
	}
	for _, id := range []string{"ses_else", "ses_none", "ses_torn", "--help"} {
		if turns, err := a.ConversationTurns(id, 0); err == nil {
			t.Errorf("%s is another folder's, or none, or not read: %+v", id, turns)
		}
	}
	if said := asked(); strings.Contains(said, "--help") {
		t.Errorf("what is no id of opencode's is not handed to it: %s", said)
	}

	if command := a.ResumeCommand("opencode", "ses_cart"); command != "opencode --session ses_cart" {
		t.Errorf("taken up again by its id: %s", command)
	}
	would, err := a.ContinueConversation(Continue{ID: "ses_cart", With: "claude", DryRun: true})
	if err != nil || would.From != (ContinuedFrom{Agent: "opencode", ID: "ses_cart", Worktree: record.Name}) || would.Turns != 12 {
		t.Errorf("it goes on with another agent from a handoff: %+v %v", would, err)
	}
	if _, err := a.ContinueConversation(Continue{ID: "ses_cart", With: "opencode", DryRun: true}); err == nil || !strings.Contains(err.Error(), "opencode --session ses_cart") {
		t.Errorf("with opencode itself it is resumed, not handed off: %v", err)
	}

	found := a.Recall("CART total", 10)
	if len(found) != 1 || found[0].Agent != "opencode" || found[0].Session != "ses_cart" || !found[0].Yours || found[0].N != 1 || found[0].Worktree != record.Name {
		t.Errorf("recalled by your own words: %+v", found)
	}

	// what a list of them looks like
	heads, _ := a.Conversations(Wanted{Agent: "opencode", Limit: 1})
	if shape, _ := json.Marshal(heads); !strings.Contains(string(shape), `"id":"ses_cart","agent":"opencode","cwd":`) {
		t.Errorf("as JSON: %s", shape)
	}
}

// A list longer than a page is asked for page by page, each from where
// the last stopped.
func TestOpencodesSessionsAreReadPageByPage(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	store := fakeOpencode(t)
	page := func(from, n int, next string) string {
		var all []string
		for i := from; i < from+n; i++ {
			listed, _ := opencodeSaid(fmt.Sprintf("ses_%04d", i), repo.Root, "")
			all = append(all, listed)
		}
		return `{"data":[` + strings.Join(all, ",") + `],"cursor":{"next":"` + next + `"}}`
	}
	script := `#!/bin/sh
echo "$3" >> "` + store + `/asked"
case "$3" in *cursor=after+1000) echo '` + page(opencodePage, 2, "after 1002") + `' ;; *) cat "` + store + `/first.json" ;; esac
`
	if err := os.WriteFile(filepath.Join(store, "first.json"), []byte(page(0, opencodePage, "after 1000")), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	sessions, err := opencodeTalks{}.sessions(folders{Root: repo.Root})
	if err != nil || len(sessions) != opencodePage+2 || sessions[len(sessions)-1].ID != "ses_1001" {
		t.Fatalf("both pages: %d %v", len(sessions), err)
	}
	if asked, _ := os.ReadFile(filepath.Join(store, "asked")); strings.Count(string(asked), "\n") != 2 {
		t.Errorf("asked twice, the second from the first's end: %s", asked)
	}
}

// A message the agent is still writing is read as far as opencode keeps
// it, and a view that follows it hears each part as it comes, every turn
// under the number it was first heard by: what it thought and said once
// each is done, a tool from when it is begun, said again as what it is
// handed is written and once it ran, and a commit it made right after it.
// opencode is asked again only once its database was written.
func TestAnOpencodeReplyIsHeardAsItIsWritten(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	store := fakeOpencode(t)
	db := filepath.Join(opencodeHome(), "opencode.db")
	if err := os.WriteFile(db, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	wal := db + "-wal"
	exports := func() int {
		data, _ := os.ReadFile(filepath.Join(store, "asked"))
		return strings.Count(string(data), "session export ses_live")
	}
	asked := `{"type":"user","id":"m1","time":{"created":1793613601000},"text":"fix the cart total"}`
	reply := func(finish string, parts ...string) string {
		return `{"type":"assistant","id":"m2","time":{"created":1793613603000},"content":[` + strings.Join(parts, ",") + `]` + finish + `}`
	}
	thinking := `{"type":"reasoning","text":"","time":{"created":1793613603000}}`
	thought := `{"type":"reasoning","text":"where is the total added up","time":{"created":1793613603000,"completed":1793613605000}}`
	saying, said := `{"type":"text","text":""}`, `{"type":"text","text":"Looking at the cart."}`
	tool := func(id, name, state string) string {
		return fmt.Sprintf(`{"type":"tool","id":%q,"name":%q,"state":%s,"time":{"created":1793613606000}}`, id, name, state)
	}
	stages := []struct {
		messages []string
		want     []string // what is heard: each turn new or grown
	}{
		{[]string{asked, reply("", thinking)}, []string{"1 asked fix the cart total"}},
		{[]string{asked, reply("", thought, saying)}, []string{"2 thought where is the total added up"}},
		{[]string{asked, reply("", thought, said, tool("t1", "read", `{"status":"streaming","input":""}`))},
			[]string{"3 said Looking at the cart.", "4 tool Using read"}},
		{[]string{asked, reply("", thought, said, tool("t1", "read", `{"status":"streaming","input":"{\"path\":\"cart/total.go\"}"}`))},
			[]string{"4 tool Read total.go"}},
		{[]string{asked, reply("", thought, said, tool("t1", "read", `{"status":"completed","input":{"path":"cart/total.go"},"content":[{"type":"text","text":"package cart"}]}`),
			tool("t2", "shell", `{"status":"running","input":{"command":"git commit -am cart"}}`))},
			[]string{"5 tool Ran git commit -am cart"}},
		{[]string{asked, reply(`,"finish":"tool-calls"`, thought, said, tool("t1", "read", `{"status":"completed","input":{"path":"cart/total.go"}}`),
			tool("t2", "shell", `{"status":"completed","input":{"command":"git commit -am cart"},"content":[{"type":"text","text":"[main 0123abc] The cart total adds up"}]}`)),
			`{"type":"assistant","id":"m3","time":{"created":1793613609000},"content":[{"type":"text","text":"It adds up."}],"finish":"stop"}`},
			[]string{"6 commit 0123abc", "7 said It adds up."}},
	}
	write := func(messages []string) {
		t.Helper()
		_, exported := opencodeSaid("ses_live", repo.Root, `,"time":{"created":1793613600000,"busy":1793613602000}`, messages...)
		if err := os.WriteFile(filepath.Join(store, "ses_live.json"), []byte(exported), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(wal, []byte(fmt.Sprint(time.Now().UnixNano())), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(stages[0].messages)
	heard := make(chan []Turn, 16)
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- a.FollowConversation("ses_live", "", 0, stop, func(turns []Turn) error { heard <- turns; return nil })
	}()
	t.Cleanup(func() { close(stop); <-done })
	for i, stage := range stages {
		if i > 0 {
			before := exports()
			time.Sleep(400 * time.Millisecond)
			if exports() != before {
				t.Fatalf("stage %d: exported again with nothing written", i)
			}
			write(stage.messages)
		}
		select {
		case turns := <-heard:
			var got []string
			for _, turn := range turns {
				got = append(got, fmt.Sprintf("%d %s %s%s", turn.N, turn.Kind, turn.Text, turn.Hash))
			}
			if strings.Join(got, "|") != strings.Join(stage.want, "|") {
				t.Fatalf("stage %d heard\n%s\nwant\n%s", i, strings.Join(got, "\n"), strings.Join(stage.want, "\n"))
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("stage %d: nothing heard within two seconds", i)
		}
	}
}

// Where opencode writes its conversations as it goes is its database and
// the log written ahead of it, and its data folder: what a server and a
// view of one look at to hear it wrote. The memory SQLite shares beside
// them is written by reading, and is not looked at.
func TestOpencodeIsHeardWritingItsDatabase(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	fakeOpencode(t)
	home := opencodeHome()
	for _, name := range []string{"opencode.db", "opencode.db-wal", "opencode.db-shm", "opencode-beta.db"} {
		if err := os.WriteFile(filepath.Join(home, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	written := a.ConversationsWritten()
	for _, want := range []string{home, filepath.Join(home, "opencode.db"), filepath.Join(home, "opencode.db-wal"), filepath.Join(home, "opencode-beta.db-wal")} {
		if !slices.Contains(written, want) {
			t.Errorf("%s is not looked at: %v", want, written)
		}
	}
	if slices.Contains(written, filepath.Join(home, "opencode.db-shm")) {
		t.Errorf("what reading writes is looked at: %v", written)
	}
	t.Setenv("OPENCODE_DB", "elsewhere.db")
	if got := (opencodeTalks{}).written(folders{}); !slices.Equal(got, []string{home, filepath.Join(home, "elsewhere.db"), filepath.Join(home, "elsewhere.db-wal")}) {
		t.Errorf("the database OPENCODE_DB names: %v", got)
	}
}
