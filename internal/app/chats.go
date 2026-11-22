package app

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/paths"
)

// A chat is a main agent of its own: a plain conversation with any agent,
// in a folder of mia's that no repository holds, a session of its own and
// the same notes, which the person comes back to and goes on with. The
// main agent is the chat named main. Everything the main agent's verbs do
// they do to the chat mia is opened at, by AtChat.

// chatsDir is where each chat's folder is, by its name.
func chatsDir() string { return paths.In("chats") }

// AtChat is mia at a chat's folder, as AtMain is at the main agent's: the
// main agent's verbs, Main, StartMain, SayToMain and the others, are then
// that chat's. main is the main agent's.
func AtChat(name string) (*App, error) {
	if name == mainName {
		return AtMain()
	}
	home := filepath.Join(chatsDir(), name)
	if info, err := os.Stat(home); chatsDir() == "" || name == "" || filepath.Base(name) != name || name == ".." || err != nil || !info.IsDir() {
		return nil, fmt.Errorf("no chat %s — `mia api chats` lists them", name)
	}
	return atChat(name, home)
}

// chatSession is what its session is named for after mia-: main, or
// chat-<name>, which no worktree is called.
func (a *App) chatSession() string {
	if a.chat == "" {
		return mainName
	}
	return "chat-" + a.chat
}

// chatWho is how a line says whose agent it is.
func (a *App) chatWho() string {
	if a.chat == "" {
		return "the main agent"
	}
	return "chat " + a.chat
}

// chatVerb is the verb that does sub to it, with what follows the name.
func (a *App) chatVerb(sub string, more ...string) string {
	verb := []string{"mia", "main", sub}
	if a.chat != "" {
		verb = []string{"mia", "chats", sub, a.chat}
	}
	return strings.Join(append(verb, more...), " ")
}

// chatEnv is what tells the agent whose it is.
func (a *App) chatEnv() string {
	if a.chat == "" {
		return "MIA_MAIN=1"
	}
	return "MIA_CHAT=" + a.chat
}

// chatNotes is what its folder tells it: the main agent's notes, and of a
// chat the same but for who it is.
func (a *App) chatNotes() string {
	_, rest, found := strings.Cut(mainNotes, "\n\nThis folder is no repository.")
	if a.chat == "" || !found {
		return mainNotes
	}
	return "# mia chat " + a.chat + "\n\n" +
		"You are one of the person's chats with mia: a conversation of its own, with no repository\n" +
		"and no worktree, that they come back to and go on with. Do what they ask on their behalf:\n" +
		"run commands, and drive the other agents through mia as the main agent does — work in a\n" +
		"repository goes to a worktree and an agent of its own.\n\n" +
		"This folder is no repository." + rest
}

// chatKept is what a chat's folder keeps of it: the agent it goes on with
// and the model and effort it is started with, its first words, said once
// it is ready when they wait, and when it was made. Of the main agent,
// only the agent it was last switched to.
type chatKept struct {
	Agent   string    `json:"agent"`
	Model   string    `json:"model,omitempty"`
	Effort  string    `json:"effort,omitempty"`
	Title   string    `json:"title,omitempty"`
	Waiting string    `json:"waiting,omitempty"`
	Made    time.Time `json:"made,omitzero"`
}

func (a *App) chatPath() string { return filepath.Join(a.MiaDir, "chat.json") }

// chatRead is what is kept of it; nothing, unreadable or not there.
func (a *App) chatRead() chatKept {
	var kept chatKept
	if data, err := os.ReadFile(a.chatPath()); err == nil {
		_ = json.Unmarshal(data, &kept)
	}
	return kept
}

