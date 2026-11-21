package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/session"
)

const chatUsage = "mia chat [--window <index>] [--conversation <id>]"

// chatTurns is how many turns the view said whole has at most: under what
// the window takes, so that turns can be added to it by patches before it
// must be said again.
const chatTurns = 1000

// patchEnvelope is more than a patch's message is around its changes: its
// version and its pane.
const patchEnvelope = 64

// What the window takes besides a chat's turns: chatFrame is more than the
// pieces a view has around them — its own few, and those of a modal over
// it, a list of chatHeads beside chatReadMost turns — chatSaid the
// characters it keeps of a status, a toast and a list's words, chatDraft
// those it keeps of what a chat's field holds, chatHeads how many
// conversations Earlier lists, and chatReadMost how many turns of one it
// reads back.
const (
	chatFrame = 720
	chatDraft = 4000
	// chatCarry is how many bytes of a conversation gone on from are said
	// in front of the first thing sent to the agent that goes on with it,
	// at most: about a hundred thousand tokens, half of what the least of
	// the agents' models reads at once, so there is room left to answer
	// in. A handoff is under it unless the whole of a very long
	// conversation was asked for: tmux pastes more than this in one go.
	chatCarry    = 400 << 10
	chatSaid     = 200
	chatHeads    = 300
	chatReadMost = 400
	// chatOwn is the place before the first turn of the app's own: after
	// any a transcript has. The window keeps a turn it was told and takes
	// another at its place only as a reply grown longer, so a turn of the
	// app's own has a place no other turn ever has, and is taken out by it.
	chatOwn = 1 << 30
	// chatRoom is how tall Earlier's modal is under its title, in points:
	// the list and the conversation read back beside it share it.
	chatRoom = 420
	// chatOver is the id of the modal over the chat: the models, or the
	// earlier conversations.
	chatOver = "over"
)

// chatMessage is a view said whole: a UI that fills its pane, the piece
// the keys are put on (once each time another is named), and how things
// stand at the right of its top line.
type chatMessage struct {
	V      int             `json:"v"`
	Pane   string          `json:"pane,omitempty"`
	Fill   bool            `json:"fill"`
	Focus  string          `json:"focus,omitempty"`
	Status *muiStatus      `json:"status,omitempty"`
	View   json.RawMessage `json:"view"`
}

// chatKey is a key an item of a list takes while the keys are on it: what
// it is called along the foot, and the id it is said by.
type chatKey struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	ID    string `json:"id"`
}

// chatBox is a col or a row of pieces.
type chatBox struct {
	T        string            `json:"t"`
	ID       string            `json:"id,omitempty"`
	Children []json.RawMessage `json:"children,omitempty"`
	Gap      int               `json:"gap,omitempty"`
	Pad      int               `json:"pad,omitempty"`
	Grow     int               `json:"grow,omitempty"`
	W        int               `json:"w,omitempty"`
	H        int               `json:"h,omitempty"`
	Split    bool              `json:"split,omitempty"`
}

// chatModal is pieces in a card over the chat, the rest dimmed behind it:
// the keys are its pieces' alone, and esc says dismiss.
type chatModal struct {
	T        string            `json:"t"`
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	W        int               `json:"w"`
	H        int               `json:"h,omitempty"`
	Children []json.RawMessage `json:"children"`
}

// chatWords is a text; chatButton a button its key presses from anywhere
// but a field; chatField the search line that walks a list.
type chatWords struct {
	T     string `json:"t"`
	ID    string `json:"id,omitempty"`
	Text  string `json:"text"`
	Style string `json:"style,omitempty"`
	Tone  string `json:"tone,omitempty"`
}

// chatTabs is a choice of a few, side by side in one box, the one selected
// filled: a press on one picks it.
type chatTabs struct {
	T        string     `json:"t"`
	ID       string     `json:"id"`
	Boxed    bool       `json:"boxed"`
	Selected string     `json:"selected"`
	Items    []chatItem `json:"items"`
}

type chatButton struct {
	T       string `json:"t"`
	ID      string `json:"id"`
	Label   string `json:"label"`
	Key     string `json:"key"`
	Primary bool   `json:"primary,omitempty"`
	Quiet   bool   `json:"quiet,omitempty"`
}

type chatField struct {
	T           string `json:"t"`
	ID          string `json:"id"`
	Placeholder string `json:"placeholder"`
	Icon        string `json:"icon"`
	Right       string `json:"right"`
	Bare        bool   `json:"bare"`
	Live        bool   `json:"live"`
	List        string `json:"list"`
}

// chatList is a list in a room of its own, max rows tall: the models, each
// with its efforts as its menu, or the worktree's conversations, the row
// under the keys said as it changes and each with what can be done to it
// as its menu.
type chatList struct {
	T        string     `json:"t"`
	ID       string     `json:"id"`
	Live     bool       `json:"live,omitempty"`
	Flat     bool       `json:"flat,omitempty"`
	Max      int        `json:"max,omitempty"`
	Menu     string     `json:"menu,omitempty"`
	Selected string     `json:"selected,omitempty"`
	Items    []chatItem `json:"items"`
}

// chatItem is what a list holds: an item, or with head and no id the quiet
// word over the items after it.
type chatItem struct {
	Head    string     `json:"head,omitempty"`
	Tag     string     `json:"tag,omitempty"`
	Note    string     `json:"note,omitempty"`
	ID      string     `json:"id,omitempty"`
	Title   string     `json:"title"`
	Right   string     `json:"right,omitempty"`
	Icon    string     `json:"icon,omitempty"`
	Running *bool      `json:"running,omitempty"`
	Checked bool       `json:"checked,omitempty"`
	More    string     `json:"more,omitempty"`
	Does    string     `json:"does,omitempty"`
	Menu    []chatItem `json:"menu,omitempty"`
	About   []string   `json:"about,omitempty"`
	Keys    []chatKey  `json:"keys,omitempty"`
}

// chatPiece is the window's chat: a conversation's turns, the field that
// types to its agent under them with reply, and Stop while working. Menu
// is what can be done with a reply, the menu of each: one chosen is said
// as a pick of the chat, of the reply by its place, with the choice as sub.
// Placeholder is what its empty field says, where that is not who answers.
type chatPiece struct {
	T           string            `json:"t"`
	ID          string            `json:"id"`
	Agent       string            `json:"agent"`
	Reply       bool              `json:"reply"`
	Working     bool              `json:"working"`
	Placeholder string            `json:"placeholder,omitempty"`
	Menu        []chatItem        `json:"menu,omitempty"`
	Turns       []json.RawMessage `json:"turns"`
}

// chatOp is one change of a patch: a turn put in the chat, or taken out
// of it by its place n; whether its agent is at work; words put in its
// field, on the clipboard, by the title as the status — none takes it
// away — or over the foot a few seconds as a toast; a piece put last in
// another, said again whole, or taken out; the keys said along the foot.
type chatOp struct {
	Op   string          `json:"op"`
	ID   string          `json:"id,omitempty"`
	Turn json.RawMessage `json:"turn,omitempty"`
	On   *bool           `json:"on,omitempty"`
	N    int             `json:"n,omitempty"`
	Text string          `json:"text,omitempty"`
	Tone string          `json:"tone,omitempty"`
	Node json.RawMessage `json:"node,omitempty"`
	Keys *[]chatKey      `json:"keys,omitempty"`
	// Window is the window a chat is of from now on, by tmux's own name for it.
	Window string `json:"window,omitempty"`
}

// chatTurn is a turn as the window is told it, its place, and what it
// says: its kind, its words and its options, by which a turn of the app's
// own and the transcript's are the same one.
type chatTurn struct {
	n     int
	kind  string
	words string
	json  json.RawMessage
}

