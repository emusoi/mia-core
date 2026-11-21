package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/session"
)

// shownPiece is a piece of a view as the window reads it.
type shownPiece struct {
	T, ID, Title, Text, Style, Tone, Label, Key, Agent, Placeholder, Icon, Right, List, Menu, Selected string
	Reply, Working, Bare, Live, Flat, Split                                                            bool
	Grow, W, H, Max                                                                                    int
	Children                                                                                           []shownPiece
	Items                                                                                              []chatItem
	Turns                                                                                              []shownTurn
}

// shownTurn is a turn of a chat as the window reads it: a transcript's, or
// a row of the chat's own.
type shownTurn struct {
	app.Turn
	From, To, Note, Tone string
}

// find is the first piece of a kind in a piece, itself too, and nil for none.
func (p *shownPiece) find(kind string) *shownPiece {
	if p == nil || p.T == kind {
		return p
	}
	for i := range p.Children {
		if found := p.Children[i].find(kind); found != nil {
			return found
		}
	}
	return nil
}

// heardByWindow is a message as the window reads it: View is the chat of
// the view said whole, which Whole is.
type heardByWindow struct {
	V      int         `json:"v"`
	Pane   string      `json:"pane"`
	Fill   bool        `json:"fill"`
	Focus  string      `json:"focus"`
	Status *muiStatus  `json:"status"`
	Whole  *shownPiece `json:"view"`
	View   *shownPiece `json:"-"`
	Keys   []chatKey   `json:"keys"`
	Patch  []struct {
		Op, ID, Text, Tone string
		N                  int
		Turn               *shownTurn
		On                 *bool
		Node               *shownPiece
	} `json:"patch"`
	Close bool `json:"close"`
}

// pieceOf is a piece read back as the window reads it.
func pieceOf(t *testing.T, said json.RawMessage) *shownPiece {
	t.Helper()
	var got shownPiece
	if err := json.Unmarshal(said, &got); err != nil {
		t.Fatalf("%v in %.200s", err, said)
	}
	return &got
}

// ops is the changes of a patch, each by its name.
func ops(got heardByWindow) []string {
	var names []string
	for _, op := range got.Patch {
		names = append(names, op.Op)
	}
	return names
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
	got.View = got.Whole.find("chat")
	return got
}

// only is the one message of some, read back.
func only(t *testing.T, messages [][]byte) heardByWindow {
	t.Helper()
	if len(messages) != 1 {
		t.Fatalf("%d messages: %.300s", len(messages), messages)
	}
	return read(t, messages[0])
}

func numbers(turns []shownTurn) []int {
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
	view := &talk{pane: "%12", about: "claude · opus · on work", agent: "claude", reply: true, working: true}
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
	if got.V != 1 || got.Pane != "%12" || got.View == nil || got.Patch != nil {
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
	most := muiPieces - chatFrame // the turns a view has room for among its other pieces
	var more []app.Turn
	for n := chatTurns + 1; n <= most; n++ {
		more = append(more, app.Turn{N: n, Kind: "tool", Text: "Read a file"})
	}
	if messages := view.heard(more); len(messages) != 1 || len(read(t, messages[0]).Patch) != len(more) {
		t.Fatalf("up to what the window takes, said in %d messages", len(messages))
	}
	if messages := view.heard([]app.Turn{{N: most, Kind: "tool", Text: "Read two files"}}); len(messages) != 1 || read(t, messages[0]).Patch == nil {
		t.Error("a turn that grew adds no piece, and is not a patch")
	}
	messages := view.heard([]app.Turn{{N: most + 1, Kind: "said", Text: "Done."}})
	if len(messages) != 1 {
		t.Fatalf("%d messages", len(messages))
	}
	got := read(t, messages[0])
	if got.View == nil || len(got.View.Turns) != chatTurns || got.View.Turns[chatTurns-1].Text != "Done." {
		t.Errorf("one turn more is said as %.120s", messages[0])
	}
	if messages := view.heard([]app.Turn{{N: most + 2, Kind: "said", Text: "And more."}}); len(messages) != 1 || read(t, messages[0]).Patch == nil {
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
	want := []chatEvent{{E: "send", ID: "chat", Text: "go on — {now}"}, {E: "choose", ID: "chat", Question: 4, Option: 1}, {E: "stop", ID: "chat"}}
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
	if events, _, _, _ := typed(rest+"q", "\"}\033[201~"); !slices.Equal(events, []chatEvent{{E: "send", ID: "chat", Text: "q"}}) {
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

// The view fills its pane: one quiet line over the chat that says who
// answers, a piece said again alone, with the buttons that show the
// models and the earlier conversations at its right, and the keys on the
// chat's field. A conversation no window has open has no field, and no
// models to switch.
func TestTheChatFillsItsPaneUnderALineThatSaysWhoAnswers(t *testing.T) {
	if got := chatAbout(app.Runs{Agent: "claude", Model: "opus", Effort: "high", Account: "work"}); got != "claude · opus · high · on work" {
		t.Errorf("about = %q", got)
	}
	if got := chatAbout(app.Runs{Agent: "opencode"}); got != "opencode" {
		t.Errorf("of an agent mia chose nothing for: %q", got)
	}
	view := &talk{pane: "%2", agent: "claude", about: "claude · opus · high · on work", reply: true}
	got := only(t, view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}}))
	if !got.Fill || got.Focus != "chat" || got.Whole.T != "col" || got.Whole.ID != "view" || len(got.Whole.Children) != 2 || got.Whole.Children[1].T != "chat" || len(got.Keys) != 0 {
		t.Fatalf("view = %+v", got.Whole)
	}
	line := got.Whole.Children[0]
	if line.T != "row" || len(line.Children) != 4 || line.Children[0].Text != "claude · opus · high · on work" || line.Children[0].ID != "about" || line.Children[0].Style != "dim" || line.Children[1].Grow != 1 {
		t.Errorf("line = %+v", line)
	}
	if m, e := line.Children[2], line.Children[3]; m.T != "button" || m.ID != "open-models" || m.Label != "models" || m.Key != "m" || e.T != "button" || e.ID != "open-earlier" || e.Label != "earlier" || e.Key != "e" {
		t.Errorf("buttons = %+v", line.Children[2:])
	}

	shown := &talk{agent: "codex", about: "codex"}
	got = only(t, shown.heard(nil))
	if line := got.Whole.Children[0]; got.Focus != "" || got.View.Reply || len(line.Children) != 3 || line.Children[2].ID != "open-earlier" {
		t.Errorf("one no window has open: focus %q, line %+v", got.Focus, line)
	}
}

// What is sent is said at once as an asked turn after the last, at a place
// of the app's own, and when the transcript says the same words that turn
// is taken out as the transcript's is put in, in one patch to the same
// chat: nothing is said twice.
func TestWhatWasSentShowsAtOnceAndTheTranscriptsTurnTakesItsPlace(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude", reply: true}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}, {N: 2, Kind: "said", Text: "Hello."}})
	got := only(t, view.sends("make it faster"))
	if len(got.Patch) != 1 || got.Patch[0].Op != "turn" || got.Patch[0].ID != "chat" || got.Patch[0].Turn.N != chatOwn+1 || got.Patch[0].Turn.Kind != "asked" || got.Patch[0].Turn.Text != "make it faster" {
		t.Fatalf("sent is said as %+v", got.Patch)
	}
	got = only(t, view.heard([]app.Turn{{N: 3, Kind: "asked", Text: "make it faster\n"}, {N: 4, Kind: "said", Text: "I will"}}))
	if !slices.Equal(ops(got), []string{"unturn", "turn", "turn"}) || got.Patch[0].ID != "chat" || got.Patch[0].N != chatOwn+1 || got.Patch[1].Turn.N != 3 || len(view.sent) != 0 || len(view.own) != 0 {
		t.Errorf("the transcript's turn: %+v, %d still its own", got.Patch, len(view.own))
	}
	if whole := read(t, view.whole()); whole.View.ID != "chat" || !slices.Equal(numbers(whole.View.Turns), []int{1, 2, 3, 4}) {
		t.Errorf("view = %+v", whole.View)
	}
}

// A window with no conversation yet is a chat all the same: what is sent
// from it is shown at once, from the first turn, and the conversation
// that then begins with those words goes on from them.
func TestAWindowWithNoConversationYetIsAChatThatCanSayTheFirstThing(t *testing.T) {
	view := &talk{pane: "%4", agent: "codex", reply: true}
	got := read(t, view.whole())
	if got.View == nil || !got.View.Reply || got.View.Agent != "codex" || len(got.View.Turns) != 0 {
		t.Fatalf("view = %+v", got.View)
	}
	view.sends("what is in here?")
	got = only(t, view.sends("and why?"))
	if len(got.Patch) != 1 || got.Patch[0].Turn.N != chatOwn+2 || got.Patch[0].Turn.Text != "and why?" {
		t.Errorf("the second thing sent: %+v", got.Patch)
	}
	got = only(t, view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "what is in here?"}, {N: 2, Kind: "asked", Text: "and why?"}, {N: 3, Kind: "said", Text: "Files."}}))
	if !slices.Equal(ops(got), []string{"unturn", "unturn", "turn", "turn", "turn"}) || got.Patch[1].N != chatOwn+2 || got.Patch[4].Turn.N != 3 || len(view.sent) != 0 {
		t.Errorf("its conversation begun: %+v, sent %+v", got.Patch, view.sent)
	}
}

