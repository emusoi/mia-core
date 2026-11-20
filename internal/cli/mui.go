package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
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
func muiOpen(pane, line string) (say func(messages ...[]byte), shut func(), err error) {
	before, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return nil, nil, err
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
