package app

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/session"
)

// Wanted is which conversations are wanted: a worktree's, an agent's, the
// first few. Its zero value is all of them.
type Wanted struct {
	Worktree string
	Agent    string
	Limit    int
}

// where is the repository's folders, and the worktrees there are now.
func (a *App) where() (folders, []model.Record) {
	records, _ := a.Store.Load()
	in := folders{Root: a.Root, Paths: []string{a.Root}}
	for _, r := range records {
		if r.Path != a.Root {
			in.Paths = append(in.Paths, r.Path)
		}
	}
	return in, records
}

// Conversations is the agent conversations of the repository, its main
// checkout and every worktree — one that is gone too — the last written to
// first. An agent that could not say what it has is named in problems, and
// the others' are listed all the same.
func (a *App) Conversations(want Wanted) (heads []Head, problems []string) {
	in, records := a.where()
	var agents []Agent
	for _, agent := range a.agents() {
		if agent.talks != nil && (want.Agent == "" || want.Agent == agent.Name) {
			agents = append(agents, agent)
		}
	}
	listed, failed := make([][]Head, len(agents)), make([]error, len(agents))
	var wg sync.WaitGroup
	for i, agent := range agents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listed[i], failed[i] = agent.talks.heads(in)
		}()
	}
	wg.Wait()
	for i, agent := range agents {
		if failed[i] != nil {
			problems = append(problems, fmt.Sprintf("agent %s: %v", agent.Name, failed[i]))
		}
		heads = append(heads, listed[i]...)
	}
	for i := range heads {
		heads[i].Worktree, heads[i].Gone = a.worktreeOf(heads[i].Cwd, records)
		// one format for both, whatever the agent's file and the disk said
		heads[i].Started, heads[i].Active = instant(heads[i].Started), instant(heads[i].Active)
	}
	heads = a.markRunning(heads, records)
	a.thread(heads)
	heads = slices.DeleteFunc(heads, func(h Head) bool {
		// an agent in a window is listed with what it has open, wanted or not
		return (want.Worktree != "" && h.Worktree != want.Worktree) || (want.Agent != "" && h.Agent != want.Agent)
	})
	slices.SortStableFunc(heads, func(x, y Head) int { return y.Active.Compare(x.Active) })
	if want.Limit > 0 && len(heads) > want.Limit {
		heads = heads[:want.Limit]
	}
	return heads, problems
}

// instant is a time as a list says it: in UTC, to the millisecond.
func instant(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }

// worktreeOf is the name of the worktree a folder is or is in, and whether
// the folder is gone. A worktree that was removed is known by what follows
// the main checkout's path in its own: <root>.<name>.
func (a *App) worktreeOf(cwd string, records []model.Record) (string, bool) {
	_, err := os.Stat(cwd)
	gone := err != nil
	under := func(dir string) bool { return cwd == dir || strings.HasPrefix(cwd, dir+string(filepath.Separator)) }
	for _, r := range records {
		if under(r.Path) {
			return r.Name, gone
		}
	}
	if under(a.Root) {
		return filepath.Base(a.Root), gone
	}
	if rest, ok := strings.CutPrefix(cwd, a.Root+"."); ok {
		name, _, _ := strings.Cut(rest, string(filepath.Separator))
		return name, gone
	}
	return "", gone
}

