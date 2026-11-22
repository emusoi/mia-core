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
	T, ID, Title, Text, Style, Tone, Label, Key, Agent, Placeholder, Icon, Right, List, Selected, Model string
	Reply, Working, Bare, Live, Flat, Split                                                             bool
	Grow, W, H, Max                                                                                     int
	// Menu is a list's word for the way into its items' menus, or a chat's
	// menu of each reply.
	Menu     any
	Children []shownPiece
	Items    []chatItem
	Turns    []shownTurn
	Buttons  []chatButton
}

// shownTurn is a turn of a chat as the window reads it: a transcript's, or
// a row of the chat's own.
type shownTurn struct {
	app.Turn // a switch's from, to, note and tone among it
	Key, ID  string
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
		Keys               []chatKey
		N                  int
		Turn               *shownTurn
		On                 *bool
		Node               *shownPiece
		Window             string
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
		{N: 7, Kind: "thought", Text: "the sort runs twice", Seconds: 3},
		{N: 8, Kind: "thought", Text: ""},
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
	if ns := numbers(got.View.Turns); !slices.Equal(ns, []int{1, 4, 5, 6, 7}) {
		t.Errorf("turns %v", ns)
	}
	if thought := got.View.Turns[4]; thought.Kind != "thought" || thought.Text != "the sort runs twice" || thought.Seconds != 3 {
		t.Errorf("what the agent thought, as the window draws it: %+v", thought)
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

// A chat reads back from a conversation's end only until what it read is
// enough: the view said whole of the last turns that are enough is the
// view of all of them.
func TestTheLastTurnsThatAreEnoughSayTheViewOfAll(t *testing.T) {
	carried := app.CarriedOpens("codex") + "\n\n" + strings.Repeat("what was said before ", 4000) + "\n\n" + app.CarriedRule + "\n\ngo on"
	for name, all := range map[string]func(n int) app.Turn{
		"short":   func(n int) app.Turn { return app.Turn{N: n, Kind: "tool", Text: "Read a file"} },
		"long":    func(n int) app.Turn { return app.Turn{N: n, Kind: "said", Text: strings.Repeat("word ", 8000)} },
		"carried": func(n int) app.Turn { return app.Turn{N: n, Kind: "asked", Text: carried} },
		"wide":    func(n int) app.Turn { return app.Turn{N: n, Kind: "said", Text: strings.Repeat("é", 2*muiProse)} },
		"unread": func(n int) app.Turn {
			return app.Turn{N: n, Kind: []string{"said", "asked", "nothing"}[n%3], Text: "a few words " + strconv.Itoa(n)}
		},
	} {
		var turns []app.Turn
		for n := 1; n <= 3000; n++ {
			turns = append(turns, all(n))
		}
		from := len(turns)
		for from > 0 && !enough(turns[from:]) {
			from--
		}
		if from == 0 {
			t.Errorf("%s: all of them were needed", name)
			continue
		}
		whole, last := &talk{agent: "claude"}, &talk{agent: "claude"}
		if want, got := read(t, whole.heard(turns)[0]), read(t, last.heard(turns[from:])[0]); !slices.Equal(numbers(got.View.Turns), numbers(want.View.Turns)) {
			t.Errorf("%s: the last %d turns say %v, all of them %v", name, len(turns)-from, numbers(got.View.Turns), numbers(want.View.Turns))
		}
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

// What is sent says at once that the agent works, before its window shows
// it — some never do — and it keeps saying so until a reply came after and
// the conversation was still a moment, the window showed it at work and
// then at rest, or a while passed.
func TestWhatWasSentSaysTheAgentWorksUntilItReplies(t *testing.T) {
	now := time.Now().Add(time.Minute) // after any turn heard here
	view := &talk{pane: "%3", agent: "opencode", reply: true}
	view.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}, {N: 2, Kind: "said", Text: "Hello."}})
	working := func(messages [][]byte) string {
		t.Helper()
		if len(messages) == 0 {
			return "unchanged"
		}
		got := only(t, messages)
		if len(got.Patch) != 1 || got.Patch[0].Op != "working" || got.Patch[0].On == nil {
			t.Fatalf("not a working patch: %+v", got.Patch)
		}
		return strconv.FormatBool(*got.Patch[0].On)
	}
	if got := working(view.patch(view.awaiting(now))); got != "true" {
		t.Fatalf("as it is sent: %s", got)
	}
	if got := working(view.seen(false, now.Add(time.Second))); got != "unchanged" || !view.working {
		t.Errorf("the window not showing it yet: %s", got)
	}
	view.heard([]app.Turn{{N: 3, Kind: "asked", Text: "make it faster"}})
	if got := working(view.seen(false, now.Add(2*time.Second))); got != "unchanged" {
		t.Errorf("its own words heard back, no reply yet: %s", got)
	}
	view.heard([]app.Turn{{N: 4, Kind: "said", Text: "It is"}})
	if got := working(view.seen(false, time.Now())); got != "unchanged" {
		t.Errorf("a reply is being written: %s", got)
	}
	if got := working(view.seen(false, time.Now().Add(chatStill))); got != "false" {
		t.Errorf("the reply came, and nothing since: %s", got)
	}

	view.awaiting(now)
	if got := working(view.seen(true, now.Add(time.Second))); got != "unchanged" {
		t.Errorf("the window shows it at work: %s", got)
	}
	if got := working(view.seen(false, now.Add(2*time.Second))); got != "false" {
		t.Errorf("then at rest, with no reply: %s", got)
	}

	view.awaiting(now)
	if got := working(view.seen(false, now.Add(chatAwaits-time.Second))); got != "unchanged" {
		t.Errorf("not yet a while: %s", got)
	}
	if got := working(view.seen(false, now.Add(chatAwaits+time.Second))); got != "false" {
		t.Errorf("after a while with nothing: %s", got)
	}
	if got := working(view.seen(true, now.Add(chatAwaits+2*time.Second))); got != "true" {
		t.Errorf("with nothing awaited, as the window shows it: %s", got)
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
	if !got.Fill || got.Focus != "chat" || got.Whole.T != "col" || got.Whole.ID != "view" || len(got.Whole.Children) != 1 || got.Whole.Children[0].T != "chat" || len(got.Keys) != 0 {
		t.Fatalf("view = %+v", got.Whole)
	}
	chat := got.Whole.Children[0]
	if chat.Model != "opus · high · on work" {
		t.Errorf("model = %q", chat.Model)
	}
	if b := chat.Buttons; len(b) != 3 || b[0].ID != "open-models" || b[0].Key != "m" || b[1].ID != "open-fork" || b[1].Label != "Fork" || b[1].Key != "f" || b[2].ID != "open-earlier" || b[2].Label != "Earlier" || b[2].Key != "e" {
		t.Errorf("buttons = %+v", b)
	}

	shown := &talk{agent: "codex", about: "codex"}
	got = only(t, shown.heard(nil))
	if b := got.View.Buttons; got.Focus != "" || got.View.Reply || got.View.Menu != nil || len(b) != 1 || b[0].ID != "open-earlier" || got.View.Model != "codex" {
		t.Errorf("one no window has open: focus %q, buttons %+v, model %q", got.Focus, b, got.View.Model)
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
	if modal.T != "modal" || modal.ID != "over" || modal.Title != "Model" || modal.W != 520 || len(modal.Children) != 7 || modal.Children[1].T != "divider" || modal.Children[3].T != "divider" || modal.Children[5].T != "divider" || modal.find("chat") != nil {
		t.Fatalf("modal = %+v", modal)
	}
	field, list := modal.Children[0], modal.Children[4]
	if does := modal.Children[2]; does.ID != "does" || !list.Live || view.under != "a|claude|opus" || len(does.Children) != 1 || does.Children[0].Children[0].Text != "opus" || does.Children[0].Children[0].Style != "strong" {
		t.Errorf("the line that says what the row under the keys does = %+v", does)
	}
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
	if foot := pieceOf(t, view.models(false)).Children; len(foot) != 8 || foot[6].ID != "mark" || foot[7].T != "text" || foot[7].Tone != "bad" || !strings.HasPrefix(foot[7].Text, "claude cannot be switched") {
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

// The line under the models' search line says what picking the row under
// the keys does, and follows the keys. A model of this chat's agent: what
// it goes from and to, in place. Another agent's: who to whom and what is
// carried, the latest turns or the whole conversation, with the counts a
// dry run gave; tab, and a press on either, flips which, the foot says
// tab only then, and a pick hands over what was chosen. An agent the
// conversation came from: it takes its own up again, and its group says
// it was in this chat.
func TestTheLineUnderTheModelsSearchSaysWhatAPickDoes(t *testing.T) {
	view := &chatView{talk: &talk{pane: "%5", agent: "claude", reply: true}, worktree: "monduli", window: "2", id: "c1",
		runs: app.Runs{Agent: "claude", Model: "opus", Effort: "high", Live: true}, agents: chatAgents(), came: []string{"opencode"},
		would: map[string]chatWould{
			"codex":    {done: app.Continued{Turns: 42, LeftOut: 6}},
			"opencode": {done: app.Continued{Turns: 8, Resumes: "o1"}},
		}}
	view.talk.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}})
	said := func(piece *shownPiece) string {
		var all []string
		var walk func(p shownPiece)
		walk = func(p shownPiece) {
			switch p.T {
			case "text":
				all = append(all, p.Text)
			case "tabs":
				all = append(all, "["+p.Selected+"]")
			case "button":
				all = append(all, "<"+p.Key+">")
			}
			for _, child := range p.Children {
				walk(child)
			}
		}
		walk(*piece)
		return strings.Join(all, " ")
	}
	does := func(got heardByWindow) string {
		t.Helper()
		modal := got.Patch[0].Node
		if modal == nil || len(modal.Children) < 3 || modal.Children[2].ID != "does" {
			t.Fatalf("no line in %+v", got.Patch)
		}
		return said(&modal.Children[2])
	}
	keys := func(got heardByWindow) string {
		if len(got.Patch) < 2 || got.Patch[len(got.Patch)-1].Op != "keys" {
			return "unsaid"
		}
		var all []string
		for _, key := range got.Patch[len(got.Patch)-1].Keys {
			all = append(all, key.Key+" "+key.Label+" "+key.ID)
		}
		return strings.Join(all, ", ")
	}
	move := func(to string) heardByWindow {
		t.Helper()
		deed := view.deed(chatEvent{E: "move", List: "models", ID: to})
		if deed != (chatDeed{do: chatUnder, id: to}) {
			t.Fatalf("the keys moved to %s ask %+v", to, deed)
		}
		view.under = deed.id
		return only(t, view.again())
	}

	// put up on the model that runs: nothing to do
	up := only(t, view.up(picking))
	if got := does(up); got != "opus · What this chat runs on." || keys(up) != "unsaid" {
		t.Errorf("on the model that runs: %q, keys %s", got, keys(up))
	}
	if deed := view.deed(chatEvent{E: "move", List: "models", ID: "a|claude|opus"}); deed.do != chatNothing {
		t.Errorf("the keys said where they are ask %+v", deed)
	}
	// this agent's other model
	if got := move("a|claude|sonnet"); does(got) != "opus → sonnet · Switches in place. Nothing is re-sent." || keys(got) != "unsaid" {
		t.Errorf("another model of this agent: %q, keys %s", does(got), keys(got))
	}
	// another agent's: the choice, the latest turns first, and tab in the foot
	got := move("a|codex|gpt-6-luna")
	if does(got) != "claude → codex carries [latest] <tab> The last 36 of 42 turns, as many as fit. Sent with your next message." || keys(got) != "tab latest or whole flip" {
		t.Errorf("another agent's: %q, keys %s", does(got), keys(got))
	}
	if deed := view.deed(chatEvent{E: "pick", List: "models", ID: "a|codex|gpt-6-luna"}); deed != (chatDeed{do: chatHand, agent: "codex", model: "gpt-6-luna"}) {
		t.Errorf("picked with the latest: %+v", deed)
	}
	// tab flips it, and what is picked then carries the whole conversation
	flip := view.deed(chatEvent{E: "press", ID: "flip"})
	if flip != (chatDeed{do: chatCarries, whole: true}) {
		t.Fatalf("tab asks %+v", flip)
	}
	view.whole = flip.whole
	if got := only(t, view.again()); does(got) != "claude → codex carries [whole] <tab> All 42 turns. Slower, and uses more of your plan." || keys(got) != "unsaid" {
		t.Errorf("flipped: %q, keys %s", does(got), keys(got))
	}
	deed := view.deed(chatEvent{E: "pick", List: "models", ID: "a|codex|gpt-6-luna", Sub: "high"})
	if deed != (chatDeed{do: chatHand, agent: "codex", model: "gpt-6-luna", effort: "high", whole: true}) {
		t.Fatalf("picked with the whole: %+v", deed)
	}
	mia := &fakeMia{done: app.Continued{Sent: true}}
	if handTo(mia, model.Record{Name: "monduli"}, view.id, "", "claude", deed.agent, deed.model, deed.effort, deed.whole, func(app.Continued) {}); !slices.Equal(mia.asked, []string{"continue c1 in monduli with codex gpt-6-luna high quiet=true whole"}) {
		t.Errorf("handed over as %q", mia.asked)
	}
	// a press on a choice takes that one, and tab again the other
	if back := view.deed(chatEvent{E: "pick", List: "carry", ID: "latest"}); back != (chatDeed{do: chatCarries}) {
		t.Errorf("a press on Latest asks %+v", back)
	}
	if again := view.deed(chatEvent{E: "press", ID: "flip"}); again != (chatDeed{do: chatCarries}) {
		t.Errorf("tab again asks %+v", again)
	}
	// an agent the conversation came from: no choice, and the foot's tab goes
	back := move("a|opencode|")
	if does(back) != "claude → opencode · opencode takes its own conversation up again · 8 turns since" || keys(back) != "" {
		t.Errorf("an agent it came from: %q, keys %q", does(back), keys(back))
	}
	list := back.Patch[0].Node.find("list")
	if at := slices.IndexFunc(list.Items, func(item chatItem) bool { return item.Head == "Opencode" }); at < 0 || list.Items[at].Tag != "was in this chat" {
		t.Errorf("its group's head: %+v", list.Items)
	}
	// what would not be done says why; taken away, the foot's keys stay none
	view.would["codex"] = chatWould{err: errors.New("nothing is said yet in claude's conversation c1")}
	if got := move("a|codex|gpt-6-luna"); does(got) != "claude → codex · nothing is said yet in claude's conversation c1" || keys(got) != "unsaid" {
		t.Errorf("refused: %q, keys %s", does(got), keys(got))
	}
	view.would["codex"] = chatWould{done: app.Continued{Turns: 1}}
	if got := only(t, view.again()); does(got) != "claude → codex carries [whole] <tab> All 1 turn. Slower, and uses more of your plan." || keys(got) != "tab latest or whole flip" {
		t.Errorf("one turn: %q, keys %s", does(got), keys(got))
	}
	// the view said whole puts the modal back with the foot's key, and esc takes both away
	if all := view.talk.said(); len(all) != 2 || !slices.Equal(ops(read(t, all[1])), []string{"add", "keys"}) {
		t.Errorf("said whole: %s", all)
	}
	if shut := only(t, view.shut()); !slices.Equal(ops(shut), []string{"remove", "keys"}) || len(shut.Patch[1].Keys) != 0 || view.talk.with != nil {
		t.Errorf("taken away: %+v", shut.Patch)
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
	got := only(t, view.shut(encode(chatOp{Op: "model", ID: view.talk.id(), Text: view.talk.model()}), view.talk.went("claude switched to sonnet low", "ok")))
	if !slices.Equal(ops(got), []string{"remove", "model", "toast"}) || got.Patch[1].ID != view.talk.id() || got.Patch[1].Text != "sonnet · low" ||
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
	asked := fmt.Sprintf("continue %s in %s with %s %s %s quiet=%v", want.ID, want.Worktree, want.With, want.Model, want.Effort, want.Quiet)
	if want.Whole {
		asked += " whole"
	}
	f.asked = append(f.asked, asked)
	return f.done, f.refuses
}

func (f *fakeMia) ReplaceWindow(record model.Record, window, with string) error {
	f.asked = append(f.asked, "close "+record.Name+" "+window+" for "+with)
	return f.stays
}

// A model of another agent picked hands the conversation to that agent,
// for every pair of the agents mia knows: it is started quiet on the
// model in a window of its own, and only once it runs is the window of
// the agent it was with closed, last, the other's put in front in its
// place: what is to be done while both are there is done before. Refused,
// ended as it started, or this chat's own window: nothing is closed, and
// what is said says why.
func TestAnotherAgentsModelHandsTheConversationOverAndClosesTheOldWindowLast(t *testing.T) {
	record := model.Record{Name: "monduli"}
	ready := func(mia *fakeMia) func(app.Continued) {
		return func(app.Continued) { mia.asked = append(mia.asked, "ready") }
	}
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
			went := handTo(mia, record, view.id, view.window, from.Name, deed.agent, deed.model, deed.effort, deed.whole, ready(mia))
			want := []string{fmt.Sprintf("continue c1 in monduli with %s %s %s quiet=true", to.Name, pick.model, sub), "ready", "close monduli 2 for " + to.Name}
			if !slices.Equal(mia.asked, want) || !went.went || went.tone != "ok" || went.text != from.Name+" stopped · "+to.Name+" goes on with this conversation" {
				t.Errorf("%s to %s: asked %q, went %+v", from.Name, to.Name, mia.asked, went)
			}
		}
	}

	mia := &fakeMia{refuses: errors.New("nothing is said yet in claude's conversation c1")}
	if went := handTo(mia, record, "c1", "2", "claude", "codex", "", "", false, ready(mia)); went.went || went.tone != "bad" || went.text != "nothing is said yet in claude's conversation c1" || len(mia.asked) != 1 {
		t.Errorf("refused: %+v, asked %q", went, mia.asked)
	}
	mia = &fakeMia{done: app.Continued{Problem: "codex ended as soon as it started, saying:\n  not signed in"}}
	if went := handTo(mia, record, "c1", "2", "claude", "codex", "", "", false, ready(mia)); went.went || went.tone != "bad" || !strings.HasPrefix(went.text, "claude still runs — codex ended as soon as it started") || len(mia.asked) != 1 {
		t.Errorf("the other agent ended: %+v, asked %q", went, mia.asked)
	}
	mia = &fakeMia{done: app.Continued{Sent: true}}
	if went := handTo(mia, record, "c1", "", "claude", "codex", "", "", false, ready(mia)); !went.went || went.text != "codex goes on with this conversation · claude's window stays" || len(mia.asked) != 2 {
		t.Errorf("with no window to close: %+v, asked %q", went, mia.asked)
	}
	mia = &fakeMia{done: app.Continued{Sent: true}, stays: errors.New("no window 2")}
	if went := handTo(mia, record, "c1", "2", "claude", "codex", "", "", false, ready(mia)); !went.went || went.tone != "warn" || !strings.Contains(went.text, "claude's window is still open: no window 2") {
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
	first := app.CarriedText("claude", handoff, "/repo/.git/mia/handoffs/monduli/claude-c1.md", "and the tests", chatCarry)
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
	cut := app.CarriedText("claude", long, "/repo/full.md", "go on", 48<<10)
	if len(cut) > 48<<10+500 || !strings.HasPrefix(cut, "The conversation so far, with claude:\n\n[Only its last turns are here: the whole of it is in /repo/full.md]\n\n### ") ||
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
	row := chat.put(chatBand{Kind: "model", From: "gpt-6-luna", To: "gpt-6-luna-mini"}, 0)
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

// A reply that says its agent hit its usage or rate limit is told from one
// that only speaks of limits: it says a limit was reached or hit, and is
// short or says so at once. When the limit ends is read where it is said.
func TestAReplyThatSaysTheAgentHitItsLimitIsTold(t *testing.T) {
	long := strings.Repeat("The retry loop backs off and the tests cover three tries. ", 8)
	for _, c := range []struct {
		agent, reply string
		hit          bool
		note         string
	}{
		{"claude", "You have reached your usage limit. It resets at 18:00.", true, "claude resets at 18:00"},
		{"claude", "Claude usage limit reached. Your limit will reset at 6pm (America/New_York).", true, "claude resets at 6pm"},
		{"claude", "5-hour limit reached ∙ resets 6pm", true, "claude resets at 6pm"},
		{"claude", "Weekly limit reached ∙ resets 9:30 am", true, "claude resets at 9:30 am"},
		{"claude", "You've hit your limit · resets 3am", true, "claude resets at 3am"},
		{"claude", "You're out of extra usage · resets 11pm", true, "claude resets at 11pm"},
		{"claude", "Usage limit reached. " + long, true, ""},
		{"codex", "You've hit your usage limit. Upgrade to Pro, or try again in 3 hours 12 minutes.", true, "codex again in 3 hours 12 minutes"},
		{"codex", "Rate limit reached for gpt-6-luna. Please try again in 20s.", true, "codex again in 20s"},
		{"opencode", "Rate limit exceeded", true, ""},
		// prose that speaks of limits
		{"claude", "The usage limit is read from the config, and the rate limit is 100 a minute.", false, ""},
		{"claude", "I added a test for when the rate limit resets at 6pm.", false, ""},
		{"codex", "The 5-hour limit reached by the loop is its cap.", false, ""},
		{"claude", long + "Then the client says the usage limit reached its cap and you have hit your limit of retries.", false, ""},
		{"codex", "Done. `wait` doubles after each failed run.", false, ""},
		{"claude", "", false, ""},
	} {
		if note, hit := limited(c.agent, c.reply); hit != c.hit || note != c.note {
			t.Errorf("%s says %.60q: %v %q, want %v %q", c.agent, c.reply, hit, note, c.hit, c.note)
		}
	}
}

// Under a last reply that says the agent hit its limit the chat offers to
// go on with another agent, as a turn of its own that names the key and
// the button it presses, with when the limit ends; it goes when the next
// turn comes or something is sent, and a chat that types to no window
// offers nothing.
func TestAReplyAtItsLimitIsFollowedByAnOfferToGoOnWithAnotherAgent(t *testing.T) {
	chat := &talk{pane: "%3", agent: "claude", reply: true}
	chat.heard([]app.Turn{{N: 1, Kind: "asked", Text: "cap the wait"}, {N: 2, Kind: "said", Text: "You have reached your usage limit. It resets at 18:00."}})
	got := read(t, chat.whole())
	offer := got.View.Turns[len(got.View.Turns)-1]
	if len(got.View.Turns) != 3 || offer.Kind != "offer" || offer.N != chatOwn+1 || offer.Text != "Go on with another agent" || offer.Note != "claude resets at 18:00" || offer.Key != "m" || offer.ID != "open-models" {
		t.Fatalf("offered as %+v", got.View.Turns)
	}
	view := &chatView{talk: chat, window: "2", runs: app.Runs{Agent: "claude"}}
	if deed := view.deed(chatEvent{E: "press", ID: offer.ID}); deed.do != chatModelsUp {
		t.Errorf("a press on it asks %+v", deed)
	}
	if again := chat.asked(nil); again != nil {
		t.Errorf("said again with nothing changed: %s", again)
	}
	// the next turn takes it away
	next := only(t, chat.heard([]app.Turn{{N: 3, Kind: "said", Text: "Back. The cap is in."}}))
	if !slices.Equal(ops(next), []string{"unturn", "turn"}) || next.Patch[0].N != chatOwn+1 || next.Patch[1].Turn.N != 3 {
		t.Errorf("the next turn: %+v", next.Patch)
	}
	// offered again under another such reply, and gone as soon as something is sent
	again := only(t, chat.heard([]app.Turn{{N: 4, Kind: "said", Text: "5-hour limit reached"}}))
	if !slices.Equal(ops(again), []string{"turn", "turn"}) || again.Patch[1].Turn.Kind != "offer" || again.Patch[1].Turn.N != chatOwn+2 || again.Patch[1].Turn.Note != "" {
		t.Errorf("another reply at its limit: %+v", again.Patch)
	}
	if sent := only(t, chat.sends("try now")); !slices.Equal(ops(sent), []string{"unturn", "turn"}) || sent.Patch[0].N != chatOwn+2 || sent.Patch[1].Turn.Text != "try now" {
		t.Errorf("sent after it: %+v", sent.Patch)
	}
	shown := &talk{pane: "%4", agent: "claude"}
	shown.heard([]app.Turn{{N: 1, Kind: "said", Text: "Usage limit reached."}})
	if got := read(t, shown.whole()); len(got.View.Turns) != 1 {
		t.Errorf("a conversation only shown offers %+v", got.View.Turns)
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
		text, _ := json.Marshal(app.CarriedText(from, "### 1 · user\n\nmake it faster\n", "/h/full.md", words, chatCarry))
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
	handoff = "## The conversation\n" + strings.Repeat("\n### 1 · user\n\n"+strings.Repeat("word ", 400)+"\n", 300)
	if text, _ := typedFor(kept, record, "5", "go on", read); !strings.Contains(text, "the whole of it is in /repo/claude-c1-full.md]") {
		t.Errorf("a long one does not say where every turn is: %.200q", text)
	}
}

// A conversation handed to another agent in a chat is kept on the new
// window, where a chat started after finds it: the turns so far over the
// line that says who goes on, and the handoff said with the first thing
// sent, after which the window keeps nothing. The old agent's window is
// closed, the new one's in front wherever it was, in the session and in a
// view of it, and not the window tmux would choose; before it is closed
// whoever shows the chat is told which window it is of from now on, and
// the chat of the new one is said whole with a toast that says so, not a
// status that stays.
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
		run.aim(app.Head{}, window, nil)
		t.Cleanup(func() { close(run.quit) })
		return run
	}
	// the old agent's window is in front, in the session and in a view of it, which has a front window of its own
	view := session.Name(record.Name) + "-desk1"
	for _, args := range [][]string{
		{"select-window", "-t", session.Target(record.Name, "old")},
		{"new-session", "-d", "-s", view, "-t", session.Target(record.Name, "")},
		{"select-window", "-t", "=" + view + ":old"},
	} {
		if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
			t.Fatalf("tmux %v: %s", args, out)
		}
	}
	first := chat(old)
	first.view.id, first.view.runs.Agent = "c1", "claude"
	first.view.talk.whole() // its view was said: what changes is said to whoever shows it
	first.act(chatDeed{do: chatHand, agent: "echo", model: "loud"})
	if len(said) != 4 || first.view.mode != chatting {
		t.Fatalf("%d messages: %s", len(said), said)
	}
	for _, in := range []string{session.Name(record.Name), view} {
		if out, _ := exec.Command("tmux", "display-message", "-p", "-t", "="+in+":", "#{window_name}").Output(); strings.TrimSpace(string(out)) != "echo" {
			t.Errorf("in front in %s is %q, not the window of the agent it was handed to", in, out)
		}
	}
	if moved := read(t, said[1]); !slices.Equal(ops(moved), []string{"window"}) || !strings.HasPrefix(moved.Patch[0].Window, "@") || moved.Patch[0].Window != a.WindowID(record, "echo") {
		t.Errorf("whoever shows the chat is told it is of %s now: %s", a.WindowID(record, "echo"), said[1])
	}
	said = said[2:]
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
	// sent with it: the switch says what was handed over, by the handoff's record, and the agent works
	if len(said) != 2 {
		t.Fatalf("the send said %s", said)
	}
	if went := read(t, said[1]); !slices.Equal(ops(went), []string{"unturn", "turn", "working"}) || went.Patch[1].Turn.N != 4 || went.Patch[1].Turn.Note != "context handed over · 2 turns" {
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

// wordsOf is what a piece says, in order: its texts, the choice of its
// tabs, and the keys of its buttons.
func wordsOf(piece shownPiece) string {
	var all []string
	var walk func(p shownPiece)
	walk = func(p shownPiece) {
		switch p.T {
		case "text":
			all = append(all, p.Text)
		case "tabs":
			all = append(all, "["+p.Selected+"]")
		case "button":
			all = append(all, "<"+p.Key+" "+p.ID+">")
		}
		for _, child := range p.Children {
			walk(child)
		}
	}
	walk(piece)
	return strings.Join(all, " ")
}

// A model or another agent picked while the agent is at work is not done:
// it is kept as what comes next, the line over the chat says it, and the
// models' line says when it is applied, with tab to apply it now. The same
// picked again lets go of it. At rest a pick is done at once, as it was.
func TestAPickWhileTheAgentWorksIsKeptAsNext(t *testing.T) {
	var said [][]byte
	view := &chatView{talk: &talk{pane: "%5", agent: "claude", reply: true, working: true}, worktree: "monduli", window: "2", id: "c1",
		runs: app.Runs{Agent: "claude", Model: "opus", Effort: "high", Account: "work", Live: true}, agents: chatAgents(),
		would: map[string]chatWould{"codex": {done: app.Continued{Turns: 42, LeftOut: 6}}}}
	run := &chatRun{view: view, say: func(messages ...[]byte) { said = append(said, messages...) }}
	view.talk.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}})
	view.up(picking)
	// what the said messages come to: the line over the chat, the models' line, the row marked, the foot's keys
	stands := func() (about, does, next string, keys []chatKey) {
		t.Helper()
		for _, message := range said {
			for _, op := range read(t, message).Patch {
				switch {
				case op.Op == "model":
					about = op.Text
				case op.Op == "set" && op.ID == chatOver:
					does, next = wordsOf(op.Node.Children[2]), ""
					for _, item := range op.Node.Children[4].Items {
						if strings.HasPrefix(item.Right, "next") {
							next += item.ID + " "
						}
					}
				case op.Op == "keys":
					keys = op.Keys
				}
			}
		}
		said = nil
		return about, does, next, keys
	}
	pick := func(id, sub string) chatDeed {
		view.under = id
		return view.deed(chatEvent{E: "pick", List: "models", ID: id, Sub: sub})
	}

	// a model of its own agent: kept, shown, and nothing asked of mia (the run has no app to ask)
	deed := pick("a|claude|sonnet", "")
	if deed != (chatDeed{do: chatSwitch, model: "sonnet", later: true}) {
		t.Fatalf("a model picked while it works asks %+v", deed)
	}
	if got := wordsOf(*pieceOf(t, view.models(false)).find("col")); got != "opus → sonnet · Switches in place. Nothing is re-sent. ⏎ keeps it as next: applied when claude finishes this turn." {
		t.Errorf("before it is picked the line says %q", got)
	}
	run.act(deed)
	about, does, next, keys := stands()
	if view.next == nil || *view.next != (chatDeed{do: chatSwitch, model: "sonnet"}) || view.mode != picking || about != "opus · high · on work · next: sonnet" {
		t.Errorf("kept as %+v, the chat's line %q", view.next, about)
	}
	if does != "opus → sonnet · Applied when claude finishes this turn. <tab now> tab stops claude and applies it now. ⏎ again clears it." || next != "a|claude|sonnet " ||
		!slices.Equal(keys, []chatKey{{Key: "tab", Label: "apply now", ID: "now"}}) {
		t.Errorf("the models say %q, next on %q, keys %+v", does, next, keys)
	}
	// a view said whole says it too, and the modal put back has its key
	if whole := read(t, view.talk.whole()); whole.View.Model != about || view.talk.with == nil {
		t.Errorf("said whole: %q", whole.View.Model)
	}
	for _, item := range pieceOf(t, view.models(false)).Children[4].Items {
		if want := map[string]string{"a|claude|sonnet": "clear", "a|claude|opus": "keep as next", "a|codex|gpt-6-luna": "keep as next"}[item.ID]; want != "" && item.Does != want {
			t.Errorf("⏎ on %s is said to %q", item.ID, item.Does)
		}
	}
	if now := view.deed(chatEvent{E: "press", ID: "now"}); now != (chatDeed{do: chatNow}) {
		t.Errorf("tab on it asks %+v", now)
	}

	// another picked takes its place: an agent, with its model and what is carried
	view.whole = true
	run.act(pick("a|codex|gpt-6-luna", "high"))
	about, does, next, _ = stands()
	if *view.next != (chatDeed{do: chatHand, agent: "codex", model: "gpt-6-luna", effort: "high", whole: true}) || about != "opus · high · on work · next: codex · gpt-6-luna · high" ||
		does != "claude → codex · Applied when claude finishes this turn. <tab now> Carries the whole conversation. tab stops claude and applies it now. ⏎ again clears it." || next != "a|codex|gpt-6-luna " {
		t.Errorf("an agent kept as %+v: %q, %q, next on %q", view.next, about, does, next)
	}

	// picked again it is let go of: the line over the chat is as it was, and tab is the choice's again
	again := pick("a|codex|gpt-6-luna", "")
	if again != (chatDeed{do: chatForget}) {
		t.Fatalf("picked again asks %+v", again)
	}
	run.act(again)
	about, does, next, keys = stands()
	if view.next != nil || about != "opus · high · on work" || next != "" || !strings.HasPrefix(does, "claude → codex carries [whole] <tab flip>") ||
		!slices.Equal(keys, []chatKey{{Key: "tab", Label: "latest or whole", ID: "flip"}}) {
		t.Errorf("let go of: %+v, %q, %q, next on %q, keys %+v", view.next, about, does, next, keys)
	}
	if now := view.deed(chatEvent{E: "press", ID: "now"}); now != (chatDeed{}) {
		t.Errorf("tab with nothing kept asks %+v", now)
	}

	// what cannot be done is said, at work or not, and at rest a pick is done at once
	if deed := pick("a|opencode|", ""); deed.do != chatHand || !deed.later {
		t.Errorf("an agent with no model: %+v", deed)
	}
	view.talk.working = false
	if deed := pick("a|claude|sonnet", ""); deed != (chatDeed{do: chatSwitch, model: "sonnet"}) {
		t.Errorf("at rest a pick asks %+v", deed)
	}
}