// chatBand is a row of the chat's own among its turns, across its column.
// A switch is where the conversation went from one agent to another, with
// what happened at its right: tone warn while what the new agent goes on
// from has still to be sent to it; with no agent it went to, it is words of
// the chat's own, as where a message was sent to several agents at once. A
// model is the lighter line where the
// same agent went from one model to another. An offer is something the
// person can do now, in its words, with what is known at its right and
// the key that does it: a press on it says the button of its id.
type chatBand struct {
	N    int    `json:"n"`
	Kind string `json:"kind"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Text string `json:"text,omitempty"`
	Note string `json:"note,omitempty"`
	Tone string `json:"tone,omitempty"`
	Key  string `json:"key,omitempty"`
	ID   string `json:"id,omitempty"`
}

// chatLimits is how each agent says it has no more of its plan to answer
// with, for now: that a limit was reached or hit, never a limit only
// spoken of. The last is any other agent's.
var chatLimits = []struct {
	agent string
	says  *regexp.Regexp
}{
	{"claude", regexp.MustCompile(`(?i)\b(?:(?:usage|5-hour|weekly|session|opus|sonnet) limit reached|(?:reached|hit) your (?:(?:usage|5-hour|weekly|session) )?limit|out of extra usage)\b`)},
	{"codex", regexp.MustCompile(`(?i)\b(?:(?:reached|hit) your (?:usage|rate) limit|(?:usage|rate) limit (?:reached|exceeded))\b`)},
	{"", regexp.MustCompile(`(?i)\b(?:(?:reached|hit) your (?:usage|rate) limit|(?:usage|rate) limit (?:reached|exceeded))\b`)},
}

// chatResets is when a limit ends, where the reply says: the hour it
// resets at, or how long until it can be tried again.
var chatResets = regexp.MustCompile(`(?i)\bresets?(?: at)? (\d{1,2}(?::\d{2})? ?(?:am|pm)?)|\btry again in ([^.,;()\n]+)`)

// limited reports whether a reply says its agent hit its usage or rate
// limit, and what is known of when that ends ("claude resets at 18:00"; ""
// when the reply does not say). Only a reply that is short, or that says
// it at once, is taken to: a long one that comes to a limit further on is
// prose about limits.
func limited(agent, reply string) (note string, hit bool) {
	reply = strings.TrimSpace(reply)
	at := slices.IndexFunc(chatLimits, func(one struct {
		agent string
		says  *regexp.Regexp
	}) bool {
		return one.agent == agent || one.agent == ""
	})
	found := chatLimits[at].says.FindStringIndex(reply)
	if found == nil || len(reply) > 300 && found[0] > 40 {
		return "", false
	}
	switch when := chatResets.FindStringSubmatch(reply); {
	case when == nil:
	case when[1] != "":
		note = agent + " resets at " + strings.TrimSpace(when[1])
	default:
		note = agent + " again in " + strings.TrimSpace(when[2])
	}
	return note, true
}

// banded is a band as a turn of the chat, at its place.
func banded(band chatBand) chatTurn { return chatTurn{n: band.N, kind: band.Kind, json: encode(band)} }

// waiting is a switch as it stands until what it says was handed over is
// sent, with the person's next message: at the place before its own.
func waiting(band chatBand) chatBand {
	note := "context goes with your next message"
	if fork, _, _ := strings.Cut(band.Note, " · "); strings.HasPrefix(fork, forkedFrom) {
		note = fork + " · " + note // a fork says where it was made from all along
	}
	band.N, band.Note, band.Tone = band.N-1, note, "warn"
	return band
}

// forkedFrom is how the band of a fork begins: the number of the reply it
// was made from follows.
const forkedFrom = "forked from reply "

// switched is the band of a handing: what was handed over, in how many
// turns and how many of them left out, that the agent took its own
// conversation up again and how many turns were said since, or of a fork
// the reply it was made from.
func switched(handed app.Handed) *chatBand {
	note, since := "context handed over", ""
	if handed.Resumed {
		note, since = "took its own conversation up again", " since"
	}
	if handed.Upto > 0 {
		note = forkedFrom + strconv.Itoa(handed.Upto)
	}
	if handed.Turns > 0 {
		note += " · " + counted(handed.Turns, "turn") + since
	}
	if handed.LeftOut > 0 {
		note += fmt.Sprintf(" · %d left out", handed.LeftOut)
	}
	return &chatBand{Kind: "switch", From: handed.From.Agent, To: handed.To.Agent, Note: note}
}

// chatCut is where turns of the conversation followed stand in the chat:
// those numbered from on stand add after their own numbers, behind the
// turns of the conversations before it and the rows of the chat's own.
type chatCut struct{ from, add int }

// chatPart is one stretch of a chat that went from agent to agent, in the
// order it is read: another conversation's turns read back, which stay as
// they are, or with own those of the conversation followed up to the one
// numbered until (0: all the rest); then the row of the chat's own that
// stands after the stretch, which with waits says a handing whose
// conversation has still to be sent.
type chatPart struct {
	turns []app.Turn
	own   bool
	until int
	then  *chatBand
	waits bool
}

// drawn reports whether the window reads a turn: one of a kind it knows,
// with what that kind must have. One it cannot read is left out of a view,
// and in a patch drops the whole patch.
func drawn(turn app.Turn) bool {
	switch turn.Kind {
	case "asked", "said", "tool", "question":
		return turn.Text != ""
	case "answer":
		return turn.Text != "" && turn.Question > 0
	case "commit":
		return turn.Hash != "" && turn.Subject != ""
	}
	return false
}

// worded is a turn as the window is told it: its words cut to what the
// window keeps of them, and by half again until the turn is no more than
// half a message, so that one turn always fits.
func worded(turn app.Turn) chatTurn {
	words := turn.Kind + "\x00" + strings.TrimSpace(turn.Text) + "\x00" + strings.Join(turn.Options, "\x00")
	if text := []rune(turn.Text); len(text) > muiProse {
		turn.Text = string(text[:muiProse])
	}
	for {
		data := encode(turn)
		text := []rune(turn.Text)
		if len(data) <= muiLimit/2 || len(text) < 2 {
			return chatTurn{turn.N, turn.Kind, words, data}
		}
		turn.Text = string(text[:len(text)/2])
	}
}

// lastThatFit is the last of the turns that a view with room bytes for
// them can say: no more than most of them.
func lastThatFit(turns []chatTurn, room, most int) []chatTurn {
	from := len(turns)
	for from > 0 && len(turns)-from < most && room > len(turns[from-1].json) {
		room -= len(turns[from-1].json) + 1 // and the comma between two
		from--
	}
	return turns[from:]
}

// patches is changes as patches, in order: each patch no more changes and
// no more bytes than the window takes.
func patches(pane string, changes []json.RawMessage) [][]byte {
	var all [][]byte
	var ops []json.RawMessage
	size := patchEnvelope
	flush := func() {
		if len(ops) > 0 {
			all = append(all, encode(muiMessage{V: 1, Pane: pane, Patch: ops}))
		}
		ops, size = nil, patchEnvelope
	}
	for _, op := range changes {
		if len(ops) == muiOps || size+len(op)+1 > muiLimit {
			flush()
		}
		ops, size = append(ops, op), size+len(op)+1
	}
	flush()
	return all
}

// chatLine is the quiet line over a chat: its words, a piece said again
// alone when they change, and at its right end what follows them.
func chatLine(words string, right ...json.RawMessage) json.RawMessage {
	left := []json.RawMessage{chatHead(words), encode(chatBox{T: "col", Grow: 1})}
	return encode(chatBox{T: "row", Pad: 8, Gap: 8, Children: append(left, right...)})
}

func chatHead(words string) json.RawMessage {
	return encode(chatWords{T: "text", ID: "about", Text: words, Style: "dim"})
}

// chatAbout is the line over a chat: the agent, the model and the effort
// it runs on where mia chose them, and the account it is on.
func chatAbout(runs app.Runs) string {
	said := []string{cmp.Or(runs.Agent, "no agent mia knows")}
	for _, one := range []string{runs.Model, runs.Effort} {
		if one != "" {
			said = append(said, one)
		}
	}
	if runs.Account != "" {
		said = append(said, "on "+runs.Account)
	}
	return strings.Join(said, " · ")
}

// chatRuns is what a window runs with the model its conversation says it
// runs on, where mia chose none for the window and the agent is the
// conversation's: opencode says a conversation's.
func chatRuns(runs app.Runs, head app.Head) app.Runs {
	if runs.Model == "" && runs.Agent == head.Agent {
		runs.Model = head.Model
	}
	return runs
}

// talk is a conversation as the window is shown it.
type talk struct {
	pane, agent    string
	about          string // the line over it
	hint           string // what its empty field says, "" for who answers
	reply, working bool
	// status is how the last thing done went, said at the right of the top
	// line; nil when there is nothing to say.
	status *muiStatus
	turns  []chatTurn   // every turn of the transcript the window can read, in order
	shown  map[int]bool // the turns the window was told; nil until the view was said
	// sent is what was sent from here that the transcript has not said yet,
	// the first sent first: each is said as an asked turn after the last.
	sent []chatSent
	// asks is what the window's screen asks that the transcript does not: a
	// permission, a folder to trust. It is said as a question after those,
	// at the place asksAt, 0 until it has one.
	asks   *app.Asks
	asksAt int
	// offers is the turn of the app's own under a reply that says the agent
	// hit its limit, to go on with another: of which reply, by its words,
	// at which place, and nil when that reply says no such thing.
	offerOf string
	offer   *chatBand
	// placed is how many places turns of the app's own took.
	placed int
	// own is those turns of the app's own that the window was told, by
	// their place.
	own      map[int]bool
	answered map[int]bool // the transcript's questions that were answered
	// again is the transcript's turns taken out of the window's chat to be
	// said again: a question whose answer was refused is open once it is.
	again []int
	// nth is which chat of its run this one is, which its id says: the chat
	// of another conversation is another piece, as the window keeps the
	// turns a chat was told.
	nth int
	// cuts is where each turn heard stands: after the turns of the
	// conversations this one goes on from, and the rows of the chat's own.
	// last is the highest number heard, and numbers the number each turn
	// heard has in its conversation, by its place.
	cuts    []chatCut
	last    int
	numbers map[int]int
	// waits is the switch whose conversation has still to be sent to the
	// agent, as it is said once it was: until then it is said as waiting.
	waits *chatBand
	// over is the modal that stands over the chat, nil for none, and keys
	// the piece of it the keys are put on. A view said whole is said
	// without it and it is put back by a patch: the chat's turns are those
	// that fit a message whatever stands over them.
	over json.RawMessage
	keys string
	// with is the change put back with the modal: the keys its foot says.
	with json.RawMessage
}

// chatSent is words sent from here, and the place their turn was said at.
type chatSent struct {
	n    int
	text string
}

// id is the chat piece's id.
func (t *talk) id() string {
	if t.nth == 0 {
		return "chat"
	}
	return "chat-" + strconv.Itoa(t.nth)
}

// find is where the transcript's turn n is among the turns, or would be.
func (t *talk) find(n int) (int, bool) {
	return slices.BinarySearchFunc(t.turns, n, func(have chatTurn, n int) int { return cmp.Compare(have.n, n) })
}

// at is the place of the turn of a number in the conversation followed.
func (t *talk) at(n int) int {
	for _, cut := range slices.Backward(t.cuts) {
		if n >= cut.from {
			return n + cut.add
		}
	}
	return n
}

// place is the next place for a turn of the app's own.
func (t *talk) place() int {
	t.placed++
	return chatOwn + t.placed
}

// extras is the app's own turns, each at a place of its own after the
// transcript's last: under a last reply that says the agent hit its limit,
// the offer to go on with another, until anything is sent or said after
// it; what was sent and is not in the transcript yet; then what the
// window's screen asks, unless the transcript ends on a question nobody
// answered — that one is the asking.
func (t *talk) extras() []chatTurn {
	open := false
	var own []chatTurn
	if len(t.turns) > 0 {
		last := t.turns[len(t.turns)-1]
		open = last.kind == "question" && !t.answered[last.n]
		if of := strconv.Itoa(last.n) + last.words; of != t.offerOf {
			t.offerOf, t.offer = of, nil
			if note, hit := limited(t.agent, strings.TrimPrefix(strings.TrimSuffix(last.words, "\x00"), "said\x00")); t.reply && last.kind == "said" && hit {
				t.offer = &chatBand{N: t.place(), Kind: "offer", Text: "Go on with another agent", Note: note, Key: "m", ID: "open-models"}
			}
		}
		if t.offer != nil && len(t.sent) == 0 {
			own = append(own, banded(*t.offer))
		}
	}
	for _, sent := range t.sent {
		own = append(own, worded(app.Turn{N: sent.n, Kind: "asked", Text: sent.text}))
	}
	if t.asks != nil && !open {
		if t.asksAt == 0 || len(t.sent) > 0 && t.asksAt < t.sent[len(t.sent)-1].n {
			t.asksAt = t.place() // after what was sent since it was first said
		}
		own = append(own, worded(app.Turn{N: t.asksAt, Kind: "question", Text: cmp.Or(t.asks.Text, t.agent+" asks"), Options: t.asks.Options}))
	}
	return own
}

// whole is the message that says the whole view: the line over the chat —
// who answers, and at its right the keys that show the models, fork the
// conversation from its last reply and show the earlier conversations,
// buttons that tab reaches from the chat's field, which types every
// letter — and the chat with the last turns that fit, each reply with the
// menu that copies it or forks the conversation from it.
// The keys are put on the chat's field, or on the modal's piece while one
// stands over it.
func (t *talk) whole() []byte {
	own := t.extras()
	chat := chatPiece{T: "chat", ID: t.id(), Agent: t.agent, Reply: t.reply, Working: t.working, Placeholder: t.hint, Turns: []json.RawMessage{}}
	var buttons []json.RawMessage
	if t.reply {
		chat.Menu = []chatItem{{ID: "copy", Title: "Copy"}, {ID: "fork", Title: "Continue from here with…"}}
		buttons = append(buttons, encode(chatButton{T: "button", ID: "open-models", Label: "models", Key: "m"}),
			encode(chatButton{T: "button", ID: "open-fork", Label: "fork", Key: "f"}))
	}
	buttons = append(buttons, encode(chatButton{T: "button", ID: "open-earlier", Label: "earlier", Key: "e"}))
	view := func() json.RawMessage {
		return encode(chatBox{T: "col", ID: "view", Children: []json.RawMessage{chatLine(t.about, buttons...), encode(chat)}})
	}
	message := chatMessage{V: 1, Pane: t.pane, Fill: true, Status: t.status}
	switch {
	case t.over != nil:
		message.Focus = t.keys
	case t.reply:
		message.Focus = chat.ID
	}
	room := muiLimit - len(encode(message)) - len(view())
	for _, turn := range own {
		room -= len(turn.json) + 1
	}
	t.shown, t.own, t.again = map[int]bool{}, map[int]bool{}, nil
	for _, turn := range lastThatFit(t.turns, room, chatTurns) {
		chat.Turns = append(chat.Turns, turn.json)
		t.shown[turn.n] = true
	}
	for _, turn := range own {
		chat.Turns = append(chat.Turns, turn.json)
		t.shown[turn.n], t.own[turn.n] = true, true
	}
	message.View = view()
	return encode(message)
}

// said is the messages that say the view whole: the view, and after it
// the modal that stands over it, put back.
func (t *talk) said() [][]byte {
	all := [][]byte{t.whole()}
	if t.over != nil {
		changes := []json.RawMessage{encode(chatOp{Op: "add", ID: "view", Node: t.over})}
		if t.with != nil {
			changes = append(changes, t.with)
		}
		all = append(all, t.patch(changes...)...)
	}
	return all
}

// patch is changes as the messages that make them, once the view was
// said: a patch to no view is dropped.
func (t *talk) patch(changes ...json.RawMessage) [][]byte {
	if t.shown == nil {
		return nil
	}
	return patches(t.pane, changes)
}

// stands takes how things stand, nil when there is nothing to say, and is
// the change that says it alone: a view said whole carries it too.
func (t *talk) stands(status *muiStatus) json.RawMessage {
	if t.status = status; status == nil {
		return encode(chatOp{Op: "status"})
	}
	return encode(chatOp{Op: "status", Text: status.Text, Tone: status.Tone})
}

// went is the change that says how a thing done went: well, a toast, which
// goes by itself; otherwise the status, which stays until the next thing
// done.
func (t *talk) went(text, tone string) json.RawMessage {
	if tone == "ok" {
		return encode(chatOp{Op: "toast", Text: cutTo(text, chatSaid), Tone: tone})
	}
	return t.stands(&muiStatus{Text: cutTo(text, chatSaid), Tone: tone})
}

// says is the messages that make the window's chat what it now is, once
// the view was said: changed are the transcript's turns that came or grew,
// and after is what else changes with them. It is one patch: the turns of
// the app's own that no longer stand taken out by their places, then the
// turns put in. The whole view is said when it would hold more turns than
// the window takes.
func (t *talk) says(changed []chatTurn, after ...json.RawMessage) [][]byte {
	if t.shown == nil {
		return nil
	}
	stands := map[int]bool{}
	for _, turn := range t.extras() {
		if stands[turn.n] = true; !t.own[turn.n] {
			changed = append(changed, turn)
		}
	}
	for _, n := range t.again {
		if at, found := t.find(n); found {
			changed = append(changed, t.turns[at])
		}
	}
	t.again = nil
	more := 0
	for _, turn := range changed {
		if !t.shown[turn.n] {
			more++
		}
	}
	if len(t.shown)+more > muiPieces-chatFrame {
		return append(t.said(), t.patch(after...)...)
	}
	var ops []json.RawMessage
	for _, n := range slices.Sorted(maps.Keys(t.own)) {
		if !stands[n] {
			ops = append(ops, encode(chatOp{Op: "unturn", ID: t.id(), N: n}))
			delete(t.shown, n)
		}
	}
	for _, turn := range changed {
		t.shown[turn.n] = true
		ops = append(ops, encode(chatOp{Op: "turn", ID: t.id(), Turn: turn.json}))
	}
	t.own = stands
	return patches(t.pane, append(ops, after...))
}

// heard takes the turns a followed conversation gained or that grew, and
// is the messages that say them: the whole view when it was not said yet.
// A turn that says what was sent from here is put in as the one said for
// it at once is taken out; when another comes first — the agent was at
// work, and takes up what was sent later — that one is taken out too, and
// the status says it waits.
func (t *talk) heard(turns []app.Turn) [][]byte {
	var changed []chatTurn
	passed := false // a turn came that is none of what was sent from here
	for _, turn := range turns {
		t.last = max(t.last, turn.N)
		if !drawn(turn) {
			continue
		}
		number := turn.N
		if turn.N = t.at(turn.N); turn.Question > 0 {
			turn.Question = t.at(turn.Question)
		}
		if t.numbers == nil {
			t.numbers = map[int]int{}
		}
		t.numbers[turn.N] = number
		if turn.Kind == "asked" {
			turn.Text = ownWords(turn.Text) // not the conversation said in front of them
		}
		if t.answered == nil {
			t.answered = map[int]bool{}
		}
		switch {
		case turn.Kind == "answer":
			t.answered[turn.Question] = true
		case turn.Kind == "question" && turn.Answer != "":
			t.answered[turn.N] = true
		}
		one := worded(turn)
		if at, found := t.find(one.n); found {
			t.turns[at] = one
		} else {
			t.turns = slices.Insert(t.turns, at, one)
			echo := slices.IndexFunc(t.sent, func(sent chatSent) bool { return sent.text == strings.TrimSpace(turn.Text) })
			if turn.Kind == "asked" && echo >= 0 {
				t.sent = slices.Delete(t.sent, echo, echo+1)
			} else {
				passed = true
			}
		}
		changed = append(changed, one)
	}
	var after []json.RawMessage
	if t.waits != nil && slices.ContainsFunc(changed, func(turn chatTurn) bool { return turn.kind == "asked" && turn.n > t.waits.N }) {
		after = t.handed() // the agent was sent something: another chat of its window sent what it goes on from
	}
	if passed && len(t.sent) > 0 {
		waits := ""
		for _, sent := range t.sent {
			waits += " · " + sent.text
		}
		after = append(after, t.stands(&muiStatus{Text: cutTo(t.agent+" takes it up when it can — sent: "+strings.TrimPrefix(waits, " · "), chatSaid)}))
		t.sent = nil
	}
	if t.shown == nil {
		return t.said()
	}
	return t.says(changed, after...)
}

// replied is the reply at a place, the last one for place 0: its words,
// and its number in the conversation followed, 0 for one said in a
// conversation this one goes on from.
func (t *talk) replied(place int) (number int, words string, found bool) {
	for _, turn := range slices.Backward(t.turns) {
		if turn.kind == "said" && (place == 0 && t.numbers[turn.n] > 0 || turn.n == place) {
			return t.numbers[turn.n], strings.TrimSuffix(strings.TrimPrefix(turn.words, "said\x00"), "\x00"), true
		}
	}
	return 0, "", false
}

// lays makes the chat one that went from agent to agent, of its stretches
// in order: the turns read back stay, each stretch after the one before; a
// row of the chat's own stands after its stretch, with a place before its
// own kept for it as it is said while it waits; and what is heard of the
// conversation followed stands where its stretches are.
func (t *talk) lays(parts []chatPart) {
	t.answered = map[int]bool{}
	place, next := 0, 1 // the last place taken, and the first number of the conversation followed not laid yet
	for _, part := range parts {
		switch {
		case part.own:
			t.cuts = append(t.cuts, chatCut{next, place - next + 1})
			if part.until >= next {
				place, next = place+part.until-next+1, part.until+1
			}
		case len(part.turns) > 0:
			read := &talk{cuts: []chatCut{{0, place - part.turns[0].N + 1}}}
			read.heard(part.turns) // (their numbers are another conversation's: not kept)
			t.turns = append(t.turns, read.turns...)
			maps.Copy(t.answered, read.answered)
			place = read.at(part.turns[len(part.turns)-1].N)
		}
		if part.then == nil {
			continue
		}
		band := *part.then
		band.N, place = place+2, place+2
		if part.waits {
			t.waits = &band
			t.turns = append(t.turns, banded(waiting(band)))
			continue
		}
		t.turns = append(t.turns, banded(band))
	}
}

// handed takes that what the agent goes on from was sent to it, and is the
// changes that say so: the switch that waited taken out, and what it says
// was handed over put in at its own place.
func (t *talk) handed() []json.RawMessage {
	if t.waits == nil {
		return nil
	}
	now, was := *t.waits, waiting(*t.waits)
	t.waits = nil
	if at, found := t.find(was.N); found {
		t.turns[at] = banded(now)
	}
	if !t.shown[was.N] {
		return nil
	}
	delete(t.shown, was.N)
	t.shown[now.N] = true
	return []json.RawMessage{encode(chatOp{Op: "unturn", ID: t.id(), N: was.N}), encode(chatOp{Op: "turn", ID: t.id(), Turn: banded(now).json})}
}

// put stands a row of the chat's own after the last turn heard, and past
// as many turns still to come — the one that says what was just sent — and
// is the row as a turn: what is heard after it stands after it.
func (t *talk) put(band chatBand, past int) chatTurn {
	next := t.last + 1 + past
	band.N = t.at(next)
	t.cuts = append(t.cuts, chatCut{next, band.N - next + 1})
	row := banded(band)
	at, _ := t.find(row.n)
	t.turns = slices.Insert(t.turns, at, row)
	return row
}

// chatRule is the line between a conversation said in front of what the
// person sends and their own words.
const chatRule = app.CarriedRule

// carried is the first thing sent to an agent that goes on with another's
// conversation, as one message: that conversation as mia wrote it out,
// who said what, then a rule, then the person's words — so the agent
// answers with it in hand, and is told nothing else. Of one longer than
// most bytes only its last turns that fit are said, under a line saying
// where the whole of it is.
func carried(from, transcript, file, words string, most int) string {
	if len(transcript) > most {
		turns := strings.Split(transcript, "\n### ")
		kept, room := len(turns), most
		for kept > 1 && len(turns[kept-1]) < room {
			kept--
			room -= len(turns[kept]) + len("\n### ")
		}
		transcript = "[Only its last turns are here: the whole of it is in " + file + "]\n"
		for _, turn := range turns[kept:] {
			transcript += "\n### " + turn
		}
	}
	return app.CarriedOpens(from) + "\n\n" + strings.TrimSpace(transcript) + "\n\n" + chatRule + "\n\n" + words
}

// ownWords is what the person themselves sent, of a message that may have
// a conversation said in front of it: what follows the rule.
func ownWords(text string) string { return app.OwnWords(text) }

// atWork takes whether the agent is at work, and is the patch that says
// so when it changed and the view was said.
func (t *talk) atWork(on bool) [][]byte {
	if on == t.working {
		return nil
	}
	t.working = on
	return t.patch(encode(chatOp{Op: "working", ID: t.id(), On: &on}))
}

// sends takes what the person sent, and is the patch that says it at once
// as an asked turn after the last: it is taken out as the transcript's own
// turn is put in, when the agent writes it.
func (t *talk) sends(text string) [][]byte {
	t.sent = append(t.sent, chatSent{t.place(), text})
	return t.says(nil)
}

// refused takes that the last thing sent did not go, and is the patch
// that takes its turn out, gives its words back to the chat's field, and
// says why as the status.
func (t *talk) refused(why error) [][]byte {
	if len(t.sent) == 0 {
		return nil
	}
	words := []rune(t.sent[len(t.sent)-1].text)
	t.sent = t.sent[:len(t.sent)-1]
	return t.says(nil, encode(chatOp{Op: "draft", ID: t.id(), Text: string(words[:min(len(words), chatDraft)])}),
		t.stands(&muiStatus{Text: cutTo(why.Error(), chatSaid), Tone: "bad"}))
}

// drafted is the change that puts words back in the chat's field: what was
// sent from it and did not go.
func (t *talk) drafted(text string) json.RawMessage {
	words := []rune(text)
	return encode(chatOp{Op: "draft", ID: t.id(), Text: string(words[:min(len(words), chatDraft)])})
}

// asked takes what the window's screen asks now, nil when nothing, and is
// the patch that says it as a question of the app's own after the last
// turn, or takes that question out once it is answered. One asked
// otherwise is another question, at another place.
func (t *talk) asked(asks *app.Asks) [][]byte {
	same := func(a, b *app.Asks) bool {
		return a == nil && b == nil || a != nil && b != nil && a.Text == b.Text && slices.Equal(a.Options, b.Options)
	}
	if !same(t.asks, asks) {
		t.asks, t.asksAt = asks, 0
	} else if len(t.again) == 0 {
		return nil
	}
	return t.says(nil)
}

// reopened takes that the answer chosen to a question was refused by the
// agent's window, and is the patch that says why as the status and opens
// the question again: the window writes an answer under a question as it
// is chosen, and keeps it while it keeps the turn. One of the app's own
// is said at another place; the transcript's is taken out, and said again
// with what is said next.
func (t *talk) reopened(question int, why error) [][]byte {
	status := t.stands(&muiStatus{Text: cutTo(why.Error(), chatSaid), Tone: "bad"})
	if question == t.asksAt {
		t.asksAt = 0
		return t.says(nil, status)
	}
	if !t.shown[question] {
		return t.patch(status)
	}
	delete(t.shown, question)
	t.again = append(t.again, question)
	return t.patch(encode(chatOp{Op: "unturn", ID: t.id(), N: question}), status)
}

// chatMode is what stands over the chat, in a modal: nothing, the models
// to pick from, the worktree's earlier conversations, or what sending to
// several agents makes, to be confirmed.
type chatMode int

const (
	chatting chatMode = iota
	picking
	looking
	confirming
)

// chatView is the chat as the person has it, and where they are in it.
type chatView struct {
	talk     *talk
	mode     chatMode
	worktree string
	// window is the window typed into, by its number; "" when none has the
	// conversation open. runs is what it runs.
	window string
	runs   app.Runs
	// id is the conversation followed, "" while the window has none mia
	// can read.
	id string

	// The models: every agent's, the setups used in this repository, and
	// what is typed to find one by.
	agents []app.AgentModels
	setups []app.Setup
	query  string
	// under is the row of them the keys are on, whole whether the whole
	// conversation is carried to another agent and not its latest turns,
	// would what handing it to each agent looked at would do, tabbed the
	// button tab presses there, which the foot says, and came the agents
	// the conversation was with before this one.
	under  string
	whole  bool
	would  map[string]chatWould
	tabbed string
	came   []string
	// fork is the reply the models were put up to fork the conversation
	// from, by its number in the conversation, 0 when they were not: a pick
	// then starts its agent in a window of its own with the conversation up
	// to that reply, and this chat stays as it is.
	fork int
	// next is the model or the agent picked while the agent was at work: it
	// is switched or handed to when the turn ends, and until then the line
	// over the chat says it. It is this chat's alone, kept while it runs.
	next *chatDeed
	// marks is the rows of the models marked while they are up, to send the
	// next message to each in a worktree of its own besides this chat's
	// agent: a fan-out. fans is those kept as the models were shut with ⏎,
	// until the message is sent or they are cleared, and plan what sending
	// it makes, while that is put up to be confirmed.
	marks []chatModel
	fans  []chatModel
	plan  *chatPlan

	// Earlier: the worktree's conversations, the one under the keys with
	// its turns read back, and the one c was pressed on once.
	heads []app.Head
	on    string
	read  []chatTurn
	armed string
	now   time.Time

	// told is what the modal says at its foot of the last thing asked in
	// it: what c does, why a pick was not done.
	told *muiStatus
}

// chatWould is what handing the conversation to an agent would do, or why
// it would not be done.
type chatWould struct {
	done app.Continued
	err  error
}

// chatMake is one agent of a fan-out: the row marked for it, and the
// worktree and the branch made for it.
type chatMake struct {
	row          chatModel
	name, branch string
}

// chatPlan is a fan-out about to be done: the words sent, what is made for
// each agent besides this chat's, and whether this worktree has changes no
// commit holds, which none of those made gets.
type chatPlan struct {
	text  string
	makes []chatMake
	dirty bool
}

// carrying is where what a window's agent goes on from is kept until it
// is said: on the window, so that it is whoever chats with it that has it.
type carrying interface {
	TakeCarried(record model.Record, window string) (app.Carried, bool)
	CarryInWindow(record model.Record, window string, carried app.Carried) error
}

// typedFor is what is typed into a window for words the person sent: the
// words, and in front of the first after a conversation was handed to its
// agent, that conversation, read from the file mia wrote it out to. It is
// taken off the window as it is typed, so it is said once whichever chat
// of the window sends first; back keeps it there again, for a send that
// did not go.
func typedFor(kept carrying, record model.Record, window, words string, read func(file string) ([]byte, error)) (text string, back func()) {
	was, on := kept.TakeCarried(record, window)
	if !on {
		return words, func() {}
	}
	back = func() { _ = kept.CarryInWindow(record, window, was) } // not kept: the next send is the words alone
	transcript, err := read(was.File)
	if err != nil {
		return "Read " + was.File + ": the conversation so far, with " + was.From + ".\n\n" + chatRule + "\n\n" + words, back
	}
	return carried(was.From, string(transcript), cmp.Or(was.Full, was.File), words, chatCarry), back
}

// agent is the agent that answers in this chat.
func (v *chatView) agent() string { return cmp.Or(v.runs.Agent, v.talk.agent) }

// coming is a pick as the line over the chat says it while it is to come:
// a model and an effort of this chat's agent, or another agent and its.
func (d chatDeed) coming() string {
	said := []string{}
	for _, one := range []string{d.agent, d.model, d.effort} {
		if one != "" {
			said = append(said, one)
		}
	}
	if d.do == chatSwitch {
		return strings.Join(said, " ")
	}
	return strings.Join(said, " · ")
}

// lined makes the line over the chat what it now is, who answers and what
// was picked to come next, and is the change that says it.
func (v *chatView) lined() json.RawMessage {
	v.talk.about = chatAbout(v.runs)
	if v.next != nil {
		v.talk.about += " · next: " + v.next.coming()
	}
	if len(v.fans) > 0 {
		var to []string
		for _, row := range v.fans {
			to = append(to, chatAbout(app.Runs{Agent: row.agent, Model: row.model}))
		}
		v.talk.about += " · fan-out: " + strings.Join(to, ", ")
	}
	return encode(chatOp{Op: "set", ID: "about", Node: chatHead(v.talk.about)})
}

// fanned keeps the rows the next message also goes to, none to let go of
// them, and is the view said again whole: the line over the chat names
// them, and the chat's field says how many agents hear what is typed.
func (v *chatView) fanned(rows []chatModel) [][]byte {
	v.fans, v.talk.hint = rows, ""
	if len(rows) > 0 {
		v.talk.hint = fmt.Sprintf("Say something to %d agents", len(rows)+1)
	}
	v.lined()
	return v.talk.said()
}

// here reports whether a row of the models is what this chat runs on.
func (v *chatView) here(row chatModel) bool {
	return row.agent == v.agent() && row.model == v.runs.Model && row.model != "" && (!row.used || row.effort == v.runs.Effort)
}

// marked is where a row is among those marked, -1 when it is not.
func (v *chatView) marked(row chatModel) int {
	return slices.IndexFunc(v.marks, func(one chatModel) bool { return one.id() == row.id() })
}

// chatIcon is an agent's icon in the window's own set.
func chatIcon(agent string) string {
	if agent == "cursor-agent" {
		return "cursor"
	}
	return agent
}

// effortWord is an effort as a row says it: "High"; none, "Default".
func effortWord(effort string) string {
	if effort == "" {
		return "Default"
	}
	return strings.ToUpper(effort[:1]) + effort[1:]
}

// chatModel is a row of the models: an agent's model, and with used the
// effort a setup of this repository ran it at.
type chatModel struct {
	used                        bool
	agent, model, label, effort string
}

// id is the row as its list knows it; chatModelOf reads it back. A model's
// id is last: it is the one that may hold any character.
func (m chatModel) id() string {
	if m.used {
		return "u|" + m.agent + "|" + m.effort + "|" + m.model
	}
	return "a|" + m.agent + "|" + m.model
}

func chatModelOf(id string) (chatModel, bool) {
	switch parts := strings.SplitN(id, "|", 4); {
	case len(parts) == 4 && parts[0] == "u":
		return chatModel{used: true, agent: parts[1], effort: parts[2], model: parts[3]}, true
	case len(parts) >= 3 && parts[0] == "a":
		return chatModel{agent: parts[1], model: strings.Join(parts[2:], "|")}, true
	}
	return chatModel{}, false
}

// modelled is the line that says a window's agent went from what it ran on
// to what it runs on now: its models, or with one model its efforts; false
// when neither changed.
func modelled(was, now app.Runs) (chatBand, bool) {
	from, to := cmp.Or(was.Model, "default"), cmp.Or(now.Model, "default")
	if from == to {
		from, to = strings.TrimSpace(from+" "+was.Effort), strings.TrimSpace(to+" "+now.Effort)
	}
	return chatBand{Kind: "model", From: from, To: to}, from != to
}

// chatModels is the rows of the models: the setups used in this
// repository first, then the models of the agent that answers and of each
// agent installed, an agent with none once; with a query, those whose
// model or agent holds it, whatever the case.
func chatModels(agents []app.AgentModels, setups []app.Setup, here, query string) []chatModel {
	label := func(agent, model string) string {
		for _, one := range agents {
			for _, m := range one.Models {
				if one.Name == agent && m.ID == model {
					return m.Label
				}
			}
		}
		return cmp.Or(model, agent)
	}
	var rows []chatModel
	for _, setup := range setups {
		known := slices.ContainsFunc(agents, func(one app.AgentModels) bool { return one.Name == setup.Agent })
		if setup.Role == "" && known {
			rows = append(rows, chatModel{used: true, agent: setup.Agent, model: setup.Model, label: label(setup.Agent, setup.Model), effort: setup.Effort})
		}
	}
	for _, agent := range agents {
		if !agent.Installed && agent.Name != here {
			continue
		}
		for _, m := range agent.Models {
			rows = append(rows, chatModel{agent: agent.Name, model: m.ID, label: m.Label})
		}
		if len(agent.Models) == 0 {
			rows = append(rows, chatModel{agent: agent.Name, label: agent.Name})
		}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	return slices.DeleteFunc(rows, func(m chatModel) bool {
		return !strings.Contains(strings.ToLower(m.label+" "+m.model+" "+m.agent), query)
	})
}

// models is the modal that says the models over the chat: a bare search
// line over one list in a room of its own, "Used here" first and then each
// agent's under its head, the one that runs checked, an item's menu its
// agent's efforts. placed puts the keys' row on the one that runs.
func (v *chatView) models(placed bool) json.RawMessage {
	rows := chatModels(v.agents, v.setups, v.agent(), v.query)
	list := chatList{T: "list", ID: "models", Live: true, Max: 11, Menu: "effort", Items: []chatItem{}}
	ids := []string{}
	group, held := "", 0
	for _, row := range rows {
		var agent app.AgentModels
		for _, one := range v.agents {
			if one.Name == row.agent {
				agent = one
			}
		}
		if held += 2 + len(agent.Efforts); held > chatHeads+chatReadMost {
			break // no more than a modal holds: what is typed finds the rest
		}
		mine := row.agent == v.agent()
		// what it would run at: the setup's, what this chat runs at, or what the config starts it with
		effort := cmp.Or(row.effort, agent.Effort)
		if mine && !row.used {
			effort = cmp.Or(v.runs.Effort, agent.Effort)
		}
		head := chatItem{Head: strings.ToUpper(row.agent[:1]) + row.agent[1:]}
		if mine {
			head.Tag = "this chat"
			if v.runs.Account != "" {
				head.Note = "on " + v.runs.Account
			}
		} else if slices.Contains(v.came, row.agent) {
			head.Tag = "was in this chat"
		}
		if row.used {
			head = chatItem{Head: "Used here"}
		}
		if head.Head != group {
			group = head.Head
			list.Items = append(list.Items, head)
		}
		item := chatItem{ID: row.id(), Title: row.label, Icon: chatIcon(row.agent), Does: "go on in " + row.agent}
		item.Checked = v.here(row) || v.marked(row) >= 0
		switch {
		case !mine:
			item.About = []string{"goes on in " + row.agent + " with what was said and each thing done, not what " + v.agent() + " thought or its tools gave back",
				v.agent() + "'s window is closed once " + row.agent + " runs"}
		case row.model == "" && !row.used:
			item.Does = "nothing"
			item.About = []string{row.agent, "mia starts it with no model: pick one in its terminal"}
		case agent.Live:
			item.Does = "switch"
			item.About = []string{chatAbout(app.Runs{Agent: row.agent, Account: v.runs.Account}), "switches this window now"}
		case agent.Again:
			item.Does = "switch"
			item.About = []string{chatAbout(app.Runs{Agent: row.agent, Account: v.runs.Account}), "starts it again in this window, on this conversation"}
		default:
			item.Does = "nothing"
			item.About = []string{row.agent, "cannot be switched while it runs"}
		}
		if v.fork > 0 {
			item.Does = "continue in " + row.agent
			item.About = []string{"starts " + row.agent + " in a window of its own, with the conversation up to that reply", "this chat stays as it is, and " + v.agent() + " with it"}
		}
		if row.model == "" && !row.used {
			item.Note = "its model is chosen inside it"
		}
		// one named by what comes after its provider says whose it is
		if provider, name, ok := strings.Cut(row.model, "/"); ok && name == row.label {
			item.Note = provider
		}
		if row.used {
			item.Note, item.Right = row.agent, effortWord(row.effort)
		}
		if len(agent.Efforts) > 0 {
			item.More = effortWord(effort)
		}
		for _, one := range agent.Efforts {
			item.Menu = append(item.Menu, chatItem{ID: one, Title: effortWord(one), Checked: one == effort})
		}
		// while the agent works: ⏎ keeps a row as what comes next, and lets go of the one that is
		switch pick := v.deed(chatEvent{E: "pick", List: "models", ID: item.ID}); {
		case pick.later:
			item.Does = "keep as next"
		case pick.do == chatForget:
			item.Does, item.Right = "clear", strings.TrimSuffix("next · "+item.Right, " · ")
			if placed && list.Selected == "" {
				list.Selected = item.ID // the keys come to what is to come, not to what runs
			}
		}
		if len(v.marks) > 0 {
			item.Does = "keep them"
		}
		if placed && v.here(row) && list.Selected == "" {
			list.Selected = item.ID
		}
		list.Items = append(list.Items, item)
		ids = append(ids, item.ID)
	}
	// the row the keys are on, as the window has it: the one placed or moved to while it is there, else the first
	switch {
	case list.Selected != "":
		v.under = list.Selected
	case !slices.Contains(ids, v.under):
		v.under = slices.Concat(ids, []string{""})[0]
	}
	field := chatField{T: "field", ID: "q", Placeholder: "Search models", Icon: "search", Right: counted(len(rows), "model"), Bare: true, Live: true, List: "models"}
	does, _ := v.does()
	inside := []json.RawMessage{encode(field), encode(chatBox{T: "divider"}), does, encode(chatBox{T: "divider"}), encode(list)}
	title := "Model"
	if v.fork > 0 {
		title = "Continue from reply " + strconv.Itoa(v.fork)
	} else {
		// (the one key the search line lets through that types nothing)
		inside = append(inside, encode(chatBox{T: "divider"}), encode(chatButton{T: "button", ID: "mark", Label: "Mark, to send your next message to several agents", Key: "⇧⏎", Quiet: true}))
	}
	return encode(chatModal{T: "modal", ID: chatOver, Title: title, W: 520, Children: append(inside, v.foot()...)})
}

// does is the line under the models' search line, which says what picking
// the row under the keys does, and whether it holds a choice. A model of
// this chat's agent: what it goes from and to, in place. Another agent's:
// who to whom, and what is carried, the conversation's latest turns or the
// whole of it, as tab chooses, with how many turns that is under it. An
// agent the conversation came from: that it takes its own up again, and
// how many turns were said since. What is not done says why. While the
// agent is at work a pick is kept as next, and the line says so; on the
// row that is next it says when it is applied, with the key that stops
// the agent and applies it now. Put up to fork the conversation, it says
// who goes on from which reply with what, in a window of its own. With
// rows marked it says how many agents the next message is sent to, this
// chat's here and each of the others in a worktree of its own. tab is the
// button its key presses in it: tab, or esc on the one that clears the
// marks.
func (v *chatView) does() (line json.RawMessage, tab string) {
	said := func(words, style string) json.RawMessage {
		return encode(chatWords{T: "text", Text: words, Style: style})
	}
	row := func(pieces ...json.RawMessage) json.RawMessage {
		return encode(chatBox{T: "row", Gap: 8, Children: pieces})
	}
	var lines []json.RawMessage
	if len(v.marks) > 0 {
		// rows are marked: who the next message goes to, and the key that clears them
		var to []string
		for _, row := range v.marks {
			to = append(to, row.agent)
		}
		each := to[0] + " in a worktree of its own"
		if len(to) > 1 {
			each = strings.Join(to[:len(to)-1], ", ") + " and " + to[len(to)-1] + " each in a worktree of their own"
		}
		lines = append(lines, row(said(fmt.Sprintf("Sends your next message to %d agents", len(to)+1), "strong"), encode(chatBox{T: "col", Grow: 1}), encode(chatButton{T: "button", ID: "unmark", Key: "esc"})),
			said(v.agent()+" here · "+each, "dim"))
		return encode(chatBox{T: "col", ID: "does", Gap: 4, Pad: 4, Children: lines}), "unmark"
	}
	deed := v.deed(chatEvent{E: "pick", List: "models", ID: v.under})
	if deed.do == chatForget {
		// the row picked to come next: when it is done, the key that does it now, and how it is let go of
		next := *v.next
		lead, carries := v.agent()+" → "+next.agent, "Carries the conversation's latest turns. "
		if next.whole {
			carries = "Carries the whole conversation. "
		}
		if next.do == chatSwitch {
			band, _ := modelled(v.runs, app.Runs{Model: cmp.Or(next.model, v.runs.Model), Effort: cmp.Or(next.effort, v.runs.Effort)})
			lead, carries = band.From+" → "+band.To, ""
		}
		lines = append(lines, row(said(lead, "strong"), said("· Applied when "+v.agent()+" finishes this turn.", "dim"), encode(chatBox{T: "col", Grow: 1}), encode(chatButton{T: "button", ID: "now", Key: "tab"})),
			said(carries+"tab stops "+v.agent()+" and applies it now. ⏎ again clears it.", "dim"))
		return encode(chatBox{T: "col", ID: "does", Gap: 4, Pad: 4, Children: lines}), "now"
	}
	switch deed.do {
	case chatHand, chatFork:
		lead := said(v.agent()+" → "+deed.agent, "strong")
		would, looked := v.would[deed.agent]
		turns, kept := would.done.Turns, would.done.Turns-would.done.LeftOut
		// where it goes: with the person's next message here, or of a fork in the window it opens
		sent := "Sent with your next message."
		if deed.do == chatFork {
			sent = "In a new window; this chat stays." // (which reply, the modal's title says)
		}
		switch {
		case !looked:
			lines = append(lines, row(lead))
		case would.err != nil:
			lines = append(lines, row(lead, said("· "+would.err.Error(), "dim")))
		case would.done.Resumes != "":
			since := ""
			if turns > 0 {
				since = " · " + counted(turns, "turn") + " since"
			}
			lines = append(lines, row(lead, said("· "+deed.agent+" takes its own conversation up again"+since, "dim")))
		default:
			tab = "flip"
			tabs := chatTabs{T: "tabs", ID: "carry", Boxed: true, Selected: "latest", Items: []chatItem{{ID: "latest", Title: "Latest"}, {ID: "whole", Title: "Whole conversation"}}}
			under := fmt.Sprintf("The last %d of %d turns, as many as fit. %s", kept, turns, sent)
			switch {
			case v.whole && deed.do == chatFork:
				tabs.Selected, under = "whole", "All "+counted(turns, "turn")+". "+sent
			case v.whole:
				tabs.Selected, under = "whole", "All "+counted(turns, "turn")+". Slower, and uses more of your plan."
			case kept == turns:
				under = "All " + counted(turns, "turn") + ": they fit. " + sent
			}
			lines = append(lines, row(lead, said("carries", "dim"), encode(tabs), encode(chatBox{T: "col", Grow: 1}), encode(chatButton{T: "button", ID: "flip", Key: "tab"})), said(under, "dim"))
		}
	case chatSwitch:
		now := app.Runs{Model: cmp.Or(deed.model, v.runs.Model), Effort: cmp.Or(deed.effort, v.runs.Effort)}
		how := "· Switches in place. Nothing is re-sent."
		if at := slices.IndexFunc(v.agents, func(one app.AgentModels) bool { return one.Name == v.agent() }); at >= 0 && !v.agents[at].Live {
			how = "· Starts again in this window, on this conversation. Nothing is re-sent."
		}
		if band, changes := modelled(v.runs, now); changes {
			lines = append(lines, row(said(band.From+" → "+band.To, "strong"), said(how, "dim")))
		} else {
			lines = append(lines, row(said(cmp.Or(now.Model, "default"), "strong"), said("· What this chat runs on.", "dim")))
		}
	default:
		lines = append(lines, said(deed.text, "dim"))
	}
	if deed.later {
		lines = append(lines, said("⏎ keeps it as next: applied when "+v.agent()+" finishes this turn.", "dim"))
	}
	return encode(chatBox{T: "col", ID: "does", Gap: 4, Pad: 4, Children: lines}), tab
}

// footed is the change that says the keys along the foot, when they are
// others than were said: tab, while the line over the models holds its
// choice or the key that applies now what is next, esc while it says rows
// are marked, and none otherwise. It is put back with the modal too.
func (v *chatView) footed() []json.RawMessage {
	tab := ""
	if v.mode == picking {
		_, tab = v.does()
	}
	if tab == v.tabbed {
		return nil
	}
	keys := []chatKey{}
	if v.tabbed, v.talk.with = tab, nil; tab != "" {
		key := map[string]string{"unmark": "esc"}[tab]
		keys = append(keys, chatKey{Key: cmp.Or(key, "tab"), Label: map[string]string{"flip": "latest or whole", "now": "apply now", "unmark": "clear marks"}[tab], ID: tab})
		v.talk.with = encode(chatOp{Op: "keys", Keys: &keys})
	}
	return []json.RawMessage{encode(chatOp{Op: "keys", Keys: &keys})}
}

// foot is what a modal says at its foot: nothing, until something asked in
// it was not done at once.
func (v *chatView) foot() []json.RawMessage {
	if v.told == nil {
		return nil
	}
	return []json.RawMessage{encode(chatWords{T: "text", Text: v.told.Text, Style: "dim", Tone: v.told.Tone})}
}

// headOf is the conversation of an id among those Earlier lists.
func (v *chatView) headOf(id string) (app.Head, bool) {
	at := slices.IndexFunc(v.heads, func(h app.Head) bool { return h.ID == id })
	if at < 0 {
		return app.Head{}, false
	}
	return v.heads[at], true
}

// earlier is the modal that says the worktree's conversations over the
// chat: a list of them — what was first asked, the agent and when at the
// right, the one a window has open marked as running — and beside it the
// one under the keys read back, a chat with no field; the two split the
// modal's height, each from its top. An item's menu is what can be done to it: taken up again or gone
// to, handed to this chat's agent when it is another's, as c does, and its
// id copied. placed puts the keys' row on the conversation read back.
func (v *chatView) earlier(placed bool) json.RawMessage {
	list := chatList{T: "list", ID: "earlier", Live: true, Flat: true, Max: 12, Menu: "more", Items: []chatItem{}}
	if placed {
		list.Selected = v.on
	}
	room := muiLimit * 3 / 4 // the list's, of a message: the rest is the turns read back
	for _, head := range v.heads {
		running := head.Running && head.Window != nil
		item := chatItem{ID: head.ID, Title: cmp.Or(cutTo(head.Asked, chatSaid), "nothing asked yet"), Icon: chatIcon(head.Agent),
			Right: head.Agent + " · " + gitAge(v.now.Sub(head.Active)), Running: &running, Does: "go on with it",
			Menu: []chatItem{{ID: "resume", Title: "Resume"}}}
		if running {
			item.Does, item.Menu[0].Title = "back to it", "Go to it"
		}
		if head.Agent != v.agent() {
			item.Keys = []chatKey{{Key: "c", Label: "continue", ID: "continue"}}
			item.Menu = append(item.Menu, chatItem{ID: "continue", Title: "Continue with " + v.agent()})
		}
		item.Menu = append(item.Menu, chatItem{ID: "copy", Title: "Copy its id"})
		if room -= len(encode(item)) + 1; room < 0 {
			break
		}
		list.Items = append(list.Items, item)
	}
	reading, _ := v.headOf(v.on)
	read := chatPiece{T: "chat", ID: cutTo("read-"+v.on, chatSaid), Agent: reading.Agent, Turns: []json.RawMessage{}}
	modal := func() json.RawMessage {
		inside := []json.RawMessage{
			encode(chatWords{T: "text", Text: "in " + v.worktree + " · " + counted(len(v.heads), "conversation"), Style: "dim"}),
			encode(chatBox{T: "row", Split: true, Gap: 16, Children: []json.RawMessage{
				encode(chatBox{T: "col", W: 300, Children: []json.RawMessage{encode(list)}}),
				encode(chatBox{T: "col", Children: []json.RawMessage{encode(read)}}),
			}}),
		}
		// (Of a height: its sides are split in a room, each from the top.)
		return encode(chatModal{T: "modal", ID: chatOver, Title: "Earlier", W: 720, H: chatRoom, Children: append(inside, v.foot()...)})
	}
	for _, turn := range lastThatFit(v.read, muiLimit-2*patchEnvelope-len(modal()), chatReadMost) {
		read.Turns = append(read.Turns, turn.json)
	}
	return modal()
}

// modal is the modal of the mode.
func (v *chatView) modal(placed bool) json.RawMessage {
	switch v.mode {
	case picking:
		return v.models(placed)
	case confirming:
		return v.confirm()
	}
	return v.earlier(placed)
}

// confirm is the modal put up before a fan-out makes anything: a row for
// each agent besides this chat's — the worktree and the branch made for
// it, the agent and its model — that what is not committed here is not in
// them, where there is such, and Create, which ⏎ presses, beside Cancel.
func (v *chatView) confirm() json.RawMessage {
	var inside []json.RawMessage
	for _, one := range v.plan.makes {
		inside = append(inside, encode(chatWords{T: "text", Text: "worktree " + one.name + " on branch " + one.branch + " · " + chatAbout(app.Runs{Agent: one.row.agent, Model: one.row.model})}))
	}
	if v.plan.dirty {
		inside = append(inside, encode(chatWords{T: "text", Text: "Uncommitted changes here are not copied.", Style: "dim"}))
	}
	inside = append(inside, encode(chatBox{T: "row", Gap: 8, Children: []json.RawMessage{
		encode(chatButton{T: "button", ID: "create", Label: "Create", Key: "⏎", Primary: true}),
		encode(chatButton{T: "button", ID: "cancel", Label: "Cancel", Key: "esc"}),
	}}))
	return encode(chatModal{T: "modal", ID: chatOver, Title: fmt.Sprintf("Send to %d agents", len(v.plan.makes)+1), W: 640, Children: inside})
}

// up is the patch that puts a modal over the chat, the keys' row placed
// in it. The window puts the keys on the first piece of it that takes
// them, the search line or the list, which a view said whole names too.
func (v *chatView) up(mode chatMode) [][]byte {
	v.mode = mode
	placed := v.modal(true)
	v.talk.over, v.talk.keys = v.modal(false), map[chatMode]string{picking: "q", looking: "earlier", confirming: "create"}[mode]
	return v.talk.patch(append([]json.RawMessage{encode(chatOp{Op: "add", ID: "view", Node: placed})}, v.footed()...)...)
}

// again is the patch that says the modal again as it now is: the keys
// stay where they are, and what is typed in its field.
func (v *chatView) again() [][]byte {
	v.talk.over = v.modal(false)
	return v.talk.patch(append([]json.RawMessage{encode(chatOp{Op: "set", ID: chatOver, Node: v.talk.over})}, v.footed()...)...)
}

// shut is the patch that takes the modal away, and makes with it the
// changes that say what was done in it; the chat behind is as it was.
func (v *chatView) shut(with ...json.RawMessage) [][]byte {
	if v.mode == chatting {
		return v.talk.patch(with...) // none stands: what was picked to come next is done with none over the chat
	}
	v.mode, v.fork, v.told, v.talk.over, v.talk.keys = chatting, 0, nil, nil, ""
	v.marks, v.plan = nil, nil
	return v.talk.patch(slices.Concat([]json.RawMessage{encode(chatOp{Op: "remove", ID: chatOver})}, v.footed(), with)...)
}

// listed takes the worktree's conversations for Earlier, the last written
// to first: one of each id, no more than a list holds, the keys on the
// one the chat is of.
func (v *chatView) listed(heads []app.Head, now time.Time) {
	v.heads, v.now, v.armed = nil, now, ""
	for _, head := range heads {
		if _, twice := v.headOf(head.ID); !twice && len(v.heads) < chatHeads {
			v.heads = append(v.heads, head)
		}
	}
	if _, there := v.headOf(v.id); there {
		v.on = v.id
	} else if v.on = ""; len(v.heads) > 0 {
		v.on = v.heads[0].ID
	}
}

// reads takes the turns of the conversation under the keys in Earlier.
func (v *chatView) reads(id string, turns []app.Turn) {
	v.on, v.read = id, nil
	for _, turn := range turns {
		if drawn(turn) {
			v.read = append(v.read, worded(turn))
		}
	}
}

// chatEvent is what the person did in the window: send with the words
// typed, choose with the question's turn and its option counted from 0,
// stop; a button or a key pressed, with the item of a list it was pressed
// on; an item picked, with the item of its menu; the row under the keys
// moved; the search line changed; a modal dismissed with esc; close.
type chatEvent struct {
	E        string `json:"e"`
	ID       string `json:"id"`
	List     string `json:"list"`
	Item     string `json:"item"`
	Sub      string `json:"sub"`
	Text     string `json:"text"`
	Question int    `json:"question"`
	Option   int    `json:"option"`
}

// chatEvents is the chat's events whole in what was read, and what of it
// is kept for the next read.
func chatEvents(buffer string) ([]chatEvent, string) {
	return muiEvents[chatEvent](buffer)
}

// typed is what one read of the terminal says to a chat.
func typed(buffer, data string) (events []chatEvent, rest string, sight, leave bool) {
	return muiTyped[chatEvent](buffer, data)
}

// chatDo is what an event asks of the app.
type chatDo int

const (
	chatNothing  chatDo = iota
	chatLeave           // end
	chatSend            // type the words to the agent
	chatChoose          // press a question's option in the agent's window
	chatStop            // stop the agent
	chatModelsUp        // show the models
	chatEarlier         // show the earlier conversations
	chatBack            // take the modal away
	chatNarrow          // the models that hold the words
	chatSwitch          // switch this window's agent to a model and an effort
	chatHand            // hand this conversation to another agent, and go on with that one
	chatSay             // only say something at the modal's foot
	chatCopy            // put a conversation's id on the clipboard
	chatRead            // read back the conversation under the keys
	chatGoOn            // take a conversation up again, and show it
	chatShow            // show the conversation a window has open
	chatContinue        // hand a conversation to this chat's agent, and show that
	chatUnder           // say what the model the keys came to does
	chatCarries         // carry the whole conversation to another agent, or its latest turns
	chatForkUp          // show the models, to fork the conversation from a reply
	chatFork            // start an agent in a window of its own on the conversation up to a reply
	chatCopied          // put a reply's words on the clipboard
	chatForget          // let go of what was picked to come next
	chatNow             // stop the agent, so that what was picked to come next is done now
	chatToggle          // mark a row of the models, or take its mark off
	chatUnmark          // take every mark off, and let go of the fan-out kept
	chatFan             // keep the rows marked: the next message goes to each too
	chatCreate          // make what a fan-out makes, and send the message
)

// chatDeed is what an event does: with the words, the conversation, the
// model and effort or the question and option it needs. later, a switch
// or a handing is not done but kept as what comes next: the agent is at
// work.
type chatDeed struct {
	do               chatDo
	text, id, agent  string
	model, effort    string
	question, option int
	whole            bool
	later            bool
}

// deed is what an event asks of the app as the view stands. In the chat a
// conversation no window has open is only shown: nothing is sent,
// answered or stopped. A model of this chat's agent is switched to; one of
// another agent hands the conversation to that agent, when there is one
// mia can read; while the agent is at work either is kept as what comes
// next, the same picked again lets go of it, and tab on it stops the agent
// so that it is done now. A reply's menu copies it, or puts the models up
// to fork the conversation from it, as the key that forks does from the
// last reply; a pick then forks, at work or not. In Earlier ⏎
// on the conversation a window has open shows it and on another takes it
// up again, as its menu's first item does; c says what handing it over
// does the first time it is pressed on a conversation, and does it the
// second, and the menu's item, which names it, does it at once. esc in a
// modal takes it away, and nothing else is done. ⇧⏎ on a row of the models
// marks it, or takes its mark off; with rows marked ⏎ keeps them, the next
// message going to each as well, and esc takes the marks off, the modal
// staying. What is sent then is first put up to be confirmed: Create makes
// it, and Cancel, or esc, nothing.
func (v *chatView) deed(event chatEvent) chatDeed {
	switch {
	case event.E == "close":
		return chatDeed{do: chatLeave}
	case v.mode == picking && event.E == "dismiss" && event.ID == chatOver && len(v.marks) > 0:
		return chatDeed{do: chatUnmark} // esc where the search line has not the keys: the marks first
	case v.mode != chatting && event.E == "dismiss" && event.ID == chatOver:
		return chatDeed{do: chatBack}
	case v.mode == picking && event.E == "change" && event.ID == "q":
		return chatDeed{do: chatNarrow, text: event.Text}
	case v.mode == picking && event.E == "press" && event.ID == "mark":
		return v.marking(event.Item)
	case v.mode == picking && event.E == "press" && event.ID == "unmark":
		return chatDeed{do: chatUnmark}
	case v.mode == picking && event.E == "pick" && event.List == "models" && len(v.marks) > 0:
		return chatDeed{do: chatFan}
	case v.mode == confirming && event.E == "press" && event.ID == "create":
		return chatDeed{do: chatCreate}
	case v.mode == confirming && event.E == "press" && event.ID == "cancel":
		return chatDeed{do: chatBack}
	case v.mode == picking && event.E == "pick" && event.List == "models":
		deed := v.picked(event)
		switch {
		case !v.talk.working || deed.do != chatSwitch && deed.do != chatHand:
		case v.comes(deed):
			return chatDeed{do: chatForget}
		default:
			deed.later = true
		}
		return deed
	case v.mode == picking && event.E == "press" && event.ID == "now":
		if v.next != nil {
			return chatDeed{do: chatNow}
		}
	case v.mode == picking && event.E == "move" && event.List == "models":
		if event.ID != v.under {
			return chatDeed{do: chatUnder, id: event.ID}
		}
	case v.mode == picking && event.E == "press" && event.ID == "flip":
		return chatDeed{do: chatCarries, whole: !v.whole}
	case v.mode == picking && event.E == "pick" && event.List == "carry":
		return chatDeed{do: chatCarries, whole: event.ID == "whole"}
	case v.mode == looking && event.E == "move" && event.List == "earlier":
		if _, there := v.headOf(event.ID); there && event.ID != v.on {
			return chatDeed{do: chatRead, id: event.ID}
		}
	case v.mode == looking && event.E == "pick" && event.List == "earlier":
		head, there := v.headOf(event.ID)
		switch {
		case !there:
		case event.Sub == "copy":
			return chatDeed{do: chatCopy, id: head.ID}
		case event.Sub == "continue":
			if head.Agent != v.agent() {
				return chatDeed{do: chatContinue, id: head.ID}
			}
		case head.Running && head.Window != nil:
			return chatDeed{do: chatShow, id: head.ID}
		default:
			return chatDeed{do: chatGoOn, id: head.ID}
		}
	case v.mode == looking && event.E == "press" && event.ID == "continue":
		head, there := v.headOf(event.Item)
		switch {
		case !there || head.Agent == v.agent():
		case v.armed == head.ID:
			return chatDeed{do: chatContinue, id: head.ID}
		default:
			return chatDeed{do: chatSay, id: head.ID, text: fmt.Sprintf("c again hands this to %s in a new window: what was said and each thing done carry over, what %s thought and its tools' output do not",
				v.agent(), head.Agent)}
		}
	case v.mode != chatting:
	case event.E == "press" && event.ID == "open-earlier":
		return chatDeed{do: chatEarlier}
	case v.window == "":
	case event.E == "press" && event.ID == "open-models":
		return chatDeed{do: chatModelsUp}
	case event.E == "press" && event.ID == "open-fork":
		return chatDeed{do: chatForkUp}
	case event.E == "pick" && event.List == v.talk.id():
		place, _ := strconv.Atoi(event.ID)
		switch event.Sub {
		case "copy":
			return chatDeed{do: chatCopied, question: place}
		case "fork":
			return chatDeed{do: chatForkUp, question: place}
		}
	case event.E == "send" && strings.TrimSpace(event.Text) != "":
		return chatDeed{do: chatSend, text: strings.TrimSpace(event.Text)}
	case event.E == "choose":
		return chatDeed{do: chatChoose, question: event.Question, option: event.Option}
	case event.E == "stop":
		return chatDeed{do: chatStop}
	}
	return chatDeed{}
}

// comes reports whether a pick is the one kept as what comes next: its
// agent and model, and its effort where the pick says one.
func (v *chatView) comes(deed chatDeed) bool {
	next := v.next
	return next != nil && next.do == deed.do && next.agent == deed.agent && next.model == deed.model && (deed.effort == "" || deed.effort == next.effort)
}

// marking is what ⇧⏎ on a row of the models asks: its mark put on or taken
// off, or why sending to several is not offered now — while what this
// chat's agent goes on from has still to be sent, while something was
// picked to come next — or for that row, the one this chat runs on.
func (v *chatView) marking(id string) chatDeed {
	row, ok := chatModelOf(id)
	switch {
	case !ok:
		return chatDeed{}
	case v.talk.waits != nil:
		return chatDeed{do: chatSay, text: "what " + v.agent() + " goes on from has still to be sent with your next message: send that first, then mark"}
	case v.next != nil:
		return chatDeed{do: chatSay, text: v.next.coming() + " is picked to come next: clear it first, then mark"}
	case v.here(row):
		return chatDeed{do: chatSay, text: v.agent() + " gets your next message here in any case: mark the others it goes to as well"}
	}
	return chatDeed{do: chatToggle, id: id}
}

// picked is what picking a row of the models asks, whatever the agent is
// doing: put up to fork the conversation, any agent's row forks it.
func (v *chatView) picked(event chatEvent) chatDeed {
	row, ok := chatModelOf(event.ID)
	if !ok {
		return chatDeed{}
	}
	effort := cmp.Or(event.Sub, row.effort)
	var agent app.AgentModels
	for _, one := range v.agents {
		if one.Name == row.agent {
			agent = one
		}
	}
	switch {
	case v.fork > 0:
		return chatDeed{do: chatFork, agent: row.agent, model: row.model, effort: effort, whole: v.whole}
	case row.agent != v.agent() && v.id == "":
		return chatDeed{do: chatSay, text: "nothing is said here that mia can read yet: there is nothing for " + row.agent + " to go on with"}
	case row.agent != v.agent():
		return chatDeed{do: chatHand, agent: row.agent, model: row.model, effort: effort, whole: v.whole}
	case row.model == "" && effort == "":
		return chatDeed{do: chatSay, text: row.agent + " is started with no model, and takes none from mia: it is chosen in its terminal"}
	case !agent.Live && !agent.Again:
		return chatDeed{do: chatSay, text: row.agent + " cannot be switched to another model while it runs — restart it with --model"}
	}
	return chatDeed{do: chatSwitch, model: row.model, effort: effort}
}

// chatOf is the conversation a chat shows, of a worktree's, the last
// written to first: the one named, the one the window named has open, and
// with neither the one a window has open, else the last written to.
func chatOf(heads []app.Head, worktree, window, id string) (app.Head, error) {
	open := func(h app.Head) bool { return h.Running && h.Window != nil }
	is := func(h app.Head) bool {
		return (id == "" || h.ID == id) && (window == "" || open(h) && strconv.Itoa(h.Window.Index) == window)
	}
	at := slices.IndexFunc(heads, func(h app.Head) bool { return is(h) && open(h) })
	if at < 0 {
		at = slices.IndexFunc(heads, is)
	}
	switch {
	case at >= 0:
		return heads[at], nil
	case window != "" && id != "":
		return app.Head{}, fmt.Errorf("window %s of %s does not have conversation %s open — `mia api conversations --worktree %s` says which window has which", window, worktree, id, worktree)
	case window != "":
		return app.Head{}, fmt.Errorf("window %s of %s has no conversation open — `mia api conversations --worktree %s` says which window has which", window, worktree, worktree)
	case id != "":
		return app.Head{}, fmt.Errorf("no conversation %s in %s — `mia api conversations --worktree %s` lists what is", id, worktree, worktree)
	}
	return app.Head{}, fmt.Errorf("%s has no conversation yet — `mia window new %s <agent>` starts an agent in it", worktree, worktree)
}

// handing is what handing a conversation to another agent asks of mia.
type handing interface {
	ContinueConversation(want app.Continue) (app.Continued, error)
	ReplaceWindow(record model.Record, window, with string) error
}

// handed is how a handing went: whether the chat is now the other agent's
// window's, and what is said of it.
type handed struct {
	done       app.Continued
	went       bool
	text, tone string
}

// handTo hands a conversation to another agent: its turns written out
// as one file, as mia conversation continue writes them, and the agent
// started on a model and an effort in a window of its own, told nothing;
// whole, no turn of it is left out of what is written.
// Only once that agent runs is the window of the one it was with closed,
// so one agent goes on, and the other's window is put in front wherever
// that one was; window is that window, "" when it is not to be closed.
// ready is called between the two, the other agent running and nothing
// closed yet: whoever shows this chat may end it as its window goes.
// Refused, or the other agent not running, nothing is closed and the
// reason is said.
func handTo(mia handing, record model.Record, id, window, from, to, model, effort string, whole bool, ready func(app.Continued)) handed {
	done, err := mia.ContinueConversation(app.Continue{ID: id, Worktree: record.Name, With: to, Model: model, Effort: effort, Quiet: true, Whole: whole})
	switch {
	case err != nil:
		return handed{text: err.Error(), tone: "bad"}
	case !done.Sent:
		return handed{done: done, tone: "bad", text: from + " still runs — " + done.Problem}
	}
	if ready(done); window == "" {
		return handed{done: done, went: true, tone: "ok", text: to + " goes on with this conversation · " + from + "'s window stays"}
	}
	if err := mia.ReplaceWindow(record, window, done.To.Window.Name); err != nil {
		return handed{done: done, went: true, tone: "warn", text: fmt.Sprintf("%s goes on with this conversation · %s's window is still open: %v", to, from, err)}
	}
	return handed{done: done, went: true, tone: "ok", text: from + " stopped · " + to + " goes on with this conversation"}
}

// chatBatch is turns a follower read, and chatEnd that it ended: each with
// which follower, as one let go of may still say something.
type chatBatch struct {
	of    int
	turns []app.Turn
}

type chatEnd struct {
	of  int
	err error
}

// chatRun is a chat running: the view, and what does what it asks.
type chatRun struct {
	a      *app.App
	record model.Record
	view   *chatView
	say    func(messages ...[]byte)
	done   <-chan struct{} // closed as the program leaves
	// The follower of the conversation shown: which it is, the channel that
	// ends it, and where every follower says what it read and how it ended.
	of      int
	quit    chan struct{}
	batches chan chatBatch
	ended   chan chatEnd
	// pressed is when an answer was last pressed in the window: its screen
	// is not asked what it asks for a moment after, while it still shows it.
	pressed time.Time
	// told has a value when mia serve says the window's agent went to work
	// or stopped, or its screen asks another thing; nil while no server of
	// this build answers, gone once one went, and the window is looked at
	// each second. owed: what it was told came in that moment, and is
	// looked at once it is over.
	told   <-chan struct{}
	hangUp func()
	gone   bool
	owed   bool
	// models is the models changed in this chat, the first first.
	models []chatModelled
}

// note says how things stand as the status, alone: nothing, and it is
// taken away.
func (r *chatRun) note(text, tone string) {
	t := r.view.talk
	if text == "" && t.status == nil {
		return
	}
	var status *muiStatus
	if text != "" {
		status = &muiStatus{Text: cutTo(text, chatSaid), Tone: tone}
	}
	r.say(t.patch(t.stands(status))...)
}

// tell says at the modal's foot why what was asked in it was not done: as
// the status, of what was picked to come next and is done with no modal
// over the chat.
func (r *chatRun) tell(why error) {
	if r.view.mode == chatting {
		r.note(why.Error(), "bad")
		return
	}
	r.view.told = &muiStatus{Text: why.Error(), Tone: "bad"}
	r.say(r.view.again()...)
}

// shows says whole the chat aim began, and how what began it went.
func (r *chatRun) shows(text, tone string) {
	t := r.view.talk
	went := t.went(text, tone)
	r.say(t.said()...)
	r.say(t.patch(went)...)
}

// aim makes the chat one of a window, of a conversation, or of both: the
// conversation is followed from its first turn, and a window with none
// mia can read yet is a chat all the same — what is sent is typed to it
// and shown, and its conversation is looked for each second. The chat
// before it is let go of with what stood over it, and this one is another
// piece to the window.
func (r *chatRun) aim(head app.Head, window string) {
	if r.quit != nil {
		close(r.quit)
	}
	r.of, r.quit = r.of+1, make(chan struct{})
	v, nth := r.view, 0
	if v.talk != nil {
		nth = v.talk.nth + 1
	}
	v.mode, v.fork, v.told, v.tabbed, v.next, v.window, v.id = chatting, 0, nil, "", nil, window, head.ID
	r.listen()
	v.marks, v.fans, v.plan = nil, nil, nil
	v.runs = app.Runs{Agent: head.Agent, Model: head.Model, Effort: head.Effort, Account: head.Account, Reads: true}
	if window != "" {
		if runs := r.a.WindowRuns(r.record, window); runs.Agent != "" {
			v.runs = chatRuns(runs, head)
		}
	}
	v.talk = &talk{pane: os.Getenv("TMUX_PANE"), agent: cmp.Or(v.runs.Agent, "window "+window), about: chatAbout(v.runs), reply: window != "", nth: nth}
	if window != "" {
		v.talk.working = r.a.AtWork(r.record, r.index())
		if v.runs.Agent != "" && !v.runs.Reads {
			r.note(v.runs.Agent+"'s replies are in its terminal: mia reads none of its conversations. What is sent here is typed to it", "warn")
		}
	}
	var from app.Talk // what its agent goes on from and was not sent yet
	if carried, on := r.a.CarriedInWindow(r.record, window); window != "" && on {
		from = app.Talk{Agent: carried.From, ID: carried.ID}
	}
	v.talk.lays(r.parts(app.Talk{Agent: v.runs.Agent, ID: head.ID}, from))
	v.came = nil
	for _, handed := range r.a.Handings(v.runs.Agent, head.ID, from) {
		v.came = append(v.came, handed.From.Agent)
	}
	if head.ID != "" {
		r.follow(head.ID)
	}
}

// chatModelled is a model's change in a conversation, kept while the chat
// runs: after which of its turns, and the line that says it.
type chatModelled struct {
	of    app.Talk
	after int
	band  chatBand
}

// parts is the stretches of the chat of a conversation that went from
// agent to agent, by the handings that led to it: each conversation it was
// with by its turns said between two handings, the switch after it, and
// last the conversation followed. A conversation its agent took up again
// has its earlier turns before those said with the other agent meanwhile,
// by when each turn was said; a turn that says no time is in the last
// stretch its conversation has. from is the conversation the window's
// agent has still to be sent: the last switch waits. A model changed in
// this chat stands after the turn it was changed after.
func (r *chatRun) parts(of, from app.Talk) []chatPart {
	chain := r.a.Handings(of.Agent, of.ID, from)
	read := map[app.Talk][]app.Turn{}
	turns := func(of app.Talk) []app.Turn {
		if _, done := read[of]; !done {
			read[of], _ = r.a.ConversationIn(of.ID, r.record.Name, 0)
		}
		return read[of]
	}
	var parts []chatPart
	laid := 0 // the last number of the conversation followed that has its stretch
	stretch := func(with app.Talk, in func(app.Turn) bool, then *chatBand, waits bool) {
		var all []app.Turn
		for _, turn := range turns(with) {
			if in(turn) {
				all = append(all, turn)
			}
		}
		own := with == of && of.ID != ""
		if own && len(all) > 0 {
			laid = all[len(all)-1].N
		}
		for _, row := range r.models {
			at := slices.IndexFunc(all, func(turn app.Turn) bool { return turn.N == row.after }) + 1
			if row.of != with || at == 0 {
				continue
			}
			parts = append(parts, chatPart{turns: all[:at], own: own, until: row.after, then: &row.band})
			all = all[at:]
		}
		part := chatPart{turns: all, own: own && len(all) > 0, then: then, waits: waits}
		if part.own {
			part.until = all[len(all)-1].N
		}
		parts = append(parts, part)
	}
	for i, handed := range chain {
		var after time.Time
		if i > 0 {
			after = chain[i-1].At
		}
		with := handed.From
		again := slices.ContainsFunc(chain[i+1:], func(later app.Handed) bool { return later.From == with }) || with == of && of.ID != ""
		stretch(with, func(turn app.Turn) bool {
			if handed.Upto > 0 && turn.N > handed.Upto {
				return false // what the conversation it was forked from went on to say is not the fork's
			}
			if turn.At.IsZero() {
				return !again
			}
			return turn.At.After(after) && !turn.At.After(handed.At)
		}, switched(handed), i == len(chain)-1 && from.ID != "")
	}
	if of.ID != "" && slices.ContainsFunc(r.models, func(row chatModelled) bool { return row.of == of }) {
		stretch(of, func(turn app.Turn) bool { return turn.N > laid }, nil, false)
		parts = parts[:len(parts)-1] // what is left of it is heard as it is followed
	}
	if len(parts) == 0 {
		return nil
	}
	return append(parts, chatPart{own: true})
}

// index is the window typed into, by its number.
func (r *chatRun) index() int {
	index, _ := strconv.Atoi(r.view.window)
	return index
}

// follow reads a conversation as it is written, for the run to hear.
func (r *chatRun) follow(id string) {
	of, quit := r.of, r.quit
	go func() {
		err := r.a.FollowConversation(id, r.record.Name, 0, quit, func(turns []app.Turn) error {
			select {
			case r.batches <- chatBatch{of, turns}:
				return nil
			case <-quit:
				return errors.New("left")
			}
		})
		select {
		case r.ended <- chatEnd{of, err}:
		case <-quit:
		case <-r.done:
		}
	}()
}

// listen has mia serve tell the chat of its window, when one of this build
// answers: the window's question asked each look, once for all who listen.
func (r *chatRun) listen() {
	if r.hangUp != nil {
		r.hangUp()
	}
	r.told, r.hangUp = nil, nil
	if r.view.window == "" || r.gone {
		return
	}
	r.told, r.hangUp = listenServer(asked("window", r.record.Path, r.view.window, "--worktree", r.record.Name))
	r.gone = r.told == nil
}

// looks is what a second brings, or mia serve tells of: the conversation
// of a window that had none, once it has one; whether the agent is at
// work; and what its window's screen asks.
func (r *chatRun) looks() {
	v := r.view
	if v.window == "" {
		return
	}
	if v.id == "" && (v.runs.Reads || v.runs.Agent == "") {
		heads, _ := r.a.Conversations(app.Wanted{Worktree: r.record.Name})
		if head, err := chatOf(heads, r.record.Name, v.window, ""); err == nil {
			v.id = head.ID
			if runs := chatRuns(r.a.WindowRuns(r.record, v.window), head); runs.Agent != "" && runs != v.runs {
				v.runs, v.talk.agent = runs, runs.Agent
				v.lined()
				r.say(v.talk.said()...)
			}
			r.follow(head.ID)
		}
	}
	working := r.a.AtWork(r.record, r.index())
	r.say(v.talk.atWork(working)...)
	if !working {
		r.rests()
	}
	if r.owed = time.Since(r.pressed) < 2*time.Second; r.owed {
		return
	}
	var asks *app.Asks
	if now, on := r.a.AsksInWindow(r.record, v.window); on {
		asks = &now
	}
	r.say(v.talk.asked(asks)...)
}

// act does a deed, and says what it changes: the chat's own changes, a
// modal put over it, said again or taken away, and how the thing went — a
// toast when it went well, the status or the modal's foot when not. A
// chat of another window or conversation is said whole.
func (r *chatRun) act(deed chatDeed) {
	v := r.view
	switch deed.do {
	case chatNothing, chatLeave:
	case chatSend:
		if len(v.fans) > 0 {
			r.plans(deed.text) // nothing is made or sent until it is confirmed
			return
		}
		// what was picked to come next and is not done yet, the turn just over: first, so the words go to who was picked
		if v.next != nil && !r.a.AtWork(r.record, r.index()) {
			r.say(v.talk.atWork(false)...)
			if !r.rests() {
				words := []rune(deed.text)
				r.say(v.talk.patch(encode(chatOp{Op: "draft", ID: v.talk.id(), Text: string(words[:min(len(words), chatDraft)])}))...)
				return
			}
		} else {
			r.note("", "")
		}
		r.types(deed.text)
	case chatCreate:
		r.fansOut()
	case chatToggle:
		row, _ := chatModelOf(deed.id)
		if at := v.marked(row); at >= 0 {
			v.marks = slices.Delete(v.marks, at, at+1)
		} else {
			v.marks = append(v.marks, row)
		}
		v.told = nil
		r.say(v.again()...)
	case chatUnmark:
		v.marks, v.told = nil, nil
		r.say(v.again()...)
		if len(v.fans) > 0 {
			r.say(v.fanned(nil)...)
		}
	case chatFan:
		rows := v.marks
		r.say(v.shut()...)
		r.say(v.fanned(rows)...)
	case chatChoose:
		r.note("", "")
		if err := r.a.PressInWindow(r.record, v.window, strconv.Itoa(deed.option+1)); err != nil {
			r.say(v.talk.reopened(deed.question, err)...)
			return
		}
		// answered: the question of the app's own goes, until the screen asks again
		r.pressed = time.Now()
		r.say(v.talk.asked(nil)...)
	case chatStop:
		if err := r.a.StopInWindow(r.record, v.window); err != nil {
			r.note(err.Error(), "bad")
			return
		}
		r.note("", "")
	case chatCopied:
		if _, words, found := v.talk.replied(deed.question); found {
			r.say(v.talk.patch(encode(chatOp{Op: "copy", Text: words}), v.talk.went("copied", "ok"))...)
		}
	case chatModelsUp, chatForkUp:
		v.fork = 0
		if deed.do == chatForkUp {
			if v.fork, _, _ = v.talk.replied(deed.question); v.fork == 0 {
				r.note("only a reply of this conversation's own is forked from: there is none here yet, or that one was said with another agent", "warn")
				return
			}
			r.note("", "")
		}
		v.agents, v.setups, v.query = r.a.AgentModels(), r.a.Setups(filepath.Base(r.a.Root)), ""
		v.under, v.whole, v.would = "", false, map[string]chatWould{}
		if v.marks = nil; deed.do == chatModelsUp {
			v.marks = slices.Clone(v.fans) // the fan-out kept, to add to or clear
		}
		v.mode = picking
		v.models(true) // which row the keys come to
		r.weighs()
		r.say(v.up(picking)...)
	case chatEarlier:
		heads, _ := r.a.Conversations(app.Wanted{Worktree: r.record.Name})
		v.listed(heads, time.Now())
		r.read(v.on)
		r.say(v.up(looking)...)
	case chatBack:
		var back []json.RawMessage
		if v.plan != nil {
			back = append(back, v.talk.drafted(v.plan.text)) // nothing is made: the words are the field's again
		}
		r.say(v.shut(back...)...)
	case chatNarrow:
		v.query, v.told = deed.text, nil
		v.models(false) // which row the keys are left on
		r.weighs()
		r.say(v.again()...)
	case chatUnder:
		v.under = deed.id
		r.weighs()
		r.say(v.again()...)
	case chatCarries:
		v.whole = deed.whole
		r.say(v.again()...)
		r.say(v.talk.patch(encode(chatOp{Op: "focus", ID: "q"}))...) // a press on a choice took the keys from the search line
	case chatSay:
		v.armed, v.told = deed.id, &muiStatus{Text: deed.text, Tone: "warn"}
		r.say(v.again()...)
	case chatCopy:
		r.say(v.talk.patch(encode(chatOp{Op: "copy", Text: deed.id}), v.talk.went("copied "+deed.id, "ok"))...)
	case chatSwitch, chatHand:
		if !deed.later {
			r.applies(deed)
			return
		}
		deed.later = false
		v.next = &deed
		r.say(v.talk.patch(v.lined())...)
		r.say(v.again()...)
	case chatFork:
		from, reply := v.agent(), v.fork
		r.say(v.shut(v.talk.stands(&muiStatus{Text: cutTo(fmt.Sprintf("forking this conversation from reply %d, for %s…", reply, deed.agent), chatSaid)}))...)
		done, err := r.a.ContinueConversation(app.Continue{ID: v.id, Worktree: r.record.Name, With: deed.agent, Model: deed.model, Effort: deed.effort, Quiet: true, Whole: deed.whole, Upto: reply})
		switch {
		case err != nil:
			r.note(err.Error(), "bad")
			return
		case !done.Sent:
			r.note(done.Problem, "bad")
			return
		}
		// kept on its window: the chat of that window says it in front of the first thing sent there
		text, tone := fmt.Sprintf("%s goes on from reply %d in window %s · this chat stays as it is", deed.agent, reply, done.To.Window.Name), "ok"
		if err := r.carry(strconv.Itoa(done.To.Window.Index), from, v.id, done); err != nil {
			text, tone = text+" · "+err.Error(), "warn"
		}
		r.say(v.talk.patch(v.talk.stands(nil), v.talk.went(text, tone))...)
	case chatForget:
		v.next = nil
		r.say(v.talk.patch(v.lined())...)
		r.say(v.again()...)
	case chatNow:
		if err := r.a.StopInWindow(r.record, v.window); err != nil {
			r.tell(err)
			return
		}
		r.say(v.shut(v.talk.stands(&muiStatus{Text: cutTo("stopping "+v.agent()+": next is "+v.next.coming(), chatSaid)}))...)
	case chatRead:
		r.read(deed.id)
		r.say(v.again()...)
	case chatShow:
		head, _ := v.headOf(deed.id)
		if head.ID == v.id && strconv.Itoa(head.Window.Index) == v.window {
			r.say(v.shut()...)
			return
		}
		r.aim(head, strconv.Itoa(head.Window.Index))
		r.say(v.talk.said()...)
	case chatGoOn:
		done, err := r.a.ResumeConversation(deed.id, r.record.Name, r.record.Name, "", "", "")
		if err != nil {
			r.tell(err)
			return
		}
		head, _ := v.headOf(deed.id)
		head.Model, head.Effort, head.Account = done.Model, done.Effort, cmp.Or(done.Account, head.Account)
		r.aim(head, strconv.Itoa(done.Window.Index))
		r.shows(head.Agent+" goes on with it in window "+done.Window.Name, "ok")
	case chatContinue:
		// As a switch of agents is: this window's agent started on it and told
		// nothing, the conversation carried in front of the first thing sent.
		head, _ := v.headOf(deed.id)
		went := handTo(r.a, r.record, deed.id, "", head.Agent, v.agent(), "", "", false, func(app.Continued) {})
		if !went.went {
			v.armed = ""
			r.note(went.text, went.tone)
			return
		}
		done := went.done
		to := r.nowAt(done.To.Window)
		kept := r.carry(to, head.Agent, deed.id, done)
		r.aim(app.Head{ID: done.Resumes, Agent: done.To.Agent, Model: done.Model, Effort: done.Effort, Account: done.Account}, to)
		if kept != nil {
			went.text, went.tone = went.text+" · "+kept.Error(), "warn"
		}
		r.shows(went.text, went.tone)
	}
}

// types types what the person sent into this chat's window, in front of
// it what its agent has still to go on from, and says it at once as their
// turn; false when it did not go, and the words are the field's again.
func (r *chatRun) types(text string) bool {
	v := r.view
	r.say(v.talk.sends(text)...)
	send := r.a.SendToWindow
	words, back := typedFor(r.a, r.record, v.window, text, os.ReadFile)
	if strings.Contains(words, "\n") {
		send = r.a.SayToWindow // pasted: a line break in it is not an Enter
	}
	if err := send(r.record, v.window, words); err != nil {
		back()
		r.say(v.talk.refused(err)...)
		return false
	}
	if words != text {
		r.say(v.talk.patch(v.talk.handed()...)...) // what the agent goes on from went with it
	}
	return true
}

// plans puts up what sending words to the agents kept makes, to be
// confirmed: for each a worktree beside this one, named for it and on a
// branch named for it, with a number where that name or branch is taken.
// Nothing is made yet.
func (r *chatRun) plans(text string) {
	v := r.view
	plan := &chatPlan{text: text}
	plan.dirty, _ = git.Dirty(r.record.Path)
	var named []string
	for _, row := range v.fans {
		name, branch := r.a.Beside(r.record, row.agent, named)
		named = append(named, name)
		plan.makes = append(plan.makes, chatMake{row, name, branch})
	}
	v.plan = plan
	r.say(v.up(confirming)...)
}

// fansOut does a fan-out that was confirmed: for each agent in turn its
// worktree made, the agent started there and sent the words, each said as
// it is done; then the words typed to this chat's own agent, here, and a
// row of the chat's own under them that says who else they went to and
// where. Where one fails it stops: what was made stays as it is, nothing
// more is made or sent, this chat's agent is sent nothing, the words are
// the field's again, the agents not sent to are still kept, and the status
// says what was made and what failed.
func (r *chatRun) fansOut() {
	v := r.view
	plan := *v.plan
	r.say(v.shut(v.talk.stands(&muiStatus{Text: "making " + counted(len(plan.makes), "worktree") + "…"}))...)
	head := git.Head(r.record.Path) // what is committed here: not what is only in its files
	var made []string
	for i, one := range plan.makes {
		there, err := r.fan(one, plan.text, head)
		if err != nil {
			if there {
				made = append(made, "worktree "+one.name)
			}
			v.talk.stands(&muiStatus{Text: cutTo(fmt.Sprintf("stopped at %s: nothing sent to %s · made: %s · %v", one.row.agent, v.agent(), cmp.Or(strings.Join(made, ", "), "nothing"), err), chatSaid), Tone: "bad"})
			r.say(v.fanned(v.fans[i:])...)
			r.say(v.talk.patch(v.talk.drafted(plan.text))...)
			return
		}
		made = append(made, one.row.agent+" in "+one.name)
		r.say(v.talk.patch(v.talk.went(one.row.agent+" started in "+one.name, "ok"))...)
	}
	v.talk.stands(nil)
	r.say(v.fanned(nil)...)
	if !r.types(plan.text) {
		return
	}
	row := v.talk.put(chatBand{Kind: "switch", From: fmt.Sprintf("sent to %d agents", len(plan.makes)+1), Note: strings.Join(made, " · ")}, 1)
	if v.talk.shown != nil {
		v.talk.shown[row.n] = true
		r.say(v.talk.patch(encode(chatOp{Op: "turn", ID: v.talk.id(), Turn: row.json}))...)
	}
}

// fan makes the worktree of one agent of a fan-out, a new branch at head,
// as mia new makes one, starts the agent in a window of its session and
// sends it the words. Where this chat has a conversation the agent goes on
// from it as a fork does, up to its last turn: written out as one file and
// said in front of the words. there is whether the worktree was made,
// whatever came after.
func (r *chatRun) fan(one chatMake, text, head string) (there bool, err error) {
	v := r.view
	record, err := r.a.NewNamed(one.branch, head, one.name)
	if err != nil {
		return record.Path != "", err
	}
	if v.id == "" {
		// nothing said here that mia can read: started as mia window new starts one, and sent the words once ready
		command, chosen, err := r.a.CommandWith(r.a.AgentCommand(one.row.agent), one.row.model, one.row.effort, "")
		if err != nil {
			return true, err
		}
		argv, err := r.a.LaunchArgv(record, command)
		if err != nil {
			return true, err
		}
		window, err := r.a.NewWindow(record, one.row.agent, session.What{Kind: session.KindAgent, Label: one.row.agent}, argv)
		if err != nil {
			return true, err
		}
		if chosen != (app.Chosen{}) {
			if err := r.a.KeepChosen(record, window, one.row.agent, chosen); err != nil {
				return true, err
			}
		}
		return true, r.a.SayOnceReady(record, window, text)
	}
	done, err := r.a.ContinueConversation(app.Continue{ID: v.id, Worktree: r.record.Name, In: record.Name, With: one.row.agent, Model: one.row.model, Effort: one.row.effort, Quiet: true, Upto: v.talk.last})
	switch {
	case err != nil:
		return true, err
	case !done.Sent:
		return true, errors.New(done.Problem)
	}
	transcript, err := os.ReadFile(done.Handoff)
	if err != nil {
		return true, err
	}
	return true, r.a.SayToWindow(record, done.To.Window.Name, carried(v.agent(), string(transcript), cmp.Or(done.Full, done.Handoff), text, chatCarry))
}

// applies switches this window's agent to a model and an effort, or hands
// the conversation to another agent and makes the chat that agent's, and
// says how it went; false when it was not done.
func (r *chatRun) applies(deed chatDeed) bool {
	v := r.view
	if deed.do == chatSwitch {
		now, err := r.a.SwitchWindow(r.record, v.window, deed.model, deed.effort)
		if err != nil {
			r.tell(err)
			return false
		}
		was := v.runs
		v.runs.Model, v.runs.Effort = now.Model, now.Effort
		changes := []json.RawMessage{v.lined(),
			v.talk.went(v.runs.Agent+" switched to "+strings.TrimSpace(deed.model+" "+deed.effort), "ok")}
		if band, changed := modelled(was, v.runs); changed {
			// kept while the chat runs: mia keeps what a window runs on now, not what it ran on before
			r.models = append(r.models, chatModelled{app.Talk{Agent: v.agent(), ID: v.id}, v.talk.last, band})
			row := v.talk.put(band, 0)
			if v.talk.shown != nil {
				v.talk.shown[row.n] = true
				changes = append(changes, encode(chatOp{Op: "turn", ID: v.talk.id(), Turn: row.json}))
			}
		}
		r.say(v.shut(changes...)...)
		return true
	}
	from, window := v.agent(), v.window
	if r.a.WindowHasPane(r.record, window, v.talk.pane) {
		window = "" // this chat's own window: closing it would end the chat
	}
	r.say(v.shut(v.talk.stands(&muiStatus{Text: cutTo("handing this conversation to "+deed.agent+"…", chatSaid)}))...)
	var kept error
	went := handTo(r.a, r.record, v.id, window, from, deed.agent, deed.model, deed.effort, deed.whole, func(done app.Continued) {
		// kept on its window: whichever chat of it sends first says it, this one or one started after
		kept = r.carry(done.To.Window.Name, from, v.id, done)
		if window != "" {
			// whoever shows this chat behind the window about to close shows it behind the other's from now on
			r.say(v.talk.patch(encode(chatOp{Op: "window", Window: r.a.WindowID(r.record, done.To.Window.Name)}))...)
		}
	})
	if !went.went {
		r.note(went.text, went.tone)
		return false
	}
	done := went.done
	to := r.nowAt(done.To.Window)
	r.aim(app.Head{ID: done.Resumes, Agent: done.To.Agent, Model: done.Model, Effort: done.Effort, Account: done.Account}, to)
	if done.Resumes != "" {
		went.text = from + " stopped · " + done.To.Agent + " takes its own conversation up again, with what was said since"
	}
	if kept != nil {
		went.text, went.tone = went.text+" · "+kept.Error(), "warn"
	}
	r.shows(went.text, went.tone)
	return true
}

// rests does what was picked to come next, now that the agent is not at
// work; false when there was something and it was not done.
func (r *chatRun) rests() bool {
	v := r.view
	if v.next == nil {
		return true
	}
	deed := *v.next
	v.next = nil
	r.say(v.talk.patch(v.lined(), v.talk.stands(nil))...)
	return r.applies(deed)
}

// weighs looks at what handing the conversation to the agent of the model
// under the keys would do, or forking it for that agent from the reply the
// models were put up for, writing and starting nothing: once for an agent
// while the models are up, so moving the keys reads no conversation twice.
func (r *chatRun) weighs() {
	v := r.view
	row, ok := chatModelOf(v.under)
	if _, looked := v.would[row.agent]; !ok || looked || row.agent == v.agent() && v.fork == 0 || v.id == "" {
		return
	}
	done, err := r.a.ContinueConversation(app.Continue{ID: v.id, Worktree: r.record.Name, With: row.agent, DryRun: true, Upto: v.fork})
	v.would[row.agent] = chatWould{done, err}
}

// carry keeps on a window what its agent goes on from, for the first thing
// sent to it: nothing, where an agent that took its own conversation up
// again was said nothing since.
func (r *chatRun) carry(window, from, id string, done app.Continued) error {
	if done.Resumes != "" && done.Turns == 0 {
		return nil
	}
	return r.a.CarryInWindow(r.record, window, app.Carried{From: from, ID: id, File: done.Handoff, Full: done.Full, Resumed: done.Resumes != ""})
}

// nowAt is where a window stands now, by its name: closing the window of
// the agent a conversation was with moves those after it down one where
// tmux renumbers them, so where it was opened is no longer where it is.
func (r *chatRun) nowAt(window app.WindowRef) string {
	for _, one := range r.a.Windows(r.record) {
		if one.Name == window.Name {
			return strconv.Itoa(one.Index)
		}
	}
	return strconv.Itoa(window.Index)
}

// read reads back the conversation under the keys in Earlier, and what
// the modal's foot says is why it could not be.
func (r *chatRun) read(id string) {
	if r.view.told = nil; id == "" {
		return
	}
	turns, err := r.a.ConversationIn(id, r.record.Name, 0)
	if err != nil {
		r.view.told = &muiStatus{Text: err.Error(), Tone: "bad"}
	}
	r.view.reads(id, turns)
}

// cmdChat shows a conversation of the worktree it is run in as a chat
// drawn by mia desk over its pane, and follows it as it is written. What
// the person sends, chooses and stops there is typed into the window that
// has the conversation open; one no window has open is only shown. A
// window named that has no conversation mia can read yet is a chat all the
// same: the first thing said to an agent can be said from it.
func cmdChat(a *app.App, args []string) int {
	window, id := takeValue(&args, "--window"), takeValue(&args, "--conversation")
	if index, err := strconv.Atoi(window); len(args) > 0 || window != "" && (err != nil || index < 0) {
		return usageErr(chatUsage)
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		return fail(errors.New("chat reads what you do from a terminal: run it in a pane of mia desk"))
	}
	record, err := a.RecordOrHere("")
	if err != nil {
		return fail(err)
	}
	heads, problems := a.Conversations(app.Wanted{Worktree: record.Name})
	head, err := chatOf(heads, record.Name, window, id)
	switch {
	case err == nil && head.Running && head.Window != nil:
		window = strconv.Itoa(head.Window.Index)
	case err == nil:
		window = "" // no window has it open: only shown
	case window == "" || id != "":
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, "mia:", problem)
		}
		return fail(err)
	}

	stop := make(chan struct{})
	defer close(stop)
	run := &chatRun{a: a, record: record, view: &chatView{worktree: record.Name}, done: stop, batches: make(chan chatBatch), ended: make(chan chatEnd)}
	run.aim(head, window)
	defer func() {
		if run.hangUp != nil {
			run.hangUp()
		}
	}()
	keys := muiKeys(stop)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)

	say, shut, err := muiOpen(run.view.talk.pane, "mia chat: the plain terminal. In mia desk "+run.view.talk.agent+" · "+record.Name+" is drawn over it as a chat; q or ⌃C leaves.")
	if err != nil {
		return fail(err)
	}
	var failure error
	defer func() {
		if failure != nil {
			fmt.Fprintf(os.Stderr, "mia: %v\n", failure) // once the terminal is itself again
		}
	}()
	defer shut()
	run.say = say
	if run.view.id == "" {
		say(run.view.talk.said()...) // else with its turns, as they are first read
	}

	second := time.NewTicker(time.Second)
	defer second.Stop()
	buffer := ""
	for {
		select {
		case <-signals:
			return exitOK
		case end := <-run.ended:
			if end.of != run.of {
				continue
			}
			if failure = end.err; failure != nil {
				return exitFailed
			}
			return exitOK
		case batch := <-run.batches:
			if batch.of == run.of {
				say(run.view.talk.heard(batch.turns)...)
			}
		case _, open := <-run.told:
			if !open { // the server went: looked at each second
				run.told, run.gone = nil, true
				continue
			}
			run.looks()
		case <-second.C:
			if run.told == nil || run.owed {
				run.looks()
			}
		case data, open := <-keys:
			if !open {
				return exitOK
			}
			events, rest, sight, leave := typed(buffer, data)
			if leave {
				return exitOK
			}
			buffer = rest
			for i, event := range events {
				deed := run.view.deed(event)
				if deed.do == chatLeave {
					return exitOK
				}
				if (deed.do == chatRead || deed.do == chatUnder) && i+1 < len(events) && events[i+1].E == "move" {
					continue // the keys held down: only where they came to rest is read
				}
				run.act(deed)
			}
			if sight {
				say(run.view.talk.said()...) // the pane came into sight: said again whole
			}
		}
	}
}