// markRunning marks the conversation each agent in a window has open: the
// one the agent itself says it has, for an agent that says, and otherwise
// the newest of that agent's in the window's worktree. One the agent says
// it has and has written nothing of yet — just started, or cleared — is in
// no list an agent gives: it is added, with nothing asked or said.
// ponytail: two conversations of one agent in one worktree are told apart
// only by which was written to last, unless the agent says which it holds —
// claude does, in sessions/<pid>.json. A SessionStart hook that records the
// id for each window is the upgrade, for codex and pi most of all.
func (a *App) markRunning(heads []Head, records []model.Record) []Head {
	panes := session.Here().AllPanes()
	if len(panes) == 0 {
		return heads
	}
	table := a.agents()
	chosen := a.chosen()
	// every process of a Mac takes ps a tenth of a second: asked only for a
	// pane whose program tmux knows by its launcher's name
	behind := sync.OnceValue(foreground)
	for _, record := range records {
		if a.sessionOf(record).SSH != "" {
			continue // its windows are on a machine, and its conversations with them
		}
		its := panes[session.Name(record.Name)]
		titles := map[int]string{}
		for _, pane := range slices.Backward(its) {
			titles[pane.Index] = pane.Title // a window is its first pane
		}
		for _, window := range session.WindowsFrom(its) {
			pane := struct {
				Index, PID int
				Window     string
			}{window.Index, window.PID, window.Name}
			command := window.Command
			if command == "node" || command == "bun" {
				command = behind()[window.PID]
			}
			name := agentIn(table, command, titles[window.Index])
			if name == "" {
				continue
			}
			agent, _ := agentNamed(table, name)
			said := agent.holding(pane.PID)
			held := said.ID
			kept := chosen[chosenKey(session.Name(record.Name), pane.PID)]
			account := kept.Account
			if account == "" && agent.AccountEnv != "" {
				account = DefaultAccount
			}
			var its *Head
			for i := range heads {
				h := &heads[i]
				if h.Agent != name || h.Worktree != record.Name || h.Running {
					continue
				}
				if held == "" && h.Account != "" && h.Account != account {
					continue // another account's: not the one this window can have open
				}
				if held != "" && h.ID == held {
					// a copy on another account has the same id: the window's own is the one
					if its == nil || h.Account == account {
						its = h
					}
					continue
				}
				if held == "" && (its == nil || h.Active.After(its.Active)) {
					its = h
				}
			}
			if its == nil && held != "" && !slices.ContainsFunc(heads, func(h Head) bool { return h.Agent == name && h.ID == held }) {
				if cwd := cmp.Or(said.Cwd, record.Path); a.inWorktree(cwd, record, records) {
					heads = append(heads, unsaid(name, said, account, cwd, record.Name))
					its = &heads[len(heads)-1]
				}
			}
			if its != nil {
				its.Running, its.Window = true, &WindowRef{Index: pane.Index, Name: pane.Window}
				its.Model, its.Effort = kept.Model, kept.Effort
				if its.Account == "" && agent.AccountEnv != "" {
					its.Account = account // a plugin's agent, which does not say whose folder it is in
				}
			}
		}
	}
	return heads
}

// holds is what an agent wrote of the conversation one of its processes
// has open: which, and when it says them where it is, when it was begun
// and when the agent last wrote of it.
type holds struct {
	ID      string
	Cwd     string
	Started time.Time
	Active  time.Time
}

// holding is what the agent says the program in front in a pane has open;
// nothing for an agent that does not say.
func (a Agent) holding(pid int) holds {
	if a.held == "" || pid <= 0 {
		return holds{}
	}
	out, _ := run.Local("sh").Capture("-c", a.held, "sh", strconv.Itoa(pid))
	return heldIn(out)
}

// inWorktree reports whether a folder is a worktree's own or in it, and no
// other's: where a conversation of one of its windows is made.
func (a *App) inWorktree(cwd string, record model.Record, records []model.Record) bool {
	name, _ := a.worktreeOf(cwd, records)
	return name == record.Name
}

// unsaid is the head of a conversation an agent says it has open and has
// written nothing of: begun when the agent says, and otherwise now, and
// last written to when the agent last wrote of it — claude does as it
// starts one — so that it is among the first, and a list asked for twice
// says the same.
func unsaid(agent string, said holds, account, cwd, worktree string) Head {
	started := cmp.Or(said.Started, said.Active, time.Now())
	return Head{ID: said.ID, Agent: agent, Account: account, Cwd: cwd, Worktree: worktree,
		Started: instant(started), Active: instant(cmp.Or(said.Active, started))}
}

// foreground is the command line in front in each terminal, by the process
// the terminal was started with: what a pane runs, whatever shell is
// between.
func foreground() map[int]string {
	out, err := run.Local("ps").Capture("-axo", "pid=,tpgid=,args=")
	if err != nil {
		return nil
	}
	front, args := map[int]int{}, map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		group, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		front[pid], args[pid] = group, strings.Join(fields[2:], " ")
	}
	commands := map[int]string{}
	for pid, group := range front {
		if group > 0 {
			commands[pid] = args[group]
		}
	}
	return commands
}

// ConversationTurns is one conversation as turns for a chat view, in
// order, those after turn number after: a view that polls asks for what
// came after the last it has. One that was copied to another worktree is
// the copy last written to.
func (a *App) ConversationTurns(id string, after int) ([]Turn, error) {
	return a.ConversationIn(id, "", after)
}

