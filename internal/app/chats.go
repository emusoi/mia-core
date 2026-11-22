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