// An agent at work takes up what was sent later: when another turn comes
// first, the turn said for it is taken out of the same chat, and the
// status, a change of the same patch, says it waits. It is said once the
// transcript has it.
func TestWhatAnAgentAtWorkTakesUpLaterIsNotSaidTwice(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude", reply: true}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}})
	view.sends("and the tests")
	got := only(t, view.heard([]app.Turn{{N: 2, Kind: "tool", Text: "Read a file"}}))
	if got.View != nil || !slices.Equal(ops(got), []string{"unturn", "turn", "status"}) || got.Patch[0].ID != "chat" || got.Patch[0].N != chatOwn+1 || got.Patch[1].Turn.N != 2 || got.Patch[1].Turn.Kind != "tool" {
		t.Fatalf("said %+v", got)
	}
	if status := got.Patch[2]; status.Text != "claude takes it up when it can — sent: and the tests" || status.Tone != "" || view.status == nil || view.status.Text != status.Text {
		t.Errorf("status = %+v, kept %+v", status, view.status)
	}
	got = only(t, view.heard([]app.Turn{{N: 3, Kind: "asked", Text: "and the tests"}}))
	if len(got.Patch) != 1 || got.Patch[0].ID != "chat" || got.Patch[0].Turn.N != 3 {
		t.Errorf("the transcript's turn: %+v", got)
	}
	// the view said whole, as when its pane comes into sight, carries the status
	if whole := read(t, view.whole()); whole.Status == nil || !strings.HasPrefix(whole.Status.Text, "claude takes it up") || !slices.Equal(numbers(whole.View.Turns), []int{1, 2, 3}) {
		t.Errorf("said whole: %+v, %+v", whole.Status, whole.View)
	}
}

// A send the window refuses takes its turn out, gives the words back to
// the chat's field as they were typed, and says why as the status: one
// patch, and the view is not said again.
func TestASendRefusedTakesItsTurnOutAndGivesTheWordsBack(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude", reply: true}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}})
	view.sends("go on\nwith it")
	got := only(t, view.refused(errors.New("claude asks first: Trust this folder?")))
	if got.View != nil || !slices.Equal(ops(got), []string{"unturn", "draft", "status"}) || len(view.sent) != 0 || len(view.own) != 0 {
		t.Fatalf("said %+v, sent %+v", got, view.sent)
	}
	if out, draft, status := got.Patch[0], got.Patch[1], got.Patch[2]; out.ID != "chat" || out.N != chatOwn+1 || draft.ID != "chat" || draft.Text != "go on\nwith it" ||
		status.Tone != "bad" || status.Text != "claude asks first: Trust this folder?" {
		t.Errorf("patch = %+v", got.Patch)
	}
	view.sends(strings.Repeat("é", 2*chatDraft))
	if got = only(t, view.refused(errors.New(strings.Repeat("why ", 100)))); len([]rune(got.Patch[1].Text)) != chatDraft || len([]rune(got.Patch[2].Text)) != chatSaid {
		t.Errorf("long ones: %d characters given back, %d said", len([]rune(got.Patch[1].Text)), len([]rune(got.Patch[2].Text)))
	}
	if messages := view.refused(errors.New("nothing was sent")); messages != nil {
		t.Errorf("with nothing sent: %s", messages)
	}
}

// The status is said alone, as a change, and taken away by one with no
// words. How a thing done went is a toast when it went well, which goes
// by itself and is not kept; otherwise the status, which the view said
// whole carries.
func TestTheStatusIsAChangeAndWhatWentWellAToast(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude", reply: true}
	if messages := view.patch(view.stands(&muiStatus{Text: "said before the view"})); messages != nil || view.status == nil {
		t.Errorf("before the view is said: %s", messages)
	}
	view.heard(nil)
	messages := view.patch(view.stands(&muiStatus{Text: "claude's replies are in its terminal", Tone: "warn"}))
	if len(messages) != 1 || string(messages[0]) != `{"v":1,"pane":"%3","patch":[{"op":"status","text":"claude's replies are in its terminal","tone":"warn"}]}` {
		t.Errorf("status = %s", messages)
	}
	if messages = view.patch(view.stands(nil)); len(messages) != 1 || string(messages[0]) != `{"v":1,"pane":"%3","patch":[{"op":"status"}]}` || view.status != nil {
		t.Errorf("taken away = %s", messages)
	}
	toast := only(t, view.patch(view.went("claude stopped · codex goes on with this conversation", "ok")))
	if len(toast.Patch) != 1 || toast.Patch[0].Op != "toast" || toast.Patch[0].Text != "claude stopped · codex goes on with this conversation" || toast.Patch[0].Tone != "ok" || view.status != nil {
		t.Errorf("went well = %+v, status %+v", toast.Patch, view.status)
	}
	got := only(t, view.patch(view.went("codex goes on with this conversation · claude's window is still open: no window 2", "warn")))
	if len(got.Patch) != 1 || got.Patch[0].Op != "status" || got.Patch[0].Tone != "warn" || view.status == nil || read(t, view.whole()).Status == nil {
		t.Errorf("went otherwise = %+v", got.Patch)
	}
}

// What the window's screen asks that the transcript does not — a
// permission — is a question of the app's own after the last turn, and
// taken out once answered; asked otherwise, it is another question at
// another place. A transcript that ends on a question nobody answered is
// the asking itself. An answer the window refused is said as the status,
// and the question is open again: the app's own at another place, the
// transcript's taken out and said again with what is said next.
func TestWhatTheScreenAsksIsAQuestionOfTheChatsOwn(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude", reply: true}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}, {N: 2, Kind: "tool", Text: "Run make"}})
	asks := &app.Asks{Text: "Run make?", Options: []string{"Yes", "No"}, Selected: 1}
	got := only(t, view.asked(asks))
	if len(got.Patch) != 1 || got.Patch[0].Turn.N != chatOwn+1 || got.Patch[0].Turn.Kind != "question" || got.Patch[0].Turn.Text != "Run make?" || !slices.Equal(got.Patch[0].Turn.Options, []string{"Yes", "No"}) {
		t.Fatalf("asked is said as %+v", got.Patch)
	}
	if messages := view.asked(&app.Asks{Text: "Run make?", Options: []string{"Yes", "No"}, Selected: 2}); messages != nil {
		t.Errorf("the cursor moved, and it is said again: %s", messages)
	}
	// sent while it asks: the question stands after what was sent
	got = only(t, view.sends("yes, run it"))
	if !slices.Equal(ops(got), []string{"unturn", "turn", "turn"}) || got.Patch[0].N != chatOwn+1 || got.Patch[1].Turn.N != chatOwn+2 || got.Patch[1].Turn.Kind != "asked" || got.Patch[2].Turn.N != chatOwn+3 || got.Patch[2].Turn.Kind != "question" {
		t.Errorf("sent while it asks: %+v", got.Patch)
	}
	view.refused(errors.New("claude asks first"))
	got = only(t, view.reopened(chatOwn+3, errors.New("the cursor did not move to option 2, and nothing was answered")))
	if !slices.Equal(ops(got), []string{"unturn", "turn", "status"}) || got.Patch[0].N != chatOwn+3 || got.Patch[1].Turn.N != chatOwn+4 || got.Patch[1].Turn.Text != "Run make?" || got.Patch[2].Tone != "bad" || !strings.HasPrefix(got.Patch[2].Text, "the cursor did not move") {
		t.Errorf("its own question refused: %+v", got.Patch)
	}
	got = only(t, view.asked(nil))
	if got.View != nil || len(got.Patch) != 1 || got.Patch[0].Op != "unturn" || got.Patch[0].ID != "chat" || got.Patch[0].N != chatOwn+4 {
		t.Errorf("answered: %+v", got)
	}
	if got = only(t, view.asked(&app.Asks{Options: []string{"Yes", "No"}})); got.Patch[0].Turn.Text != "claude asks" || got.Patch[0].Turn.N != chatOwn+5 {
		t.Errorf("one with no words: %+v", got.Patch[0].Turn)
	}
	view.asked(nil)

	view.heard([]app.Turn{{N: 3, Kind: "question", Text: "Which one?", Options: []string{"A", "B"}}})
	if messages := view.asked(asks); len(messages) != 0 {
		t.Errorf("the transcript's own question, said twice: %s", messages)
	}
	got = only(t, view.reopened(3, errors.New("the cursor did not move to option 2, and nothing was answered")))
	if got.View != nil || !slices.Equal(ops(got), []string{"unturn", "status"}) || got.Patch[0].ID != "chat" || got.Patch[0].N != 3 || got.Patch[1].Tone != "bad" || !strings.HasPrefix(got.Patch[1].Text, "the cursor did not move") {
		t.Errorf("refused: %+v", got)
	}
	if got = only(t, view.asked(asks)); len(got.Patch) != 1 || got.Patch[0].Op != "turn" || got.Patch[0].Turn.N != 3 || got.Patch[0].Turn.Kind != "question" || got.Patch[0].Turn.Answer != "" {
		t.Errorf("said again, open: %+v", got.Patch)
	}
	if messages := view.asked(asks); len(messages) != 0 {
		t.Errorf("said again twice: %s", messages)
	}
	// answered, and the screen asks on: its question comes after what came since
	got = only(t, view.heard([]app.Turn{{N: 4, Kind: "answer", Text: "A", Question: 3}, {N: 5, Kind: "tool", Text: "Run make"}}))
	if len(got.Patch) != 3 || got.Patch[2].Turn.N <= chatOwn || got.Patch[2].Turn.Kind != "question" || got.Patch[2].Turn.Text != "Run make?" {
		t.Errorf("after an answered question: %+v", got)
	}
}

