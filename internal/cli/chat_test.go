package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/plugin"
)

// shownPiece is a piece of a view as the window reads it.
type shownPiece struct {
	T, ID, Text, Style, Label, Key, Agent, Placeholder, Icon, Right, List, Menu, Selected string
	Reply, Working, Bare, Live, Flat, Scroll, Split                                       bool
	Grow, W                                                                               int
	Children                                                                              []shownPiece
	Items                                                                                 []chatItem
	Turns                                                                                 []app.Turn
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
// answers, with the buttons that show the models and the earlier
// conversations at its right, and the keys on the chat's field. A
// conversation no window has open has no field, and no models to switch.
func TestTheChatFillsItsPaneUnderALineThatSaysWhoAnswers(t *testing.T) {
	if got := chatAbout(app.Runs{Agent: "claude", Model: "opus", Effort: "high", Account: "work"}); got != "claude · opus · high · on work" {
		t.Errorf("about = %q", got)
	}
	if got := chatAbout(app.Runs{Agent: "opencode"}); got != "opencode" {
		t.Errorf("of an agent mia chose nothing for: %q", got)
	}
	view := &talk{pane: "%2", agent: "claude", about: "claude · opus · high · on work", reply: true}
	got := only(t, view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}}))
	if !got.Fill || got.Focus != "chat" || got.Whole.T != "col" || len(got.Whole.Children) != 2 || got.Whole.Children[1].T != "chat" {
		t.Fatalf("view = %+v", got.Whole)
	}
	line := got.Whole.Children[0]
	if line.T != "row" || len(line.Children) != 4 || line.Children[0].Text != "claude · opus · high · on work" || line.Children[0].Style != "dim" || line.Children[1].Grow != 1 {
		t.Errorf("line = %+v", line)
	}
	if m, e := line.Children[2], line.Children[3]; m.T != "button" || m.ID != "models" || m.Key != "m" || e.T != "button" || e.ID != "earlier" || e.Key != "e" {
		t.Errorf("buttons = %+v", line.Children[2:])
	}

	shown := &talk{agent: "codex", about: "codex"}
	got = only(t, shown.heard(nil))
	if line := got.Whole.Children[0]; got.Focus != "" || got.View.Reply || len(line.Children) != 3 || line.Children[2].ID != "earlier" {
		t.Errorf("one no window has open: focus %q, line %+v", got.Focus, line)
	}
}

// What is sent is said at once as an asked turn after the last, and the
// transcript's own turn of the same words takes its place: nothing is
// said twice, and the chat is not begun anew.
func TestWhatWasSentShowsAtOnceAndTheTranscriptsTurnTakesItsPlace(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude", reply: true}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}, {N: 2, Kind: "said", Text: "Hello."}})
	got := only(t, view.sends("make it faster"))
	if len(got.Patch) != 1 || got.Patch[0].Op != "turn" || got.Patch[0].ID != "chat" || got.Patch[0].Turn.N != 3 || got.Patch[0].Turn.Kind != "asked" || got.Patch[0].Turn.Text != "make it faster" {
		t.Fatalf("sent is said as %+v", got.Patch)
	}
	got = only(t, view.heard([]app.Turn{{N: 3, Kind: "asked", Text: "make it faster\n"}, {N: 4, Kind: "said", Text: "I will"}}))
	if len(got.Patch) != 2 || got.Patch[0].ID != "chat" || view.anew != 0 || len(view.sent) != 0 || len(view.own) != 0 {
		t.Errorf("the transcript's turn: %+v, begun anew %d times, %d still its own", got.Patch, view.anew, len(view.own))
	}
	if ns := numbers(read(t, view.whole()).View.Turns); !slices.Equal(ns, []int{1, 2, 3, 4}) {
		t.Errorf("turns %v", ns)
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
	if len(got.Patch) != 1 || got.Patch[0].Turn.N != 2 || got.Patch[0].Turn.Text != "and why?" {
		t.Errorf("the second thing sent: %+v", got.Patch)
	}
	got = only(t, view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "what is in here?"}, {N: 2, Kind: "asked", Text: "and why?"}, {N: 3, Kind: "said", Text: "Files."}}))
	if got.Patch == nil || view.anew != 0 || len(view.sent) != 0 {
		t.Errorf("its conversation begun: %+v, anew %d, sent %q", got, view.anew, view.sent)
	}
}