// What was picked while the agent worked is done when its turn ends, not
// before: nothing is typed into an agent at work. tab stops it, and the
// pick is done as it comes to rest. Of what the person sends as the turn
// has just ended, the pick is done first, so the words go to the agent
// picked, with the conversation in front of them.
func TestWhatWasPickedWhileTheAgentWorkedIsDoneWhenItsTurnEnds(t *testing.T) {
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
	record, err := a.New("next")
	if err != nil {
		t.Fatal(err)
	}
	var said [][]byte
	chat := func(window string) *chatRun {
		run := &chatRun{a: a, record: record, view: &chatView{worktree: record.Name}, say: func(messages ...[]byte) { said = append(said, messages...) }, done: make(chan struct{}), batches: make(chan chatBatch, 100), ended: make(chan chatEnd, 100)}
		run.aim(app.Head{}, window, nil)
		t.Cleanup(func() { close(run.quit) })
		run.view.talk.whole() // said, so what changes is said
		return run
	}
	index := func(name string) string {
		for _, w := range a.Windows(record) {
			if w.Name == name {
				return strconv.Itoa(w.Index)
			}
		}
		t.Fatalf("no window %s", name)
		return ""
	}
	titled := func(window, title string) {
		t.Helper()
		if out, err := exec.Command("tmux", "select-pane", "-t", session.Target(record.Name, window), "-T", title).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}

	// a model of its own agent, picked while it works
	if _, err := a.NewWindow(record, "echo", session.What{}, []string{"echo-agent"}); err != nil {
		t.Fatal(err)
	}
	screenShows(t, record.Name, "echo", "echo-agent")
	titled("echo", "⠋ at work") // as an agent titles its pane while it works
	own := chat(index("echo"))
	if v := own.view; v.runs.Agent != "echo" || !v.talk.working {
		t.Fatalf("a chat of a window at work: %+v, working %v", v.runs, v.talk.working)
	}
	own.act(chatDeed{do: chatModelsUp})
	deed := own.view.deed(chatEvent{E: "pick", List: "models", ID: "a|echo|soft"})
	own.act(deed)
	own.looks()
	if screen, _ := session.Here().Capture(record.Name, "echo", false); !deed.later || own.view.next == nil || own.view.runs.Model != "" || strings.Contains(screen, "/model") || own.view.talk.about != "echo · next: soft" {
		t.Fatalf("picked while it works: %+v, kept %+v, runs %+v, the line %q, its screen:\n%s", deed, own.view.next, own.view.runs, own.view.talk.about, screen)
	}
	// tab: it is stopped, and nothing else is done until it rests
	own.act(own.view.deed(chatEvent{E: "press", ID: "now"}))
	if status := own.view.talk.status; own.view.mode != chatting || own.view.next == nil || status == nil || status.Text != "stopping echo: next is soft" {
		t.Fatalf("applied now: mode %v, kept %+v, status %+v", own.view.mode, own.view.next, status)
	}
	titled("echo", "echo-agent at rest")
	said = nil
	own.looks()
	screenShows(t, record.Name, "echo", "/model soft")
	if own.view.next != nil || own.view.runs.Model != "soft" || own.view.talk.about != "echo · soft" || own.view.talk.status != nil || own.view.talk.working {
		t.Errorf("once it rests: kept %+v, runs %+v, the line %q, status %+v", own.view.next, own.view.runs, own.view.talk.about, own.view.talk.status)
	}
	var changes []string
	for _, message := range said {
		changes = append(changes, ops(read(t, message))...)
	}
	// no modal stands, and none is taken away: a change to a piece that is not there drops its patch
	if !slices.Equal(changes, []string{"working", "model", "status", "model", "toast", "turn"}) {
		t.Errorf("done as %q", changes)
	}

	// another agent, picked while the agent works: handed to with what the person sends as the turn has ended
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
	other := chat(index("old"))
	other.view.id, other.view.runs.Agent, other.view.talk.working = "c1", "claude", true
	other.act(chatDeed{do: chatModelsUp})
	other.act(other.view.deed(chatEvent{E: "pick", List: "models", ID: "a|echo|loud"}))
	other.act(chatDeed{do: chatBack})
	if next := other.view.next; next == nil || *next != (chatDeed{do: chatHand, agent: "echo", model: "loud"}) || !session.Here().HasWindow(record.Name, "old") || other.view.talk.about != "claude · next: echo · loud" {
		t.Fatalf("an agent picked while it works: kept %+v, the line %q", next, other.view.talk.about)
	}
	other.act(chatDeed{do: chatSend, text: "and the tests"})
	if other.view.next != nil || other.view.runs.Agent != "echo" || other.view.runs.Model != "loud" || session.Here().HasWindow(record.Name, "old") {
		t.Fatalf("sent as its turn ended: kept %+v, runs %+v", other.view.next, other.view.runs)
	}
	screenShows(t, record.Name, other.view.window, "The conversation so far, with claude:", "It is faster.", chatRule, "and the tests")
}

