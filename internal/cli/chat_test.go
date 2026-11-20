package cli

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// heardByWindow is a message as the window reads it.
type heardByWindow struct {
	V     int    `json:"v"`
	Pane  string `json:"pane"`
	Title string `json:"title"`
	View  *struct {
		T, ID, Agent   string
		Reply, Working bool
		Turns          []app.Turn
	} `json:"view"`
	Patch []struct {
		Op, ID string
		Turn   *app.Turn
		On     *bool
	} `json:"patch"`
	Close bool `json:"close"`
}

// read is a message read back, which no message the window takes fails:
// in ASCII alone, and no longer than a message may be.
func read(t *testing.T, message []byte) heardByWindow {
	t.Helper()
	if len(message) > muiLimit {
		t.Fatalf("a message of %d bytes, over the %d the window takes", len(message), muiLimit)
	}
	if at := slices.IndexFunc(message, func(b byte) bool { return b >= 0x80 || b < 0x20 }); at >= 0 {
		t.Fatalf("byte %#x in a message: %.80q", message[at], message)
	}
	var got heardByWindow
	if err := json.Unmarshal(message, &got); err != nil {
		t.Fatalf("%v in %.200s", err, message)
	}
	return got
}

func numbers(turns []app.Turn) []int {
	var ns []int
	for _, turn := range turns {
		ns = append(ns, turn.N)
	}
	return ns
}

// The view is said whole the first time turns come, an empty conversation
// too, with the turns the window can read: one of a kind it does not know,
// or with no words, would be left out by it, and drop a patch whole.
func TestAChatIsSaidWholeTheFirstTimeTurnsCome(t *testing.T) {
	view := &talk{pane: "%12", title: "claude · monduli", agent: "claude", reply: true, working: true}
	messages := view.heard([]app.Turn{
		{N: 1, Kind: "asked", Text: "make the list faster — “now”"},
		{N: 2, Kind: "said", Text: ""},
		{N: 3, Kind: "did", Text: "mia new goba", Verb: "new"},
		{N: 4, Kind: "question", Text: "Go on?", Options: []string{"Yes", "No"}},
		{N: 5, Kind: "answer", Text: "Yes", Question: 4},
		{N: 6, Kind: "commit", Hash: "0123abc", Subject: "Sort once"},
	})
	if len(messages) != 1 {
		t.Fatalf("%d messages", len(messages))
	}
	got := read(t, messages[0])
	if got.V != 1 || got.Pane != "%12" || got.Title != "claude · monduli" || got.View == nil || got.Patch != nil {
		t.Fatalf("message = %s", messages[0])
	}
	if v := got.View; v.T != "chat" || v.ID != "chat" || v.Agent != "claude" || !v.Reply || !v.Working {
		t.Errorf("view = %+v", v)
	}
	if ns := numbers(got.View.Turns); !slices.Equal(ns, []int{1, 4, 5, 6}) {
		t.Errorf("turns %v", ns)
	}
	if first := got.View.Turns[0]; first.Text != "make the list faster — “now”" {
		t.Errorf("asked = %q", first.Text)
	}

	empty := &talk{agent: "pi"}
	got = read(t, empty.heard(nil)[0])
	if got.View == nil || got.View.Turns == nil || len(got.View.Turns) != 0 || got.View.Reply || got.Pane != "" {
		t.Errorf("an empty conversation is said as %+v", got.View)
	}
}