func chatAgents() []app.AgentModels {
	return []app.AgentModels{
		{Name: "claude", Models: []plugin.Model{{ID: "opus", Label: "Opus"}, {ID: "sonnet", Label: "Sonnet"}}, Efforts: []string{"low", "high"}, Effort: "low", Live: true, Installed: true},
		{Name: "codex", Models: []plugin.Model{{ID: "gpt-6-luna", Label: "GPT-6 Luna"}}, Efforts: []string{"low", "high"}, Live: true, Installed: true},
		{Name: "pi", Models: []plugin.Model{{ID: "anthropic/claude-opus-5", Label: "Claude Opus 5"}}},
		{Name: "opencode", Installed: true},
	}
}

// The models are a modal over the chat: a bare search line over one list
// in a room of its own — the setups used here first, then each installed
// agent's under its head, the chat's own agent's installed or not, an
// agent with none once — the one that runs checked and under the keys, an
// item's menu its efforts with the one it runs at checked; what is typed
// narrows them. It is put over the chat by a patch, and the chat is not
// said again.
func TestTheModelsAreAModalOverTheChat(t *testing.T) {
	view := &chatView{talk: &talk{pane: "%5", agent: "claude", reply: true}, worktree: "monduli", window: "2",
		runs:   app.Runs{Agent: "claude", Model: "opus", Effort: "high", Account: "work", Live: true},
		agents: chatAgents(),
		setups: []app.Setup{{Agent: "codex", Model: "gpt-6-luna", Effort: "high"}, {Agent: "claude", Role: "reviewer"}, {Agent: "gone", Model: "x"}}}
	view.talk.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}})
	got := only(t, view.up(picking))
	if got.View != nil || len(got.Patch) != 1 || got.Patch[0].Op != "add" || got.Patch[0].ID != "view" || view.mode != picking || view.talk.keys != "q" {
		t.Fatalf("put up as %+v", got)
	}
	modal := got.Patch[0].Node
	if modal.T != "modal" || modal.ID != "over" || modal.Title != "Model" || modal.W != 520 || len(modal.Children) != 3 || modal.Children[1].T != "divider" || modal.find("chat") != nil {
		t.Fatalf("modal = %+v", modal)
	}
	field, list := modal.Children[0], modal.Children[2]
	if field.T != "field" || field.ID != "q" || !field.Bare || !field.Live || field.List != "models" || field.Right != "5 models" || field.Icon != "search" {
		t.Errorf("field = %+v", field)
	}
	if list.T != "list" || list.ID != "models" || list.Menu != "effort" || list.Max != 11 || list.Selected != "a|claude|opus" {
		t.Fatalf("list = %+v", list)
	}
	var said []string
	for _, item := range list.Items {
		said = append(said, cmp.Or(item.ID, "# "+item.Head))
	}
	if want := []string{"# Used here", "u|codex|high|gpt-6-luna", "# Claude", "a|claude|opus", "a|claude|sonnet", "# Codex", "a|codex|gpt-6-luna", "# Opencode", "a|opencode|"}; !slices.Equal(said, want) {
		t.Errorf("items %q", said)
	}
	if head := list.Items[2]; head.Tag != "this chat" || head.Note != "on work" {
		t.Errorf("head = %+v", head)
	}
	opus := list.Items[3]
	if !opus.Checked || opus.Title != "Opus" || opus.Icon != "claude" || opus.Does != "switch" || opus.More != "High" || len(opus.Menu) != 2 || opus.Menu[0].Checked || !opus.Menu[1].Checked || opus.Menu[1].ID != "high" || opus.Menu[1].Title != "High" {
		t.Errorf("the one that runs = %+v", opus)
	}
	if used := list.Items[1]; used.Checked || used.Note != "codex" || used.Right != "High" || used.Does != "go on in codex" || len(used.About) != 2 {
		t.Errorf("a setup used here = %+v", used)
	}
	if none := list.Items[8]; none.Note != "its model is chosen inside it" || len(none.Menu) != 0 {
		t.Errorf("an agent with no models = %+v", none)
	}

	// what is typed says the modal again, narrowed: the keys and the field's words stay the person's
	view.query = " LUNA"
	got = only(t, view.again())
	if len(got.Patch) != 1 || got.Patch[0].Op != "set" || got.Patch[0].ID != "over" || got.Patch[0].Node.T != "modal" {
		t.Fatalf("said again as %+v", got.Patch)
	}
	if list := got.Patch[0].Node.find("list"); len(list.Items) != 4 || list.Items[1].ID != "u|codex|high|gpt-6-luna" || list.Items[3].ID != "a|codex|gpt-6-luna" || list.Selected != "" {
		t.Errorf("narrowed to %+v", list.Items)
	}
	if right := got.Patch[0].Node.find("field").Right; right != "2 models" {
		t.Errorf("the field counts %q", right)
	}
	// why a pick was not done is said at the modal's foot, in it
	view.told = &muiStatus{Text: "claude cannot be switched to another model while it runs", Tone: "bad"}
	if foot := pieceOf(t, view.models(false)).Children; len(foot) != 4 || foot[3].T != "text" || foot[3].Tone != "bad" || !strings.HasPrefix(foot[3].Text, "claude cannot be switched") {
		t.Errorf("foot = %+v", foot)
	}

	// more models than a modal holds are cut to those that fit, and counted all the same
	var many []plugin.Model
	for n := range 2 * muiPieces {
		many = append(many, plugin.Model{ID: "p/m" + strconv.Itoa(n), Label: "m" + strconv.Itoa(n)})
	}
	crowd := &chatView{talk: &talk{agent: "opencode"}, mode: picking, runs: app.Runs{Agent: "opencode"}, agents: []app.AgentModels{{Name: "opencode", Models: many, Installed: true}}}
	cut := pieceOf(t, crowd.models(false))
	if rows := len(cut.find("list").Items); rows > chatFrame-16 || rows < chatHeads || cut.find("field").Right != counted(len(many), "model") || len(crowd.models(false)) > muiLimit-2*patchEnvelope {
		t.Errorf("%d models are said in %d rows, %d bytes", len(many), rows, len(crowd.models(false)))
	}

	if rows := chatModels(chatAgents(), nil, "pi", ""); !slices.ContainsFunc(rows, func(m chatModel) bool { return m.agent == "pi" && m.model == "anthropic/claude-opus-5" }) {
		t.Errorf("the chat's own agent, not installed, is not listed: %+v", rows)
	}
	if row, ok := chatModelOf(chatModel{agent: "pi", model: "a|b/c"}.id()); !ok || row.agent != "pi" || row.model != "a|b/c" {
		t.Errorf("a model's id read back: %+v", row)
	}
}

// A modal is taken away by a patch that changes nothing else: esc leaves
// the chat, its turns, its status and what runs as they were. A pick that
// switched takes it away in the patch that says the line over the chat
// again and what was done as a toast. While it is up the chat's own
// changes go on being said, and the view said whole — its pane come into
// sight — is the chat with the keys on the modal's piece, then the modal
// put back.
func TestAModalIsPutOverTheChatAndTakenAwayByPatches(t *testing.T) {
	for mode, keys := range map[chatMode]string{picking: "q", looking: "earlier"} {
		view := &chatView{talk: &talk{pane: "%5", agent: "claude", about: "claude · opus", reply: true, status: &muiStatus{Text: "as it was"}}, worktree: "monduli", window: "2", id: "c2",
			runs: app.Runs{Agent: "claude", Model: "opus", Live: true}, agents: chatAgents()}
		view.listed([]app.Head{{ID: "c2", Agent: "claude"}, {ID: "c1", Agent: "codex"}}, time.Now())
		view.talk.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}})
		view.up(mode)
		if got := view.deed(chatEvent{E: "dismiss", ID: "over"}); got != (chatDeed{do: chatBack}) {
			t.Fatalf("esc in mode %d asks %+v", mode, got)
		}
		// a turn comes while it is up: a patch to the chat behind it
		if got := only(t, view.talk.heard([]app.Turn{{N: 2, Kind: "said", Text: "Hello."}})); len(got.Patch) != 1 || got.Patch[0].Op != "turn" || got.Patch[0].ID != "chat" {
			t.Errorf("in mode %d a turn is said as %+v", mode, got)
		}
		again := view.talk.said()
		if len(again) != 2 {
			t.Fatalf("in mode %d said whole in %d messages", mode, len(again))
		}
		whole, back := read(t, again[0]), read(t, again[1])
		if whole.Focus != keys || whole.Whole.find("modal") != nil || !slices.Equal(numbers(whole.View.Turns), []int{1, 2}) ||
			len(back.Patch) != 1 || back.Patch[0].Op != "add" || back.Patch[0].ID != "view" || back.Patch[0].Node.T != "modal" || back.Patch[0].Node.find("list").Selected != "" {
			t.Errorf("in mode %d said whole as focus %q, then %+v", mode, whole.Focus, back.Patch)
		}
		messages := view.shut()
		if len(messages) != 1 || string(messages[0]) != `{"v":1,"pane":"%5","patch":[{"op":"remove","id":"over"}]}` {
			t.Errorf("in mode %d taken away as %s", mode, messages)
		}
		if view.mode != chatting || view.talk.over != nil || view.talk.status == nil || view.runs.Model != "opus" || view.id != "c2" || len(view.talk.said()) != 1 {
			t.Errorf("in mode %d, after esc: %+v", mode, view)
		}
		if whole = read(t, view.talk.whole()); whole.Focus != "chat" || whole.Status == nil || whole.Status.Text != "as it was" {
			t.Errorf("in mode %d, the chat after esc: focus %q, status %+v", mode, whole.Focus, whole.Status)
		}
	}

	view := &chatView{talk: &talk{pane: "%5", agent: "claude", about: "claude · opus", reply: true}, window: "2", runs: app.Runs{Agent: "claude", Model: "opus", Live: true}, agents: chatAgents()}
	view.talk.heard(nil)
	view.up(picking)
	view.talk.about = "claude · sonnet · low"
	got := only(t, view.shut(encode(chatOp{Op: "set", ID: "about", Node: chatHead(view.talk.about)}), view.talk.went("claude switched to sonnet low", "ok")))
	if !slices.Equal(ops(got), []string{"remove", "set", "toast"}) || got.Patch[1].ID != "about" || got.Patch[1].Node.ID != "about" || got.Patch[1].Node.Text != "claude · sonnet · low" ||
		got.Patch[2].Text != "claude switched to sonnet low" || got.Patch[2].Tone != "ok" || view.talk.status != nil {
		t.Errorf("a switch is said as %+v", got.Patch)
	}
}