// A reply has a menu: its words copied, or the models put up to fork the
// conversation from it, as the key that forks does from the last reply.
// Only a reply of the conversation followed is forked from, by its number
// there. Put up so, a pick of any agent's row, this chat's too, forks: the
// line says who goes on from which reply with what, and the band of a
// fork says the reply it was made from, waiting or sent.
func TestAReplysMenuCopiesItOrForksTheConversationFromIt(t *testing.T) {
	view := &chatView{talk: &talk{pane: "%5", agent: "claude", reply: true, working: true}, worktree: "monduli", window: "2", id: "c1",
		runs: app.Runs{Agent: "claude", Model: "opus", Live: true}, agents: chatAgents(),
		would: map[string]chatWould{"codex": {done: app.Continued{Turns: 9, LeftOut: 2}}, "claude": {done: app.Continued{Turns: 9}}}}
	// the conversation followed stands after two turns of the one it goes on from
	view.talk.lays([]chatPart{{turns: []app.Turn{{N: 1, Kind: "asked", Text: "before"}, {N: 2, Kind: "said", Text: "Said with another."}}}, {own: true}})
	got := only(t, view.talk.heard([]app.Turn{{N: 1, Kind: "asked", Text: "make it faster"}, {N: 2, Kind: "said", Text: "It is faster."}, {N: 3, Kind: "asked", Text: "and the tests"}, {N: 4, Kind: "said", Text: "They pass."}}))
	if menu, _ := json.Marshal(got.View.Menu); string(menu) != `[{"id":"copy","title":"Copy"},{"id":"fork","title":"Continue from here with…"}]` || !slices.Equal(numbers(got.View.Turns), []int{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("the chat's menu %s, its turns %v", menu, numbers(got.View.Turns))
	}
	for _, c := range []struct {
		place, number int
		words         string
		found         bool
	}{{0, 4, "They pass.", true}, {4, 2, "It is faster.", true}, {2, 0, "Said with another.", true}, {5, 0, "", false}, {9, 0, "", false}} {
		if number, words, found := view.talk.replied(c.place); number != c.number || words != c.words || found != c.found {
			t.Errorf("the reply at %d: %d %q %v", c.place, number, words, found)
		}
	}
	for event, want := range map[chatEvent]chatDeed{
		{E: "pick", List: "chat", ID: "4", Sub: "copy"}:  {do: chatCopied, question: 4},
		{E: "pick", List: "chat", ID: "4", Sub: "fork"}:  {do: chatForkUp, question: 4},
		{E: "press", ID: "open-fork"}:                    {do: chatForkUp},
		{E: "pick", List: "chat", ID: "4"}:               {},
		{E: "pick", List: "other", ID: "4", Sub: "fork"}: {},
	} {
		if deed := view.deed(event); deed != want {
			t.Errorf("%+v asks %+v", event, deed)
		}
	}
	var said [][]byte
	run := &chatRun{view: view, say: func(messages ...[]byte) { said = append(said, messages...) }}
	run.act(chatDeed{do: chatCopied, question: 4})
	if copied := only(t, said); !slices.Equal(ops(copied), []string{"copy", "toast"}) || copied.Patch[0].Text != "It is faster." {
		t.Errorf("copied as %s", said)
	}
	said = nil
	run.act(chatDeed{do: chatForkUp, question: 2}) // another conversation's: nothing is put up
	if status := view.talk.status; view.mode != chatting || view.fork != 0 || status == nil || status.Tone != "warn" || !strings.Contains(status.Text, "only a reply of this conversation's own") {
		t.Errorf("forked from another conversation's reply: mode %v, %+v", view.mode, status)
	}

	// the models put up to fork from reply 2: any row forks, at work or not, and its own agent's too
	view.fork = 2
	modal := only(t, view.up(picking)).Patch[0].Node
	view.under = "a|codex|gpt-6-luna"
	if line, tab := view.does(); modal.Title != "Continue from reply 2" || tab != "flip" ||
		wordsOf(*pieceOf(t, line)) != "claude → codex carries [latest] <tab flip> The last 7 of 9 turns, as many as fit. In a new window; this chat stays." {
		t.Errorf("the models to fork with: %q, %q", modal.Title, wordsOf(*pieceOf(t, line)))
	}
	for _, item := range modal.Children[4].Items {
		if item.ID != "" && (item.Does != "continue in "+strings.Split(item.ID, "|")[1] || len(item.About) != 2 || !strings.Contains(item.About[1], "this chat stays as it is")) {
			t.Errorf("a row to fork with: %+v", item)
		}
	}
	if deed := view.deed(chatEvent{E: "pick", List: "models", ID: "a|codex|gpt-6-luna", Sub: "high"}); deed != (chatDeed{do: chatFork, agent: "codex", model: "gpt-6-luna", effort: "high"}) {
		t.Errorf("a pick asks %+v", deed)
	}
	view.whole, view.under = true, "a|claude|sonnet"
	if deed := view.deed(chatEvent{E: "pick", List: "models", ID: "a|claude|sonnet"}); deed != (chatDeed{do: chatFork, agent: "claude", model: "sonnet", whole: true}) {
		t.Errorf("its own agent's row asks %+v", deed)
	}
	if line, _ := view.does(); wordsOf(*pieceOf(t, line)) != "claude → claude carries [whole] <tab flip> All 9 turns. In a new window; this chat stays." {
		t.Errorf("its own agent's line: %q", wordsOf(*pieceOf(t, line)))
	}
	view.shut()
	if view.fork != 0 {
		t.Error("taken away, the models still fork")
	}

	// the band of a fork
	band := switched(app.Handed{From: app.Talk{Agent: "claude"}, To: app.Talk{Agent: "codex"}, Turns: 9, LeftOut: 2, Upto: 14})
	if band.Note != "forked from reply 14 · 9 turns · 2 left out" || waiting(*band).Note != "forked from reply 14 · context goes with your next message" {
		t.Errorf("a fork's band: %q, waiting %q", band.Note, waiting(*band).Note)
	}
	if plain := switched(app.Handed{Turns: 9}); waiting(*plain).Note != "context goes with your next message" {
		t.Errorf("a handing's band waits as %q", waiting(*plain).Note)
	}
}

// A conversation forked from a reply goes on with another agent in a
// window of its own, with only the turns up to that reply: the window of
// the agent it was with stays open and this chat is as it was. The fork is
// kept as a handing is, with the reply it was made from, so the chat of
// its window shows the turns up to that reply and the band that says so,
// and sends them in front of the first thing said there.
func TestAConversationForkedFromAReplyGoesOnInAWindowOfItsOwn(t *testing.T) {
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
	record, err := a.New("forked")
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(record.Path, "-"))
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, _ := json.Marshal(record.Path)
	said := func(who, at, text string) string {
		content, _ := json.Marshal(text)
		if who == "assistant" {
			content = []byte(`[{"type":"text","text":` + string(content) + `}]`)
		}
		return `{"type":"` + who + `","cwd":` + string(cwd) + `,"timestamp":"2026-11-02T10:00:` + at + `Z","message":{"content":` + string(content) + `}}` + "\n"
	}
	lines := said("user", "00", "make the list faster") + said("assistant", "05", "It is faster.") + said("user", "10", "now drop the cache") + said("assistant", "15", "The cache is gone.")
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
	var messages [][]byte
	chat := func(window string) *chatRun {
		run := &chatRun{a: a, record: record, view: &chatView{worktree: record.Name}, say: func(said ...[]byte) { messages = append(messages, said...) }, done: make(chan struct{}), batches: make(chan chatBatch, 100), ended: make(chan chatEnd, 100)}
		run.aim(app.Head{}, window, nil)
		t.Cleanup(func() { close(run.quit) })
		return run
	}
	first := chat(old)
	first.view.id, first.view.runs.Agent = "c1", "claude"
	turns, err := a.ConversationIn("c1", record.Name, 0)
	if err != nil || len(turns) != 4 {
		t.Fatalf("the conversation: %+v, %v", turns, err)
	}
	first.view.talk.heard(turns)

	// from its first reply, by the reply's menu: the models, and a pick of them
	first.act(first.view.deed(chatEvent{E: "pick", List: "chat", ID: "2", Sub: "fork"}))
	if first.view.mode != picking || first.view.fork != 2 {
		t.Fatalf("the models to fork with: mode %v, from reply %d", first.view.mode, first.view.fork)
	}
	// looked at for the agent of the row under the keys, the conversation's own too
	under, _ := chatModelOf(first.view.under)
	if would := first.view.would[under.agent]; would.err != nil || would.done.Turns != 2 || would.done.Upto != 2 || !would.done.DryRun {
		t.Errorf("what forking would do, under the keys %q: %+v", first.view.under, would)
	}
	messages = nil
	first.act(first.view.deed(chatEvent{E: "pick", List: "models", ID: "a|echo|loud"}))
	if toast := read(t, messages[len(messages)-1]); first.view.mode != chatting || first.view.window != old || first.view.id != "c1" || first.view.runs.Agent != "claude" ||
		!slices.Equal(ops(toast), []string{"status", "toast"}) || toast.Patch[1].Tone != "ok" || toast.Patch[1].Text != "echo goes on from reply 2 in window echo · this chat stays as it is" {
		t.Fatalf("forked: window %s, %+v, said %s", first.view.window, first.view.runs, messages)
	}
	if !session.Here().HasWindow(record.Name, "old") || !session.Here().HasWindow(record.Name, "echo") {
		t.Fatal("the window it was forked from is closed, or none was opened")
	}
	screenShows(t, record.Name, "echo", "echo-agent --model loud")
	window := ""
	for _, w := range a.Windows(record) {
		if w.Name == "echo" {
			window = strconv.Itoa(w.Index)
		}
	}
	carried, on := a.CarriedInWindow(record, window)
	if !on || carried.From != "claude" || carried.ID != "c1" || filepath.Base(carried.File) != "claude-c1-to-2.md" {
		t.Fatalf("on the fork's window: %+v, %v", carried, on)
	}
	if text, _ := os.ReadFile(carried.File); !strings.Contains(string(text), "It is faster.") || strings.Contains(string(text), "drop the cache") || !strings.Contains(string(text), "2 turns, in order") {
		t.Errorf("the fork's handoff:\n%s", text)
	}
	if records, _ := filepath.Glob(filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "claude-c1-to-2.*.json")); len(records) != 1 {
		t.Errorf("the fork's records: %v", records)
	} else if kept, _ := os.ReadFile(records[0]); !strings.Contains(string(kept), `"turns":2`) || !strings.Contains(string(kept), `"upto":2`) {
		t.Errorf("the record keeps %s", kept)
	}
	if _, on := a.CarriedInWindow(record, old); on {
		t.Error("the window it was forked from is to be sent something")
	}

	// the chat of the fork's window: the turns up to the reply, and the band that says where it was forked from
	second := chat(window)
	got := read(t, second.view.talk.whole())
	if band := got.View.Turns[len(got.View.Turns)-1]; len(got.View.Turns) != 3 || got.View.Turns[1].Text != "It is faster." ||
		band.Kind != "switch" || band.From != "claude" || band.To != "echo" || band.Tone != "warn" || band.Note != "forked from reply 2 · context goes with your next message" {
		t.Fatalf("the fork's chat shows %+v", got.View.Turns)
	}
	messages = nil
	second.act(chatDeed{do: chatSend, text: "and the tests"})
	if went := read(t, messages[len(messages)-1]); !slices.Equal(ops(went), []string{"unturn", "turn", "working"}) || went.Patch[1].Turn.Note != "forked from reply 2 · 2 turns" {
		t.Errorf("once sent, the band is said as %s", messages[len(messages)-1])
	}
	screen := screenShows(t, record.Name, window, "The conversation so far, with claude:", "It is faster.", chatRule, "and the tests")
	if strings.Contains(screen, "drop the cache") {
		t.Errorf("the fork was sent what came after the reply:\n%s", screen)
	}
	// and the conversation it was forked from is typed to as ever, with nothing in front
	first.act(chatDeed{do: chatSend, text: "go on"})
	if screen := screenShows(t, record.Name, old, "go on"); strings.Contains(screen, "The conversation so far") {
		t.Errorf("the window it was forked from was sent the conversation:\n%s", screen)
	}
}