// A conversation longer than a message may be is said by its last turns
// that fit, in bytes and in turns, and a turn too long for any message by
// the first of its words.
func TestAConversationTooLongIsSaidByItsLastTurnsThatFit(t *testing.T) {
	var many []app.Turn
	for n := 1; n <= 3000; n++ {
		many = append(many, app.Turn{N: n, Kind: "tool", Text: "Read a file"})
	}
	view := &talk{agent: "claude"}
	got := read(t, view.heard(many)[0])
	if ns := numbers(got.View.Turns); len(ns) != chatTurns || ns[0] != 3000-chatTurns+1 || ns[len(ns)-1] != 3000 {
		t.Errorf("%d turns, %d to %d", len(ns), ns[0], ns[len(ns)-1])
	}

	var long []app.Turn
	for n := 1; n <= 20; n++ {
		long = append(long, app.Turn{N: n, Kind: "said", Text: strings.Repeat("word ", 8000)})
	}
	view = &talk{agent: "claude"}
	got = read(t, view.heard(long)[0])
	if ns := numbers(got.View.Turns); len(ns) != 6 || ns[0] != 15 || ns[5] != 20 {
		t.Errorf("of 20 turns of 40 KB, said %v", ns)
	}

	view = &talk{agent: "claude"}
	got = read(t, view.heard([]app.Turn{{N: 1, Kind: "said", Text: strings.Repeat("é", 2*muiProse)}})[0])
	if len(got.View.Turns) != 1 || !strings.HasPrefix(got.View.Turns[0].Text, "ééé") || len([]rune(got.View.Turns[0].Text)) != muiProse/4 {
		t.Errorf("one turn too long is said as %d turns", len(got.View.Turns))
	}
}

// What a followed conversation gains is said as one patch of turn changes,
// a turn that grew again under its n; more than a patch may change, or
// hold, as several.
func TestTurnsFollowedAreSaidAsAPatchOfTurnChanges(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude"}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "make it faster"}})
	messages := view.heard([]app.Turn{{N: 2, Kind: "said", Text: "I will"}, {N: 3, Kind: "did", Text: "mia ls"}})
	messages = append(messages, view.heard([]app.Turn{{N: 2, Kind: "said", Text: "I will look"}})...)
	if len(messages) != 2 {
		t.Fatalf("%d messages", len(messages))
	}
	for i, want := range []string{"I will", "I will look"} {
		got := read(t, messages[i])
		if got.V != 1 || got.Pane != "%3" || got.View != nil || len(got.Patch) != 1 {
			t.Fatalf("message = %s", messages[i])
		}
		if op := got.Patch[0]; op.Op != "turn" || op.ID != "chat" || op.Turn == nil || op.Turn.N != 2 || op.Turn.Kind != "said" || op.Turn.Text != want {
			t.Errorf("change = %s", messages[i])
		}
	}
	if messages := view.heard([]app.Turn{{N: 4, Kind: "did", Text: "mia ls"}}); len(messages) != 0 {
		t.Errorf("turns the window cannot read are said: %s", messages)
	}

	var many []app.Turn
	for n := 10; n < 10+muiOps+1; n++ {
		many = append(many, app.Turn{N: n, Kind: "tool", Text: "Read a file"})
	}
	messages = view.heard(many)
	if len(messages) != 2 || len(read(t, messages[0]).Patch) != muiOps || len(read(t, messages[1]).Patch) != 1 {
		t.Errorf("%d turns are said in %d patches", len(many), len(messages))
	}
	messages = view.heard([]app.Turn{
		{N: 600, Kind: "said", Text: strings.Repeat("word ", 12000)},
		{N: 601, Kind: "said", Text: strings.Repeat("word ", 12000)},
		{N: 602, Kind: "said", Text: strings.Repeat("word ", 12000)},
		{N: 603, Kind: "said", Text: strings.Repeat("word ", 12000)},
		{N: 604, Kind: "said", Text: strings.Repeat("word ", 12000)},
	})
	if len(messages) != 2 || len(read(t, messages[0]).Patch) != 4 || len(read(t, messages[1]).Patch) != 1 {
		t.Errorf("five turns of 60 KB are said in %d patches", len(messages))
	}
}

