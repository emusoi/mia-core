package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/session"
)

// An agent cannot load another's conversation: each keeps its own format.
// What goes on with another agent is a handoff, the conversation written
// out as Markdown for it to read, kept in the repository's git folder so
// the worktree stays as it was.

// Continue is what to go on with: a worktree's conversation, another agent.
type Continue struct {
	Worktree string // "" for the one this runs in
	// From says which conversation there: a window of the session, by name
	// or number, a conversation's id, or an agent, whose last it is; with
	// none, the one last written to in the worktree.
	From string
	// With is the agent it goes on with, started with a model and an
	// effort and on an account as a window's is.
	With, Model, Effort, Account string
	// DryRun writes nothing and starts nothing.
	DryRun bool
}

// Continued is what going on with another agent did, or with DryRun would.
type Continued struct {
	Worktree     string `json:"worktree"`
	Conversation string `json:"conversation"`
	From         string `json:"from"`  // the agent it was with
	Agent        string `json:"agent"` // the one it goes on with
	// Handoff is the file the conversation was written out to, of Turns
	// turns.
	Handoff string `json:"handoff"`
	Turns   int    `json:"turns"`
	// Window is the one opened, and Command what it runs.
	Window  WindowRef `json:"window"`
	Command string    `json:"command"`
	// Instruction is the agent's first message, and Sent whether mia typed
	// it: an agent that asked something first, or ended, was not typed
	// into, and Problem says which.
	Instruction string `json:"instruction"`
	Sent        bool   `json:"sent"`
	Problem     string `json:"problem,omitempty"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	Account     string `json:"account,omitempty"`
	DryRun      bool   `json:"dry_run"`
}

// ContinueConversation writes a worktree's conversation out as a handoff
// and opens a window in the worktree's session with another agent, told to
// read it and go on from where it stops.
func (a *App) ContinueConversation(want Continue) (Continued, error) {
	record, err := a.RecordOrHere(want.Worktree)
	if err != nil {
		return Continued{}, err
	}
	agent, known := agentNamed(a.agents(), want.With)
	if !known {
		return Continued{}, fmt.Errorf("mia knows no agent %s — `mia api agents` lists those it does", want.With)
	}
	host := a.sessionOf(record)
	if host.SSH != "" {
		return Continued{}, fmt.Errorf("%s's session is on a machine, and its conversations with it — go on with one in a worktree that is here", record.Name)
	}
	heads, problems := a.Conversations(Wanted{Worktree: record.Name})
	head, found := headFrom(heads, want.From)
	if !found {
		none := fmt.Sprintf("no conversation in %s", record.Name)
		if want.From != "" {
			none = fmt.Sprintf("no conversation %s names in %s: no window, no conversation's id and no agent with one", want.From, record.Name)
		}
		if len(problems) > 0 {
			return Continued{}, fmt.Errorf("%s — %s", none, strings.Join(problems, "; "))
		}
		return Continued{}, fmt.Errorf("%s — `mia api conversations --worktree %s` lists what is", none, record.Name)
	}
	in, _ := a.where()
	turns, _, err := a.turnsIn(in, head.ID, head.Cwd, head.Agent, 0)
	if err != nil {
		return Continued{}, err
	}
	if len(turns) == 0 {
		return Continued{}, fmt.Errorf("nothing is said yet in %s's conversation %s in %s: there is nothing to go on from", head.Agent, head.ID, record.Name)
	}

	// as `mia window new <worktree> <agent>` starts one: the window of that name the config launches, when it runs the agent
	command := agent.start()
	if launch, set := a.Config.Launch[agent.Name]; set && a.AgentOf(launch) == agent.Name {
		command = launch
	}
	command, chosen, err := a.CommandWith(command, want.Model, want.Effort, want.Account)
	if err != nil {
		return Continued{}, err
	}
	window := agent.Name
	for i := 2; host.HasWindow(record.Name, window); i++ {
		window = fmt.Sprintf("%s-%d", agent.Name, i)
	}
	file := filepath.Join(a.MiaDir, "handoffs", record.Name, notInName.ReplaceAllString(head.Agent+"-"+head.ID, "-")+".md")
	done := Continued{Worktree: record.Name, Conversation: head.ID, From: head.Agent, Agent: agent.Name, Handoff: file, Turns: len(turns),
		Window: WindowRef{Name: window}, Command: command, Instruction: instruction(head.Agent, file),
		Model: chosen.Model, Effort: chosen.Effort, Account: chosen.Account, DryRun: want.DryRun}
	if done.Account == "" && agent.AccountEnv != "" {
		done.Account = DefaultAccount
	}
	argv, err := a.LaunchArgv(record, command)
	if err != nil || want.DryRun {
		return done, err
	}

	// the person's words and the agent's: theirs alone to read
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return done, err
	}
	if err := os.WriteFile(file, []byte(handoff(head, turns, time.Now())), 0o600); err != nil {
		return done, err
	}
	if done.Window.Name, err = a.NewWindow(record, window, session.What{Kind: session.KindAgent, Label: agent.Name}, argv); err != nil {
		return done, err
	}
	for _, w := range host.WindowsOf(record.Name) {
		if w.Name == done.Window.Name {
			done.Window.Index = w.Index
		}
	}
	if chosen != (Chosen{}) {
		if err := a.KeepChosen(record, done.Window.Name, agent.Name, chosen); err != nil {
			return done, fmt.Errorf("%s: window %s opened, and what it was started with is not kept: %w", record.Name, done.Window.Name, err)
		}
	}
	// as SayOnceReady says it, but never into the shell an agent that ended leaves behind
	if said, ended := settle(host, record.Name, done.Window.Name); ended {
		done.Problem = fmt.Sprintf("%s ended as soon as it started, saying:\n%s", agent.Name, said)
	} else if runsNothing(host, record.Name, done.Window.Name) {
		done.Problem = fmt.Sprintf("%s ended as soon as it started, and its window is a shell now: nothing is typed into a shell", agent.Name)
	} else if err := say(host, record.Name, done.Window.Name, agent.Name, "mia window press "+record.Name+" "+done.Window.Name+" --", done.Instruction); err != nil {
		done.Problem = err.Error()
	}
	done.Sent = done.Problem == ""
	return done, nil
}

// runsNothing reports whether the shell a window was started with has
// nothing running under it: the agent started there has ended, and what is
// typed now is typed into a shell. False when ps does not say.
func runsNothing(host session.Host, worktree, window string) bool {
	all := processes()
	for _, w := range host.WindowsOf(worktree) {
		if all == nil || w.Name != window {
			continue
		}
		for _, one := range all {
			if one.parent == w.PID {
				return false
			}
		}
		return true
	}
	return false
}

// notInName is what of an agent's name and a conversation's id is not put
// in a file's name.
var notInName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// headFrom is the conversation a word names among a worktree's, the last
// written to first: the one a window of that name or number has open, else
// the one of that id, else the last of that agent's; with no word, the
// last written to.
func headFrom(heads []Head, from string) (Head, bool) {
	if from == "" {
		if len(heads) == 0 {
			return Head{}, false
		}
		return heads[0], true
	}
	for _, names := range []func(Head) bool{
		func(h Head) bool {
			return h.Running && h.Window != nil && (h.Window.Name == from || strconv.Itoa(h.Window.Index) == from)
		},
		func(h Head) bool { return h.ID == from },
		func(h Head) bool { return h.Agent == from },
	} {
		if at := slices.IndexFunc(heads, names); at >= 0 {
			return heads[at], true
		}
	}
	return Head{}, false
}

// instruction is the first message of the agent a conversation goes on
// with: one line, as it is typed.
func instruction(from, file string) string {
	return "Read " + file + " — a conversation the person had with " + from + " in this worktree, written out by mia because you cannot load " + from + "'s own. " +
		"Go on from where it stops, as if it had been with you: do not start over, and do not do again what it says is done. " +
		"Look at the worktree as it is now (git status, git log) before you change anything, then say in a few lines where things stand and what you do next."
}

// handoffMessage is how many characters of one message a handoff has: what
// is over is left out, and said.
const handoffMessage = 6000

// handoff is a conversation written out for another agent to go on from:
// who said what, in order, and each thing the agent did in a line — the
// command it ran, the file it changed — with none of what came back.
func handoff(head Head, turns []Turn, now time.Time) string {
	const stamp = "2006-01-02 15:04 MST"
	var b strings.Builder
	fmt.Fprintf(&b, "# Handoff: a conversation with %s, to go on from\n\n", head.Agent)
	fmt.Fprintf(&b, "- From: %s, conversation `%s`", head.Agent, head.ID)
	if head.Account != "" {
		fmt.Fprintf(&b, ", on its account %s", head.Account)
	}
	fmt.Fprintf(&b, "\n- Worktree: %s, `%s`\n", head.Worktree, head.Cwd)
	if !head.Started.IsZero() {
		fmt.Fprintf(&b, "- Begun %s, last written to %s\n", head.Started.UTC().Format(stamp), head.Active.UTC().Format(stamp))
	}
	fmt.Fprintf(&b, "- Written out by mia %s: %d turns, in order\n\n", now.UTC().Format(stamp), len(turns))
	fmt.Fprintf(&b, "What the user and %s said is as they wrote it; a message over %d characters is cut, and says by how much. "+
		"Each thing %s did is one line, the command it ran or the file it changed, with none of what came back: read the files and run the commands again for that.\n\n## The conversation\n",
		head.Agent, handoffMessage, head.Agent)
	listing := false // in a run of things done
	said := func(n int, who string, at time.Time, text string) {
		listing = false
		fmt.Fprintf(&b, "\n### %d · %s", n, who)
		if !at.IsZero() {
			fmt.Fprintf(&b, " · %s", at.UTC().Format(stamp))
		}
		if r := []rune(text); len(r) > handoffMessage {
			text = fmt.Sprintf("%s\n\n[… %d more characters of this message are left out]", string(r[:handoffMessage]), len(r)-handoffMessage)
		}
		fmt.Fprintf(&b, "\n\n%s\n", strings.TrimSpace(text))
	}
	for _, turn := range turns {
		switch turn.Kind {
		case turnAsked:
			said(turn.N, "user", turn.At, turn.Text)
		case turnSaid:
			said(turn.N, head.Agent, turn.At, turn.Text)
		case turnQuestion:
			text := turn.Text
			for _, option := range turn.Options {
				text += "\n- " + option
			}
			said(turn.N, head.Agent+" asked", turn.At, text)
		case turnAnswer:
			said(turn.N, "user answered", turn.At, turn.Text)
		default: // what the agent did: a tool, a commit, a verb of mia's
			if !listing {
				fmt.Fprintf(&b, "\n%s did:\n\n", head.Agent)
				listing = true
			}
			line := turn.Text
			if turn.Kind == turnCommit {
				line = "Committed " + turn.Hash + " " + turn.Subject
			}
			if turn.File != "" {
				line += " (`" + turn.File + "`)"
			}
			fmt.Fprintf(&b, "- %s\n", strings.Join(strings.Fields(line), " "))
		}
	}
	return b.String()
}