// chatWrite keeps what is kept of it, whole or not at all.
func (a *App) chatWrite(kept chatKept) error {
	if err := os.MkdirAll(a.MiaDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(kept)
	if err != nil {
		return err
	}
	temp := a.chatPath() + ".new"
	if err := os.WriteFile(temp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temp, a.chatPath())
}

// chatOwn is what its agent is started with when nothing else is said:
// of the main agent [main]'s, claude by default, unless it was switched
// to another; of a chat, what it was made with or last switched to.
func (a *App) chatOwn() config.Main {
	kept := a.chatRead()
	if a.chat == "" {
		own := a.Config.Main
		if own.Agent = cmp.Or(own.Agent, "claude"); kept.Agent == "" || kept.Agent == own.Agent {
			return own
		}
	}
	return config.Main{Agent: kept.Agent, Model: kept.Model, Effort: kept.Effort}
}

// ChatState is a chat as a list of them says it. Title is the first thing
// asked in it, on a line, mia for the main agent; Last the last thing said
// in its conversation, and At when that was, or when it was made.
type ChatState struct {
	Name    string    `json:"name"`
	Main    bool      `json:"main"`
	Agent   string    `json:"agent"`
	Model   string    `json:"model"`
	Effort  string    `json:"effort"`
	Title   string    `json:"title"`
	Last    string    `json:"last"`
	At      time.Time `json:"at,omitzero"`
	Asks    bool      `json:"asks"`
	Working bool      `json:"working"`
	Running bool      `json:"running"`
	Machine string    `json:"machine"`
}

// Chats is the main agent and every chat, the main agent first and the
// others the last said in first. mia is to be at the main agent's.
func (a *App) Chats() []ChatState {
	all := []ChatState{a.chatState()}
	entries, _ := os.ReadDir(chatsDir())
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		c, err := atChat(entry.Name(), filepath.Join(chatsDir(), entry.Name()))
		if err != nil {
			continue
		}
		c.Plugins = a.Plugins
		all = append(all, c.chatState())
	}
	slices.SortStableFunc(all[1:], func(x, y ChatState) int { return y.At.Compare(x.At) })
	return all
}

// ChatsWritten is what Chats is read from beyond mia's own files: the
// transcripts of the main agent's and every chat's conversations, as
// ConversationsWritten says them.
func (a *App) ChatsWritten() []string {
	written := a.ConversationsWritten()
	entries, _ := os.ReadDir(chatsDir())
	for _, entry := range entries {
		if c, err := atChat(entry.Name(), filepath.Join(chatsDir(), entry.Name())); err == nil && entry.IsDir() {
			c.Plugins = a.Plugins
			written = append(written, c.ConversationsWritten()...)
		}
	}
	return written
}

// chatState is the chat mia is at, as Chats lists it.
func (a *App) chatState() ChatState {
	state := a.Main()
	kept, own := a.chatRead(), a.chatOwn()
	one := ChatState{Name: state.Name, Main: a.chat == "", Agent: state.Agent, Model: cmp.Or(state.Model, own.Model), Effort: cmp.Or(state.Effort, own.Effort),
		Title: kept.Title, At: instant(kept.Made), Asks: state.Asks != nil, Working: state.Working, Running: state.Running, Machine: state.Machine}
	if state.Running {
		one.Model, one.Effort = state.Model, state.Effort
	}
	if one.Main {
		one.Title = "mia"
	}
	heads, _ := a.Conversations(Wanted{})
	if len(heads) > 0 { // the last written to first
		last := heads[0]
		one.Last, one.At = cmp.Or(last.Said, OwnWords(last.Asked)), last.Active
		if one.Title == "" {
			one.Title = OwnWords(heads[len(heads)-1].Asked)
		}
	}
	return one
}

// chatTitle is a chat's first words on one line, as a list shows them.
const chatTitle = 80

// NewChat makes a chat with an agent, a model and an effort, and its
// folder, under a name the registry gives it as it gives a worktree's: mia
// at it, its agent not started. Its first words are kept, to be said once
// it has started and is ready. mia is to be at the main agent's.
func (a *App) NewChat(agentName, model, effort, words string) (*App, error) {
	agent, known := agentNamed(a.agents(), agentName)
	if !known {
		return nil, fmt.Errorf("mia knows no agent %s — `mia api agents` lists those it does", agentName)
	}
	if err := allowed(agent, model, effort, a.Config.Agents[agent.Name]); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(chatsDir(), 0o755); err != nil {
		return nil, err
	}
	root := git.Resolve(chatsDir())
	name, err := a.Names.AllocateFor(func(name string) (string, bool) {
		_, err := os.Stat(filepath.Join(root, name))
		return filepath.Join(root, name), name != mainName && errors.Is(err, os.ErrNotExist)
	})
	if err != nil {
		return nil, err
	}
	c, err := atChat(name, filepath.Join(root, name))
	if err != nil {
		return nil, err
	}
	c.Plugins = a.Plugins
	title := []rune(strings.Join(strings.Fields(words), " "))
	if len(title) > chatTitle {
		title = append(title[:chatTitle-1], '…')
	}
	if err := c.chatWrite(chatKept{Agent: agent.Name, Model: model, Effort: effort, Title: string(title), Waiting: strings.TrimSpace(words), Made: time.Now().UTC()}); err != nil {
		return nil, err
	}
	return c, nil
}