// ConversationIn is ConversationTurns of the copy in one worktree, named as
// a list of conversations names it; with no worktree, the last written to.
func (a *App) ConversationIn(id, worktree string, after int) ([]Turn, error) {
	in, _ := a.where()
	cwd, whose := "", ""
	var heads []Head
	if worktree != "" {
		heads, _ = a.Conversations(Wanted{Worktree: worktree})
		at := slices.IndexFunc(heads, func(h Head) bool { return h.ID == id })
		if at < 0 {
			return nil, fmt.Errorf("no conversation %s in %s — `mia api conversations --worktree %s` lists what is", id, worktree, worktree)
		}
		cwd, whose = heads[at].Cwd, heads[at].Agent
	}
	turns, found, err := a.turnsIn(in, id, cwd, whose, after)
	if err == nil && !found {
		if worktree == "" {
			heads, _ = a.Conversations(Wanted{})
		}
		if slices.ContainsFunc(heads, func(h Head) bool { return h.ID == id && h.Running }) {
			return []Turn{}, nil // a window has it open, and nothing is written of it yet
		}
		err = fmt.Errorf("no conversation %s here — `mia api conversations` lists them", id)
	}
	return turns, err
}

// turnsIn is a conversation made in the folders as turns, of the first
// agent that has it, or of the one named; found is false when none has.
func (a *App) turnsIn(in folders, id, cwd, whose string, after int) (turns []Turn, found bool, err error) {
	for _, agent := range a.agents() {
		if agent.talks == nil || (whose != "" && agent.Name != whose) {
			continue
		}
		turns, found, err := agent.talks.turns(in, id, cwd, after)
		if err != nil {
			if found {
				return nil, true, fmt.Errorf("agent %s: %w", agent.Name, err)
			}
			continue // an agent that cannot say what it has does not hide another's
		}
		if found {
			return append([]Turn{}, turns...), true, nil
		}
	}
	return nil, false, nil
}

// Resumed is what resuming a conversation did.
type Resumed struct {
	Worktree string    `json:"worktree"`
	Window   WindowRef `json:"window"`
	Command  string    `json:"command,omitempty"`
	Already  bool      `json:"already"` // a window had it open: nothing was opened
	Copied   bool      `json:"copied"`  // it was made in another folder or account, and copied to this one's
	// Model and Effort are what mia started the agent with, or the window
	// that had it open was started with or last switched to; left out when
	// mia did not choose them.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	// Account is the one the agent was started on, or the window that had
	// it open runs on; left out for an agent mia keeps no accounts of.
	Account string `json:"account,omitempty"`
}