// opencode's models are under its head as it lists them, each named by
// what comes after its provider with the provider beside it, the one its
// conversation runs on checked; a pick starts it again on the model in
// its window, and from another agent's chat hands the conversation to it.
func TestOpencodesModelsAreInThePaletteAndAPickStartsItAgain(t *testing.T) {
	agents := []app.AgentModels{
		{Name: "claude", Models: []plugin.Model{{ID: "opus", Label: "Opus"}}, Efforts: []string{"low", "high"}, Live: true, Installed: true},
		{Name: "pi", Models: []plugin.Model{{ID: "anthropic/claude-opus-5", Label: "Claude Opus 5"}}, Live: true, Installed: true},
		{Name: "opencode", Models: []plugin.Model{{ID: "openai/gpt-5", Label: "gpt-5"}, {ID: "ollama/qwen3:4b", Label: "qwen3:4b"}}, Efforts: []string{}, Again: true, Installed: true},
	}
	// mia chose no model for the window: the conversation's own is the one that runs
	runs := chatRuns(app.Runs{Agent: "opencode", Reads: true, Again: true}, app.Head{Agent: "opencode", Model: "openai/gpt-5"})
	if runs.Model != "openai/gpt-5" || chatAbout(runs) != "opencode · openai/gpt-5" {
		t.Fatalf("runs = %+v", runs)
	}
	if kept := chatRuns(app.Runs{Agent: "opencode", Model: "ollama/qwen3:4b"}, app.Head{Agent: "opencode", Model: "openai/gpt-5"}); kept.Model != "ollama/qwen3:4b" {
		t.Errorf("what mia chose is not said over: %+v", kept)
	}
	if other := chatRuns(app.Runs{Agent: "claude"}, app.Head{Agent: "opencode", Model: "openai/gpt-5"}); other.Model != "" {
		t.Errorf("another agent's conversation says nothing of this one: %+v", other)
	}
	view := &chatView{talk: &talk{pane: "%5", agent: "opencode"}, mode: picking, worktree: "monduli", window: "3", id: "ses_1", runs: runs, agents: agents}
	list := pieceOf(t, view.models(true)).find("list")
	var said []string
	for _, item := range list.Items {
		said = append(said, cmp.Or(item.ID, "# "+item.Head))
	}
	if want := []string{"# Claude", "a|claude|opus", "# Pi", "a|pi|anthropic/claude-opus-5", "# Opencode", "a|opencode|openai/gpt-5", "a|opencode|ollama/qwen3:4b"}; !slices.Equal(said, want) || list.Selected != "a|opencode|openai/gpt-5" {
		t.Fatalf("items %q, the keys on %q", said, list.Selected)
	}
	if head := list.Items[4]; head.Tag != "this chat" {
		t.Errorf("head = %+v", head)
	}
	on := list.Items[5]
	if !on.Checked || on.Title != "gpt-5" || on.Note != "openai" || on.Icon != "opencode" || on.Does != "switch" || on.More != "" || len(on.Menu) != 0 ||
		len(on.About) != 2 || on.About[1] != "starts it again in this window, on this conversation" {
		t.Errorf("the one that runs = %+v", on)
	}
	if other := list.Items[6]; other.Checked || other.Title != "qwen3:4b" || other.Note != "ollama" || other.Does != "switch" {
		t.Errorf("another of its models = %+v", other)
	}
	if named := list.Items[3]; named.Note != "" || named.Does != "go on in pi" {
		t.Errorf("a model with a name of its own says no provider: %+v", named)
	}
	if got := view.deed(chatEvent{E: "pick", List: "models", ID: "a|opencode|ollama/qwen3:4b"}); got != (chatDeed{do: chatSwitch, model: "ollama/qwen3:4b"}) {
		t.Errorf("a pick of its own asks %+v", got)
	}
	if got := view.deed(chatEvent{E: "pick", List: "models", ID: "a|claude|opus", Sub: "high"}); got != (chatDeed{do: chatHand, agent: "claude", model: "opus", effort: "high"}) {
		t.Errorf("a pick of another agent's asks %+v", got)
	}
	view.query = "OLLAMA"
	if list := pieceOf(t, view.models(false)).find("list"); len(list.Items) != 2 || list.Items[1].ID != "a|opencode|ollama/qwen3:4b" {
		t.Errorf("narrowed by a provider to %+v", list.Items)
	}

	// from claude's chat, one of opencode's is the conversation handed over to it on that model
	view = &chatView{talk: &talk{pane: "%5", agent: "claude"}, mode: picking, worktree: "monduli", window: "2", id: "c2", runs: app.Runs{Agent: "claude", Live: true}, agents: agents}
	if got := view.deed(chatEvent{E: "pick", List: "models", ID: "a|opencode|openai/gpt-5"}); got != (chatDeed{do: chatHand, agent: "opencode", model: "openai/gpt-5"}) {
		t.Errorf("a pick of opencode's from another agent's chat asks %+v", got)
	}
	// with none listed it is as it was: one row, its model chosen inside it
	agents[2].Models = []plugin.Model{}
	view.agents, view.query = agents, ""
	list = pieceOf(t, view.models(false)).find("list")
	if none := list.Items[len(list.Items)-1]; none.ID != "a|opencode|" || none.Note != "its model is chosen inside it" {
		t.Errorf("with no models listed = %+v", none)
	}
}

// What an event asks of the app, as the view stands.
func TestWhatAnEventAsksOfTheChat(t *testing.T) {
	running := app.Head{ID: "c2", Agent: "claude", Running: true, Window: &app.WindowRef{Index: 2, Name: "claude"}}
	view := &chatView{talk: &talk{agent: "claude", reply: true}, worktree: "monduli", window: "2", id: "c2",
		runs: app.Runs{Agent: "claude", Model: "opus", Effort: "high", Live: true}, agents: chatAgents()}
	view.listed([]app.Head{running, {ID: "c1", Agent: "codex"}, {ID: "c0", Agent: "claude"}, {ID: "c1", Agent: "codex", Account: "other"}}, time.Now())
	if len(view.heads) != 3 || view.on != "c2" {
		t.Fatalf("listed %d, the keys on %q", len(view.heads), view.on)
	}
	for _, c := range []struct {
		mode  chatMode
		event chatEvent
		want  chatDeed
	}{
		{chatting, chatEvent{E: "close"}, chatDeed{do: chatLeave}},
		{chatting, chatEvent{E: "send", Text: " go on \n"}, chatDeed{do: chatSend, text: "go on"}},
		{chatting, chatEvent{E: "send", Text: "  "}, chatDeed{}},
		{chatting, chatEvent{E: "choose", Question: 4, Option: 1}, chatDeed{do: chatChoose, question: 4, option: 1}},
		{chatting, chatEvent{E: "stop"}, chatDeed{do: chatStop}},
		{chatting, chatEvent{E: "press", ID: "open-models"}, chatDeed{do: chatModelsUp}},
		{chatting, chatEvent{E: "press", ID: "open-earlier"}, chatDeed{do: chatEarlier}},
		{chatting, chatEvent{E: "dismiss", ID: "over"}, chatDeed{}},
		{picking, chatEvent{E: "send", Text: "typed into the chat before"}, chatDeed{}},
		{picking, chatEvent{E: "press", ID: "open-earlier"}, chatDeed{}},
		{picking, chatEvent{E: "dismiss", ID: "over"}, chatDeed{do: chatBack}},
		{picking, chatEvent{E: "change", ID: "q", Text: "lu"}, chatDeed{do: chatNarrow, text: "lu"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "a|claude|sonnet"}, chatDeed{do: chatSwitch, model: "sonnet"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "a|claude|sonnet", Sub: "low"}, chatDeed{do: chatSwitch, model: "sonnet", effort: "low"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "u|claude|high|opus"}, chatDeed{do: chatSwitch, model: "opus", effort: "high"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "a|codex|gpt-6-luna", Sub: "high"}, chatDeed{do: chatHand, agent: "codex", model: "gpt-6-luna", effort: "high"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "u|codex|high|gpt-6-luna"}, chatDeed{do: chatHand, agent: "codex", model: "gpt-6-luna", effort: "high"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "a|opencode|"}, chatDeed{do: chatHand, agent: "opencode"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "nonsense"}, chatDeed{}},
		{looking, chatEvent{E: "dismiss", ID: "over"}, chatDeed{do: chatBack}},
		{looking, chatEvent{E: "dismiss", ID: "another"}, chatDeed{}},
		{looking, chatEvent{E: "move", List: "earlier", ID: "c1"}, chatDeed{do: chatRead, id: "c1"}},
		{looking, chatEvent{E: "move", List: "earlier", ID: "c2"}, chatDeed{}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c2"}, chatDeed{do: chatShow, id: "c2"}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c0"}, chatDeed{do: chatGoOn, id: "c0"}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c9"}, chatDeed{}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c2", Sub: "resume"}, chatDeed{do: chatShow, id: "c2"}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c0", Sub: "resume"}, chatDeed{do: chatGoOn, id: "c0"}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c1", Sub: "continue"}, chatDeed{do: chatContinue, id: "c1"}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c0", Sub: "continue"}, chatDeed{}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c1", Sub: "copy"}, chatDeed{do: chatCopy, id: "c1"}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c9", Sub: "copy"}, chatDeed{}},
		{looking, chatEvent{E: "press", ID: "continue", Item: "c0"}, chatDeed{}},
		{looking, chatEvent{E: "press", ID: "continue", Item: "c1"}, chatDeed{do: chatSay, id: "c1",
			text: "c again hands this to claude in a new window: what was said and each thing done carry over, what codex thought and its tools' output do not"}},
	} {
		view.mode = c.mode
		if got := view.deed(c.event); got != c.want {
			t.Errorf("%+v in mode %d asks %+v, want %+v", c.event, c.mode, got, c.want)
		}
	}
	view.mode, view.armed = looking, "c1"
	if got := view.deed(chatEvent{E: "press", ID: "continue", Item: "c1"}); got != (chatDeed{do: chatContinue, id: "c1"}) {
		t.Errorf("c pressed again asks %+v", got)
	}
	view.agents[0].Live = false
	view.mode = picking
	if got := view.deed(chatEvent{E: "pick", List: "models", ID: "a|claude|sonnet"}); got.do != chatSay || !strings.Contains(got.text, "cannot be switched") {
		t.Errorf("an agent that is not switched while it runs asks %+v", got)
	}

	// a conversation no window has open is only shown: its earlier ones too
	view.mode, view.window = chatting, ""
	for _, event := range []chatEvent{{E: "send", Text: "go on"}, {E: "choose"}, {E: "stop"}, {E: "press", ID: "open-models"}} {
		if got := view.deed(event); got != (chatDeed{}) {
			t.Errorf("with no window, %+v asks %+v", event, got)
		}
	}
	if got := view.deed(chatEvent{E: "press", ID: "open-earlier"}); got.do != chatEarlier {
		t.Errorf("with no window, earlier asks %+v", got)
	}
}

