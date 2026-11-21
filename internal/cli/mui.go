package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"
)

// What mia desk takes of a message, as its docs/apps.md says: a message's
// bytes, a UI's pieces — a chat is one and each of its turns one more — a
// patch's changes, and the characters it keeps of a turn's words.
const (
	muiLimit  = 256 << 10
	muiPieces = 2000
	muiOps    = 500
	muiProse  = 64000
)

// muiMessage is one message to the window: the whole view, a patch of
// changes to the one shown, or close.
type muiMessage struct {
	V      int               `json:"v"`
	Pane   string            `json:"pane,omitempty"`
	Title  string            `json:"title,omitempty"`
	Status *muiStatus        `json:"status,omitempty"`
	View   *chatPiece        `json:"view,omitempty"`
	Patch  []json.RawMessage `json:"patch,omitempty"`
	Close  bool              `json:"close,omitempty"`
}

type muiStatus struct {
	Text string `json:"text"`
	Tone string `json:"tone,omitempty"`
}

// encode is a value as JSON with every character past ASCII a \u escape,
// as the window's own events are: no byte of a message is one a terminal
// on the way reads as its own.
func encode(value any) []byte {
	data, _ := json.Marshal(value) // of these types, none fails
	var out bytes.Buffer
	for _, r := range string(data) {
		switch {
		case r < utf8.RuneSelf:
			out.WriteByte(byte(r))
		case r > 0xFFFF:
			high, low := utf16.EncodeRune(r)
			fmt.Fprintf(&out, `\u%04x\u%04x`, high, low)
		default:
			fmt.Fprintf(&out, `\u%04x`, r)
		}
	}
	return out.Bytes()
}

// framed is a message as it is written to the terminal: an APC string,
// and inside tmux in tmux's passthrough, each ESC of it doubled.
func framed(message []byte, tmux bool) []byte {
	apc := "\033_Mui " + string(message) + "\033\\"
	if tmux {
		apc = "\033Ptmux;" + strings.ReplaceAll(apc, "\033", "\033\033") + "\033\\"
	}
	return []byte(apc)
}

// chatMark is what the window types before each event, in a bracketed
// paste; chatEventMost is more than one event is long.
const (
	chatMark      = "\x1fmia:"
	chatEventMost = 64 << 10
)

// muiEvents is the events whole in what was read, each read as an app's
// own kind of event, and what of it is kept for the next read: an event,
// or its mark, cut short.
func muiEvents[E any](buffer string) ([]E, string) {
	var found []E
	for {
		at := strings.Index(buffer, chatMark)
		if at < 0 {
			break
		}
		rest := buffer[at+len(chatMark):]
		decoder := json.NewDecoder(strings.NewReader(rest))
		var event E
		err := decoder.Decode(&event)
		if (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) && len(rest) < chatEventMost {
			return found, buffer[at:]
		}
		if err == nil {
			found = append(found, event)
			rest = rest[decoder.InputOffset():]
		}
		buffer = rest // what does not read is passed over
	}
	if at := strings.LastIndexByte(buffer, chatMark[0]); at >= 0 && strings.HasPrefix(chatMark, buffer[at:]) {
		return found, buffer[at:]
	}
	return found, ""
}

// muiTyped is what one read of the terminal says: the events in it, what
// is kept for the next read, whether the pane came into sight, and whether
// the person typed q or ⌃C in the plain terminal, to leave.
func muiTyped[E any](buffer, data string) (events []E, rest string, sight, leave bool) {
	sight = strings.Contains(data, "\033[I")
	data = strings.NewReplacer("\033[I", "", "\033[O", "").Replace(data)
	if !strings.Contains(buffer+data, chatMark) && (data == "q" || data == "\x03") {
		return nil, buffer, sight, true
	}
	events, rest = muiEvents[E](buffer + data)
	return events, rest, sight, false
}