// SayWaiting says a chat's first words once it is ready, and whether it
// did: they wait while it asks something first, for the answer to it.
func (a *App) SayWaiting() (bool, error) {
	kept := a.chatRead()
	if kept.Waiting == "" {
		return false, nil
	}
	host := a.mainHost()
	window, _, running := a.mainWindow(host)
	if !running {
		return false, a.down()
	}
	if _, on := askingIn(host, a.chatSession(), window.Name); on {
		return false, nil
	}
	if err := a.SayToMain(kept.Waiting); err != nil {
		return false, err
	}
	kept.Waiting = ""
	return true, a.chatWrite(kept)
}

// RemoveChat ends a chat's session and takes its folder and its name. Its
// conversations stay where its agents keep them. The main agent is no chat
// to remove. mia is to be at the main agent's.
func (a *App) RemoveChat(name string) error {
	if name == mainName {
		return errors.New("the main agent is not removed — `mia main stop` stops it")
	}
	c, err := AtChat(name)
	if err != nil {
		return err
	}
	if err := c.StopMain(); err != nil {
		return err
	}
	if err := a.Names.Release(c.Root); err != nil {
		return err
	}
	return os.RemoveAll(c.Root)
}

// threadBefore is the turns of the conversations a chat went on from
// before the one it has open, by the switches that led to it, each
// followed by its switch turn, numbered from 1 — the last one waiting
// while what it handed over is still to be sent — and own, how many of
// the open conversation's own turns are among them: those it had before
// it went on with another agent and took its own up again. A turn that
// says no time is in the last stretch its conversation has.
func (a *App) threadBefore(state MainState) (before []Turn, own int) {
	_, all := a.handedOnIn()
	if len(all) == 0 {
		return nil, 0
	}
	if slices.ContainsFunc(all, func(one handedOn) bool { return one.To.ID == "" }) {
		a.Conversations(Wanted{}) // which conversation each began, found and kept
	}
	var from Talk // the conversation the agent is still to be sent
	if state.Window != nil {
		if carried, on := a.CarriedInWindow(model.Record{Name: a.chatSession()}, state.Window.Name); on {
			from = Talk{Agent: carried.From, ID: carried.ID}
		}
	}
	open := Talk{Agent: state.Agent, ID: state.Conversation}
	chain := a.Handings(open.Agent, open.ID, from)
	folders, _ := a.where()
	read := map[Talk][]Turn{}
	for i, handed := range chain {
		var since time.Time
		if i > 0 {
			since = chain[i-1].At
		}
		with := handed.From
		if _, done := read[with]; !done {
			read[with], _, _ = a.turnsIn(folders, with.ID, "", with.Agent, 0)
		}
		again := with == open || slices.ContainsFunc(chain[i+1:], func(later Handed) bool { return later.From == with })
		numbered := map[int]int{}
		for _, turn := range read[with] {
			if turn.At.IsZero() && again || !turn.At.IsZero() && (!turn.At.After(since) || turn.At.After(handed.At)) {
				continue
			}
			if with == open {
				own++
			}
			numbered[turn.N] = len(before) + 1
			turn.N, turn.Question = len(before)+1, numbered[turn.Question]
			if turn.Kind == turnAsked {
				turn.Text = OwnWords(turn.Text)
			}
			before = append(before, turn)
		}
		note, tone := handed.Note(), ""
		if i == len(chain)-1 && from.ID != "" {
			note, tone = CarriedWaits, "warn"
		}
		before = append(before, Turn{N: len(before) + 1, Kind: turnSwitch, At: handed.At, From: handed.From.Agent, To: handed.To.Agent, Note: note, Tone: tone})
	}
	return before, own
}