// Earlier is a modal over the chat: the worktree's conversations as a
// list in a room of its own beside the one under the keys read back, a
// chat with no field in a column of a height of its own. A row says what
// was first asked, the agent and when at the right, the one a window has
// open running, c on one of another agent; its menu, which the pointer's
// other button opens too, is what can be done to it.
func TestEarlierIsAModalOfAListBesideTheConversationUnderTheKeys(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	view := &chatView{talk: &talk{pane: "%5", agent: "claude"}, worktree: "monduli", window: "2", id: "c2", runs: app.Runs{Agent: "claude"}}
	view.listed([]app.Head{
		{ID: "c2", Agent: "claude", Asked: "make the list faster", Active: now, Running: true, Window: &app.WindowRef{Index: 2, Name: "claude"}},
		{ID: "c1", Agent: "codex", Asked: "why is the tab a pill", Active: now.Add(-4 * 24 * time.Hour)},
		{ID: "c0", Agent: "claude", Active: now.Add(-3 * time.Hour)},
	}, now)
	view.reads("c1", []app.Turn{{N: 1, Kind: "asked", Text: "why is the tab a pill"}, {N: 2, Kind: "did", Text: "mia ls"}, {N: 3, Kind: "said", Text: "The hover style won."}})
	view.talk.heard(nil)
	got := only(t, view.up(looking))
	if got.View != nil || len(got.Patch) != 1 || got.Patch[0].Op != "add" || got.Patch[0].ID != "view" || view.mode != looking || view.talk.keys != "earlier" {
		t.Fatalf("put up as %+v", got)
	}
	modal := got.Patch[0].Node
	if modal.T != "modal" || modal.ID != "over" || modal.Title != "Earlier" || modal.W != 720 || modal.H != 420 || len(modal.Children) != 2 {
		t.Fatalf("modal = %+v", modal)
	}
	if line := modal.Children[0]; line.T != "text" || line.Text != "in monduli · 3 conversations" || line.Style != "dim" {
		t.Errorf("line = %+v", line)
	}
	split := modal.Children[1]
	if split.T != "row" || !split.Split || len(split.Children) != 2 || split.Children[0].T != "col" || split.Children[0].W != 300 || split.Children[1].T != "col" || split.Children[1].H != 0 {
		t.Fatalf("split = %+v", split)
	}
	list := split.Children[0].find("list")
	if list == nil || list.ID != "earlier" || !list.Live || !list.Flat || list.Max != 12 || list.Menu != "more" || list.Selected != "c1" || len(list.Items) != 3 {
		t.Fatalf("list = %+v", list)
	}
	// (a short list is made as tall as the chat beside it by heads with no words, which the keys pass over)
	if slices.ContainsFunc(list.Items[3:], func(gap chatItem) bool { return gap.ID != "" || gap.Head != " " || gap.Title != "" }) {
		t.Errorf("under its rows = %+v", list.Items[3:])
	}
	first, second, third := list.Items[0], list.Items[1], list.Items[2]
	if first.Title != "make the list faster" || first.Right != "claude · now" || first.Running == nil || !*first.Running || first.Icon != "claude" || len(first.Keys) != 0 || first.Does != "back to it" {
		t.Errorf("the running one = %+v", first)
	}
	if second.Right != "codex · 4d" || *second.Running || len(second.Keys) != 1 || second.Keys[0] != (chatKey{Key: "c", Label: "continue", ID: "continue"}) || second.Does != "go on with it" {
		t.Errorf("another agent's = %+v", second)
	}
	if third.Title != "nothing asked yet" || third.Right != "claude · 3h" || len(third.Keys) != 0 {
		t.Errorf("one with nothing asked = %+v", third)
	}
	menu := func(item chatItem) []string {
		var said []string
		for _, one := range item.Menu {
			said = append(said, one.ID+": "+one.Title)
		}
		return said
	}
	if got := menu(first); !slices.Equal(got, []string{"resume: Go to it", "copy: Copy its id"}) {
		t.Errorf("the running one's menu = %q", got)
	}
	if got := menu(second); !slices.Equal(got, []string{"resume: Resume", "continue: Continue with claude", "copy: Copy its id"}) {
		t.Errorf("another agent's menu = %q", got)
	}
	if got := menu(third); !slices.Equal(got, []string{"resume: Resume", "copy: Copy its id"}) {
		t.Errorf("the same agent's menu = %q", got)
	}
	if beside := split.Children[1].Children[0]; beside.T != "chat" || beside.ID != "read-c1" || beside.Agent != "codex" || beside.Reply || !slices.Equal(numbers(beside.Turns), []int{1, 3}) {
		t.Errorf("read back = %+v", beside)
	}

	// the keys moved: the modal is said again with that one read back, and the keys are not put anywhere
	view.reads("c0", nil)
	got = only(t, view.again())
	if len(got.Patch) != 1 || got.Patch[0].Op != "set" || got.Patch[0].ID != "over" || got.Patch[0].Node.find("list").Selected != "" || got.Patch[0].Node.find("chat").ID != "read-c0" {
		t.Errorf("said again as %+v", got.Patch)
	}
	// what c does, said the first time it is pressed, is at the modal's foot
	view.told = &muiStatus{Text: "c again hands this to claude in a new window", Tone: "warn"}
	if foot := pieceOf(t, view.earlier(false)).Children; len(foot) != 3 || foot[2].T != "text" || foot[2].Tone != "warn" || foot[2].Text != "c again hands this to claude in a new window" {
		t.Errorf("foot = %+v", foot)
	}
	view.told = nil

	var long []app.Turn
	for n := 1; n <= 3*chatReadMost; n++ {
		long = append(long, app.Turn{N: n, Kind: "tool", Text: "Read a file"})
	}
	view.reads("c0", long)
	if beside := pieceOf(t, view.earlier(false)).find("chat"); len(beside.Turns) != chatReadMost || beside.Turns[chatReadMost-1].N != 3*chatReadMost {
		t.Errorf("a long one is read back by %d turns", len(beside.Turns))
	}
	// as many conversations as it lists, each read back by as many turns: a modal the window takes over a chat grown full
	var heads []app.Head
	for n := range 2 * chatHeads {
		heads = append(heads, app.Head{ID: "c" + strconv.Itoa(n), Agent: "codex", Asked: strings.Repeat("w", chatSaid)})
	}
	view.listed(heads, now)
	view.reads("c0", long)
	full := pieceOf(t, view.earlier(true))
	if rows, turns := len(full.find("list").Items), len(full.find("chat").Turns); rows != chatHeads || turns != chatReadMost || rows+turns+16 > chatFrame || len(view.earlier(true)) > muiLimit-2*patchEnvelope {
		t.Errorf("%d rows beside %d turns, in %d bytes", rows, turns, len(view.earlier(true)))
	}
	// and of rows whose words are long in a message's bytes, those that fit it
	for n := range heads {
		heads[n].Asked = strings.Repeat("é", chatSaid)
	}
	view.listed(heads, now)
	full = pieceOf(t, view.earlier(true))
	if rows := len(full.find("list").Items); rows == 0 || rows >= chatHeads || len(full.find("chat").Turns) != chatReadMost || len(view.earlier(true)) > muiLimit-2*patchEnvelope {
		t.Errorf("%d long rows, in %d bytes", rows, len(view.earlier(true)))
	}
}

