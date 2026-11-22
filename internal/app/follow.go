package app

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"
)

// polling follows a conversation there is no file of to read on from: ask
// gives all of it after a turn each time, and each call is what is new or
// changed since the call before.
func polling(ask func() ([]Turn, bool, error)) func() ([]Turn, bool, error) {
	sent := map[int]Turn{}
	return func() ([]Turn, bool, error) {
		turns, found, err := ask()
		var changed []Turn
		for _, turn := range turns {
			if was, ok := sent[turn.N]; !ok || !reflect.DeepEqual(was, turn) {
				sent[turn.N] = turn
				changed = append(changed, turn)
			}
		}
		return changed, found, err
	}
}

// whenWritten is next asked only when one of the paths written gives has
// been written since it was last asked, or backstop after, whatever they
// say; the first call always asks. A reader that has an agent's own
// program give it a whole conversation each time has it do so only when
// the agent wrote.
func whenWritten(written func() []string, backstop time.Duration, next func() ([]Turn, bool, error)) func() ([]Turn, bool, error) {
	var was string
	var asked time.Time
	found := false
	return func() ([]Turn, bool, error) {
		is := stamps(written())
		if !asked.IsZero() && is == was && time.Since(asked) < backstop {
			return nil, found, nil
		}
		was, asked = is, time.Now()
		turns, ok, err := next()
		found = ok
		return turns, ok, err
	}
}

// stamps is when each of the paths was last written and how long it is,
// and which are not there: it changes when one of them does.
func stamps(paths []string) string {
	var all strings.Builder
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil {
			fmt.Fprintf(&all, "%d %d\n", info.ModTime().UnixNano(), info.Size())
		} else {
			all.WriteString("-\n")
		}
	}
	return all.String()
}

// follow is turnsIn as a conversation is written to: say is handed what
// turnsIn gives, and then, each time the conversation gains turns or one
// grows, those turns, until stop is closed or say fails. Until an agent
// has the conversation, unwritten says whether that is an error: none
// when a window has it open and nothing of it is written yet. dress is
// what is done to turns before they are said. With enough, of an agent
// whose transcripts keep steps, what is said first is its last turns, read
// back until enough says they are enough.
func (a *App) follow(in folders, id, cwd, whose string, after int, enough func([]Turn) bool, unwritten func() error, dress func([]Turn) []Turn, stop <-chan struct{}, say func([]Turn) error) error {
	type follower struct {
		agent string
		next  func() ([]Turn, bool, error)
		every time.Duration
	}
	var all []follower
	for _, agent := range a.agents() {
		if agent.talks != nil && (whose == "" || agent.Name == whose) {
			next, every := agent.talks.follow(in, id, cwd, after)
			if t, ok := agent.talks.(transcripts); ok && enough != nil {
				next, every = t.tail(in, id, cwd, enough)
			}
			all = append(all, follower{agent.Name, next, every})
		}
	}
	var on *follower // the agent's that has it, once one has
	for first := true; ; first = false {
		var turns []Turn
		if on == nil {
			for i := range all {
				got, found, err := all[i].next()
				if err != nil && found {
					return fmt.Errorf("agent %s: %w", all[i].agent, err)
				}
				if err == nil && found { // an agent that cannot say what it has does not hide another's
					on, turns = &all[i], got
					break
				}
			}
			if on == nil && first {
				if err := unwritten(); err != nil {
					return err
				}
			}
		} else {
			got, _, err := on.next()
			if err != nil {
				return fmt.Errorf("agent %s: %w", on.agent, err)
			}
			turns = got
		}
		if first || len(turns) > 0 {
			if say(dress(append([]Turn{}, turns...))) != nil {
				return nil // whoever read it has gone
			}
		}
		every := time.Second // looked for again, while no agent has it
		if on != nil {
			every = on.every
		}
		select {
		case <-stop:
			return nil
		case <-time.After(every):
		}
	}
}