// The window takes no patch that leaves a UI of more pieces than it may
// have: the view is then said again whole, by its last turns, with room
// for what comes after.
func TestAChatGrownPastWhatTheWindowTakesIsSaidAgainWhole(t *testing.T) {
	var turns []app.Turn
	for n := 1; n <= chatTurns; n++ {
		turns = append(turns, app.Turn{N: n, Kind: "tool", Text: "Read a file"})
	}
	view := &talk{agent: "claude"}
	view.heard(turns)
	var more []app.Turn
	for n := chatTurns + 1; n < muiPieces; n++ {
		more = append(more, app.Turn{N: n, Kind: "tool", Text: "Read a file"})
	}
	if messages := view.heard(more); len(messages) != 1 || len(read(t, messages[0]).Patch) != len(more) {
		t.Fatalf("up to what the window takes, said in %d messages", len(messages))
	}
	if messages := view.heard([]app.Turn{{N: muiPieces - 1, Kind: "tool", Text: "Read two files"}}); len(messages) != 1 || read(t, messages[0]).Patch == nil {
		t.Error("a turn that grew adds no piece, and is not a patch")
	}
	messages := view.heard([]app.Turn{{N: muiPieces, Kind: "said", Text: "Done."}})
	if len(messages) != 1 {
		t.Fatalf("%d messages", len(messages))
	}
	got := read(t, messages[0])
	if got.View == nil || len(got.View.Turns) != chatTurns || got.View.Turns[chatTurns-1].Text != "Done." {
		t.Errorf("one turn more is said as %.120s", messages[0])
	}
	if messages := view.heard([]app.Turn{{N: muiPieces + 1, Kind: "said", Text: "And more."}}); len(messages) != 1 || read(t, messages[0]).Patch == nil {
		t.Error("the turn after is not a patch again")
	}
}

// Whether the agent is at work is said when it changes, and only once the
// view was said: a patch to no view is dropped.
func TestWorkingIsSaidWhenItChanges(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude"}
	if messages := view.atWork(true); len(messages) != 0 || !view.working {
		t.Errorf("before the view is said: %s", messages)
	}
	if got := read(t, view.heard(nil)[0]); !got.View.Working {
		t.Error("the view does not say it works")
	}
	if messages := view.atWork(true); len(messages) != 0 {
		t.Errorf("unchanged, and said: %s", messages)
	}
	messages := view.atWork(false)
	if len(messages) != 1 {
		t.Fatalf("%d messages", len(messages))
	}
	if string(messages[0]) != `{"v":1,"pane":"%3","patch":[{"op":"working","id":"chat","on":false}]}` {
		t.Errorf("patch = %s", messages[0])
	}
}

// Inside tmux a message goes in tmux's passthrough, each ESC of it doubled.
func TestAMessageInTmuxGoesInItsPassthrough(t *testing.T) {
	if got := string(framed([]byte(`{"v":1}`), false)); got != "\033_Mui {\"v\":1}\033\\" {
		t.Errorf("outside tmux: %q", got)
	}
	if got := string(framed([]byte(`{"v":1}`), true)); got != "\033Ptmux;\033\033_Mui {\"v\":1}\033\033\\\033\\" {
		t.Errorf("inside tmux: %q", got)
	}
}

// Each event the window types is read once, wherever a read cuts it, its
// mark and its paste's brackets too.
func TestAnEventCutAcrossTwoReadsIsReadOnce(t *testing.T) {
	paste := func(event string) string { return "\033[200~\x1fmia:" + event + "\033[201~" }
	all := paste(`{"e":"send","id":"chat","text":"go on — {now}"}`) + paste(`{"e":"choose","id":"chat","question":4,"option":1}`) + paste(`{"e":"stop","id":"chat"}`)
	want := []chatEvent{{E: "send", Text: "go on — {now}"}, {E: "choose", Question: 4, Option: 1}, {E: "stop"}}
	for cut := 0; cut <= len(all); cut++ {
		first, rest, _, leave := typed("", all[:cut])
		second, rest, _, left := typed(rest, all[cut:])
		if got := append(first, second...); !slices.Equal(got, want) || rest != "" || leave || left {
			t.Fatalf("cut at %d (%q): events %+v, kept %q", cut, all[:cut], got, rest)
		}
	}
	// what does not read is passed over, and the event after it is read
	if got, rest := chatEvents("\x1fmia:nonsense" + paste(`{"e":"close"}`)); !slices.Equal(got, []chatEvent{{E: "close"}}) || rest != "" {
		t.Errorf("events %+v, kept %q", got, rest)
	}
	if _, rest := chatEvents("\x1f and then typing"); rest != "" {
		t.Errorf("kept %q, which begins no mark", rest)
	}
	if _, rest := chatEvents("\x1fmia:{\"e\":\"" + strings.Repeat("x", chatEventMost)); rest != "" {
		t.Errorf("kept %d bytes of an event that never ends", len(rest))
	}
}

