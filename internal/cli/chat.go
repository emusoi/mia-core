package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
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
const chatTurns = 1500

// patchEnvelope is more than a patch's message is around its changes: its
// version and its pane.
const patchEnvelope = 64

// What the window takes besides a chat's turns: chatFrame is more than the
// pieces a view has around them, chatSaid the characters it keeps of a
// status and of a list's words, chatHeads how many conversations Earlier
// lists, and chatReadMost how many turns of one it reads back.
const (
	chatFrame = 16
	// chatCarry is how many bytes of a conversation gone on from are said
	// in front of the first thing sent to the agent that goes on with it:
	// what is pasted into a window in one go.
	chatCarry    = 48 << 10
	chatSaid     = 200
	chatHeads    = 300
	chatReadMost = 400
)

// chatMessage is a view said whole: a UI that fills its pane, the piece
// the keys are put on (once each time another is named), how things stand
// at the right of its top line, and the keys it takes whatever piece has
// them.
type chatMessage struct {
	V      int             `json:"v"`
	Pane   string          `json:"pane,omitempty"`
	Fill   bool            `json:"fill"`
	Focus  string          `json:"focus,omitempty"`
	Status *muiStatus      `json:"status,omitempty"`
	View   json.RawMessage `json:"view"`
	Keys   []chatKey       `json:"keys,omitempty"`
}

// chatKey is a key the view takes, or an item of a list while the keys are
// on it: what it is called along the foot, and the id it is said by.
type chatKey struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	ID    string `json:"id"`
}

// chatBox is a col or a row of pieces.
type chatBox struct {
	T        string            `json:"t"`
	Children []json.RawMessage `json:"children,omitempty"`
	Gap      int               `json:"gap,omitempty"`
	Pad      int               `json:"pad,omitempty"`
	Grow     int               `json:"grow,omitempty"`
	W        int               `json:"w,omitempty"`
	Scroll   bool              `json:"scroll,omitempty"`
	Split    bool              `json:"split,omitempty"`
}

