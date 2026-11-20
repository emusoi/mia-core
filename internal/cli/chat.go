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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/model"
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
	// in front of the first thing sent to the agent that goes on with it:
	// what is pasted into a window in one go.
	chatCarry    = 48 << 10
	chatSaid     = 200
	chatHeads    = 300
	chatReadMost = 400
	// chatOwn is the place before the first turn of the app's own: after
	// any a transcript has. The window keeps a turn it was told and takes
	// another at its place only as a reply grown longer, so a turn of the
	// app's own has a place no other turn ever has, and is taken out by it.
	chatOwn = 1 << 30
	// chatRoom is how tall the conversation read back in Earlier is, in
	// points: what the window gives a chat among other pieces.
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

type chatButton struct {
	T     string `json:"t"`
	ID    string `json:"id"`
	Label string `json:"label"`
	Key   string `json:"key"`
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
// types to its agent under them with reply, and Stop while working.
type chatPiece struct {
	T       string            `json:"t"`
	ID      string            `json:"id"`
	Agent   string            `json:"agent"`
	Reply   bool              `json:"reply"`
	Working bool              `json:"working"`
	Turns   []json.RawMessage `json:"turns"`
}

// chatOp is one change of a patch: a turn put in the chat, or taken out
// of it by its place n; whether its agent is at work; words put in its
// field, on the clipboard, by the title as the status — none takes it
// away — or over the foot a few seconds as a toast; a piece put last in
// another, said again whole, or taken out.
type chatOp struct {
	Op   string          `json:"op"`
	ID   string          `json:"id,omitempty"`
	Turn json.RawMessage `json:"turn,omitempty"`
	On   *bool           `json:"on,omitempty"`
	N    int             `json:"n,omitempty"`
	Text string          `json:"text,omitempty"`
	Tone string          `json:"tone,omitempty"`
	Node json.RawMessage `json:"node,omitempty"`
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
	// offset is added to the place of each turn heard: the turns of the
	// conversation this one goes on from stand before them.
	offset int
	// over is the modal that stands over the chat, nil for none, and keys
	// the piece of it the keys are put on. A view said whole is said
	// without it and it is put back by a patch: the chat's turns are those
	// that fit a message whatever stands over them.
	over json.RawMessage
	keys string
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

// place is the next place for a turn of the app's own.
func (t *talk) place() int {
	t.placed++
	return chatOwn + t.placed
}

// extras is the app's own turns, each at a place of its own after the
// transcript's last: what was sent and is not in it yet, then what the
// window's screen asks, unless the transcript ends on a question nobody
// answered — that one is the asking.
func (t *talk) extras() []chatTurn {
	open := false
	if len(t.turns) > 0 {
		last := t.turns[len(t.turns)-1]
		open = last.kind == "question" && !t.answered[last.n]
	}
	var own []chatTurn
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
// who answers, and at its right the keys that show the models and the
// earlier conversations, buttons that tab reaches from the chat's field,
// which types every letter — and the chat with the last turns that fit.
// The keys are put on the chat's field, or on the modal's piece while one
// stands over it.
func (t *talk) whole() []byte {
	own := t.extras()
	chat := chatPiece{T: "chat", ID: t.id(), Agent: t.agent, Reply: t.reply, Working: t.working, Turns: []json.RawMessage{}}
	var buttons []json.RawMessage
	if t.reply {
		buttons = append(buttons, encode(chatButton{T: "button", ID: "open-models", Label: "models", Key: "m"}))
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
		all = append(all, t.patch(encode(chatOp{Op: "add", ID: "view", Node: t.over}))...)
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
		if !drawn(turn) {
			continue
		}
		if turn.N += t.offset; turn.Question > 0 {
			turn.Question += t.offset
		}
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

// goesOn makes the chat go on from another: that one's turns stay, over a
// quiet line that says who goes on, and what this one hears comes after.
func (t *talk) goesOn(from *talk) {
	last := 0
	if len(from.turns) > 0 {
		last = from.turns[len(from.turns)-1].n
	}
	line := worded(app.Turn{N: last + 1, Kind: "tool", Text: "continued by " + t.agent})
	t.turns, t.offset, t.answered = append(slices.Clone(from.turns), line), line.n, from.answered
}

// chatRule is the line between a conversation said in front of what the
// person sends and their own words.
const chatRule = "----- end of the conversation so far -----"

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
	return "The conversation so far, with " + from + ":\n\n" + strings.TrimSpace(transcript) + "\n\n" + chatRule + "\n\n" + words
}

// ownWords is what the person themselves sent, of a message that may have
// a conversation said in front of it: what follows the rule.
func ownWords(text string) string {
	if at := strings.LastIndex(text, chatRule); at >= 0 {
		return strings.TrimSpace(text[at+len(chatRule):])
	}
	return text
}

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
// to pick from, or the worktree's earlier conversations.
type chatMode int

const (
	chatting chatMode = iota
	picking
	looking
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
	list := chatList{T: "list", ID: "models", Max: 11, Menu: "effort", Items: []chatItem{}}
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
		}
		if row.used {
			head = chatItem{Head: "Used here"}
		}
		if head.Head != group {
			group = head.Head
			list.Items = append(list.Items, head)
		}
		item := chatItem{ID: row.id(), Title: row.label, Icon: chatIcon(row.agent), Does: "go on in " + row.agent}
		item.Checked = mine && row.model == v.runs.Model && row.model != "" && (!row.used || row.effort == v.runs.Effort)
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
		if placed && item.Checked && list.Selected == "" {
			list.Selected = item.ID
		}
		list.Items = append(list.Items, item)
	}
	field := chatField{T: "field", ID: "q", Placeholder: "Search models", Icon: "search", Right: counted(len(rows), "model"), Bare: true, Live: true, List: "models"}
	inside := []json.RawMessage{encode(field), encode(chatBox{T: "divider"}), encode(list)}
	return encode(chatModal{T: "modal", ID: chatOver, Title: "Model", W: 520, Children: append(inside, v.foot()...)})
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
// one under the keys read back, a chat with no field in a room of its own
// height. An item's menu is what can be done to it: taken up again or gone
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
	// A modal's row stands its sides at its middle, and a chat in it is
	// chatRoom points tall: heads with no words, which the keys pass over,
	// make a short list as tall, so that it begins at the top.
	if rows := len(list.Items); rows > 0 {
		for tall := 30*rows + 40; tall <= chatRoom-12; tall += 30 {
			list.Items = append(list.Items, chatItem{Head: " "})
		}
	}
	reading, _ := v.headOf(v.on)
	read := chatPiece{T: "chat", ID: cutTo("read-"+v.on, chatSaid), Agent: reading.Agent, Turns: []json.RawMessage{}}
	modal := func() json.RawMessage {
		inside := []json.RawMessage{
			encode(chatWords{T: "text", Text: "in " + v.worktree + " · " + counted(len(v.heads), "conversation"), Style: "dim"}),
			encode(chatBox{T: "row", Split: true, Gap: 16, Children: []json.RawMessage{
				encode(chatBox{T: "col", W: 300, Children: []json.RawMessage{encode(list)}}),
				encode(chatBox{T: "col", H: chatRoom, Children: []json.RawMessage{encode(read)}}),
			}}),
		}
		return encode(chatModal{T: "modal", ID: chatOver, Title: "Earlier", W: 720, Children: append(inside, v.foot()...)})
	}
	for _, turn := range lastThatFit(v.read, muiLimit-2*patchEnvelope-len(modal()), chatReadMost) {
		read.Turns = append(read.Turns, turn.json)
	}
	return modal()
}

// modal is the modal of the mode.
func (v *chatView) modal(placed bool) json.RawMessage {
	if v.mode == picking {
		return v.models(placed)
	}
	return v.earlier(placed)
}

// up is the patch that puts a modal over the chat, the keys' row placed
// in it. The window puts the keys on the first piece of it that takes
// them, the search line or the list, which a view said whole names too.
func (v *chatView) up(mode chatMode) [][]byte {
	v.mode = mode
	v.talk.over, v.talk.keys = v.modal(false), map[chatMode]string{picking: "q", looking: "earlier"}[mode]
	return v.talk.patch(encode(chatOp{Op: "add", ID: "view", Node: v.modal(true)}))
}

// again is the patch that says the modal again as it now is: the keys
// stay where they are, and what is typed in its field.
func (v *chatView) again() [][]byte {
	v.talk.over = v.modal(false)
	return v.talk.patch(encode(chatOp{Op: "set", ID: chatOver, Node: v.talk.over}))
}

// shut is the patch that takes the modal away, and makes with it the
// changes that say what was done in it; the chat behind is as it was.
func (v *chatView) shut(with ...json.RawMessage) [][]byte {
	v.mode, v.told, v.talk.over, v.talk.keys = chatting, nil, nil, ""
	return v.talk.patch(append([]json.RawMessage{encode(chatOp{Op: "remove", ID: chatOver})}, with...)...)
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
)

// chatDeed is what an event does: with the words, the conversation, the
// model and effort or the question and option it needs.
type chatDeed struct {
	do               chatDo
	text, id, agent  string
	model, effort    string
	question, option int
}

// deed is what an event asks of the app as the view stands. In the chat a
// conversation no window has open is only shown: nothing is sent,
// answered or stopped. A model of this chat's agent is switched to; one of
// another agent hands the conversation to that agent, when there is one
// mia can read. In Earlier ⏎
// on the conversation a window has open shows it and on another takes it
// up again, as its menu's first item does; c says what handing it over
// does the first time it is pressed on a conversation, and does it the
// second, and the menu's item, which names it, does it at once. esc in a
// modal takes it away, and nothing else is done.
func (v *chatView) deed(event chatEvent) chatDeed {
	switch {
	case event.E == "close":
		return chatDeed{do: chatLeave}
	case v.mode != chatting && event.E == "dismiss" && event.ID == chatOver:
		return chatDeed{do: chatBack}
	case v.mode == picking && event.E == "change" && event.ID == "q":
		return chatDeed{do: chatNarrow, text: event.Text}
	case v.mode == picking && event.E == "pick" && event.List == "models":
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
		case row.agent != v.agent() && v.id == "":
			return chatDeed{do: chatSay, text: "nothing is said here that mia can read yet: there is nothing for " + row.agent + " to go on with"}
		case row.agent != v.agent():
			return chatDeed{do: chatHand, agent: row.agent, model: row.model, effort: effort}
		case row.model == "" && effort == "":
			return chatDeed{do: chatSay, text: row.agent + " is started with no model, and takes none from mia: it is chosen in its terminal"}
		case !agent.Live && !agent.Again:
			return chatDeed{do: chatSay, text: row.agent + " cannot be switched to another model while it runs — restart it with --model"}
		}
		return chatDeed{do: chatSwitch, model: row.model, effort: effort}
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
	case event.E == "send" && strings.TrimSpace(event.Text) != "":
		return chatDeed{do: chatSend, text: strings.TrimSpace(event.Text)}
	case event.E == "choose":
		return chatDeed{do: chatChoose, question: event.Question, option: event.Option}
	case event.E == "stop":
		return chatDeed{do: chatStop}
	}
	return chatDeed{}
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
	CloseWindow(record model.Record, window string) error
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
// started on a model and an effort in a window of its own, told nothing.
// Only once that agent runs is the window of the one it was with closed,
// so one agent goes on; window is that window, "" when it is not to be
// closed. Refused, or the other agent not running, nothing is closed and
// the reason is said.
func handTo(mia handing, record model.Record, id, window, from, to, model, effort string) handed {
	done, err := mia.ContinueConversation(app.Continue{ID: id, Worktree: record.Name, With: to, Model: model, Effort: effort, Quiet: true})
	switch {
	case err != nil:
		return handed{text: err.Error(), tone: "bad"}
	case !done.Sent:
		return handed{done: done, tone: "bad", text: from + " still runs — " + done.Problem}
	case window == "":
		return handed{done: done, went: true, tone: "ok", text: to + " goes on with this conversation · " + from + "'s window stays"}
	}
	if err := mia.CloseWindow(record, window); err != nil {
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

// tell says at the modal's foot why what was asked in it was not done.
func (r *chatRun) tell(why error) {
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
	v.mode, v.told, v.window, v.id = chatting, nil, window, head.ID
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
	if carried, on := r.a.CarriedInWindow(r.record, window); window != "" && on {
		// its agent goes on from another's conversation: that one's turns stay over what it says
		before := &talk{}
		turns, _ := r.a.ConversationIn(carried.ID, r.record.Name, 0)
		before.heard(turns)
		v.talk.goesOn(before)
	}
	if head.ID != "" {
		r.follow(head.ID)
	}
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

// looks is what a second brings: the conversation of a window that had
// none, once it has one; whether the agent is at work; and what its
// window's screen asks.
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
				v.runs, v.talk.agent, v.talk.about = runs, runs.Agent, chatAbout(runs)
				r.say(v.talk.said()...)
			}
			r.follow(head.ID)
		}
	}
	r.say(v.talk.atWork(r.a.AtWork(r.record, r.index()))...)
	if time.Since(r.pressed) < 2*time.Second {
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
		r.note("", "")
		r.say(v.talk.sends(deed.text)...)
		send := r.a.SendToWindow
		words, back := typedFor(r.a, r.record, v.window, deed.text, os.ReadFile)
		if strings.Contains(words, "\n") {
			send = r.a.SayToWindow // pasted: a line break in it is not an Enter
		}
		if err := send(r.record, v.window, words); err != nil {
			back()
			r.say(v.talk.refused(err)...)
		}
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
	case chatModelsUp:
		v.agents, v.setups, v.query = r.a.AgentModels(), r.a.Setups(filepath.Base(r.a.Root)), ""
		r.say(v.up(picking)...)
	case chatEarlier:
		heads, _ := r.a.Conversations(app.Wanted{Worktree: r.record.Name})
		v.listed(heads, time.Now())
		r.read(v.on)
		r.say(v.up(looking)...)
	case chatBack:
		r.say(v.shut()...)
	case chatNarrow:
		v.query, v.told = deed.text, nil
		r.say(v.again()...)
	case chatSay:
		v.armed, v.told = deed.id, &muiStatus{Text: deed.text, Tone: "warn"}
		r.say(v.again()...)
	case chatCopy:
		r.say(v.talk.patch(encode(chatOp{Op: "copy", Text: deed.id}), v.talk.went("copied "+deed.id, "ok"))...)
	case chatSwitch:
		now, err := r.a.SwitchWindow(r.record, v.window, deed.model, deed.effort)
		if err != nil {
			r.tell(err)
			return
		}
		v.runs.Model, v.runs.Effort = now.Model, now.Effort
		v.talk.about = chatAbout(v.runs)
		r.say(v.shut(encode(chatOp{Op: "set", ID: "about", Node: chatHead(v.talk.about)}),
			v.talk.went(v.runs.Agent+" switched to "+strings.TrimSpace(deed.model+" "+deed.effort), "ok"))...)
	case chatHand:
		from, window := v.agent(), v.window
		if r.a.WindowHasPane(r.record, window, v.talk.pane) {
			window = "" // this chat's own window: closing it would end the chat
		}
		r.say(v.shut(v.talk.stands(&muiStatus{Text: cutTo("handing this conversation to "+deed.agent+"…", chatSaid)}))...)
		went := handTo(r.a, r.record, v.id, window, from, deed.agent, deed.model, deed.effort)
		if !went.went {
			r.note(went.text, went.tone)
			return
		}
		done := went.done
		to := r.nowAt(done.To.Window)
		// kept on its window: whichever chat of it sends first says it, this one or one started after
		kept := r.a.CarryInWindow(r.record, to, app.Carried{From: from, ID: v.id, File: done.Handoff, Full: done.Full})
		r.aim(app.Head{Agent: done.To.Agent, Model: done.Model, Effort: done.Effort, Account: done.Account}, to)
		if kept != nil {
			went.text, went.tone = went.text+" · "+kept.Error(), "warn"
		}
		r.shows(went.text, went.tone)
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
		went := handTo(r.a, r.record, deed.id, "", head.Agent, v.agent(), "", "")
		if !went.went {
			v.armed = ""
			r.note(went.text, went.tone)
			return
		}
		done := went.done
		to := r.nowAt(done.To.Window)
		kept := r.a.CarryInWindow(r.record, to, app.Carried{From: head.Agent, ID: deed.id, File: done.Handoff, Full: done.Full})
		r.aim(app.Head{Agent: done.To.Agent, Model: done.Model, Effort: done.Effort, Account: done.Account}, to)
		if kept != nil {
			went.text, went.tone = went.text+" · "+kept.Error(), "warn"
		}
		r.shows(went.text, went.tone)
	}
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
		case <-second.C:
			run.looks()
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
				if deed.do == chatRead && i+1 < len(events) && events[i+1].E == "move" {
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
