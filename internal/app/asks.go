package app

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/session"
)

// Asks is a question an agent's screen shows with the options it is
// answered by: claude's trust in a folder it has not seen, a permission, a
// choice it puts to the person. Anything typed into the agent then, Enter
// above all, answers it.
type Asks struct {
	Text    string   `json:"text"`
	Options []string `json:"options"`
	// Selected is the option the agent's cursor is on, the first being 1:
	// what Enter alone answers.
	Selected int `json:"selected"`
	// numbered is an agent that takes an option's number; one drawn with no
	// numbers takes no digit, and is answered by moving its cursor.
	numbered bool
}

// listed is the option an agent's cursor is on, drawn with no number:
// claude's "❯ No, exit". A ">" is not one: it starts quotes and prompts.
var listed = regexp.MustCompile(`^([\s│|]*[❯›▶]\s+)(\S.*?)[\s│|]*$`)

// hint is the line under a choice drawn with no numbers that says how it
// is made: claude's "Enter to confirm · Esc to cancel".
var hint = regexp.MustCompile(`(?i)\b(enter|esc) to\b`)

// asksBelow is how many lines may be drawn under a choice for it to be the
// one the agent waits on: an agent draws that last, its hints and its
// status line under it.
const asksBelow = 8

// asksOn is the question a screen ends with, when it shows one: a run of
// options with the agent's cursor on one of them, numbered or not.
func asksOn(screen string) (Asks, bool) {
	lines := strings.Split(strings.TrimRight(screen, " \n"), "\n")
	var asks Asks
	run := numberedOn(lines)
	if run.cursor > 0 {
		asks = Asks{Options: run.options, Selected: run.cursor, numbered: true}
	} else {
		run = choices{}
	}
	if other := listedOn(lines); len(other.options) > 0 && other.last > run.last {
		run, asks = other, Asks{Options: other.options, Selected: other.cursor}
	}
	if len(run.options) == 0 || drawn(lines[run.last+1:]) > asksBelow {
		return Asks{}, false
	}
	asks.Text = questionAbove(lines[:run.first])
	return asks, true
}

// listedOn is the options around the last line an agent's cursor is on,
// drawn with no numbers: the lines next to it that start where its option
// does, with a hint under them.
func listedOn(lines []string) choices {
	at := len(lines) - 1
	for ; at >= 0 && (!listed.MatchString(lines[at]) || option.MatchString(lines[at])); at-- {
	}
	if at < 0 {
		return choices{}
	}
	column := len([]rune(listed.FindStringSubmatch(lines[at])[1]))
	sibling := func(line string) (string, bool) {
		indent := len([]rune(line)) - len([]rune(strings.TrimLeft(line, " \t│|")))
		text := strings.TrimRight(strings.TrimLeft(line, " \t│|"), " \t│|")
		return text, indent == column && text != "" && !hint.MatchString(text)
	}
	run := choices{first: at, last: at}
	for run.first > 0 {
		if _, ok := sibling(lines[run.first-1]); !ok {
			break
		}
		run.first--
	}
	for run.last < len(lines)-1 {
		if _, ok := sibling(lines[run.last+1]); !ok {
			break
		}
		run.last++
	}
	for i := run.first; i <= run.last; i++ {
		text, _ := sibling(lines[i])
		if i == at {
			text, run.cursor = listed.FindStringSubmatch(lines[i])[2], i-run.first+1
		}
		run.options = append(run.options, text)
	}
	hinted := false
	for _, line := range lines[run.last+1 : min(len(lines), run.last+1+asksBelow)] {
		hinted = hinted || hint.MatchString(line)
	}
	if len(run.options) < 2 || !hinted {
		return choices{}
	}
	return run
}

// drawn is how many of lines have something on them.
func drawn(lines []string) int {
	n := 0
	for _, line := range lines {
		if strings.Trim(line, " \t│|─╭╮╰╯") != "" {
			n++
		}
	}
	return n
}

// questionAbove is what is asked over a run of options: the sentence of
// the nearest paragraph above them that ends in a question mark, or that
// paragraph when none does.
func questionAbove(lines []string) string {
	var paragraph []string
	nearest := ""
	ask := func() (string, bool) {
		text := strings.Join(paragraph, " ")
		paragraph = nil
		if nearest == "" {
			nearest = text
		}
		q := strings.LastIndex(text, "?")
		if q < 0 {
			return "", false
		}
		start := 0
		for _, stop := range []string{". ", ": ", "! ", "? "} {
			if at := strings.LastIndex(text[:q], stop); at >= 0 {
				start = max(start, at+len(stop))
			}
		}
		return oneLine(text[start : q+1]), true
	}
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-20; i-- {
		line := strings.Trim(lines[i], " \t│|─╭╮╰╯")
		if line != "" {
			paragraph = append([]string{line}, paragraph...)
			continue
		}
		if len(paragraph) == 0 {
			continue
		}
		if question, ok := ask(); ok {
			return question
		}
	}
	if len(paragraph) > 0 {
		if question, ok := ask(); ok {
			return question
		}
	}
	return oneLine(nearest)
}