// chatWords is a text; chatButton a button its key presses from anywhere
// but a field; chatField the search line that walks a list.
type chatWords struct {
	T     string `json:"t"`
	Text  string `json:"text"`
	Style string `json:"style,omitempty"`
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

// chatList is a list: the models, each with its efforts as its menu, or
// the worktree's conversations, the row under the keys said as it changes.
type chatList struct {
	T        string     `json:"t"`
	ID       string     `json:"id"`
	Live     bool       `json:"live,omitempty"`
	Flat     bool       `json:"flat,omitempty"`
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

// chatOp is one change of a patch to the chat: a turn put in it, or
// whether its agent is at work.
type chatOp struct {
	Op   string          `json:"op"`
	ID   string          `json:"id"`
	Turn json.RawMessage `json:"turn,omitempty"`
	On   *bool           `json:"on,omitempty"`
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

// patches is turns as patches of turn changes to the chat of an id, in
// order: each patch no more changes and no more bytes than the window
// takes.
func patches(pane, id string, turns []chatTurn) [][]byte {
	var all [][]byte
	var ops []json.RawMessage
	size := patchEnvelope
	flush := func() {
		if len(ops) > 0 {
			all = append(all, encode(muiMessage{V: 1, Pane: pane, Patch: ops}))
		}
		ops, size = nil, patchEnvelope
	}
	for _, turn := range turns {
		op := encode(chatOp{Op: "turn", ID: id, Turn: turn.json})
		if len(ops) == muiOps || size+len(op)+1 > muiLimit {
			flush()
		}
		ops, size = append(ops, op), size+len(op)+1
	}
	flush()
	return all
}

// chatLine is a quiet line of a view: words, and at its right end what
// follows them.
func chatLine(words string, right ...json.RawMessage) json.RawMessage {
	left := []json.RawMessage{encode(chatWords{T: "text", Text: words, Style: "dim"}), encode(chatBox{T: "col", Grow: 1})}
	return encode(chatBox{T: "row", Pad: 8, Gap: 8, Children: append(left, right...)})
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
	// the first sent first: each is said as an asked turn after its last.
	sent []string
	// asks is what the window's screen asks that the transcript does not: a
	// permission, a folder to trust. It is said as a question after those.
	asks *app.Asks
	// own is those turns of the app's own that the window was told, by
	// their place: what each said.
	own      map[int]string
	answered map[int]bool // the transcript's questions that were answered
	// anew is how many times the chat was begun anew, which its id says:
	// the window keeps a turn it was told, and takes another at its place
	// only as a reply grown longer, so a turn taken back or said otherwise
	// is said in a chat of another id.
	anew int
	// offset is added to the place of each turn heard: the turns of the
	// conversation this one goes on from stand before them.
	offset int
}

// id is the chat piece's id.
func (t *talk) id() string {
	if t.anew == 0 {
		return "chat"
	}
	return "chat-" + strconv.Itoa(t.anew)
}

// find is where the transcript's turn n is among the turns, or would be.
func (t *talk) find(n int) (int, bool) {
	return slices.BinarySearchFunc(t.turns, n, func(have chatTurn, n int) int { return cmp.Compare(have.n, n) })
}

// extras is the app's own turns, placed after the transcript's last: what
// was sent and is not in it yet, then what the window's screen asks,
// unless the transcript ends on a question nobody answered — that one is
// the asking.
func (t *talk) extras() []chatTurn {
	n, open := 0, false
	if len(t.turns) > 0 {
		last := t.turns[len(t.turns)-1]
		n, open = last.n, last.kind == "question" && !t.answered[last.n]
	}
	var own []chatTurn
	for _, text := range t.sent {
		n++
		own = append(own, worded(app.Turn{N: n, Kind: "asked", Text: text}))
	}
	if t.asks != nil && !open {
		own = append(own, worded(app.Turn{N: n + 1, Kind: "question", Text: cmp.Or(t.asks.Text, t.agent+" asks"), Options: t.asks.Options}))
	}
	return own
}

// whole is the message that says the whole view: the line over the chat —
// who answers, and at its right the keys that show the models and the
// earlier conversations, buttons that tab reaches from the chat's field,
// which types every letter — and the chat with the last turns that fit.
func (t *talk) whole() []byte {
	own := t.extras()
	chat := chatPiece{T: "chat", ID: t.id(), Agent: t.agent, Reply: t.reply, Working: t.working, Turns: []json.RawMessage{}}
	var buttons []json.RawMessage
	if t.reply {
		buttons = append(buttons, encode(chatButton{T: "button", ID: "models", Label: "models", Key: "m"}))
	}
	buttons = append(buttons, encode(chatButton{T: "button", ID: "earlier", Label: "earlier", Key: "e"}))
	view := func() json.RawMessage {
		return encode(chatBox{T: "col", Children: []json.RawMessage{chatLine(t.about, buttons...), encode(chat)}})
	}
	message := chatMessage{V: 1, Pane: t.pane, Fill: true, Status: t.status}
	if t.reply {
		message.Focus = chat.ID
	}
	room := muiLimit - len(encode(message)) - len(view())
	for _, turn := range own {
		room -= len(turn.json) + 1
	}
	t.shown, t.own = map[int]bool{}, map[int]string{}
	for _, turn := range lastThatFit(t.turns, room, chatTurns) {
		chat.Turns = append(chat.Turns, turn.json)
		t.shown[turn.n] = true
	}
	for _, turn := range own {
		chat.Turns = append(chat.Turns, turn.json)
		t.shown[turn.n], t.own[turn.n] = true, turn.words
	}
	message.View = view()
	return encode(message)
}

// says is the messages that make the window's chat what it now is, once
// the view was said: changed are the transcript's turns that came or grew.
// What is new is a patch of turn changes; the whole view is said when it
// would hold more turns than the window takes, and in a chat begun anew
// when a turn of the app's own that the window was told is no longer what
// stands at its place.
func (t *talk) says(changed []chatTurn) [][]byte {
	if t.shown == nil {
		return nil
	}
	own := t.extras()
	wanted := map[int]string{}
	for _, turn := range own {
		wanted[turn.n] = turn.words
	}
	for n, was := range t.own {
		now, stands := wanted[n]
		if at, found := t.find(n); !stands && found {
			now, stands = t.turns[at].words, true // the transcript's own turn came where it was said
		}
		if !stands || now != was {
			t.anew++
			return [][]byte{t.whole()}
		}
	}
	for _, turn := range own {
		if _, told := t.own[turn.n]; !told {
			changed = append(changed, turn)
		}
	}
	more := 0
	for _, turn := range changed {
		if !t.shown[turn.n] {
			more++
		}
	}
	if len(t.shown)+more > muiPieces-chatFrame {
		return [][]byte{t.whole()}
	}
	for _, turn := range changed {
		t.shown[turn.n] = true
	}
	clear(t.own)
	for _, turn := range own {
		t.own[turn.n] = turn.words
	}
	return patches(t.pane, t.id(), changed)
}

// heard takes the turns a followed conversation gained or that grew, and
// is the messages that say them: the whole view when it was not said yet.
// A turn that says what was sent from here takes the place of the one said
// for it at once; when another comes first — the agent was at work, and
// takes up what was sent later — that one is taken back, and the status
// says it waits.
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
			if echo := slices.Index(t.sent, strings.TrimSpace(turn.Text)); turn.Kind == "asked" && echo >= 0 {
				t.sent = slices.Delete(t.sent, echo, echo+1)
			} else {
				passed = true
			}
		}
		changed = append(changed, one)
	}
	if passed && len(t.sent) > 0 {
		t.status = &muiStatus{Text: cutTo(t.agent+" takes it up when it can — sent: "+strings.Join(t.sent, " · "), chatSaid)}
		t.sent = nil
		if t.shown != nil {
			t.anew++ // with the status, which only a view said whole carries
		}
		return [][]byte{t.whole()}
	}
	if t.shown == nil {
		return [][]byte{t.whole()}
	}
	return t.says(changed)
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
	if t.shown == nil {
		return nil
	}
	return [][]byte{encode(muiMessage{V: 1, Pane: t.pane, Patch: []json.RawMessage{encode(chatOp{Op: "working", ID: t.id(), On: &on})}})}
}