// muiKeys is what is typed into the terminal, a read at a time, until
// stop; it is closed when the terminal ends.
func muiKeys(stop <-chan struct{}) <-chan string {
	keys := make(chan string)
	go func() {
		data := make([]byte, 64<<10)
		for {
			n, err := os.Stdin.Read(data)
			if n > 0 {
				select {
				case keys <- string(data[:n]):
				case <-stop:
					return
				}
			}
			if err != nil {
				close(keys)
				return
			}
		}
	}()
	return keys
}

// muiOpen makes the terminal one a UI is said through: in raw mode, what
// the window types a bracketed paste, the pane coming into sight
// reported, and line on the plain terminal under the UI. say writes
// messages to the window; shut closes the UI and gives the terminal back
// as it was.
//
// A terminal held past its own three is closed first: mia desk starts a
// UI in a pty with the pty's master still open in it, and a UI holding
// that copy keeps its pty from ever ending, so that when desk lets go
// nothing is read to its end, no hangup comes, and what it writes, once
// nobody reads it, waits forever.
func muiOpen(pane, line string) (say func(messages ...[]byte), shut func(), err error) {
	before, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return nil, nil, err
	}
	held, _ := os.ReadDir("/dev/fd")
	for _, entry := range held {
		if fd, err := strconv.Atoi(entry.Name()); err == nil && fd > 2 && term.IsTerminal(uintptr(fd)) {
			_ = syscall.Close(fd)
		}
	}
	tmux := os.Getenv("TMUX") != ""
	say = func(messages ...[]byte) {
		for _, message := range messages {
			_, _ = os.Stdout.Write(framed(message, tmux))
		}
	}
	shut = func() {
		say(encode(muiMessage{V: 1, Pane: pane, Close: true}))
		_, _ = os.Stdout.WriteString("\033[?2004l\033[?1004l\r\n")
		_ = term.Restore(os.Stdin.Fd(), before)
	}
	_, _ = os.Stdout.WriteString("\033[?2004h\033[?1004h" + line + "\r\n")
	return say, shut, nil
}

// editorWords is the editor's command: $EDITOR, else nvim.
func editorWords() []string {
	if words := strings.Fields(os.Getenv("EDITOR")); len(words) > 0 {
		return words
	}
	return []string{"nvim"}
}

// editorAt is the command that edits a file at a line: the editor
// with +<line> and the file's path from the worktree's top, run there
// with this terminal as its own.
func editorAt(top, path string, line int) *exec.Cmd {
	words := editorWords()
	if strings.HasPrefix(path, "-") || strings.HasPrefix(path, "+") { // a path, not an option
		path = "./" + path
	}
	command := exec.Command(words[0], append(words[1:], fmt.Sprintf("+%d", line), path)...)
	command.Dir = top
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command
}

// muiKeysInTurn is what is typed into the terminal, a read at a time, until
// stop. The next read is begun only once next says the last was dealt
// with: while an editor has the terminal, nothing typed to it is read
// here. It is closed when the terminal ends.
func muiKeysInTurn(stop, next <-chan struct{}) <-chan string {
	keys := make(chan string)
	go func() {
		data := make([]byte, 64<<10)
		for {
			n, err := os.Stdin.Read(data)
			if n > 0 {
				select {
				case keys <- string(data[:n]):
				case <-stop:
					return
				}
			}
			if err != nil {
				close(keys)
				return
			}
			if n > 0 {
				select {
				case <-next:
				case <-stop:
					return
				}
			}
		}
	}()
	return keys
}

// muiLent is the terminal taken back from a program it was lent to, an
// editor: ⌃C typed in one that takes it as a signal was the editor's,
// and is passed over; any other signal come in meanwhile is this
// program's, and it reports false: it is to leave.
func muiLent(signals <-chan os.Signal) bool {
	for {
		select {
		case signal := <-signals:
			if signal != syscall.SIGINT {
				return false
			}
		default:
			return true
		}
	}
}

// muiChoice is a choice of the menu an item of a list carries, opened
// beside it with → and under the pointer's other button: what it does,
// and at its right the key that does the same; or a rule between two.
// Picked, it is said with its item, as the pick's sub.
type muiChoice struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	Right string `json:"right,omitempty"`
	Rule  bool   `json:"rule,omitempty"`
}