// The id of a conversation is put on the clipboard by a change, and a
// toast says so.
func TestAConversationsIdIsCopiedByAChange(t *testing.T) {
	view := &talk{pane: "%5", agent: "claude"}
	view.heard(nil)
	messages := view.patch(encode(chatOp{Op: "copy", Text: "c1"}), view.went("copied c1", "ok"))
	if len(messages) != 1 || string(messages[0]) != `{"v":1,"pane":"%5","patch":[{"op":"copy","text":"c1"},{"op":"toast","text":"copied c1","tone":"ok"}]}` {
		t.Errorf("copied as %s", messages)
	}
}

// fakeMia is mia as a handing asks of it: what was asked, in order, and
// what it answers.
type fakeMia struct {
	asked   []string
	done    app.Continued
	refuses error
	stays   error
}

func (f *fakeMia) ContinueConversation(want app.Continue) (app.Continued, error) {
	f.asked = append(f.asked, fmt.Sprintf("continue %s in %s with %s %s %s quiet=%v", want.ID, want.Worktree, want.With, want.Model, want.Effort, want.Quiet))
	return f.done, f.refuses
}

func (f *fakeMia) CloseWindow(record model.Record, window string) error {
	f.asked = append(f.asked, "close "+record.Name+" "+window)
	return f.stays
}

// A model of another agent picked hands the conversation to that agent,
// for every pair of the agents mia knows: it is started quiet on the
// model in a window of its own, and only once it runs is the window of
// the agent it was with closed. Refused, ended as it started, or this
// chat's own window: nothing is closed, and what is said says why.
func TestAnotherAgentsModelHandsTheConversationOverAndClosesTheOldWindowLast(t *testing.T) {
	record := model.Record{Name: "monduli"}
	agents := []app.AgentModels{
		{Name: "claude", Models: []plugin.Model{{ID: "opus"}}, Efforts: []string{"high"}, Live: true, Installed: true},
		{Name: "codex", Models: []plugin.Model{{ID: "gpt-6-luna"}}, Efforts: []string{"high"}, Live: true, Installed: true},
		{Name: "pi", Models: []plugin.Model{{ID: "anthropic/claude-opus-5"}}, Efforts: []string{"high"}, Live: true, Installed: true},
		{Name: "opencode", Installed: true}, {Name: "gemini", Installed: true}, {Name: "cursor-agent", Installed: true},
	}
	for _, from := range agents {
		for _, to := range agents {
			if from.Name == to.Name {
				continue
			}
			view := &chatView{talk: &talk{agent: from.Name, reply: true}, mode: picking, worktree: "monduli", window: "2", id: "c1", runs: app.Runs{Agent: from.Name}, agents: agents}
			pick := chatModel{agent: to.Name}
			sub := ""
			if len(to.Models) > 0 {
				pick.model, sub = to.Models[0].ID, "high"
			}
			deed := view.deed(chatEvent{E: "pick", List: "models", ID: pick.id(), Sub: sub})
			if deed != (chatDeed{do: chatHand, agent: to.Name, model: pick.model, effort: sub}) {
				t.Fatalf("%s to %s asks %+v", from.Name, to.Name, deed)
			}
			mia := &fakeMia{done: app.Continued{Sent: true, To: app.ContinuedTo{Agent: to.Name, Window: app.WindowRef{Index: 5, Name: to.Name}}}}
			went := handTo(mia, record, view.id, view.window, from.Name, deed.agent, deed.model, deed.effort)
			want := []string{fmt.Sprintf("continue c1 in monduli with %s %s %s quiet=true", to.Name, pick.model, sub), "close monduli 2"}
			if !slices.Equal(mia.asked, want) || !went.went || went.tone != "ok" || went.text != from.Name+" stopped · "+to.Name+" goes on with this conversation" {
				t.Errorf("%s to %s: asked %q, went %+v", from.Name, to.Name, mia.asked, went)
			}
		}
	}

	mia := &fakeMia{refuses: errors.New("nothing is said yet in claude's conversation c1")}
	if went := handTo(mia, record, "c1", "2", "claude", "codex", "", ""); went.went || went.tone != "bad" || went.text != "nothing is said yet in claude's conversation c1" || len(mia.asked) != 1 {
		t.Errorf("refused: %+v, asked %q", went, mia.asked)
	}
	mia = &fakeMia{done: app.Continued{Problem: "codex ended as soon as it started, saying:\n  not signed in"}}
	if went := handTo(mia, record, "c1", "2", "claude", "codex", "", ""); went.went || went.tone != "bad" || !strings.HasPrefix(went.text, "claude still runs — codex ended as soon as it started") || len(mia.asked) != 1 {
		t.Errorf("the other agent ended: %+v, asked %q", went, mia.asked)
	}
	mia = &fakeMia{done: app.Continued{Sent: true}}
	if went := handTo(mia, record, "c1", "", "claude", "codex", "", ""); !went.went || went.text != "codex goes on with this conversation · claude's window stays" || len(mia.asked) != 1 {
		t.Errorf("with no window to close: %+v, asked %q", went, mia.asked)
	}
	mia = &fakeMia{done: app.Continued{Sent: true}, stays: errors.New("no window 2")}
	if went := handTo(mia, record, "c1", "2", "claude", "codex", "", ""); !went.went || went.tone != "warn" || !strings.Contains(went.text, "claude's window is still open: no window 2") {
		t.Errorf("a window that would not close: %+v", went)
	}

	// with nothing mia can read there is nothing to hand over
	view := &chatView{talk: &talk{agent: "gemini", reply: true}, mode: picking, worktree: "monduli", window: "2", runs: app.Runs{Agent: "gemini"}, agents: agents}
	if deed := view.deed(chatEvent{E: "pick", List: "models", ID: "a|codex|gpt-6-luna"}); deed.do != chatSay || !strings.Contains(deed.text, "nothing for codex to go on with") {
		t.Errorf("with no conversation asks %+v", deed)
	}
	if deed := view.deed(chatEvent{E: "pick", List: "models", ID: "a|gemini|"}); deed.do != chatSay || !strings.Contains(deed.text, "takes none from mia") {
		t.Errorf("its own agent, which takes no model, asks %+v", deed)
	}
}

// The first thing sent to an agent that goes on with another's
// conversation has that conversation in front of it, in one message, and
// the second is the words alone; a conversation longer than is pasted in
// one go is said by its last turns under where the whole of it is. The
// chat shows the person's own words: of the transcript's turn that has
// the conversation in front, only what follows the rule.
func TestTheFirstThingSentAfterAHandingCarriesTheConversation(t *testing.T) {
	handoff := "# Handoff: a conversation with claude, to go on from\n\n## The conversation\n\n### 1 · user\n\nmake it faster\n\n### 2 · claude\n\nIt is faster.\n"
	first := carried("claude", handoff, "/repo/.git/mia/handoffs/monduli/claude-c1.md", "and the tests", chatCarry)
	want := "The conversation so far, with claude:\n\n" + strings.TrimSpace(handoff) + "\n\n" + chatRule + "\n\nand the tests"
	if first != want {
		t.Errorf("the first thing sent is %q", first)
	}
	if got := ownWords(first); got != "and the tests" {
		t.Errorf("the person's own words of it: %q", got)
	}
	if got := ownWords("a --- rule of the person's own"); got != "a --- rule of the person's own" {
		t.Errorf("words with no conversation in front: %q", got)
	}

	long := "# Handoff\n\n## The conversation\n"
	for n := 1; n <= 400; n++ {
		long += fmt.Sprintf("\n### %d · user\n\n%s\n", n, strings.Repeat("word ", 60))
	}
	cut := carried("claude", long, "/repo/full.md", "go on", chatCarry)
	if len(cut) > chatCarry+500 || !strings.HasPrefix(cut, "The conversation so far, with claude:\n\n[Only its last turns are here: the whole of it is in /repo/full.md]\n\n### ") ||
		!strings.Contains(cut, "\n### 400 · user\n") || strings.Contains(cut, "\n### 1 · user\n") || !strings.HasSuffix(cut, chatRule+"\n\ngo on") {
		t.Errorf("a long one is said in %d bytes: %.200q … %.80q", len(cut), cut, cut[len(cut)-80:])
	}

	// the chat goes on from the one before: its turns stay over the switch, which waits, and what is heard comes after
	chat := &talk{pane: "%3", agent: "codex", reply: true, nth: 1}
	chat.lays([]chatPart{{turns: []app.Turn{{N: 1, Kind: "asked", Text: "make it faster"}, {N: 2, Kind: "said", Text: "It is faster."}},
		then: switched(app.Handed{From: app.Talk{Agent: "claude"}, To: app.Talk{Agent: "codex"}, Turns: 2}), waits: true}, {own: true}})
	got := read(t, chat.whole())
	if band := got.View.Turns[2]; !slices.Equal(numbers(got.View.Turns), []int{1, 2, 3}) || band.Kind != "switch" || band.From != "claude" || band.To != "codex" ||
		band.Note != "context goes with your next message" || band.Tone != "warn" || got.View.ID != "chat-1" {
		t.Fatalf("gone on: %+v", got.View)
	}
	sent := only(t, chat.sends("and the tests"))
	if sent.Patch[0].Turn.N != chatOwn+1 || sent.Patch[0].Turn.Text != "and the tests" {
		t.Errorf("sent is said as %+v", sent.Patch[0].Turn)
	}
	// sent with it: the switch says what was handed over, at the place kept for it, and waits no more
	went := only(t, chat.patch(chat.handed()...))
	if band := went.Patch[1].Turn; !slices.Equal(ops(went), []string{"unturn", "turn"}) || went.Patch[0].N != 3 || band.N != 4 || band.Kind != "switch" || band.Note != "context handed over · 2 turns" || band.Tone != "" || chat.handed() != nil {
		t.Errorf("handed over: %+v", went.Patch)
	}
	heard := only(t, chat.heard([]app.Turn{{N: 1, Kind: "asked", Text: first}, {N: 2, Kind: "question", Text: "Which?", Options: []string{"A", "B"}}, {N: 3, Kind: "answer", Text: "A", Question: 2}}))
	if !slices.Equal(ops(heard), []string{"unturn", "turn", "turn", "turn"}) || heard.Patch[0].N != chatOwn+1 || heard.Patch[1].ID != "chat-1" || heard.Patch[1].Turn.N != 5 || heard.Patch[1].Turn.Text != "and the tests" ||
		heard.Patch[3].Turn.N != 7 || heard.Patch[3].Turn.Question != 6 || len(chat.sent) != 0 {
		t.Errorf("its transcript's turns: %+v, sent %+v", heard.Patch, chat.sent)
	}
	// a model changed now stands after the last turn heard, and what is heard next after it
	row := chat.put(chatBand{Kind: "model", From: "gpt-6-luna", To: "gpt-6-luna-mini"})
	if next := only(t, chat.heard([]app.Turn{{N: 4, Kind: "said", Text: "A it is."}})); row.n != 8 || next.Patch[0].Turn.N != 9 {
		t.Errorf("a model's line at %d, the turn after it at %+v", row.n, next.Patch[0].Turn)
	}
	// another chat of the window sent what it goes on from: the switch waits no more once the agent writes of it
	other := &talk{pane: "%4", agent: "codex", reply: true}
	other.lays([]chatPart{{turns: []app.Turn{{N: 1, Kind: "asked", Text: "make it faster"}}, then: switched(app.Handed{From: app.Talk{Agent: "claude"}, To: app.Talk{Agent: "codex"}}), waits: true}, {own: true}})
	other.whole()
	if told := only(t, other.heard([]app.Turn{{N: 1, Kind: "asked", Text: first}})); !slices.Equal(ops(told), []string{"turn", "unturn", "turn"}) || told.Patch[1].N != 2 || told.Patch[2].Turn.N != 3 || told.Patch[2].Turn.Note != "context handed over" {
		t.Errorf("sent by another chat: %+v", told.Patch)
	}
}