// sends takes what the person sent, and is the patch that says it at once
// as an asked turn after the last: the transcript's own turn takes its
// place when the agent writes it.
func (t *talk) sends(text string) [][]byte {
	t.sent = append(t.sent, text)
	return t.says(nil)
}

// unsent is what is said of words that did not go: why, and the words —
// the chat's field cannot be handed them back — each cut so that both fit
// what the window keeps of a status.
func unsent(why error, words string) string {
	words = cutTo(strings.Join(strings.Fields(words), " "), chatSaid/2)
	const between = " — not sent: "
	return cutTo(why.Error(), chatSaid-len([]rune(words))-len([]rune(between))) + between + words
}

// refused takes that the last thing sent did not go, and is the message
// that takes its turn back and says why with the words.
func (t *talk) refused(why error) [][]byte {
	if len(t.sent) == 0 {
		return nil
	}
	words := t.sent[len(t.sent)-1]
	t.sent = t.sent[:len(t.sent)-1]
	t.status = &muiStatus{Text: unsent(why, words), Tone: "bad"}
	t.anew++
	return [][]byte{t.whole()}
}

// asked takes what the window's screen asks now, nil when nothing, and is
// the messages that say it as a question of the app's own after the last
// turn, or take that question out once it is answered.
func (t *talk) asked(asks *app.Asks) [][]byte {
	same := func(a, b *app.Asks) bool {
		return a == nil && b == nil || a != nil && b != nil && a.Text == b.Text && slices.Equal(a.Options, b.Options)
	}
	if same(t.asks, asks) {
		return nil
	}
	t.asks = asks
	return t.says(nil)
}

// reopened is the message that says the chat again with each question
// open: the window writes an answer under a question as it is chosen, and
// one the agent's window refused was not given.
func (t *talk) reopened(why error) [][]byte {
	t.status = &muiStatus{Text: cutTo(why.Error(), chatSaid), Tone: "bad"}
	t.anew++
	return [][]byte{t.whole()}
}

// chatMode is what stands in the pane: the chat, the models to pick from,
// or the worktree's earlier conversations.
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
	// carry is the conversation this chat's agent goes on from, until the
	// first thing is sent to it: said in front of that.
	carry *chatCarried

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
}

// chatCarried is a conversation handed to another agent: whose it was, the
// file mia wrote it out to, and what the file says.
type chatCarried struct{ from, file, text string }

