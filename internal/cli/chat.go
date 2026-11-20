package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/emusoi/mia-core/internal/app"
)

const chatUsage = "mia chat [--window <index>] [--conversation <id>]"

// chatTurns is how many turns the view said whole has at most: under what
// the window takes, so that turns can be added to it by patches before it
// must be said again.
const chatTurns = 1500

// patchEnvelope is more than a patch's message is around its changes: its
// version and its pane.
const patchEnvelope = 64

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

// chatTurn is a turn as the window is told it, and its place.
type chatTurn struct {
	n    int
	json json.RawMessage
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
	if text := []rune(turn.Text); len(text) > muiProse {
		turn.Text = string(text[:muiProse])
	}
	for {
		data := encode(turn)
		text := []rune(turn.Text)
		if len(data) <= muiLimit/2 || len(text) < 2 {
			return chatTurn{turn.N, data}
		}
		turn.Text = string(text[:len(text)/2])
	}
}

// lastThatFit is the last of the turns that a view with room bytes for
// them can say: no more than chatTurns of them.
func lastThatFit(turns []chatTurn, room int) []chatTurn {
	from := len(turns)
	for from > 0 && len(turns)-from < chatTurns && room > len(turns[from-1].json) {
		room -= len(turns[from-1].json) + 1 // and the comma between two
		from--
	}
	return turns[from:]
}

// patches is turns as patches of turn changes, in order: each patch no
// more changes and no more bytes than the window takes.
func patches(pane string, turns []chatTurn) [][]byte {
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
		op := encode(chatOp{Op: "turn", ID: "chat", Turn: turn.json})
		if len(ops) == muiOps || size+len(op)+1 > muiLimit {
			flush()
		}
		ops, size = append(ops, op), size+len(op)+1
	}
	flush()
	return all
}

// talk is a conversation as the window is shown it.
type talk struct {
	pane, title, agent string
	reply, working     bool
	status             string       // what went wrong with the last thing done, said beside the title
	turns              []chatTurn   // every turn the window can read, in order
	shown              map[int]bool // the turns the window was told; nil until the view was said
}

// whole is the message that says the whole view: the last turns that fit.
func (t *talk) whole() []byte {
	message := muiMessage{V: 1, Pane: t.pane, Title: t.title,
		View: &chatPiece{T: "chat", ID: "chat", Agent: t.agent, Reply: t.reply, Working: t.working, Turns: []json.RawMessage{}}}
	if t.status != "" {
		message.Status = &muiStatus{Text: t.status, Tone: "bad"}
	}
	t.shown = map[int]bool{}
	for _, turn := range lastThatFit(t.turns, muiLimit-len(encode(message))) {
		message.View.Turns = append(message.View.Turns, turn.json)
		t.shown[turn.n] = true
	}
	return encode(message)
}

// heard takes the turns a followed conversation gained or that grew, and
// is the messages that say them: patches of turn changes, or the whole
// view when it was not said yet or would hold more turns than the window
// takes.
func (t *talk) heard(turns []app.Turn) [][]byte {
	var changed []chatTurn
	more := 0
	for _, turn := range turns {
		if !drawn(turn) {
			continue
		}
		one := worded(turn)
		at, found := slices.BinarySearchFunc(t.turns, one.n, func(have chatTurn, n int) int { return cmp.Compare(have.n, n) })
		if found {
			t.turns[at] = one
		} else {
			t.turns = slices.Insert(t.turns, at, one)
		}
		if !t.shown[one.n] {
			more++
		}
		changed = append(changed, one)
	}
	if t.shown == nil || len(t.shown)+more > muiPieces-1 {
		return [][]byte{t.whole()}
	}
	for _, one := range changed {
		t.shown[one.n] = true
	}
	return patches(t.pane, changed)
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
	return [][]byte{encode(muiMessage{V: 1, Pane: t.pane, Patch: []json.RawMessage{encode(chatOp{Op: "working", ID: "chat", On: &on})}})}
}

// chatEvent is what the person did in the window's chat: send with the
// words typed, choose with the question's turn and its option counted
// from 0, stop, close.
type chatEvent struct {
	E        string `json:"e"`
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

// cmdChat shows a conversation of the worktree it is run in as a chat
// drawn by mia desk over its pane, and follows it as it is written. What
// the person sends, chooses and stops there is typed into the window that
// has the conversation open; one no window has open is only shown.
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
	if err != nil {
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, "mia:", problem)
		}
		return fail(err)
	}
	view := &talk{pane: os.Getenv("TMUX_PANE"), title: head.Agent + " · " + record.Name, agent: head.Agent}
	index := 0
	if head.Running && head.Window != nil {
		view.reply, index = true, head.Window.Index
		view.working = a.AtWork(record, index)
	}
	window = strconv.Itoa(index)

	stop := make(chan struct{})
	defer close(stop)
	batches, failed := make(chan []app.Turn), make(chan error, 1)
	go func() {
		failed <- a.FollowConversation(head.ID, record.Name, 0, stop, func(turns []app.Turn) error {
			select {
			case batches <- turns:
				return nil
			case <-stop:
				return errors.New("left")
			}
		})
	}()
	works := make(chan bool)
	if view.reply {
		go func() {
			for {
				select {
				case <-stop:
					return
				case <-time.After(time.Second):
				}
				select {
				case works <- a.AtWork(record, index):
				case <-stop:
					return
				}
			}
		}()
	}
	keys := muiKeys(stop)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)

	say, shut, err := muiOpen(view.pane, "mia chat: the plain terminal. In mia desk "+view.title+" is drawn over it as a chat; q or ⌃C leaves.")
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

	buffer := ""
	for {
		select {
		case <-signals:
			return exitOK
		case failure = <-failed:
			if failure != nil {
				return exitFailed
			}
			return exitOK
		case batch := <-batches:
			say(view.heard(batch)...)
		case on := <-works:
			say(view.atWork(on)...)
		case data, open := <-keys:
			if !open {
				return exitOK
			}
			events, rest, sight, leave := typed(buffer, data)
			if leave {
				return exitOK
			}
			buffer = rest
			if sight && view.shown != nil { // the pane came into sight: said again whole
				say(view.whole())
			}
			for _, event := range events {
				var err error
				switch {
				case event.E == "close":
					return exitOK
				case !view.reply:
					continue
				case event.E == "send" && strings.TrimSpace(event.Text) != "":
					// the agent writes what it was asked in its transcript, and the turn comes from there
					err = a.SendToWindow(record, window, strings.TrimSpace(event.Text))
				case event.E == "choose":
					err = a.PressInWindow(record, window, strconv.Itoa(event.Option+1))
				case event.E == "stop":
					err = a.StopInWindow(record, window)
				default:
					continue
				}
				was := view.status
				view.status = ""
				if err != nil {
					text := []rune(err.Error())
					view.status = string(text[:min(len(text), 200)]) // what the window keeps of it
				}
				if view.status != was && view.shown != nil {
					say(view.whole())
				}
			}
		}
	}
}