// askingIn is the question a window shows, when it shows one: the one check
// each verb that types into an agent makes before it does.
func askingIn(host session.Host, worktree, window string) (Asks, bool) {
	screen, err := host.Capture(worktree, window, false)
	if err != nil {
		return Asks{}, false
	}
	return asksOn(screen)
}

// askedFirst is what a verb that would type into an agent says when the
// agent asks something first: nothing is typed, as Enter would answer it.
func askedFirst(agent string, asks Asks, press string) error {
	return errors.New(asks.Said(agent, press))
}

// Said is the question as a line for the person: who asks, what, and how
// it is answered, press being the verb that presses in its window.
func (asks Asks) Said(agent, press string) string {
	options := make([]string, len(asks.Options))
	for i, one := range asks.Options {
		options[i] = strconv.Itoa(i+1) + " " + one
	}
	question := ""
	if asks.Text != "" {
		question = ": " + asks.Text
	}
	return fmt.Sprintf("%s asks first%s — answer it: `%s <n>` (%s)", agent, question, press, strings.Join(options, " · "))
}

// pressAnswer presses keys in a window with no Enter after them. A number,
// where the window asks with a cursor and no numbers, moves the cursor to
// that option and presses Enter, as those agents take no digit — and only
// once the cursor is seen there, so another option is never answered.
func pressAnswer(host session.Host, worktree, window, keys string) error {
	asks, on := askingIn(host, worktree, window)
	n, err := strconv.Atoi(keys)
	if !on || asks.numbered || err != nil || n < 1 || n > len(asks.Options) {
		return host.Press(worktree, window, keys)
	}
	for moved := 0; asks.Selected != n; moved++ {
		if moved > len(asks.Options) {
			return fmt.Errorf("the cursor did not come to option %d, and nothing was answered", n)
		}
		key := "Down"
		if n < asks.Selected {
			key = "Up"
		}
		if err := host.Keys(worktree, window, []string{key}); err != nil {
			return err
		}
		was := asks.Selected
		for range 10 {
			time.Sleep(50 * time.Millisecond)
			if asks, on = askingIn(host, worktree, window); !on || asks.Selected != was {
				break
			}
		}
		if !on || asks.Selected == was {
			return fmt.Errorf("the cursor did not move to option %d, and nothing was answered", n)
		}
	}
	time.Sleep(typedApart) // the agent is given time to take the key before Enter
	return host.Keys(worktree, window, []string{"Enter"})
}

// settleFor is how long an agent just started is watched before its start
// is said: one that asks something first, or ends, does it by then.
const settleFor = 3 * time.Second

// settle watches a window just started until it asks something, ends, or
// has drawn the same for a second, settleFor at most. It is kept on exit
// meanwhile, so an agent that ends leaves what it said: its last lines
// when it ended, and the window is closed then.
func settle(host session.Host, worktree, window string) (said string, ended bool) {
	kept := host.KeepOnExit(worktree, window, true) == nil
	var screen string
	still := time.Now()
	for start := time.Now(); time.Since(start) < settleFor; time.Sleep(100 * time.Millisecond) {
		now, err := host.Capture(worktree, window, false)
		if ended = err != nil || host.Ended(worktree, window); ended {
			// tmux's line that says so scrolls what it printed up out of the screen
			if back, err := host.CaptureBack(worktree, window, false, session.GeometryHeight); err == nil {
				screen = back
			}
			break
		}
		if _, on := asksOn(now); on {
			break
		}
		if now != screen || strings.TrimSpace(now) == "" {
			screen, still = now, time.Now()
		} else if time.Since(still) >= time.Second {
			break
		}
	}
	if kept {
		_ = host.KeepOnExit(worktree, window, false)
	}
	if !ended {
		return "", false
	}
	_ = host.KillWindow(worktree, window)
	var last []string
	for _, line := range strings.Split(screen, "\n") {
		if line = strings.TrimRight(line, " "); strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "Pane is dead") {
			last = append(last, "  "+line)
		}
	}
	return strings.Join(last[max(0, len(last)-6):], "\n"), true
}
