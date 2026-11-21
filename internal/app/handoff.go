package app

import (
	"cmp"
	"encoding/json"
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

// Continue is what to go on with: one conversation, another agent.
type Continue struct {
	// ID is the conversation, as a list of them names it; of one copied to
	// another worktree, Worktree says whose copy is meant.
	ID, Worktree string
	// Window says which instead of an id: the one conversation that window
	// of Worktree's session has open, by the window's name or number — the
	// session of the worktree this runs in when none is named.
	Window string
	// With is the agent it goes on with, started with a model and an
	// effort and on an account as a window's is.
	With, Model, Effort, Account string
	// DryRun writes nothing and starts nothing.
	DryRun bool
	// Quiet starts the agent and tells it nothing: whoever asked says the
	// handoff to it themselves, with the first thing the person sends.
	Quiet bool
}

// Talk names one conversation: whose it is, and its id.
type Talk struct {
	Agent string `json:"agent"`
	ID    string `json:"id"`
}

// ContinuedFrom is the conversation a handoff was written from, and the
// window that had it open then.
type ContinuedFrom struct {
	Agent    string     `json:"agent"`
	ID       string     `json:"id"`
	Worktree string     `json:"worktree"`
	Window   *WindowRef `json:"window,omitempty"`
}

// ContinuedTo is the agent a conversation goes on with and the window
// opened for it. ID is its own conversation there, which no agent says as
// it starts: a list of conversations finds it once the agent has written
// of it, and it is kept from then on.
type ContinuedTo struct {
	Agent  string    `json:"agent"`
	Window WindowRef `json:"window"`
	ID     string    `json:"id,omitempty"`
}

// Continued is what going on with another agent did, or with DryRun would.
type Continued struct {
	From ContinuedFrom `json:"from"`
	To   ContinuedTo   `json:"to"`
	// Resumes is the agent's own conversation it took up again, when this
	// one had gone on from it: the handoff is then only what was said
	// since, and To.ID is that conversation.
	Resumes string `json:"resumes,omitempty"`
	// Handoff is the file the conversation was written out to, of Turns
	// turns. One over handoffBudget has LeftOut turns of its middle left
	// out, and Full is the file that has them all.
	Handoff string `json:"handoff"`
	Full    string `json:"full,omitempty"`
	Turns   int    `json:"turns"`
	LeftOut int    `json:"left_out,omitempty"`
	// Command is what the window runs.
	Command string `json:"command"`
	// Instruction is the agent's first message, and Sent whether mia typed
	// it: an agent that asked something first, or ended, was not typed
	// into, and Problem says which. Of one started quiet, Sent is that it
	// runs, with nothing typed.
	Instruction string `json:"instruction"`
	Sent        bool   `json:"sent"`
	Problem     string `json:"problem,omitempty"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	Account     string `json:"account,omitempty"`
	DryRun      bool   `json:"dry_run"`
}

// ContinueConversation writes a conversation out as a handoff and opens a
// window in its worktree's session with another agent, told to read it and
// go on from where it stops. Which conversation is said exactly, by its id
// or by the window that has it open: it is never the one last written to.
func (a *App) ContinueConversation(want Continue) (Continued, error) {
	agent, known := agentNamed(a.agents(), want.With)
	if !known {
		return Continued{}, fmt.Errorf("mia knows no agent %s — `mia api agents` lists those it does", want.With)
	}
	heads, problems := a.Conversations(Wanted{})
	var head Head
	if want.ID == "" {
		here, err := a.RecordOrHere(want.Worktree)
		if err != nil {
			return Continued{}, err
		}
		if head, err = headIn(heads, here.Name, want.Window); err != nil {
			return Continued{}, err
		}
	} else {
		var in []string // the worktrees it is in: one copied to another is in both
		for _, h := range heads {
			if h.ID == want.ID && (want.Worktree == "" || h.Worktree == want.Worktree) && !slices.Contains(in, h.Worktree) {
				if in = append(in, h.Worktree); len(in) == 1 {
					head = h // of two accounts' in one worktree, the last written to
				}
			}
		}
		switch {
		case len(in) > 1:
			return Continued{}, fmt.Errorf("conversation %s is in %s, each going its own way — say which with --worktree", want.ID, strings.Join(in, " and "))
		case len(in) == 0 && want.Worktree != "" && slices.ContainsFunc(heads, func(h Head) bool { return h.ID == want.ID }):
			return Continued{}, fmt.Errorf("no conversation %s in %s — `mia api conversations` says which worktrees it is in", want.ID, want.Worktree)
		case len(in) == 0 && len(problems) > 0:
			return Continued{}, fmt.Errorf("no conversation %s here — %s", want.ID, strings.Join(problems, "; "))
		case len(in) == 0:
			return Continued{}, fmt.Errorf("no conversation %s here — `mia api conversations` lists them, each with its id", want.ID)
		}
	}
	if head.Agent == agent.Name {
		return Continued{}, a.itsOwn(head, want)
	}
	if head.Worktree == "" || head.Gone {
		return Continued{}, fmt.Errorf("%s is gone, and that is where this conversation was — `mia conversation resume %s --in <worktree>` takes it up in another, to go on from there", cmp.Or(head.Worktree, head.Cwd), head.ID)
	}
	record, err := a.RecordOrHere(head.Worktree)
	if err != nil {
		return Continued{}, err
	}
	host := a.sessionOf(record)
	if host.SSH != "" {
		return Continued{}, fmt.Errorf("%s's session is on a machine, and its conversations with it — go on with one in a worktree that is here", record.Name)
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
	// Back to the agent this conversation went on from: that agent takes
	// its own up again, with all it knew, and is said only what was said
	// since it was handed on, none of it what was said in front of the
	// person's first words then.
	resumes := ""
	if back, since := a.wentOnFrom(head, agent, heads); back != "" {
		if again, err := agent.resume(back); err == nil {
			resumes, command = back, again
			turns = slices.DeleteFunc(slices.Clone(turns), func(t Turn) bool { return !t.At.IsZero() && t.At.Before(since) })
			for i := range turns {
				if turns[i].Kind == turnAsked {
					turns[i].Text = OwnWords(turns[i].Text)
				}
			}
		}
	}
	command, chosen, err := a.CommandWith(command, want.Model, want.Effort, want.Account)
	if err != nil {
		return Continued{}, err
	}
	window := agent.Name
	for i := 2; host.HasWindow(record.Name, window); i++ {
		window = fmt.Sprintf("%s-%d", agent.Name, i)
	}
	now := time.Now()
	named := filepath.Join(a.MiaDir, "handoffs", record.Name, notInName.ReplaceAllString(head.Agent+"-"+head.ID, "-"))
	file, full := named+".md", named+"-full.md"
	text, left := handoff(head, turns, now, handoffBudget, full)
	if left == 0 {
		full = ""
	}
	done := Continued{From: ContinuedFrom{Agent: head.Agent, ID: head.ID, Worktree: record.Name, Window: head.Window},
		To: ContinuedTo{Agent: agent.Name, Window: WindowRef{Name: window}, ID: resumes}, Resumes: resumes, Handoff: file, Full: full, Turns: len(turns), LeftOut: left,
		Command: command, Instruction: instruction(head.Agent, file),
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
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		return done, err
	}
	if full != "" {
		whole, _ := handoff(head, turns, now, 0, "")
		if err := os.WriteFile(full, []byte(whole), 0o600); err != nil {
			return done, err
		}
	}
	if done.To.Window.Name, err = a.NewWindow(record, window, session.What{Kind: session.KindAgent, Label: agent.Name}, argv); err != nil {
		return done, err
	}
	for _, w := range host.WindowsOf(record.Name) {
		if w.Name == done.To.Window.Name {
			done.To.Window.Index = w.Index
		}
	}
	// which went on as which is kept, for a list of conversations to say
	link := fmt.Sprintf("%s.%d.json", named, now.UnixMilli())
	if err := writeHandedOn(link, handedOn{From: done.From, To: done.To, Handoff: file, At: instant(now)}); err != nil {
		return done, fmt.Errorf("%s: window %s opened, and that it goes on from %s's %s is not kept: %w", record.Name, done.To.Window.Name, head.Agent, head.ID, err)
	}
	if chosen != (Chosen{}) {
		if err := a.KeepChosen(record, done.To.Window.Name, agent.Name, chosen); err != nil {
			return done, fmt.Errorf("%s: window %s opened, and what it was started with is not kept: %w", record.Name, done.To.Window.Name, err)
		}
	}
	// as SayOnceReady says it, but never into the shell an agent that ended leaves behind
	if said, ended := settle(host, record.Name, done.To.Window.Name); ended {
		done.Problem = fmt.Sprintf("%s ended as soon as it started, saying:\n%s", agent.Name, said)
	} else if runsNothing(host, record.Name, done.To.Window.Name) {
		done.Problem = fmt.Sprintf("%s ended as soon as it started, and its window is a shell now: nothing is typed into a shell", agent.Name)
	} else if want.Quiet {
		// it runs, and is told nothing
	} else if err := say(host, record.Name, done.To.Window.Name, agent.Name, "mia window press "+record.Name+" "+done.To.Window.Name+" --", done.Instruction); err != nil {
		done.Problem = err.Error()
	}
	done.Sent = done.Problem == ""
	return done, nil
}

// wentOnFrom is the conversation of agent's that head went on from, and
// when it was handed on: the last handoff to head from one of agent's
// that is still there, in head's worktree, with no window having it open
// and a way to take it up again. "" when head went on from none.
func (a *App) wentOnFrom(head Head, agent Agent, heads []Head) (id string, since time.Time) {
	if agent.resume == nil {
		return "", time.Time{}
	}
	_, all := a.handedOnIn()
	for _, one := range all {
		if one.To.Agent != head.Agent || one.To.ID != head.ID || one.From.Agent != agent.Name || one.At.Before(since) {
			continue
		}
		if slices.ContainsFunc(heads, func(h Head) bool {
			return h.Agent == agent.Name && h.ID == one.From.ID && h.Worktree == head.Worktree && !h.Gone && !h.Running
		}) {
			id, since = one.From.ID, one.At
		}
	}
	return id, since
}

// itsOwn is why a conversation is not handed to the agent it is with: that
// agent goes on with its own, nothing lost, and how.
func (a *App) itsOwn(head Head, want Continue) error {
	resume := "mia conversation resume " + head.ID
	if want.Model != "" {
		resume += " --model " + want.Model
	}
	how := fmt.Sprintf("`%s` takes it up again (%s)", resume, a.ResumeCommand(head.Agent, head.ID))
	if head.Running && head.Window != nil {
		how = fmt.Sprintf("window %d %s of %s has it open, and `mia window model %s %s <model>` switches its model there", head.Window.Index, head.Window.Name, head.Worktree, head.Worktree, head.Window.Name)
	}
	return fmt.Errorf("conversation %s is %s's own, and %s goes on with it as it is, with nothing lost: %s — a handoff is for another agent, and carries over less", head.ID, head.Agent, head.Agent, how)
}

// headIn is the one conversation a window of a worktree's session has
// open, the window said by its name or number. Two windows of one name, or
// a window with none open, is no one conversation: the ids of those open
// there are said, to choose by.
func headIn(heads []Head, worktree, window string) (Head, error) {
	var its []Head
	var open []string
	for _, h := range heads {
		if h.Worktree != worktree || !h.Running || h.Window == nil {
			continue
		}
		open = append(open, fmt.Sprintf("%s (%s, window %d %s)", h.ID, h.Agent, h.Window.Index, h.Window.Name))
		if h.Window.Name == window || strconv.Itoa(h.Window.Index) == window {
			its = append(its, h)
		}
	}
	switch {
	case len(its) == 1:
		return its[0], nil
	case len(open) == 0:
		return Head{}, fmt.Errorf("no window of %s has a conversation open — say which by its id: `mia api conversations --worktree %s` lists them", worktree, worktree)
	case len(its) == 0:
		return Head{}, fmt.Errorf("window %s of %s has no conversation open — say which by its id: %s", window, worktree, strings.Join(open, ", "))
	}
	return Head{}, fmt.Errorf("%d windows of %s are %s, each with a conversation open — say which by its id: %s", len(its), worktree, window, strings.Join(open, ", "))
}

// handedOn is the record of one handoff, kept beside it: which conversation
// went on with which agent, in which window, and when.
type handedOn struct {
	From    ContinuedFrom `json:"from"`
	To      ContinuedTo   `json:"to"`
	Handoff string        `json:"handoff"`
	At      time.Time     `json:"at"`
}

// writeHandedOn keeps a handoff's record: whole or not at all, whoever
// else writes one then.
func writeHandedOn(file string, one handedOn) error {
	data, err := json.Marshal(one)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(file), ".handed-*")
	if err != nil {
		return err
	}
	_, err = temp.Write(append(data, '\n'))
	if closed := temp.Close(); err == nil {
		err = closed
	}
	if err == nil {
		err = os.Rename(temp.Name(), file)
	}
	if err != nil {
		os.Remove(temp.Name())
	}
	return err
}

// handedOnIn is every handoff's record in the repository and the file each
// is kept in, the oldest of one conversation's first.
func (a *App) handedOnIn() (files []string, all []handedOn) {
	dir := filepath.Join(a.MiaDir, "handoffs")
	worktrees, _ := os.ReadDir(dir)
	for _, worktree := range worktrees {
		records, _ := os.ReadDir(filepath.Join(dir, worktree.Name()))
		for _, record := range records { // by name: <agent>-<id>.<when>.json
			if filepath.Ext(record.Name()) != ".json" {
				continue
			}
			file := filepath.Join(dir, worktree.Name(), record.Name())
			var one handedOn
			if data, err := os.ReadFile(file); err == nil && json.Unmarshal(data, &one) == nil && one.Handoff != "" {
				files, all = append(files, file), append(all, one)
			}
		}
	}
	return files, all
}

// thread says of each conversation which it goes on from and which went on
// from it, by the handoffs' records. The conversation a handoff began is
// looked for until it is found, then kept in the record. Of one handed on
// more than once, the last is said.
func (a *App) thread(heads []Head) {
	files, all := a.handedOnIn()
	for i, one := range all {
		if one.To.ID == "" {
			one.To.ID = begunBy(heads, one, func(h Head) string {
				in, _ := a.where()
				turns, _, _ := a.turnsIn(in, h.ID, h.Cwd, h.Agent, 0)
				if at := slices.IndexFunc(turns, func(t Turn) bool { return t.Kind == turnAsked }); at >= 0 {
					return turns[at].Text
				}
				return ""
			})
			if one.To.ID == "" {
				continue
			}
			writeHandedOn(files[i], one) // not kept is looked for again
		}
		for j := range heads {
			h := &heads[j]
			switch {
			case h.Agent == one.From.Agent && h.ID == one.From.ID:
				h.ContinuedBy = &Talk{Agent: one.To.Agent, ID: one.To.ID}
			case h.Agent == one.To.Agent && h.ID == one.To.ID:
				h.Continues = &Talk{Agent: one.From.Agent, ID: one.From.ID}
			}
		}
	}
}

// begunBy is the conversation a handoff began: the first of its agent in
// its worktree, begun no earlier than the handoff, whose first message
// names the handoff's file; "" while there is none. A list has the first
// message cut short, so one cut inside the file's name is read whole, with
// first.
func begunBy(heads []Head, one handedOn, first func(Head) string) string {
	at := one.At.Truncate(time.Second) // an agent may say when to the second
	var found *Head
	for i := range heads {
		h := &heads[i]
		if h.Agent != one.To.Agent || h.Worktree != one.From.Worktree || h.Started.Before(at) || (found != nil && !h.Started.Before(found.Started)) {
			continue
		}
		// (Told to read the file, or sent it in front of the person's first words.)
		names := strings.Contains(h.Asked, one.Handoff) || strings.HasPrefix(h.Asked, CarriedOpens(one.From.Agent))
		if shown, cut := strings.CutSuffix(h.Asked, "…"); !names && cut && endsWithin(shown, one.Handoff) {
			names = strings.Contains(first(*h), one.Handoff)
		}
		if names {
			found = h
		}
	}
	if found == nil {
		return ""
	}
	return found.ID
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

// endsWithin reports whether text ends part of the way through a word:
// with its beginning, and not all of it.
func endsWithin(text, word string) bool {
	for n := min(len(text), len(word)-1); n > 0; n-- {
		if strings.HasSuffix(text, word[:n]) {
			return true
		}
	}
	return false
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

// handoffBudget is how many characters of conversation a handoff has: the
// other agent reads all of it before it does anything, so one over this
// keeps the first thing asked and the last turns, as many as fit, and says
// where the rest is.
const handoffBudget = 60000

// handoff is a conversation written out for another agent to go on from:
// who said what, in order, and each thing the agent did in a line — the
// command it ran, the file it changed — with none of what came back and
// nothing it only thought. Over budget characters, the turns between the
// first thing asked and the last that fit are left out, left of them, with
// a line saying so and naming full, the file that has every one; a budget
// of 0 leaves none out.
func handoff(head Head, turns []Turn, now time.Time, budget int, full string) (text string, left int) {
	const stamp = "2006-01-02 15:04 MST"
	// each turn as it is written, and whether it is a thing done: a run of those is one list
	pieces, did := make([]string, len(turns)), make([]bool, len(turns))
	said := func(n int, who string, at time.Time, text string) string {
		var b strings.Builder
		fmt.Fprintf(&b, "\n### %d · %s", n, who)
		if !at.IsZero() {
			fmt.Fprintf(&b, " · %s", at.UTC().Format(stamp))
		}
		if r := []rune(text); len(r) > handoffMessage {
			text = fmt.Sprintf("%s\n\n[… %d more characters of this message are left out]", string(r[:handoffMessage]), len(r)-handoffMessage)
		}
		fmt.Fprintf(&b, "\n\n%s\n", strings.TrimSpace(text))
		return b.String()
	}
	for i, turn := range turns {
		switch turn.Kind {
		case turnAsked:
			pieces[i] = said(turn.N, "user", turn.At, turn.Text)
		case turnSaid:
			pieces[i] = said(turn.N, head.Agent, turn.At, turn.Text)
		case turnQuestion:
			text := turn.Text
			for _, option := range turn.Options {
				text += "\n- " + option
			}
			pieces[i] = said(turn.N, head.Agent+" asked", turn.At, text)
		case turnAnswer:
			pieces[i] = said(turn.N, "user answered", turn.At, turn.Text)
		default: // what the agent did: a tool, a commit, a verb of mia's
			line := turn.Text
			if turn.Kind == turnCommit {
				line = "Committed " + turn.Hash + " " + turn.Subject
			}
			if turn.File != "" {
				line += " (`" + turn.File + "`)"
			}
			pieces[i], did[i] = "- "+strings.Join(strings.Fields(line), " ")+"\n", true
		}
	}
	runes := func(text string) int { return len([]rune(text)) }
	// kept is the turns before until and from from on: all of them, unless over budget
	until, from := len(turns), len(turns)
	if size := 0; budget > 0 {
		for _, piece := range pieces {
			size += runes(piece)
		}
		if first := slices.IndexFunc(turns, func(t Turn) bool { return t.Kind == turnAsked }); size > budget && first >= 0 {
			room := budget
			for _, piece := range pieces[:first+1] {
				room -= runes(piece)
			}
			// the last turn whatever its size, then each before it that fits
			for from > first+1 && (from == len(turns) || runes(pieces[from-1]) <= room) {
				from--
				room -= runes(pieces[from])
			}
			if until, left = first+1, from-first-1; left == 0 {
				until = len(turns)
			}
		}
	}

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
	fmt.Fprintf(&b, "- Written out by mia %s: %d turns, in order", now.UTC().Format(stamp), len(turns))
	if left > 0 {
		fmt.Fprintf(&b, ", %d of them left out here and all of them in `%s`", left, full)
	}
	fmt.Fprintf(&b, "\n\nHow to use this: it is what was said, not %s's memory. What the user and %s said is as they wrote it; a message over %d characters is cut, and says by how much. "+
		"Each thing %s did is one line, the command it ran or the file it changed, with none of what came back: read the files and run the commands again for that. "+
		"Nothing %s only thought is here.\n\n## The conversation\n",
		head.Agent, head.Agent, handoffMessage, head.Agent, head.Agent)
	write := func(from, to int) {
		for i := from; i < to; i++ {
			if did[i] && (i == from || !did[i-1]) {
				fmt.Fprintf(&b, "\n%s did:\n\n", head.Agent)
			}
			b.WriteString(pieces[i])
		}
	}
	write(0, until)
	if left > 0 {
		fmt.Fprintf(&b, "\n[… %d turns, %d to %d, are left out here to keep this short: every turn is in `%s` — look there for what you need of them]\n",
			left, turns[until].N, turns[from-1].N, full)
		write(from, len(turns))
	}
	return b.String(), left
}