// What a window runs on, changed in place, is said in a line: its models,
// or its efforts where the model stays; nothing changed, nothing is said.
func TestAModelChangedInPlaceIsSaidInALine(t *testing.T) {
	for _, c := range []struct {
		was, now app.Runs
		from, to string
	}{
		{app.Runs{Model: "opus", Effort: "high"}, app.Runs{Model: "sonnet", Effort: "high"}, "opus", "sonnet"},
		{app.Runs{}, app.Runs{Model: "sonnet"}, "default", "sonnet"},
		{app.Runs{Model: "opus", Effort: "high"}, app.Runs{Model: "opus", Effort: "low"}, "opus high", "opus low"},
		{app.Runs{Model: "opus", Effort: "high"}, app.Runs{Model: "opus", Effort: "high"}, "", ""},
	} {
		band, changed := modelled(c.was, c.now)
		if changed != (c.from != "") || changed && band != (chatBand{Kind: "model", From: c.from, To: c.to}) {
			t.Errorf("%+v to %+v: %+v %v", c.was, c.now, band, changed)
		}
	}
}

// A chat of a conversation that went from agent to agent shows each switch
// where it happened, by the handoffs' records: the turns said with each
// agent in the order they were said, the agent that took its own
// conversation up again with its earlier turns before those said with the
// other meanwhile, a model changed in this chat after the turn it was
// changed after, and the last switch waiting while its window still keeps
// what it goes on from. Nothing of it is kept by the chat: one started
// again says the same.
func TestAChatSaysEachSwitchOfAgentsWhereItHappened(t *testing.T) {
	gittest.Isolate(t)
	claude := filepath.Join(os.Getenv("HOME"), ".claude")
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("switched")
	if err != nil {
		t.Fatal(err)
	}
	write := func(file string, lines ...string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cwd, _ := json.Marshal(record.Path)
	carries := func(from, words string) string {
		text, _ := json.Marshal(carried(from, "### 1 · user\n\nmake it faster\n", "/h/full.md", words, chatCarry))
		return string(text)
	}
	says := func(kind, at, content string) string {
		return `{"type":"` + kind + `","cwd":` + string(cwd) + `,"timestamp":"2026-11-02T` + at + `Z","message":{"content":` + content + `}}`
	}
	write(filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(record.Path, "-"), "c1.jsonl"),
		says("user", "10:00:00", `"make it faster"`), says("assistant", "10:00:05", `[{"type":"text","text":"It is faster."}]`),
		says("user", "10:04:00", carries("pi", "does it need a test")), says("assistant", "10:04:05", `[{"type":"text","text":"Yes, one."}]`))
	write(filepath.Join(os.Getenv("HOME"), ".pi", "agent", "sessions", "-"+strings.ReplaceAll(record.Path, "/", "-")+"--", "2026-11-02T10-02-00-000Z_p1.jsonl"),
		`{"type":"session","version":3,"id":"p1","timestamp":"2026-11-02T10:02:00.000Z","cwd":`+string(cwd)+`}`,
		`{"type":"message","timestamp":"2026-11-02T10:02:00.000Z","message":{"role":"user","content":[{"type":"text","text":`+carries("claude", "cap the wait")+`}]}}`,
		`{"type":"message","timestamp":"2026-11-02T10:02:05.000Z","message":{"role":"assistant","content":[{"type":"text","text":"Capped."}]}}`)
	handoffs := filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name)
	name, _ := json.Marshal(record.Name)
	write(filepath.Join(handoffs, "claude-c1.1.json"), `{"from":{"agent":"claude","id":"c1","worktree":`+string(name)+`},"to":{"agent":"pi","window":{"index":2,"name":"pi"},"id":"p1"},"handoff":"/h/claude-c1.md","at":"2026-11-02T10:01:00Z","turns":42,"left_out":6}`)
	write(filepath.Join(handoffs, "pi-p1.2.json"), `{"from":{"agent":"pi","id":"p1","worktree":`+string(name)+`},"to":{"agent":"claude","window":{"index":3,"name":"claude"},"id":"c1"},"handoff":"/h/pi-p1.md","at":"2026-11-02T10:03:00Z","turns":2,"resumes":"c1"}`)
	// (One written before a handing's turns were kept: it says who went on from whom, and no count.)
	write(filepath.Join(handoffs, "claude-c1.3.json"), `{"from":{"agent":"claude","id":"c1","worktree":`+string(name)+`},"to":{"agent":"codex","window":{"index":4,"name":"codex"}},"handoff":"/h/claude-c1.md","at":"2026-11-02T10:05:00Z"}`)

	shows := func(run *chatRun, of, from app.Talk) []string {
		t.Helper()
		chat := &talk{pane: "%1", agent: of.Agent, reply: true}
		chat.lays(run.parts(of, from))
		if from.ID == "" { // (What a chat that waits hears is in another test: an asked turn after the switch ends the wait.)
			turns, err := a.ConversationIn(of.ID, record.Name, 0)
			if err != nil {
				t.Fatal(err)
			}
			chat.heard(turns)
		}
		var all []string
		for _, turn := range read(t, chat.whole()).View.Turns {
			switch turn.Kind {
			case "switch", "model":
				all = append(all, strings.TrimSpace(fmt.Sprintf("%s: %s > %s | %s | %s", turn.Kind, turn.From, turn.To, turn.Note, turn.Tone)))
			default:
				all = append(all, turn.Kind+": "+turn.Text)
			}
		}
		return all
	}
	run := &chatRun{a: a, record: record}
	claudes, pis := app.Talk{Agent: "claude", ID: "c1"}, app.Talk{Agent: "pi", ID: "p1"}
	first := []string{"asked: make it faster", "said: It is faster.", "switch: claude > pi | context handed over · 42 turns · 6 left out |"}
	with := []string{"asked: cap the wait", "said: Capped."}
	back := []string{"asked: does it need a test", "said: Yes, one."}
	// pi's, which went on from claude's: claude's turns until then, the switch, its own
	if got := shows(run, pis, app.Talk{}); !slices.Equal(got, slices.Concat(first, with)) {
		t.Errorf("the chat of the conversation it was handed to:\n%s", strings.Join(got, "\n"))
	}
	// claude's, taken up again: its turns before, those said with pi, and its own since
	again := slices.Concat(first, with, []string{"switch: pi > claude | took its own conversation up again · 2 turns since |"}, back)
	if got := shows(run, claudes, app.Talk{}); !slices.Equal(got, again) {
		t.Errorf("the chat of the conversation taken up again:\n%s", strings.Join(got, "\n"))
	}
	// while its window still keeps what was said with pi since, that switch waits: before any of its own turns is heard
	waits := slices.Concat(first[2:], with, []string{"switch: pi > claude | context goes with your next message | warn"})
	if got := shows(run, claudes, pis); !slices.Equal(got, waits) {
		t.Errorf("while it has still to be sent:\n%s", strings.Join(got, "\n"))
	}
	// handed on to codex, which has written nothing yet, with a model changed after claude's second turn in this chat
	run.models = []chatModelled{{of: claudes, after: 2, band: chatBand{Kind: "model", From: "opus", To: "sonnet"}}}
	all := slices.Concat(first[:2], []string{"model: opus > sonnet |  |"}, first[2:], with, again[len(again)-3:],
		[]string{"switch: claude > codex | context goes with your next message | warn"})
	if got := shows(run, app.Talk{Agent: "codex"}, claudes); !slices.Equal(got, all) {
		t.Errorf("handed on again, a model changed on the way:\n%s", strings.Join(got, "\n"))
	}
	// and in claude's own chat, the model's line stands where it was changed
	if got := shows(run, claudes, app.Talk{}); !slices.Equal(got, slices.Concat(first[:2], []string{"model: opus > sonnet |  |"}, again[2:])) {
		t.Errorf("a model changed, in the chat it was changed in:\n%s", strings.Join(got, "\n"))
	}
}