// Rows of the models are marked with ⇧⏎, the one key the search line lets
// through, by the button that says so under the list: the line under the
// search line then says how many agents the next message is sent to, this
// chat's here and each other in a worktree of its own, with esc on a cap to
// clear the marks, and the rows marked are checked beside the one that
// runs. The row this chat runs on is not marked, and none is while
// something is picked to come next or what the agent goes on from has
// still to be sent. ⏎ keeps them: the line over the chat names them and
// the chat's field says how many agents hear what is typed. What is sent
// then is put up to be confirmed, a row for each worktree made, and
// Cancel makes nothing and gives the words back.
func TestRowsMarkedInTheModelsSendTheNextMessageToSeveralAgents(t *testing.T) {
	var said [][]byte
	view := &chatView{talk: &talk{pane: "%5", agent: "claude", reply: true}, worktree: "monduli", window: "2", id: "c1",
		runs: app.Runs{Agent: "claude", Model: "opus", Effort: "high", Live: true}, agents: chatAgents(), would: map[string]chatWould{}}
	run := &chatRun{view: view, say: func(messages ...[]byte) { said = append(said, messages...) }}
	view.talk.heard([]app.Turn{{N: 1, Kind: "asked", Text: "hello"}})
	view.up(picking)
	if mark := pieceOf(t, view.models(false)).Children[6]; mark.T != "button" || mark.ID != "mark" || mark.Key != "⇧⏎" {
		t.Fatalf("the button that marks: %+v", mark)
	}
	// what the said messages come to: the models' line, the rows checked, the foot's keys, and the view said whole last
	stands := func() (does string, checked []string, keys []chatKey, whole *heardByWindow) {
		t.Helper()
		for _, message := range said {
			got := read(t, message)
			if got.Whole != nil {
				whole = &got
			}
			for _, op := range got.Patch {
				switch {
				case (op.Op == "set" || op.Op == "add") && op.Node != nil && op.Node.ID == chatOver:
					does, checked = wordsOf(op.Node.Children[2]), nil
					for _, item := range op.Node.Children[4].Items {
						if item.Checked {
							checked = append(checked, item.ID)
						}
					}
				case op.Op == "keys":
					keys = op.Keys
				}
			}
		}
		said = nil
		return does, checked, keys, whole
	}
	mark := func(id string) chatDeed { return view.deed(chatEvent{E: "press", ID: "mark", Item: id}) }

	// the row this chat runs on: it hears the message here whatever is marked
	if deed := mark("a|claude|opus"); deed.do != chatSay || !strings.Contains(deed.text, "claude gets your next message here in any case") {
		t.Errorf("marking the row that runs asks %+v", deed)
	}
	// one other agent's row, then another's
	if deed := mark("a|codex|gpt-6-luna"); deed != (chatDeed{do: chatToggle, id: "a|codex|gpt-6-luna"}) {
		t.Fatalf("marking a row asks %+v", deed)
	}
	run.act(mark("a|codex|gpt-6-luna"))
	does, checked, keys, _ := stands()
	if does != "Sends your next message to 2 agents <esc unmark> claude here · codex in a worktree of its own" ||
		!slices.Equal(checked, []string{"a|claude|opus", "a|codex|gpt-6-luna"}) || !slices.Equal(keys, []chatKey{{Key: "esc", Label: "clear marks", ID: "unmark"}}) {
		t.Errorf("one row marked: %q, checked %v, keys %+v", does, checked, keys)
	}
	run.act(mark("a|opencode|"))
	if does, checked, _, _ := stands(); does != "Sends your next message to 3 agents <esc unmark> claude here · codex and opencode each in a worktree of their own" ||
		!slices.Equal(checked, []string{"a|claude|opus", "a|codex|gpt-6-luna", "a|opencode|"}) {
		t.Errorf("two rows marked: %q, checked %v", does, checked)
	}
	// the same again takes a mark off
	run.act(mark("a|opencode|"))
	if does, checked, _, _ := stands(); !strings.HasPrefix(does, "Sends your next message to 2 agents") || len(checked) != 2 {
		t.Errorf("a mark taken off: %q, checked %v", does, checked)
	}
	run.act(mark("a|opencode|"))
	// esc is the marks' first, wherever the keys are; ⏎ keeps them
	for _, event := range []chatEvent{{E: "press", ID: "unmark"}, {E: "dismiss", ID: chatOver}} {
		if deed := view.deed(event); deed.do != chatUnmark {
			t.Errorf("%+v with rows marked asks %+v", event, deed)
		}
	}
	if deed := view.deed(chatEvent{E: "pick", List: "models", ID: "a|claude|sonnet"}); deed.do != chatFan {
		t.Fatalf("⏎ with rows marked asks %+v", deed)
	}
	stands()
	run.act(chatDeed{do: chatFan})
	_, _, _, whole := stands()
	if view.mode != chatting || len(view.fans) != 2 || view.marks != nil || whole == nil ||
		whole.View.Model != "opus · high · fan-out: codex · gpt-6-luna, opencode" || whole.View.Placeholder != "Say something to 3 agents" {
		t.Fatalf("kept: mode %v, %+v, said %+v", view.mode, view.fans, whole)
	}
	// put up again they are marked, to add to or clear: esc lets go of them
	view.marks = slices.Clone(view.fans)
	view.up(picking)
	run.act(view.deed(chatEvent{E: "press", ID: "unmark"}))
	does, checked, keys, whole = stands()
	if view.mode != picking || view.fans != nil || view.marks != nil || does != "opus · What this chat runs on." || !slices.Equal(checked, []string{"a|claude|opus"}) || len(keys) != 0 ||
		whole == nil || whole.View.Model != "opus · high" || whole.View.Placeholder != "" {
		t.Errorf("cleared: mode %v, %+v, line %q, checked %v, keys %+v, said %+v", view.mode, view.fans, does, checked, keys, whole)
	}
	// not offered while something waits: why is said at the modal's foot
	view.next = &chatDeed{do: chatSwitch, model: "sonnet"}
	if deed := mark("a|codex|gpt-6-luna"); deed.do != chatSay || !strings.HasPrefix(deed.text, "sonnet is picked to come next") {
		t.Errorf("marking with a pick to come asks %+v", deed)
	}
	view.next, view.talk.waits = nil, &chatBand{Kind: "switch", From: "codex", To: "claude"}
	if deed := mark("a|codex|gpt-6-luna"); deed.do != chatSay || !strings.HasPrefix(deed.text, "what claude goes on from has still to be sent") {
		t.Errorf("marking with a conversation still to send asks %+v", deed)
	}
	view.talk.waits = nil
	view.shut()

	// what sending makes, put up before any of it is made
	view.fans = []chatModel{{agent: "codex", model: "gpt-6-luna"}, {agent: "opencode"}}
	view.plan = &chatPlan{text: "try it your way", dirty: true, makes: []chatMake{{view.fans[0], "monduli-codex", "retry-codex"}, {view.fans[1], "monduli-opencode-2", "retry-opencode-2"}}}
	up := only(t, view.up(confirming))
	if modal := up.Patch[0].Node; !slices.Equal(ops(up), []string{"add"}) || modal.Title != "Send to 3 agents" || view.talk.keys != "create" ||
		wordsOf(*modal) != "worktree monduli-codex on branch retry-codex · codex · gpt-6-luna worktree monduli-opencode-2 on branch retry-opencode-2 · opencode Uncommitted changes here are not copied. <⏎ create> <esc cancel>" {
		t.Errorf("to be confirmed: %s", wordsOf(*up.Patch[0].Node))
	}
	if deed := view.deed(chatEvent{E: "press", ID: "create"}); deed.do != chatCreate {
		t.Errorf("Create asks %+v", deed)
	}
	if deed := view.deed(chatEvent{E: "send", Text: "more"}); deed.do != chatNothing {
		t.Errorf("a send under it asks %+v", deed)
	}
	for _, event := range []chatEvent{{E: "press", ID: "cancel"}, {E: "dismiss", ID: chatOver}} {
		if deed := view.deed(event); deed.do != chatBack {
			t.Errorf("%+v asks %+v", event, deed)
		}
	}
	said = nil
	run.act(chatDeed{do: chatBack})
	if back := only(t, said); !slices.Equal(ops(back), []string{"remove", "draft"}) || back.Patch[1].Text != "try it your way" || view.plan != nil || view.mode != chatting || len(view.fans) != 2 {
		t.Errorf("cancelled: %s, plan %+v, %+v", said, view.plan, view.fans)
	}
}