// ResumeConversation opens a window in a worktree's session with the agent
// taking the conversation up again: in the worktree it was made in, or in
// another when told. One a window already has open is left as it is. Of a
// conversation that was copied to another worktree, from names the
// worktree whose copy is meant. The agent is started with a model and an
// effort, those given or the configured ones, as a window's is, and on an
// account: the one given, or the one the repository starts the agent's
// sessions on. One made on another account is copied into that one's
// folder first, and stays where it was too.
func (a *App) ResumeConversation(id, from, in, model, effort, account string) (Resumed, error) {
	heads, problems := a.Conversations(Wanted{})
	at := slices.IndexFunc(heads, func(h Head) bool { return h.ID == id && (from == "" || h.Worktree == from) })
	if at < 0 {
		if from != "" && slices.ContainsFunc(heads, func(h Head) bool { return h.ID == id }) {
			return Resumed{}, fmt.Errorf("no conversation %s in %s — `mia api conversations` says which worktrees it is in", id, from)
		}
		if len(problems) > 0 {
			return Resumed{}, fmt.Errorf("no conversation %s here — %s", id, strings.Join(problems, "; "))
		}
		return Resumed{}, fmt.Errorf("no conversation %s here — `mia api conversations` lists them", id)
	}
	// one copied to another worktree is there twice, under one id: the last
	// written to is taken, as it is the first, unless one was named
	head := heads[at]
	agent, _ := agentNamed(a.agents(), head.Agent)
	if agent.resume == nil {
		return Resumed{}, fmt.Errorf("mia does not know how %s takes a conversation up again", head.Agent)
	}
	target := in
	if target == "" {
		if head.Worktree == "" || head.Gone {
			name := head.Worktree
			if name == "" {
				name = head.Cwd
			}
			return Resumed{}, fmt.Errorf("%s is gone, and that is where this conversation was — `mia unarchive %s` brings back one that was archived, `mia new <branch>` makes a worktree of its branch again; or take it up in another: mia conversation resume %s --in <worktree>", name, name, id)
		}
		target = head.Worktree
	}
	record, err := a.RecordOrHere(target)
	if err != nil {
		return Resumed{}, err
	}
	host := a.sessionOf(record)
	if host.SSH != "" {
		return Resumed{}, fmt.Errorf("%s's session is on a machine, and this conversation is kept here — resume it in a worktree that is here", record.Name)
	}
	if same := slices.IndexFunc(heads, func(h Head) bool {
		return h.ID == id && h.Agent == head.Agent && h.Worktree == record.Name && h.Running && h.Window != nil
	}); same >= 0 {
		open := heads[same]
		if model != "" || effort != "" {
			return Resumed{}, fmt.Errorf("window %s of %s has it open already — `mia window model %s %s <model>` switches it, or stop it first", open.Window.Name, record.Name, record.Name, open.Window.Name)
		}
		if account != "" && account != cmp.Or(open.Account, DefaultAccount) {
			return Resumed{}, fmt.Errorf("window %s of %s has it open already, on %s — end %s there first, then resume it on %s", open.Window.Name, record.Name, cmp.Or(open.Account, DefaultAccount), agent.Name, account)
		}
		return Resumed{Worktree: record.Name, Window: *open.Window, Already: true, Model: open.Model, Effort: open.Effort, Account: open.Account}, nil
	}
	account, err = a.account(agent, account, a.placeAccount(agent))
	if err != nil {
		return Resumed{}, err
	}
	on := "" // the account it is taken up on, by name, for an agent with accounts
	if agent.AccountEnv != "" {
		on = cmp.Or(account, DefaultAccount)
	}
	command, err := agent.resume(id)
	if err != nil {
		return Resumed{}, fmt.Errorf("agent %s: %w", agent.Name, err)
	}
	with, chosen, err := choose(agent, command, model, effort, a.Config.Agents[agent.Name])
	if err != nil {
		return Resumed{}, err
	}
	if len(with) > 0 {
		command = put(command, with)
	}
	chosen.Account = account
	command = withAccount(agent, account, command)
	done := Resumed{Worktree: record.Name, Model: chosen.Model, Effort: chosen.Effort, Account: on}
	to := record.Path
	if head.Worktree == record.Name {
		to = head.Cwd
		if head.Cwd != record.Path {
			command = "cd " + shellQuote(head.Cwd) + " && " + command // made in a folder under the worktree
		}
	}
	elsewhere := head.Worktree != record.Name && agent.byFolder          // kept under its folder's name
	otherAccount := head.Account != "" && on != "" && head.Account != on // kept in another account's folder
	if (elsewhere || otherAccount) && agent.carrier != "" {
		// taken up there only once a copy is there, as when a worktree
		// moves. One already there, from the last time, has what was said
		// since and is left alone.
		there := slices.ContainsFunc(heads, func(h Head) bool {
			return h.ID == id && h.Agent == head.Agent && h.Worktree == record.Name && (on == "" || h.Account == on)
		})
		if !there {
			local := side{}
			packed, err := local.bytesOf(carrierOf(agent, head.Account), head.Cwd, to, "pack", id)
			if err != nil || len(packed) == 0 {
				return Resumed{}, fmt.Errorf("the conversation could not be copied to %s", record.Name)
			}
			if err := local.feed(packed, landing(agent, account)); err != nil {
				return Resumed{}, err
			}
			done.Copied = true
		}
	}
	window := agent.Name
	for i := 2; host.HasWindow(record.Name, window); i++ {
		window = fmt.Sprintf("%s-%d", agent.Name, i)
	}
	argv, err := a.LaunchArgv(record, command)
	if err != nil {
		return Resumed{}, err
	}
	if window, err = a.NewWindow(record, window, session.What{Kind: session.KindAgent, Label: agent.Name}, argv); err != nil {
		return Resumed{}, err
	}
	done.Command = command
	done.Window.Name = window
	for _, w := range host.WindowsOf(record.Name) {
		if w.Name == window {
			done.Window.Index = w.Index
		}
	}
	if chosen != (Chosen{}) {
		if err := a.KeepChosen(record, window, agent.Name, chosen); err != nil {
			return done, fmt.Errorf("%s: window %s opened, and what it was started with is not kept: %w", record.Name, window, err)
		}
	}
	return done, nil
}

// ResumeCommand is the command line by which an agent takes one of its
// conversations up again, typed in the folder it was made in.
func (a *App) ResumeCommand(agent, id string) string {
	if known, ok := agentNamed(a.agents(), agent); ok && known.resume != nil {
		if command, err := known.resume(id); err == nil {
			return command
		}
	}
	return "mia conversation resume " + id
}

// LaunchArgv is a command as a window runs it: by the session's shell, as
// a login shell does, and with that shell left when the command ends.
func (a *App) LaunchArgv(record model.Record, command string) ([]string, error) {
	shell := a.Config.Dotfiles.Shell
	if shell == "" {
		host, err := a.SessionHostOf(record)
		if err != nil {
			return nil, err
		}
		if host.SSH == "" {
			shell = os.Getenv("SHELL")
		}
		if shell == "" {
			shell = "/bin/sh"
		}
	}
	return []string{shell, "-lc", command + "; exec " + run.ShellJoin([]string{shell}) + " -l"}, nil
}