// fakeWindows is what is kept on windows until it is said: what every chat
// of a window shares, as tmux keeps it.
type fakeWindows map[string]app.Carried

func (f fakeWindows) TakeCarried(record model.Record, window string) (app.Carried, bool) {
	was, on := f[window]
	delete(f, window)
	return was, on
}

func (f fakeWindows) CarryInWindow(record model.Record, window string, carried app.Carried) error {
	f[window] = carried
	return nil
}

// What a window's agent goes on from is kept on the window, not in a chat:
// of two chats of it — the one that handed the conversation over, and one
// started after — whichever sends first says it, and it is said once. A
// send that did not go keeps it there again; another window has none.
func TestWhatAWindowGoesOnFromIsSaidOnceWhicheverChatSendsFirst(t *testing.T) {
	record := model.Record{Name: "monduli"}
	handoff := "## The conversation\n\n### 1 · user\n\nmake it faster\n"
	read := func(file string) ([]byte, error) {
		if file != "/repo/claude-c1.md" {
			return nil, os.ErrNotExist
		}
		return []byte(handoff), nil
	}
	kept := fakeWindows{"5": {From: "claude", ID: "c1", File: "/repo/claude-c1.md"}}
	// the chat that handed it over is ended before it sends, and another is started for the window
	if text, _ := typedFor(kept, record, "2", "to another window", read); text != "to another window" || len(kept) != 1 {
		t.Errorf("another window is typed %q", text)
	}
	later, back := typedFor(kept, record, "5", "and the tests", read)
	if later != "The conversation so far, with claude:\n\n"+strings.TrimSpace(handoff)+"\n\n"+chatRule+"\n\nand the tests" || len(kept) != 0 {
		t.Fatalf("the first thing sent, from the chat started after: %q, kept %v", later, kept)
	}
	if first, _ := typedFor(kept, record, "5", "and the docs", read); first != "and the docs" {
		t.Errorf("the chat that handed it over then sends %q", first)
	}
	back() // it did not go
	if again, _ := typedFor(kept, record, "5", "and the tests", read); again != later || len(kept) != 0 {
		t.Errorf("sent again after it did not go: %q", again)
	}

	// a handoff that is not there any more is named, to be read
	kept["5"] = app.Carried{From: "claude", ID: "c1", File: "/repo/gone.md"}
	if text, _ := typedFor(kept, record, "5", "go on", read); text != "Read /repo/gone.md: the conversation so far, with claude.\n\n"+chatRule+"\n\ngo on" || ownWords(text) != "go on" {
		t.Errorf("with its file gone: %q", text)
	}
	kept["5"] = app.Carried{From: "claude", ID: "c1", File: "/repo/claude-c1.md", Full: "/repo/claude-c1-full.md"}
	handoff = "## The conversation\n" + strings.Repeat("\n### 1 · user\n\n"+strings.Repeat("word ", 400)+"\n", 60)
	if text, _ := typedFor(kept, record, "5", "go on", read); !strings.Contains(text, "the whole of it is in /repo/claude-c1-full.md]") {
		t.Errorf("a long one does not say where every turn is: %.200q", text)
	}
}

// A conversation handed to another agent in a chat is kept on the new
// window, where a chat started after finds it: the turns so far over the
// line that says who goes on, and the handoff said with the first thing
// sent, after which the window keeps nothing. The old agent's window is
// closed, and the chat of the new one is said whole with a toast that
// says so, not a status that stays.
func TestAHandingIsKeptOnTheNewWindowUntilTheFirstSend(t *testing.T) {
	roleLab(t)
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", t.TempDir())
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = pluginHost(config.Config{})
	record, err := a.New("handed")
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(record.Path, "-"))
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, _ := json.Marshal(record.Path)
	lines := `{"type":"user","cwd":` + string(cwd) + `,"timestamp":"2026-11-02T10:00:00Z","message":{"content":"make the list faster"}}` + "\n" +
		`{"type":"assistant","cwd":` + string(cwd) + `,"timestamp":"2026-11-02T10:00:05Z","message":{"content":[{"type":"text","text":"It is faster."}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(project, "c1.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "old", session.What{}, []string{"cat"}); err != nil {
		t.Fatal(err)
	}
	old := ""
	for _, w := range a.Windows(record) {
		if w.Name == "old" {
			old = strconv.Itoa(w.Index)
		}
	}
	var said [][]byte
	chat := func(window string) *chatRun {
		run := &chatRun{a: a, record: record, view: &chatView{worktree: record.Name}, say: func(messages ...[]byte) { said = append(said, messages...) }, done: make(chan struct{}), batches: make(chan chatBatch, 100), ended: make(chan chatEnd, 100)}
		run.aim(app.Head{}, window)
		t.Cleanup(func() { close(run.quit) })
		return run
	}
	first := chat(old)
	first.view.id, first.view.runs.Agent = "c1", "claude"
	first.act(chatDeed{do: chatHand, agent: "echo", model: "loud"})
	if len(said) != 2 || first.view.mode != chatting {
		t.Fatalf("%d messages: %s", len(said), said)
	}
	if whole, toast := read(t, said[0]), read(t, said[1]); whole.View == nil || whole.View.ID != "chat-1" || whole.View.Agent != "echo" ||
		len(toast.Patch) != 1 || toast.Patch[0].Op != "toast" || toast.Patch[0].Tone != "ok" || toast.Patch[0].Text != "claude stopped · echo goes on with this conversation" {
		t.Fatalf("said %s", said)
	}
	if session.Here().HasWindow(record.Name, "old") {
		t.Error("the old agent's window is still there")
	}
	window := first.view.window
	screenShows(t, record.Name, window, "echo-agent --model loud")
	carried, on := a.CarriedInWindow(record, window)
	if !on || carried.From != "claude" || carried.ID != "c1" || filepath.Base(carried.File) != "claude-c1.md" {
		t.Fatalf("on the new window: %+v, %v", carried, on)
	}
	if out, _ := exec.Command("tmux", "show-options", "-w", "-v", "-t", session.Target(record.Name, window), "@mia_context").Output(); !strings.Contains(string(out), `"id":"c1"`) {
		t.Errorf("tmux keeps %q", out)
	}

	// a chat started after, for the new window: the turns so far, and the first send carries the conversation
	second := chat(window)
	got := read(t, second.view.talk.whole())
	if band := got.View.Turns[2]; !slices.Equal(numbers(got.View.Turns), []int{1, 2, 3}) || got.View.Turns[1].Text != "It is faster." ||
		band.Kind != "switch" || band.From != "claude" || band.To != "echo" || band.Tone != "warn" {
		t.Fatalf("a chat started after shows %+v", got.View.Turns)
	}
	said = nil
	second.act(chatDeed{do: chatSend, text: "and the tests"})
	// sent with it: the switch says what was handed over, by the handoff's record
	if len(said) != 2 {
		t.Fatalf("the send said %s", said)
	}
	if went := read(t, said[1]); !slices.Equal(ops(went), []string{"unturn", "turn"}) || went.Patch[1].Turn.N != 4 || went.Patch[1].Turn.Note != "context handed over · 2 turns" {
		t.Errorf("once sent, the switch is said as %s", said[1])
	}
	if records, _ := filepath.Glob(filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "claude-c1.*.json")); len(records) != 1 {
		t.Errorf("the handoff's records: %v", records)
	} else if kept, _ := os.ReadFile(records[0]); !strings.Contains(string(kept), `"turns":2`) || strings.Contains(string(kept), "left_out") || strings.Contains(string(kept), "resumes") {
		t.Errorf("the record keeps %s", kept)
	}
	screenShows(t, record.Name, window, "The conversation so far, with claude:", "It is faster.", chatRule, "and the tests")
	if _, on := a.CarriedInWindow(record, window); on {
		t.Error("said, and the window still keeps it")
	}
	before, _ := session.Here().CaptureBack(record.Name, window, false, 500)
	first.act(chatDeed{do: chatSend, text: "and the docs"})
	screenShows(t, record.Name, window, "and the docs")
	if after, _ := session.Here().CaptureBack(record.Name, window, false, 500); strings.Count(after, "The conversation so far") != strings.Count(before, "The conversation so far") {
		t.Errorf("the conversation is said again by the second send:\n%s", after)
	}
	if _, on := a.TakeCarried(record, window); on {
		t.Error("taken twice")
	}
}