// muiPassing is what a view says once and the window does not keep: the
// words of what was just done, shown over the foot for a moment (a
// toast, with its tone), and words put on the person's clipboard.
type muiPassing struct {
	toast *muiStatus
	copy  string
}

// done is what was just done, in a toast: in tone ok, or bad for what
// failed and is no state of things.
func (p *muiPassing) done(text, tone string) {
	p.toast = &muiStatus{Text: cutTo(text, gitWordsMost), Tone: tone}
}

// changes is what is passing as the changes of a patch, and none of it
// is left: it is said once.
func (p *muiPassing) changes() []json.RawMessage {
	var changes []json.RawMessage
	if p.copy != "" {
		changes = append(changes, encode(struct {
			Op   string `json:"op"`
			Text string `json:"text"`
		}{"copy", p.copy}))
	}
	if p.toast != nil {
		changes = append(changes, encode(struct {
			Op   string `json:"op"`
			Text string `json:"text"`
			Tone string `json:"tone,omitempty"`
		}{"toast", p.toast.Text, p.toast.Tone}))
	}
	*p = muiPassing{}
	return changes
}

// muiTold is what a view last said whole: all of it but its status and
// its keys, and those two as they were written.
type muiTold struct {
	body   []byte
	status string
	keys   string
}

// muiApart is a view's messages apart: all of them but the first's
// status and keys, and those two as they are written.
func muiApart(messages [][]byte) muiTold {
	if len(messages) == 0 {
		return muiTold{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(messages[0], &fields); err != nil {
		return muiTold{body: bytes.Join(messages, nil)}
	}
	now := muiTold{status: string(fields["status"]), keys: string(fields["keys"])}
	delete(fields, "status")
	delete(fields, "keys")
	first, _ := json.Marshal(fields) // of what was just read, none fails
	now.body = bytes.Join(append([][]byte{first}, messages[1:]...), nil)
	return now
}

// news is what is to be said of a view whose messages, but for where the
// person is, are these, after what was told last: whole, where anything
// but its status and its keys is another; else those of the two that
// are, as the changes of a patch, which does not say the view again.
// Nothing is said of a view that is as it was told.
func (t *muiTold) news(messages [][]byte) (changes []json.RawMessage, whole bool) {
	was, now := *t, muiApart(messages)
	*t = now
	if !bytes.Equal(was.body, now.body) {
		return nil, true
	}
	if was.status != now.status {
		change := json.RawMessage(`{"op":"status"}`) // with no words they are taken away
		if now.status != "" {
			var status muiStatus
			_ = json.Unmarshal([]byte(now.status), &status)
			change = encode(struct {
				Op   string `json:"op"`
				Text string `json:"text"`
				Tone string `json:"tone,omitempty"`
			}{"status", status.Text, status.Tone})
		}
		changes = append(changes, change)
	}
	if was.keys != now.keys {
		keys := json.RawMessage(now.keys)
		if now.keys == "" {
			keys = json.RawMessage("[]")
		}
		changes = append(changes, encode(struct {
			Op   string          `json:"op"`
			Keys json.RawMessage `json:"keys"`
		}{"keys", keys}))
	}
	return changes, false
}

// says is what is written of a view: its messages whole where news says
// so and where the person is placed — placed is then the messages that
// say where, else nil — and otherwise the changes of its status and its
// keys alone; and after either, what is passing. plain is the view's
// messages but for where the person is.
func (t *muiTold) says(pane string, plain, placed [][]byte, passing *muiPassing) [][]byte {
	changes, whole := t.news(plain)
	var all [][]byte
	switch {
	case placed != nil:
		all, changes = placed, nil
	case whole:
		all = plain
	}
	if changes = append(changes, passing.changes()...); len(changes) > 0 {
		all = append(all, encode(muiMessage{V: 1, Pane: pane, Patch: changes}))
	}
	return all
}