// afterTurn is those of turns after turn number after, turns left as they
// are.
func afterTurn(turns []Turn, after int) []Turn {
	at := slices.IndexFunc(turns, func(t Turn) bool { return t.N > after })
	if at < 0 {
		return nil
	}
	return turns[at:]
}

// onFrom is turns of the conversation a chat has open as its thread
// numbers them: those after its own own, which are among the turns before,
// each number on by offset, and what the person asked their own words.
func onFrom(turns []Turn, own, offset int) []Turn {
	turns = slices.DeleteFunc(turns, func(t Turn) bool { return t.N <= own })
	for i := range turns {
		turns[i].N += offset
		if turns[i].Question > own {
			turns[i].Question += offset
		}
		if turns[i].Kind == turnAsked {
			turns[i].Text = OwnWords(turns[i].Text)
		}
	}
	return turns
}

// carrying is what is typed for words said to a chat's agent in its
// window: in front of the first after it was switched to, the conversation
// it goes on from, taken off the window as it is typed. back keeps it
// there again, for words that were not typed.
func (a *App) carrying(window, words string) (string, func()) {
	at := model.Record{Name: a.chatSession()}
	was, on := a.TakeCarried(at, window)
	if !on {
		return words, func() {}
	}
	back := func() { _ = a.CarryInWindow(at, window, was) }
	transcript, err := os.ReadFile(was.File)
	if err != nil {
		return "Read " + was.File + ": the conversation so far, with " + was.From + ".\n\n" + CarriedRule + "\n\n" + words, back
	}
	return CarriedText(was.From, string(transcript), cmp.Or(was.Full, was.File), words, CarriedMost), back
}

// Switched is what going on with another agent did: the agent the chat
// was with and the one it goes on with, the file its conversation was
// written out to and how many turns that has and leaves out, and the
// agent's own conversation it took up again, when it had one. Handoff is
// "" when nothing was said yet, and nothing goes over.
type Switched struct {
	From    string    `json:"from"`
	To      string    `json:"to"`
	Handoff string    `json:"handoff,omitempty"`
	Turns   int       `json:"turns"`
	LeftOut int       `json:"left_out,omitempty"`
	Resumes string    `json:"resumes,omitempty"`
	State   MainState `json:"state"`
}