// fanLab is a chat of a window that runs cat, in a worktree of a
// repository where the echo agent is installed, with two of echo's models
// marked and kept: what is said in the chat, and the window's number. With
// lines the worktree has claude's conversation c1 of them, and the chat is
// of it.
func fanLab(t *testing.T, turns int) (a *app.App, repo *gittest.Repo, record model.Record, run *chatRun, messages *[][]byte, old string) {
	t.Helper()
	roleLab(t)
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", t.TempDir())
	repo = gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = pluginHost(config.Config{})
	if record, err = a.New("forked"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "old", session.What{}, []string{"cat"}); err != nil {
		t.Fatal(err)
	}
	for _, w := range a.Windows(record) {
		if w.Name == "old" {
			old = strconv.Itoa(w.Index)
		}
	}
	messages = new([][]byte)
	run = &chatRun{a: a, record: record, view: &chatView{worktree: record.Name}, say: func(said ...[]byte) { *messages = append(*messages, said...) }, done: make(chan struct{}), batches: make(chan chatBatch, 100), ended: make(chan chatEnd, 100)}
	run.aim(app.Head{}, old, nil)
	t.Cleanup(func() { close(run.quit) })
	run.view.runs.Agent = "claude"
	run.say(run.view.talk.said()...)
	if turns > 0 {
		project := filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(record.Path, "-"))
		if err := os.MkdirAll(project, 0o755); err != nil {
			t.Fatal(err)
		}
		cwd, _ := json.Marshal(record.Path)
		lines := ""
		for i := range turns {
			who, content := "user", fmt.Sprintf(`"asked %d"`, i+1)
			if i%2 == 1 {
				who, content = "assistant", fmt.Sprintf(`[{"type":"text","text":"Reply %d."}]`, i+1)
			}
			lines += fmt.Sprintf(`{"type":"%s","cwd":%s,"timestamp":"2026-11-02T10:00:%02dZ","message":{"content":%s}}`+"\n", who, cwd, i, content)
		}
		if err := os.WriteFile(filepath.Join(project, "c1.jsonl"), []byte(lines), 0o644); err != nil {
			t.Fatal(err)
		}
		said, err := a.ConversationIn("c1", record.Name, 0)
		if err != nil || len(said) != turns {
			t.Fatalf("the conversation: %+v, %v", said, err)
		}
		run.view.id = "c1"
		run.view.talk.heard(said)
	}
	run.act(run.view.deed(chatEvent{E: "press", ID: "open-models"}))
	for _, id := range []string{"a|echo|loud", "a|echo|soft"} {
		run.act(run.view.deed(chatEvent{E: "press", ID: "mark", Item: id}))
	}
	run.act(run.view.deed(chatEvent{E: "pick", List: "models", ID: "a|echo|soft"}))
	if v := run.view; v.mode != chatting || len(v.fans) != 2 || v.talk.about != "claude · fan-out: echo · loud, echo · soft" || v.talk.hint != "Say something to 3 agents" {
		t.Fatalf("the fan-out kept: mode %v, %+v, %q, %q", v.mode, v.fans, v.talk.about, v.talk.hint)
	}
	return a, repo, record, run, messages, old
}