// q or ⌃C typed alone in the plain terminal leaves, and not as part of an
// event; the pane coming into sight is told from what else was read.
func TestQAndControlCLeaveAndFocusIsSeen(t *testing.T) {
	for _, key := range []string{"q", "\x03"} {
		if _, _, _, leave := typed("", key); !leave {
			t.Errorf("%q does not leave", key)
		}
	}
	cut := "\033[200~\x1fmia:{\"e\":\"send\",\"id\":\"chat\",\"text\":\""
	_, rest, _, leave := typed("", cut)
	events, _, _, left := typed(rest, "q")
	if leave || left || len(events) != 0 {
		t.Errorf("a q in what is sent leaves: %+v", events)
	}
	if events, _, _, _ := typed(rest+"q", "\"}\033[201~"); !slices.Equal(events, []chatEvent{{E: "send", Text: "q"}}) {
		t.Errorf("events = %+v", events)
	}
	if _, _, _, leave := typed("", "quit"); leave {
		t.Error("typing that begins with q leaves")
	}
	if _, _, sight, leave := typed("", "\033[O\033[I"); !sight || leave {
		t.Error("the pane came into sight, and it is not seen")
	}
	if _, _, sight, leave := typed("", "\033[Iq"); !sight || !leave {
		t.Error("q after the pane came into sight does not leave")
	}
	if _, _, sight, _ := typed("", "\033[O"); sight {
		t.Error("the pane went out of sight, and it is seen as coming in")
	}
}

// With no flag a chat is of the conversation a window has open, before one
// written to later that none has; else of the last written to. A window
// or an id picks one, and what there is none of is refused.
func TestAChatIsOfTheConversationAWindowHasOpen(t *testing.T) {
	heads := []app.Head{
		{ID: "c3", Agent: "codex"},
		{ID: "c2", Agent: "claude", Running: true, Window: &app.WindowRef{Index: 2, Name: "claude"}},
		{ID: "c1", Agent: "pi", Running: true, Window: &app.WindowRef{Index: 4, Name: "pi"}},
	}
	for _, c := range []struct{ window, id, want string }{
		{"", "", "c2"}, {"4", "", "c1"}, {"", "c3", "c3"}, {"", "c1", "c1"}, {"2", "c2", "c2"},
		{"3", "", ""}, {"", "c9", ""}, {"4", "c2", ""}, {"2", "c3", ""},
	} {
		got, err := chatOf(heads, "monduli", c.window, c.id)
		if got.ID != c.want || (err == nil) != (c.want != "") {
			t.Errorf("window %q, conversation %q: %q, %v; want %q", c.window, c.id, got.ID, err, c.want)
		}
	}
	if got, err := chatOf(heads[:1], "monduli", "", ""); got.ID != "c3" || err != nil {
		t.Errorf("with none open: %q, %v", got.ID, err)
	}
	if _, err := chatOf(nil, "monduli", "", ""); err == nil || !strings.Contains(err.Error(), "monduli has no conversation yet") {
		t.Errorf("with none at all: %v", err)
	}
}

// A chat reads what the person does from a terminal, and is refused where
// stdin is none; a window that is no number is a usage error.
func TestChatRefusesWhatIsNotATerminal(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	stdin := os.Stdin
	os.Stdin = null
	defer func() { os.Stdin = stdin }()
	if code := Run([]string{"chat"}); code != exitFailed {
		t.Errorf("chat exited %d, want %d", code, exitFailed)
	}
	for _, args := range [][]string{{"chat", "--window", "claude"}, {"chat", "monduli"}} {
		if code := Run(args); code != exitUsage {
			t.Errorf("%v exited %d, want %d", args, code, exitUsage)
		}
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Features, "chat") {
		t.Error("capabilities do not list chat")
	}
}

// A window with no conversation yet is said as one quiet line, not a chat.
func TestAWindowWithNoConversationYetIsSaidAsALineThatWaits(t *testing.T) {
	var said struct {
		V    int               `json:"v"`
		Pane string            `json:"pane"`
		View map[string]string `json:"view"`
	}
	if err := json.Unmarshal(chatWaits("%4", "2"), &said); err != nil {
		t.Fatal(err)
	}
	if said.V != 1 || said.Pane != "%4" || said.View["t"] != "text" || !strings.Contains(said.View["text"], "window 2") {
		t.Errorf("said = %+v", said)
	}
}