// SwitchChat has a chat, or the main agent, go on with another agent, its
// conversation carried over as a window's is handed to another: written
// out as a handoff, the agent it was with stopped and the other started in
// its place with a new conversation, and the handoff said in front of the
// first thing said to it. Back to an agent it went on from, that agent
// takes its own conversation up again and is handed only what was said
// since. A chat that asks something or is at work is not switched; the
// agent it runs as already only takes another model.
func (a *App) SwitchChat(to, modelID, effort string) (Switched, error) {
	agent, known := agentNamed(a.agents(), to)
	if !known {
		return Switched{}, fmt.Errorf("mia knows no agent %s — `mia api agents` lists those it does", to)
	}
	host := a.mainHost()
	state := a.Main()
	from := state.Agent
	if agent.Name == from {
		if !state.Running || modelID == "" && effort == "" {
			return Switched{}, fmt.Errorf("%s goes on with %s already — `%s` switches its model", a.chatWho(), from, a.chatVerb("model", "<model>"))
		}
		if _, err := a.SwitchMain(modelID, effort); err != nil {
			return Switched{}, err
		}
		return Switched{From: from, To: from, State: a.Main()}, nil
	}
	switch {
	case state.Asks != nil:
		return Switched{}, askedFirst(from, *state.Asks, a.chatVerb("press"))
	case state.Working:
		return Switched{}, fmt.Errorf("%s is at work — `%s` stops it, or switch it once it rests", a.chatWho(), a.chatVerb("interrupt"))
	}
	if err := allowed(agent, modelID, effort, a.Config.Agents[agent.Name]); err != nil {
		return Switched{}, err
	}

	// what goes over: the conversation it has open, or the one it was
	// handed and has said nothing of yet
	talk := Talk{Agent: from, ID: state.Conversation}
	if state.Window != nil {
		if carried, on := a.CarriedInWindow(model.Record{Name: a.chatSession()}, state.Window.Name); on {
			talk = Talk{Agent: carried.From, ID: carried.ID}
		}
	}
	// of the two agents, the one it was with and the other, which it may have gone on from
	heads, _ := a.Conversations(Wanted{Agent: talk.Agent})
	if talk.Agent != "" && talk.Agent != agent.Name {
		theirs, _ := a.Conversations(Wanted{Agent: agent.Name})
		heads = append(heads, theirs...)
	}
	at := slices.IndexFunc(heads, func(h Head) bool { return h.Agent == talk.Agent && h.ID == talk.ID && talk.ID != "" })
	done := Switched{From: from, To: agent.Name}
	var head Head
	var turns []Turn
	var since time.Time
	if at >= 0 && heads[at].Agent == agent.Name {
		// back to the agent it was handed from before anything was said to
		// the other: it takes its own up again, and nothing goes over
		done.Resumes, at = heads[at].ID, -1
	}
	if at >= 0 {
		head = heads[at]
		folders, _ := a.where()
		var err error
		if turns, _, err = a.turnsIn(folders, head.ID, head.Cwd, head.Agent, 0); err != nil {
			return Switched{}, err
		}
		turns = slices.DeleteFunc(turns, thought)
		if back, when := a.wentOnFrom(head, agent, heads); back != "" && len(turns) > 0 {
			done.Resumes, since = back, when
			turns = slices.DeleteFunc(slices.Clone(turns), func(t Turn) bool { return !t.At.IsZero() && t.At.Before(since) })
			for i := range turns {
				if turns[i].Kind == turnAsked {
					turns[i].Text = OwnWords(turns[i].Text)
				}
			}
		} else if len(turns) == 0 {
			at = -1 // nothing said yet: nothing goes over
		}
	}

	if state.Running {
		if err := host.Kill(a.chatSession()); err != nil {
			return Switched{}, err
		}
	}
	started, err := a.startMain(agent.Name, modelID, effort, "", done.Resumes == "", done.Resumes, false, true) // told nothing until the next thing said to it
	if err != nil {
		return Switched{}, fmt.Errorf("%s is stopped, and %s did not start: %w — `%s` goes on with %s's conversation", from, agent.Name, err, a.chatVerb("start"), from)
	}
	kept := a.chatRead()
	kept.Agent, kept.Model, kept.Effort = agent.Name, modelID, effort
	if err := a.chatWrite(kept); err != nil {
		return Switched{}, err
	}
	done.State = started
	if at < 0 {
		return done, nil
	}

	now := time.Now()
	named := filepath.Join(a.MiaDir, "handoffs", head.Worktree, notInName.ReplaceAllString(head.Agent+"-"+head.ID, "-"))
	file, full := named+".md", named+"-full.md"
	text, left := handoff(head, turns, now, handoffBudget, full)
	if left == 0 {
		full = ""
	}
	done.Handoff, done.Turns, done.LeftOut = file, len(turns), left
	// the person's words and the agent's: theirs alone to read
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return done, err
	}
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		return done, err
	}
	if full != "" {
		whole, _ := handoff(head, turns, now, 0, "")
		if err := os.WriteFile(full, []byte(whole), 0o600); err != nil {
			return done, err
		}
	}
	// which went on as which is kept, for its turns to say
	one := handedOn{From: ContinuedFrom{Agent: head.Agent, ID: head.ID, Worktree: head.Worktree}, To: ContinuedTo{Agent: agent.Name, Window: WindowRef{Name: agent.Name}, ID: done.Resumes},
		Handoff: file, At: instant(now), Turns: len(turns), LeftOut: left, Resumes: done.Resumes}
	if err := writeHandedOn(fmt.Sprintf("%s.%d.json", named, now.UnixMilli()), one); err != nil {
		return done, fmt.Errorf("%s goes on with %s, and that it goes on from %s's %s is not kept: %w", a.chatWho(), agent.Name, head.Agent, head.ID, err)
	}
	if done.Resumes != "" && len(turns) == 0 {
		return done, nil // nothing was said since: it is told nothing
	}
	carried := Carried{From: head.Agent, ID: head.ID, File: file, Full: full, Resumed: done.Resumes != ""}
	if err := a.CarryInWindow(model.Record{Name: a.chatSession()}, agent.Name, carried); err != nil {
		return done, fmt.Errorf("%s goes on with %s, and what it goes on from is not kept for it: %w", a.chatWho(), agent.Name, err)
	}
	done.State = a.Main()
	return done, nil
}