// A message sent with a fan-out kept is first put up to be confirmed, with
// exactly what is made: Cancel makes nothing and gives the words back.
// Create makes a worktree for each agent at this one's last commit, named
// and on a branch named for it with a number where taken, starts the agent
// in it and sends it the conversation up to its last turn in front of the
// words; then this chat's own agent is typed the words alone, the fan-out
// is let go of, and a band of the chat's own says who else they went to.
func TestAMessageFannedOutGoesToEachAgentInAWorktreeOfItsOwn(t *testing.T) {
	a, repo, record, run, messages, old := fanLab(t, 4)
	v := run.view
	if err := os.WriteFile(filepath.Join(record.Path, "scratch"), []byte("not committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	one, two := record.Name+"-echo", record.Name+"-echo-2"
	*messages = nil
	run.act(v.deed(chatEvent{E: "send", Text: "try it your way"}))
	up := only(t, *messages)
	if want := "worktree " + one + " on branch forked-echo · echo · loud worktree " + two + " on branch forked-echo-2 · echo · soft Uncommitted changes here are not copied. <⏎ create> <esc cancel>"; v.mode != confirming || wordsOf(*up.Patch[0].Node) != want {
		t.Fatalf("to be confirmed, mode %v: %s", v.mode, wordsOf(*up.Patch[0].Node))
	}
	// cancelled: nothing is made or sent, and the words are the field's again
	*messages = nil
	run.act(v.deed(chatEvent{E: "dismiss", ID: chatOver}))
	records, _ := a.Store.Load()
	if back := only(t, *messages); back.Patch[len(back.Patch)-1].Op != "draft" || back.Patch[len(back.Patch)-1].Text != "try it your way" || len(records) != 1 ||
		repo.Git("branch", "--list", "forked-echo*") != "" || len(v.fans) != 2 {
		t.Fatalf("cancelled: %s, worktrees %+v, branches %q", *messages, records, repo.Git("branch", "--list", "forked-echo*"))
	}
	if screen, _ := session.Here().Capture(record.Name, old, false); strings.Contains(screen, "try it your way") {
		t.Fatalf("cancelled, and it was sent:\n%s", screen)
	}

	// confirmed
	run.act(v.deed(chatEvent{E: "send", Text: "try it your way"}))
	*messages = nil
	run.act(v.deed(chatEvent{E: "press", ID: "create"}))
	head := repo.GitIn(record.Path, "rev-parse", "HEAD")
	for name, model := range map[string]string{one: "loud", two: "soft"} {
		path := repo.Root + "." + name
		if repo.GitIn(path, "rev-parse", "HEAD") != head || repo.GitIn(path, "symbolic-ref", "--short", "HEAD") != strings.Replace(name, record.Name, "forked", 1) {
			t.Errorf("%s is at %s on %s", name, repo.GitIn(path, "rev-parse", "HEAD"), repo.GitIn(path, "symbolic-ref", "--short", "HEAD"))
		}
		if _, err := os.Stat(filepath.Join(path, "scratch")); err == nil {
			t.Errorf("%s has what was not committed", name)
		}
		made, err := a.RecordOrHere(name)
		if runs := a.WindowRuns(made, "echo"); err != nil || runs.Agent != "echo" || runs.Model != model {
			t.Errorf("%s runs %+v, %v", name, runs, err)
		}
		screenShows(t, name, "echo", "The conversation so far, with claude:", "Reply 4.", chatRule, "try it your way")
	}
	// this chat's own agent: the words alone, in place
	if screen := screenShows(t, record.Name, old, "try it your way"); strings.Contains(screen, "The conversation so far") {
		t.Errorf("this chat's agent was sent the conversation:\n%s", screen)
	}
	var toasts []string
	var band *shownTurn
	for _, message := range *messages {
		for _, op := range read(t, message).Patch {
			switch {
			case op.Op == "toast":
				toasts = append(toasts, op.Text)
			case op.Op == "turn" && op.Turn.Kind == "switch":
				band = op.Turn
			}
		}
	}
	if !slices.Equal(toasts, []string{"echo started in " + one, "echo started in " + two}) {
		t.Errorf("said as each started: %q", toasts)
	}
	if band == nil || band.From != "sent to 3 agents" || band.To != "" || band.Note != "echo in "+one+" · echo in "+two {
		t.Errorf("the band: %+v", band)
	}
	if v.fans != nil || v.plan != nil || v.talk.about != "claude" || v.talk.hint != "" || v.talk.status != nil {
		t.Errorf("after it: %+v, %q, %q, %+v", v.fans, v.talk.about, v.talk.hint, v.talk.status)
	}
	// under what was sent, once the transcript says it; and each is kept as a fork, in the worktree it is in
	v.talk.heard([]app.Turn{{N: 5, Kind: "asked", Text: "try it your way"}, {N: 6, Kind: "said", Text: "Tried."}})
	got := read(t, v.talk.whole())
	if turns := got.View.Turns; len(turns) != 7 || turns[4].Text != "try it your way" || turns[5].From != "sent to 3 agents" || turns[6].Text != "Tried." {
		t.Errorf("the chat shows %+v", got.View.Turns)
	}
	kept, _ := filepath.Glob(filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "claude-c1-to-4.*.json"))
	if len(kept) != 2 {
		t.Fatalf("the records: %v", kept)
	}
	for _, file := range kept {
		if text, _ := os.ReadFile(file); !strings.Contains(string(text), `"upto":4`) || !strings.Contains(string(text), `"worktree":"`+one) {
			t.Errorf("a record keeps %s", text)
		}
	}
}

// A fan-out stops where one of its agents cannot be made: what was made
// before stays as it is and heard the words, nothing more is made or sent,
// this chat's own agent is sent nothing, the words are the field's again,
// the status says what was made and what failed, and the agents not sent
// to are still kept. From a chat with nothing said yet an agent is started
// plainly and sent the words alone.
func TestAFanOutThatFailsLeavesWhatWasMadeAndSendsNothingFurther(t *testing.T) {
	a, repo, record, run, messages, old := fanLab(t, 0)
	v := run.view
	one, two := record.Name+"-echo", record.Name+"-echo-2"
	run.act(v.deed(chatEvent{E: "send", Text: "try it your way"}))
	if v.mode != confirming || v.plan.makes[1].name != two || v.plan.makes[1].branch != "forked-echo-2" || v.plan.dirty {
		t.Fatalf("to be confirmed: mode %v, %+v", v.mode, v.plan)
	}
	repo.Git("branch", "forked-echo-2") // taken since it was put up
	*messages = nil
	run.act(v.deed(chatEvent{E: "press", ID: "create"}))
	screen := screenShows(t, one, "echo", "echo-agent --model loud", "try it your way")
	if strings.Contains(screen, "The conversation so far") {
		t.Errorf("the first was sent a conversation there is none of:\n%s", screen)
	}
	records, _ := a.Store.Load()
	if len(records) != 2 || session.Here().Exists(two) {
		t.Errorf("made after it failed: %+v", records)
	}
	if screen, _ := session.Here().Capture(record.Name, old, false); strings.Contains(screen, "try it your way") {
		t.Errorf("this chat's agent was sent it:\n%s", screen)
	}
	var status *muiStatus
	draft := ""
	for _, message := range *messages {
		got := read(t, message)
		if got.Whole != nil {
			status = got.Status
		}
		for _, op := range got.Patch {
			if op.Op == "draft" {
				draft = op.Text
			}
		}
	}
	if status == nil || status.Tone != "bad" || !strings.HasPrefix(status.Text, "stopped at echo: nothing sent to claude · made: echo in "+one+" · ") || !strings.Contains(status.Text, "forked-echo-2 already exists") {
		t.Errorf("the status: %+v", status)
	}
	if draft != "try it your way" || len(v.fans) != 1 || v.fans[0].model != "soft" || v.talk.hint != "Say something to 2 agents" || v.talk.about != "claude · fan-out: echo · soft" {
		t.Errorf("after it: draft %q, %+v, %q, %q", draft, v.fans, v.talk.hint, v.talk.about)
	}
}