// toType is what is typed into the agent's window for words the person
// sent: the words, and in front of the first after a conversation was
// handed to this agent, that conversation.
func (v *chatView) toType(words string) string {
	if v.carry == nil {
		return words
	}
	return carried(v.carry.from, v.carry.text, v.carry.file, words, chatCarry)
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

// models is the message that says the models in place of the chat: a bare
// search line over one list, "Used here" first and then each agent's
// under its head, the one that runs checked, an item's menu its agent's
// efforts. placed puts the keys' row on the one that runs.
func (v *chatView) models(placed bool) []byte {
	rows := chatModels(v.agents, v.setups, v.agent(), v.query)
	list := chatList{T: "list", ID: "models", Menu: "effort", Items: []chatItem{}}
	group := ""
	for _, row := range rows {
		var agent app.AgentModels
		for _, one := range v.agents {
			if one.Name == row.agent {
				agent = one
			}
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
		default:
			item.Does = "nothing"
			item.About = []string{row.agent, "cannot be switched while it runs"}
		}
		if row.model == "" && !row.used {
			item.Note = "its model is chosen inside it"
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
	return encode(chatMessage{V: 1, Pane: v.talk.pane, Fill: true, Focus: "q", Status: v.talk.status,
		Keys: []chatKey{{Key: "esc", Label: "back to the chat", ID: "back"}},
		View: encode(chatBox{T: "col", Children: []json.RawMessage{
			encode(field), encode(chatBox{T: "divider"}),
			encode(chatBox{T: "col", Grow: 1, Scroll: true, Children: []json.RawMessage{encode(list)}}),
		}})})
}

// headOf is the conversation of an id among those Earlier lists.
func (v *chatView) headOf(id string) (app.Head, bool) {
	at := slices.IndexFunc(v.heads, func(h app.Head) bool { return h.ID == id })
	if at < 0 {
		return app.Head{}, false
	}
	return v.heads[at], true
}

// earlier is the message that says the worktree's conversations in place
// of the chat: a list of them — what was first asked, the agent and when
// at the right, the one a window has open marked as running — and beside
// it the one under the keys read back, a chat with no field. On one of
// another agent c hands it to this chat's. placed puts the keys' row on
// the conversation read back.
func (v *chatView) earlier(placed bool) []byte {
	list := chatList{T: "list", ID: "earlier", Live: true, Flat: true, Items: []chatItem{}}
	if placed {
		list.Selected = v.on
	}
	for _, head := range v.heads {
		running := head.Running && head.Window != nil
		item := chatItem{ID: head.ID, Title: cmp.Or(cutTo(head.Asked, chatSaid), "nothing asked yet"), Icon: chatIcon(head.Agent),
			Right: head.Agent + " · " + gitAge(v.now.Sub(head.Active)), Running: &running, Does: "go on with it"}
		if running {
			item.Does = "back to it"
		}
		if head.Agent != v.agent() {
			item.Keys = []chatKey{{Key: "c", Label: "continue in " + v.agent(), ID: "continue"}}
		}
		list.Items = append(list.Items, item)
	}
	reading, _ := v.headOf(v.on)
	read := chatPiece{T: "chat", ID: cutTo("read-"+v.on, chatSaid), Agent: reading.Agent, Turns: []json.RawMessage{}}
	message := chatMessage{V: 1, Pane: v.talk.pane, Fill: true, Focus: "earlier", Status: v.talk.status,
		Keys: []chatKey{{Key: "esc", Label: "back to the chat", ID: "back"}}}
	view := func() json.RawMessage {
		return encode(chatBox{T: "col", Children: []json.RawMessage{
			chatLine("Earlier in " + v.worktree + " · " + counted(len(v.heads), "conversation")),
			encode(chatBox{T: "row", Split: true, Grow: 1, Children: []json.RawMessage{
				encode(chatBox{T: "col", W: 360, Scroll: true, Children: []json.RawMessage{encode(list)}}),
				encode(chatBox{T: "divider"}), encode(read),
			}}),
		}})
	}
	for _, turn := range lastThatFit(v.read, muiLimit-len(encode(message))-len(view()), min(chatReadMost, muiPieces-chatFrame-len(list.Items))) {
		read.Turns = append(read.Turns, turn.json)
	}
	message.View = view()
	return encode(message)
}

// whole is the message that says what stands in the pane now.
func (v *chatView) whole(placed bool) []byte {
	switch v.mode {
	case picking:
		return v.models(placed)
	case looking:
		return v.earlier(placed)
	}
	return v.talk.whole()
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
// moved; the search line changed; close.
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
	chatBack            // back to the chat
	chatNarrow          // the models that hold the words
	chatSwitch          // switch this window's agent to a model and an effort
	chatHand            // hand this conversation to another agent, and go on with that one
	chatSay             // only say something as the status
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
// up again, and c says what handing it over does the first time it is
// pressed on a conversation, and does it the second.
func (v *chatView) deed(event chatEvent) chatDeed {
	back := event.E == "press" && event.ID == "back"
	switch {
	case event.E == "close":
		return chatDeed{do: chatLeave}
	case v.mode == picking && back, v.mode == looking && back:
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
		case !agent.Live:
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
	case event.E == "press" && event.ID == "earlier":
		return chatDeed{do: chatEarlier}
	case v.window == "":
	case event.E == "press" && event.ID == "models":
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

// out says the chat's own messages, while the chat is what stands in the
// pane: it is said whole when the person comes back to it.
func (r *chatRun) out(messages [][]byte) {
	if r.view.mode == chatting {
		r.say(messages...)
	}
}

// note says how the last thing done went as the status, with the view.
func (r *chatRun) note(text, tone string) {
	r.view.talk.status = nil
	if text != "" {
		r.view.talk.status = &muiStatus{Text: cutTo(text, chatSaid), Tone: tone}
	}
}

// aim makes the chat one of a window, of a conversation, or of both: the
// conversation is followed from its first turn, and a window with none
// mia can read yet is a chat all the same — what is sent is typed to it
// and shown, and its conversation is looked for each second. The chat
// before it is let go of, and this one begun anew.
func (r *chatRun) aim(head app.Head, window string) {
	if r.quit != nil {
		close(r.quit)
	}
	r.of, r.quit = r.of+1, make(chan struct{})
	v, anew := r.view, 0
	if v.talk != nil {
		anew = v.talk.anew + 1
	}
	v.mode, v.window, v.id = chatting, window, head.ID
	v.runs = app.Runs{Agent: head.Agent, Model: head.Model, Effort: head.Effort, Account: head.Account, Reads: true}
	if window != "" {
		if runs := r.a.WindowRuns(r.record, window); runs.Agent != "" {
			v.runs = runs
		}
	}
	v.talk = &talk{pane: os.Getenv("TMUX_PANE"), agent: cmp.Or(v.runs.Agent, "window "+window), about: chatAbout(v.runs), reply: window != "", anew: anew}
	if window != "" {
		v.talk.working = r.a.AtWork(r.record, r.index())
		if v.runs.Agent != "" && !v.runs.Reads {
			r.note(v.runs.Agent+"'s replies are in its terminal: mia reads none of its conversations. What is sent here is typed to it", "warn")
		}
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
			if runs := r.a.WindowRuns(r.record, v.window); runs.Agent != "" && runs != v.runs {
				v.runs, v.talk.agent, v.talk.about = runs, runs.Agent, chatAbout(runs)
				r.out([][]byte{v.talk.whole()})
			}
			r.follow(head.ID)
		}
	}
	r.out(v.talk.atWork(r.a.AtWork(r.record, r.index())))
	if time.Since(r.pressed) < 2*time.Second {
		return
	}
	var asks *app.Asks
	if now, on := r.a.AsksInWindow(r.record, v.window); on {
		asks = &now
	}
	r.out(v.talk.asked(asks))
}

// act does a deed, and reports whether the view is to be said again
// whole, and whether with where the keys are.
func (r *chatRun) act(deed chatDeed) (again, placed bool) {
	v := r.view
	was := v.talk.status
	switch deed.do {
	case chatNothing, chatLeave:
		return false, false
	case chatSend:
		v.talk.status = nil
		r.out(v.talk.sends(deed.text))
		send, words := r.a.SendToWindow, v.toType(deed.text)
		if strings.Contains(words, "\n") {
			send = r.a.SayToWindow // pasted: a line break in it is not an Enter
		}
		if err := send(r.record, v.window, words); err != nil {
			r.out(v.talk.refused(err))
			return false, false
		}
		v.carry = nil
		return was != nil, false
	case chatChoose:
		v.talk.status = nil
		if err := r.a.PressInWindow(r.record, v.window, strconv.Itoa(deed.option+1)); err != nil {
			r.out(v.talk.reopened(err))
			return false, false
		}
		// answered: the question of the app's own goes, until the screen asks again
		r.pressed = time.Now()
		r.out(v.talk.asked(nil))
		return was != nil, false
	case chatStop:
		r.note("", "")
		if err := r.a.StopInWindow(r.record, v.window); err != nil {
			r.note(err.Error(), "bad")
		}
		return was != v.talk.status, false
	case chatModelsUp:
		v.agents, v.setups, v.query = r.a.AgentModels(), r.a.Setups(filepath.Base(r.a.Root)), ""
		v.mode = picking
		r.note("", "")
		return true, true
	case chatEarlier:
		heads, _ := r.a.Conversations(app.Wanted{Worktree: r.record.Name})
		v.listed(heads, time.Now())
		v.mode = looking
		r.note("", "")
		r.read(v.on)
		return true, true
	case chatBack:
		v.mode = chatting
		r.note("", "")
		return true, true
	case chatNarrow:
		v.query = deed.text
		return true, false
	case chatSay:
		v.armed = deed.id
		r.note(deed.text, "warn")
		return true, false
	case chatSwitch:
		now, err := r.a.SwitchWindow(r.record, v.window, deed.model, deed.effort)
		if err != nil {
			r.note(err.Error(), "bad")
			return true, false
		}
		v.runs.Model, v.runs.Effort = now.Model, now.Effort
		v.talk.about, v.mode = chatAbout(v.runs), chatting
		r.note(v.runs.Agent+" switched to "+strings.TrimSpace(deed.model+" "+deed.effort), "ok")
		return true, true
	case chatHand:
		from, window := v.agent(), v.window
		if r.a.WindowHasPane(r.record, window, v.talk.pane) {
			window = "" // this chat's own window: closing it would end the chat
		}
		r.note("handing this conversation to "+deed.agent+"…", "")
		r.say(v.whole(false))
		went := handTo(r.a, r.record, v.id, window, from, deed.agent, deed.model, deed.effort)
		if went.went {
			before, done := v.talk, went.done
			r.aim(app.Head{Agent: done.To.Agent, Model: done.Model, Effort: done.Effort, Account: done.Account}, strconv.Itoa(done.To.Window.Index))
			v.talk.goesOn(before)
			text, _ := os.ReadFile(done.Handoff) // mia's own file of it, written a moment ago
			v.carry = &chatCarried{from: from, file: cmp.Or(done.Full, done.Handoff), text: string(text)}
		}
		v.mode = chatting
		r.note(went.text, went.tone)
		return true, true
	case chatRead:
		r.read(deed.id)
		return true, false
	case chatShow:
		head, _ := v.headOf(deed.id)
		if head.ID != v.id || strconv.Itoa(head.Window.Index) != v.window {
			r.aim(head, strconv.Itoa(head.Window.Index))
		}
		v.mode = chatting
		r.note("", "")
		return true, true
	case chatGoOn:
		done, err := r.a.ResumeConversation(deed.id, r.record.Name, r.record.Name, "", "", "")
		if err != nil {
			r.note(err.Error(), "bad")
			return true, false
		}
		head, _ := v.headOf(deed.id)
		head.Model, head.Effort, head.Account = done.Model, done.Effort, cmp.Or(done.Account, head.Account)
		r.aim(head, strconv.Itoa(done.Window.Index))
		r.note(head.Agent+" goes on with it in window "+done.Window.Name, "ok")
		return true, true
	case chatContinue:
		done, err := r.a.ContinueConversation(app.Continue{ID: deed.id, Worktree: r.record.Name, With: v.agent()})
		if err != nil {
			v.armed = ""
			r.note(err.Error(), "bad")
			return true, false
		}
		r.aim(app.Head{Agent: done.To.Agent, Model: done.Model, Effort: done.Effort, Account: done.Account}, strconv.Itoa(done.To.Window.Index))
		r.note(fmt.Sprintf("%s goes on from %s's conversation in window %s, told to read %s", done.To.Agent, done.From.Agent, done.To.Window.Name, filepath.Base(done.Handoff)), "ok")
		if !done.Sent {
			r.note(done.To.Agent+" was not told to read the handoff: "+done.Problem, "bad")
		}
		return true, true
	}
	return false, false
}

// read reads back the conversation under the keys in Earlier.
func (r *chatRun) read(id string) {
	if id == "" {
		return
	}
	turns, err := r.a.ConversationIn(id, r.record.Name, 0)
	if err != nil {
		r.note(err.Error(), "bad")
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
		say(run.view.whole(true)) // else with its turns, as they are first read
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
				run.out(run.view.talk.heard(batch.turns))
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
			again, placed := sight, false // the pane came into sight: said again whole
			for i, event := range events {
				deed := run.view.deed(event)
				if deed.do == chatLeave {
					return exitOK
				}
				if deed.do == chatRead && i+1 < len(events) && events[i+1].E == "move" {
					continue // the keys held down: only where they came to rest is read
				}
				said, here := run.act(deed)
				again, placed = again || said, placed || here
			}
			if again {
				say(run.view.whole(placed))
			}
		}
	}
}