// An agent at work takes up what was sent later: when another turn comes
// first, the turn said for it is taken back — in a chat of another id,
// as the window keeps a turn it was told — and the status says it waits.
// It is said once the transcript has it.
func TestWhatAnAgentAtWorkTakesUpLaterIsNotSaidTwice(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude", reply: true}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}})
	view.sends("and the tests")
	got := only(t, view.heard([]app.Turn{{N: 2, Kind: "tool", Text: "Read a file"}}))
	if got.View == nil || got.View.ID != "chat-1" || got.Focus != "chat-1" || !slices.Equal(numbers(got.View.Turns), []int{1, 2}) || got.View.Turns[1].Kind != "tool" {
		t.Fatalf("view = %+v", got.View)
	}
	if got.Status == nil || got.Status.Text != "claude takes it up when it can — sent: and the tests" || got.Status.Tone != "" {
		t.Errorf("status = %+v", got.Status)
	}
	got = only(t, view.heard([]app.Turn{{N: 3, Kind: "asked", Text: "and the tests"}}))
	if len(got.Patch) != 1 || got.Patch[0].ID != "chat-1" || got.Patch[0].Turn.N != 3 {
		t.Errorf("the transcript's turn: %+v", got)
	}
}

// A send the window refuses takes its turn back, and the status says why
// with the words, which the chat's field cannot be handed: both cut to
// fit what the window keeps of a status.
func TestASendRefusedTakesItsTurnBackAndSaysTheWords(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude", reply: true}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}})
	view.sends("go on\nwith it")
	got := only(t, view.refused(errors.New("claude asks first: Trust this folder?")))
	if got.View == nil || got.View.ID != "chat-1" || !slices.Equal(numbers(got.View.Turns), []int{1}) || len(view.sent) != 0 {
		t.Fatalf("view = %+v, sent %q", got.View, view.sent)
	}
	if got.Status == nil || got.Status.Tone != "bad" || got.Status.Text != "claude asks first: Trust this folder? — not sent: go on with it" {
		t.Errorf("status = %+v", got.Status)
	}
	long := unsent(errors.New(strings.Repeat("why ", 100)), strings.Repeat("words ", 100))
	if len([]rune(long)) != chatSaid || !strings.Contains(long, " — not sent: words words") || !strings.HasPrefix(long, "why why") {
		t.Errorf("%d characters: %q", len([]rune(long)), long)
	}
	if messages := view.refused(errors.New("nothing was sent")); messages != nil {
		t.Errorf("with nothing sent: %s", messages)
	}
}

// What the window's screen asks that the transcript does not — a
// permission — is said as a question of the app's own after the last
// turn, and taken out once answered. A transcript that ends on a question
// nobody answered is the asking itself. An answer the window refused is
// said as the status, the questions open again.
func TestWhatTheScreenAsksIsAQuestionOfTheChatsOwn(t *testing.T) {
	view := &talk{pane: "%3", agent: "claude", reply: true}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}, {N: 2, Kind: "tool", Text: "Run make"}})
	asks := &app.Asks{Text: "Run make?", Options: []string{"Yes", "No"}, Selected: 1}
	got := only(t, view.asked(asks))
	if len(got.Patch) != 1 || got.Patch[0].Turn.N != 3 || got.Patch[0].Turn.Kind != "question" || got.Patch[0].Turn.Text != "Run make?" || !slices.Equal(got.Patch[0].Turn.Options, []string{"Yes", "No"}) {
		t.Fatalf("asked is said as %+v", got.Patch)
	}
	if messages := view.asked(&app.Asks{Text: "Run make?", Options: []string{"Yes", "No"}, Selected: 2}); messages != nil {
		t.Errorf("the cursor moved, and it is said again: %s", messages)
	}
	got = only(t, view.asked(nil))
	if got.View == nil || got.View.ID != "chat-1" || !slices.Equal(numbers(got.View.Turns), []int{1, 2}) {
		t.Errorf("answered: %+v", got.View)
	}
	if got = only(t, view.asked(&app.Asks{Options: []string{"Yes", "No"}})); got.Patch[0].Turn.Text != "claude asks" {
		t.Errorf("one with no words: %+v", got.Patch[0].Turn)
	}
	view.asked(nil)

	view.heard([]app.Turn{{N: 3, Kind: "question", Text: "Which one?", Options: []string{"A", "B"}}})
	if messages := view.asked(asks); len(messages) != 0 {
		t.Errorf("the transcript's own question, said twice: %s", messages)
	}
	got = only(t, view.reopened(errors.New("the cursor did not move to option 2, and nothing was answered")))
	if got.View.ID != "chat-3" || got.Status == nil || got.Status.Tone != "bad" || !strings.HasPrefix(got.Status.Text, "the cursor did not move") || got.View.Turns[2].Answer != "" {
		t.Errorf("refused: %+v, %+v", got.View, got.Status)
	}
	// answered, and the screen asks on: its question comes after what came since
	got = only(t, view.heard([]app.Turn{{N: 4, Kind: "answer", Text: "A", Question: 3}, {N: 5, Kind: "tool", Text: "Run make"}}))
	if len(got.Patch) != 3 || got.Patch[2].Turn.N != 6 || got.Patch[2].Turn.Kind != "question" || got.Patch[2].Turn.Text != "Run make?" {
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

// The models are the setups used here first, then each installed agent's
// under its head — the chat's own agent's installed or not, an agent with
// none once — the one that runs checked and under the keys, an item's
// menu its efforts with the one it runs at checked; what is typed narrows
// them.
func TestTheModelsAreAPaletteInPlaceOfTheChat(t *testing.T) {
	view := &chatView{talk: &talk{pane: "%5", agent: "claude"}, mode: picking, worktree: "monduli", window: "2",
		runs:   app.Runs{Agent: "claude", Model: "opus", Effort: "high", Account: "work", Live: true},
		agents: chatAgents(),
		setups: []app.Setup{{Agent: "codex", Model: "gpt-6-luna", Effort: "high"}, {Agent: "claude", Role: "reviewer"}, {Agent: "gone", Model: "x"}}}
	got := read(t, view.whole(true))
	if !got.Fill || got.Focus != "q" || len(got.Keys) != 1 || got.Keys[0] != (chatKey{Key: "esc", Label: "back to the chat", ID: "back"}) || got.View != nil {
		t.Fatalf("message = %+v", got)
	}
	field, list := got.Whole.find("field"), got.Whole.find("list")
	if field == nil || field.ID != "q" || !field.Bare || !field.Live || field.List != "models" || field.Right != "5 models" || field.Icon != "search" {
		t.Errorf("field = %+v", field)
	}
	if list == nil || list.ID != "models" || list.Menu != "effort" || list.Selected != "a|claude|opus" {
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

	view.query = " LUNA"
	list = read(t, view.whole(false)).Whole.find("list")
	if len(list.Items) != 4 || list.Items[1].ID != "u|codex|high|gpt-6-luna" || list.Items[3].ID != "a|codex|gpt-6-luna" || list.Selected != "" {
		t.Errorf("narrowed to %+v", list.Items)
	}
	if rows := chatModels(chatAgents(), nil, "pi", ""); !slices.ContainsFunc(rows, func(m chatModel) bool { return m.agent == "pi" && m.model == "anthropic/claude-opus-5" }) {
		t.Errorf("the chat's own agent, not installed, is not listed: %+v", rows)
	}
	if row, ok := chatModelOf(chatModel{agent: "pi", model: "a|b/c"}.id()); !ok || row.agent != "pi" || row.model != "a|b/c" {
		t.Errorf("a model's id read back: %+v", row)
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
		{chatting, chatEvent{E: "press", ID: "models"}, chatDeed{do: chatModelsUp}},
		{chatting, chatEvent{E: "press", ID: "earlier"}, chatDeed{do: chatEarlier}},
		{chatting, chatEvent{E: "press", ID: "back"}, chatDeed{}},
		{picking, chatEvent{E: "send", Text: "typed into the chat before"}, chatDeed{}},
		{picking, chatEvent{E: "press", ID: "back"}, chatDeed{do: chatBack}},
		{picking, chatEvent{E: "change", ID: "q", Text: "lu"}, chatDeed{do: chatNarrow, text: "lu"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "a|claude|sonnet"}, chatDeed{do: chatSwitch, model: "sonnet"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "a|claude|sonnet", Sub: "low"}, chatDeed{do: chatSwitch, model: "sonnet", effort: "low"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "u|claude|high|opus"}, chatDeed{do: chatSwitch, model: "opus", effort: "high"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "a|codex|gpt-6-luna", Sub: "high"}, chatDeed{do: chatHand, agent: "codex", model: "gpt-6-luna", effort: "high"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "u|codex|high|gpt-6-luna"}, chatDeed{do: chatHand, agent: "codex", model: "gpt-6-luna", effort: "high"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "a|opencode|"}, chatDeed{do: chatHand, agent: "opencode"}},
		{picking, chatEvent{E: "pick", List: "models", ID: "nonsense"}, chatDeed{}},
		{looking, chatEvent{E: "press", ID: "back"}, chatDeed{do: chatBack}},
		{looking, chatEvent{E: "move", List: "earlier", ID: "c1"}, chatDeed{do: chatRead, id: "c1"}},
		{looking, chatEvent{E: "move", List: "earlier", ID: "c2"}, chatDeed{}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c2"}, chatDeed{do: chatShow, id: "c2"}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c0"}, chatDeed{do: chatGoOn, id: "c0"}},
		{looking, chatEvent{E: "pick", List: "earlier", ID: "c9"}, chatDeed{}},
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
	for _, event := range []chatEvent{{E: "send", Text: "go on"}, {E: "choose"}, {E: "stop"}, {E: "press", ID: "models"}} {
		if got := view.deed(event); got != (chatDeed{}) {
			t.Errorf("with no window, %+v asks %+v", event, got)
		}
	}
	if got := view.deed(chatEvent{E: "press", ID: "earlier"}); got.do != chatEarlier {
		t.Errorf("with no window, earlier asks %+v", got)
	}
}

// Earlier is the worktree's conversations as a list beside the one under
// the keys read back: what was first asked, the agent and when at the
// right, the one a window has open running, c on one of another agent,
// and the chat beside it with no field.
func TestEarlierIsAListBesideTheConversationUnderTheKeys(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	view := &chatView{talk: &talk{pane: "%5", agent: "claude"}, worktree: "monduli", window: "2", id: "c2", runs: app.Runs{Agent: "claude"}}
	view.listed([]app.Head{
		{ID: "c2", Agent: "claude", Asked: "make the list faster", Active: now, Running: true, Window: &app.WindowRef{Index: 2, Name: "claude"}},
		{ID: "c1", Agent: "codex", Asked: "why is the tab a pill", Active: now.Add(-4 * 24 * time.Hour)},
		{ID: "c0", Agent: "claude", Active: now.Add(-3 * time.Hour)},
	}, now)
	view.mode = looking
	view.reads("c1", []app.Turn{{N: 1, Kind: "asked", Text: "why is the tab a pill"}, {N: 2, Kind: "did", Text: "mia ls"}, {N: 3, Kind: "said", Text: "The hover style won."}})
	got := read(t, view.whole(true))
	if !got.Fill || got.Focus != "earlier" || len(got.Keys) != 1 || got.Keys[0].Key != "esc" || got.Keys[0].ID != "back" {
		t.Fatalf("message = %+v", got)
	}
	if line := got.Whole.Children[0].Children[0]; line.Text != "Earlier in monduli · 3 conversations" {
		t.Errorf("line = %+v", line)
	}
	split := got.Whole.Children[1]
	if split.T != "row" || !split.Split || split.Grow != 1 || len(split.Children) != 3 || split.Children[0].W != 360 || !split.Children[0].Scroll || split.Children[1].T != "divider" {
		t.Fatalf("split = %+v", split)
	}
	list := split.Children[0].find("list")
	if list == nil || list.ID != "earlier" || !list.Live || !list.Flat || list.Selected != "c1" || len(list.Items) != 3 {
		t.Fatalf("list = %+v", list)
	}
	first, second, third := list.Items[0], list.Items[1], list.Items[2]
	if first.Title != "make the list faster" || first.Right != "claude · now" || first.Running == nil || !*first.Running || first.Icon != "claude" || len(first.Keys) != 0 || first.Does != "back to it" {
		t.Errorf("the running one = %+v", first)
	}
	if second.Right != "codex · 4d" || *second.Running || len(second.Keys) != 1 || second.Keys[0] != (chatKey{Key: "c", Label: "continue in claude", ID: "continue"}) || second.Does != "go on with it" {
		t.Errorf("another agent's = %+v", second)
	}
	if third.Title != "nothing asked yet" || third.Right != "claude · 3h" || len(third.Keys) != 0 {
		t.Errorf("one with nothing asked = %+v", third)
	}
	if beside := split.Children[2]; beside.T != "chat" || beside.ID != "read-c1" || beside.Agent != "codex" || beside.Reply || !slices.Equal(numbers(beside.Turns), []int{1, 3}) {
		t.Errorf("read back = %+v", beside)
	}
	if list = read(t, view.whole(false)).Whole.find("list"); list.Selected != "" {
		t.Errorf("said again, the keys are put on %q", list.Selected)
	}

	var long []app.Turn
	for n := 1; n <= 3*chatReadMost; n++ {
		long = append(long, app.Turn{N: n, Kind: "tool", Text: "Read a file"})
	}
	view.reads("c0", long)
	if beside := read(t, view.whole(false)).Whole.find("chat"); len(beside.Turns) != chatReadMost || beside.Turns[chatReadMost-1].N != 3*chatReadMost {
		t.Errorf("a long one is read back by %d turns", len(beside.Turns))
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
// chat's own window: nothing is closed, and the status says why.
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
	view := &chatView{talk: &talk{agent: "codex", reply: true}, window: "5", carry: &chatCarried{from: "claude", file: "/repo/.git/mia/handoffs/monduli/claude-c1.md", text: handoff}}
	first := view.toType("and the tests")
	want := "The conversation so far, with claude:\n\n" + strings.TrimSpace(handoff) + "\n\n" + chatRule + "\n\nand the tests"
	if first != want {
		t.Errorf("the first thing sent is %q", first)
	}
	if got := ownWords(first); got != "and the tests" {
		t.Errorf("the person's own words of it: %q", got)
	}
	view.carry = nil // as when it went
	if got := view.toType("and the docs"); got != "and the docs" {
		t.Errorf("the second thing sent is %q", got)
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

	// the chat goes on from the one before: its turns stay over a quiet line, and what is heard comes after
	before := &talk{agent: "claude"}
	before.heard([]app.Turn{{N: 1, Kind: "asked", Text: "make it faster"}, {N: 2, Kind: "said", Text: "It is faster."}})
	chat := &talk{pane: "%3", agent: "codex", reply: true, anew: 1}
	chat.goesOn(before)
	got := read(t, chat.whole())
	if !slices.Equal(numbers(got.View.Turns), []int{1, 2, 3}) || got.View.Turns[2].Kind != "tool" || got.View.Turns[2].Text != "continued by codex" || got.View.ID != "chat-1" {
		t.Fatalf("gone on: %+v", got.View)
	}
	sent := only(t, chat.sends("and the tests"))
	if sent.Patch[0].Turn.N != 4 || sent.Patch[0].Turn.Text != "and the tests" {
		t.Errorf("sent is said as %+v", sent.Patch[0].Turn)
	}
	heard := only(t, chat.heard([]app.Turn{{N: 1, Kind: "asked", Text: first}, {N: 2, Kind: "question", Text: "Which?", Options: []string{"A", "B"}}, {N: 3, Kind: "answer", Text: "A", Question: 2}}))
	if len(heard.Patch) != 3 || heard.Patch[0].Turn.N != 4 || heard.Patch[0].Turn.Text != "and the tests" || heard.Patch[2].Turn.N != 6 || heard.Patch[2].Turn.Question != 5 || chat.anew != 1 || len(chat.sent) != 0 {
		t.Errorf("its transcript's turns: %+v, anew %d, sent %q", heard.Patch, chat.anew, chat.sent)
	}
}
