package app

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/session"
)

// Agent is everything mia knows that is one coding agent's own: how to tell
// it in a window, how it goes on with a conversation, where it keeps them
// and how they cross to another machine. claude, codex, pi, opencode and
// agy are compiled in — a move runs from the binary on the other machine, and
// listing has to be fast — and a plugin adds one by declaring it in its
// manifest.
type Agent struct {
	Name string
	// Commands are the first words of a command line that mean this agent;
	// its name alone when there are none.
	Commands []string
	// Title is a pane title that means it, for one whose command says nothing.
	Title *regexp.Regexp

	// goOn makes it go on with its folder's last conversation, and has are
	// the words that say it already does.
	goOn string
	has  []string
	// resume is the command line that takes up one conversation by its id.
	resume func(id string) (string, error)
	// byFolder is an agent that keeps a conversation under its folder's
	// name, so one resumed in another folder has to be copied there first.
	byFolder bool
	// carrier is the shell script that packs a folder's conversation for
	// another path and says how to pick it up there: see carriers.
	carrier string
	// held is a shell script that prints, for a pane's process id, what the
	// agent itself wrote about the conversation it has open: JSON with a
	// sessionId. Empty for an agent that does not say.
	held string
	// talks is its conversations; nil for an agent mia cannot read them of.
	talks talks
	// notes is the file in its folder it reads its instructions from;
	// AGENTS.md when empty.
	notes string
	// stop is the key that stops it mid-work, as tmux names it; Escape
	// when empty.
	stop string

	// Models are the models it can be started with or switched to, and
	// Efforts how hard it can be made to think; either is empty for one
	// mia knows none of.
	Models  []plugin.Model
	Efforts []string
	// with is the words, put after the one that runs it, that start it
	// with a model and an effort, either "" when none is chosen.
	with func(model, effort string) []string
	// to is the lines typed into its window to switch it while it runs;
	// nil for one that cannot.
	to func(model, effort string) []string
	// lists asks the agent itself for its models, for one whose models are
	// what its providers have and not mia's to know; nil for any other.
	lists func() []plugin.Model
	// again is an agent that takes a model only as it starts, and is
	// switched by being started again on the conversation it has open.
	again bool

	// AccountEnv is the variable that names the folder it keeps its
	// sign-in, its settings and its conversations in: one folder for each
	// account. Empty for an agent mia keeps no accounts of.
	AccountEnv string
	// home is that folder when the variable is not set, under the home
	// directory; empty for one mia does not know it of.
	home string
	// signedIn is the file in that folder that is there once someone has
	// signed in; empty for one mia cannot tell by a file.
	signedIn string
	// keychain is where it may keep a sign-in out of its folder — the
	// systems it may, by Go's name for them — so that no file there says
	// nothing.
	keychain []string
	// shared is what of its own folder each account of mia's links to, so
	// a session on one has the person's setup: settings, instructions,
	// skills and the like, named one by one. Never a sign-in, a
	// conversation or anything else the agent keeps state in. Empty for
	// one mia shares nothing of.
	shared []string
	// signals is the file in its folder that makes it say what it does
	// with mia signal, and says is what mia writes there: an extension of
	// the agent's own kind. Empty for one mia has none for.
	signals string
	says    string
}

// start is the word that runs it.
func (a Agent) start() string {
	if len(a.Commands) > 0 {
		return a.Commands[0]
	}
	return a.Name
}

// takes reports whether it has words to be started with a model, and
// with an effort.
func (a Agent) takes() (model, effort bool) {
	if a.with == nil {
		return false, false
	}
	return len(a.with("a", "")) > 0, len(a.with("", "a")) > 0
}

func (a Agent) notesFile() string {
	if a.notes != "" {
		return a.notes
	}
	return "AGENTS.md"
}

func (a Agent) stopKey() string {
	if a.stop != "" {
		return a.stop
	}
	return "Escape"
}

// talks is one agent's conversations, as mia reads them.
type talks interface {
	// heads is the conversations made in the folders, in no order.
	heads(in folders) ([]Head, error)
	// turns is one conversation's turns after turn number after; found is
	// false when the conversation is not this agent's. One that was copied
	// to another folder is the copy made in cwd, or with no cwd the one
	// last written to.
	turns(in folders, id, cwd string, after int) (turns []Turn, found bool, err error)
	// follow is turns as the conversation is written: each call of next
	// gives what was written since the call before — the first, what turns
	// gives — and a turn that grew again under its n; found is false while
	// the conversation is not this agent's, or nothing of it is written
	// yet. every is how often next is worth calling.
	follow(in folders, id, cwd string, after int) (next func() (turns []Turn, found bool, err error), every time.Duration)
	// written is where the agent writes the folders' conversations as it
	// goes — each file, and each folder a new one is added to — so that a
	// change to one of them is when to read again; none where nothing
	// says.
	written(in folders) []string
	// search is the conversations in the folders that mention text, each at
	// the place it comes up.
	search(in folders, text string, limit int) ([]Recalled, error)
}

// folders is where a repository's conversations were made: its main
// checkout, and each worktree there is now. A worktree that is gone was at
// <root>.<name>, which is how its conversations are still found.
type folders struct {
	Root  string
	Paths []string // every worktree's path, the main checkout's among them
}

// holds reports whether a conversation made in cwd is this repository's.
func (f folders) holds(cwd string) bool {
	if cwd == "" {
		return false
	}
	if cwd == f.Root || strings.HasPrefix(cwd, f.Root+".") || strings.HasPrefix(cwd, f.Root+string(filepath.Separator)) {
		return true
	}
	return slices.ContainsFunc(f.Paths, func(p string) bool {
		return cwd == p || strings.HasPrefix(cwd, p+string(filepath.Separator))
	})
}

// words starts an agent that takes its model and its effort each as
// words of its own: %s in either is the one chosen.
func words(model, effort []string) func(string, string) []string {
	fill := func(words []string, chosen string) []string {
		var filled []string
		for _, word := range words {
			filled = append(filled, strings.ReplaceAll(word, "%s", chosen))
		}
		return filled
	}
	return func(m, e string) []string {
		var said []string
		if m != "" {
			said = append(said, fill(model, m)...)
		}
		if e != "" {
			said = append(said, fill(effort, e)...)
		}
		return said
	}
}

// typing switches a running agent that has a command for each: %s in
// either is the one chosen, and one it has no command for is not typed.
func typing(model, effort string) func(string, string) []string {
	return func(m, e string) []string {
		var typed []string
		if m != "" && model != "" {
			typed = append(typed, strings.ReplaceAll(model, "%s", m))
		}
		if e != "" && effort != "" {
			typed = append(typed, strings.ReplaceAll(effort, "%s", e))
		}
		return typed
	}
}

func fixed(format string) func(string) (string, error) {
	return func(id string) (string, error) { return strings.ReplaceAll(format, "%s", id), nil }
}

// builtin is each coding agent mia knows without a plugin. The first five
// have conversations mia reads; of the others it knows only how to start
// them again.
var builtin = []Agent{
	{Name: "claude", goOn: "--continue", has: []string{"-c", "--continue", "-r", "--resume"},
		resume: fixed("claude --resume %s"), byFolder: true, carrier: carriers["claude"], notes: "CLAUDE.md",
		// on macOS its sign-in is in the Keychain, under a name made from the
		// folder's path when the variable is set
		AccountEnv: "CLAUDE_CONFIG_DIR", home: ".claude", signedIn: ".credentials.json", keychain: []string{"darwin"},
		// its setup, the hooks in settings.json that tell mia what it does
		// among it; not .credentials.json, .claude.json (the account it is
		// signed in as), projects, sessions, todos, history.jsonl or caches
		shared: []string{"settings.json", "CLAUDE.md", "skills", "commands", "agents", "plugins", "output-styles", "hooks",
			"statusline.sh", "statusline-command.sh"},
		// claude writes the session of each of its processes to sessions/<pid>.json
		// in the folder of the account it runs on: its own, or one of mia's
		held: `pid="$(ps -o tpgid= -p "$1" | tr -d ' ')"
home="${CLAUDE_CONFIG_DIR:-}"; case "$home" in "" | "${XDG_CONFIG_HOME:-$HOME/.config}/mia/accounts/"*) home="$HOME/.claude" ;; esac
for h in "$home" "${XDG_CONFIG_HOME:-$HOME/.config}/mia/accounts/claude/"*; do
	[ -f "$h/sessions/$pid.json" ] && exec cat "$h/sessions/$pid.json"
done`,
		talks: transcripts{agent: "claude", files: claudeFiles, find: claudeFind, id: claudeID, decode: claudeLine},
		// the aliases it takes for the latest of each, and /model and /effort in it
		Models:  []plugin.Model{{ID: "opus", Label: "Opus"}, {ID: "sonnet", Label: "Sonnet"}, {ID: "haiku", Label: "Haiku"}, {ID: "fable", Label: "Fable"}},
		Efforts: []string{"low", "medium", "high", "xhigh", "max"},
		with:    words([]string{"--model", "%s"}, []string{"--effort", "%s"}), to: typing("/model %s", "/effort %s")},
	{Name: "codex", goOn: "resume --last", has: []string{"resume"},
		resume: fixed("codex resume %s"), carrier: carriers["codex"],
		// auth.json, or the system's keyring when its config says so
		AccountEnv: "CODEX_HOME", home: ".codex", signedIn: "auth.json", keychain: []string{"darwin", "linux", "windows"},
		talks: transcripts{agent: "codex", files: codexFiles, find: codexFind, id: codexID, decode: codexLine},
		// -c takes a value that is no TOML as it is; /model in it takes its effort with --reasoning
		Models:  []plugin.Model{{ID: "gpt-6-luna", Label: "GPT-6 Luna"}, {ID: "gpt-5.6-terra", Label: "GPT-5.6 Terra"}, {ID: "gpt-5.6-luna", Label: "GPT-5.6 Luna"}},
		Efforts: []string{"low", "medium", "high", "xhigh", "max"},
		with:    words([]string{"-m", "%s"}, []string{"-c", "model_reasoning_effort=%s"}),
		to: func(model, effort string) []string {
			line := "/model " + model
			if effort != "" {
				line += " --reasoning=" + effort
			}
			return []string{line}
		}},
	{Name: "pi", Title: regexp.MustCompile(`^\s*(π( |$)|pi - )`), goOn: "--continue", has: []string{"-c", "--continue", "-r", "--resume", "--session"},
		resume: fixed("pi --session %s"), byFolder: true, carrier: carriers["pi"],
		AccountEnv: "PI_CODING_AGENT_DIR", home: ".pi/agent", signedIn: "auth.json",
		// it loads each extensions/*.ts of its folder as it starts
		signals: "extensions/mia-signal.ts", says: piSignal,
		talks: transcripts{agent: "pi", files: piFiles, find: piFind, id: piID, decode: piLine},
		// provider/id, as /model in it takes one exactly; its effort is its thinking level
		Models: []plugin.Model{{ID: "anthropic/claude-opus-5", Label: "Claude Opus 5"}, {ID: "anthropic/claude-sonnet-5", Label: "Claude Sonnet 5"},
			{ID: "openai-codex/gpt-5.6-sol", Label: "GPT-5.6 Sol"}, {ID: "openai-codex/gpt-5.6-luna", Label: "GPT-5.6 Luna"}},
		Efforts: []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"},
		with:    words([]string{"--model", "%s"}, []string{"--thinking", "%s"}), to: typing("/model %s", "/thinking %s")},
	// its conversations are in one database whatever the folder. Its models
	// are its providers', asked of it; its own interface has no word for
	// one, so with a model it is started as its minimal one, which has, and
	// it is switched by being started so again on its conversation
	{Name: "opencode", goOn: "--continue", has: []string{"-c", "--continue", "-s", "--session"},
		resume: fixed("opencode --session %s"), talks: opencodeTalks{},
		with: words([]string{"mini", "--model", "%s"}, nil), lists: opencodeModels{}.models, again: true},
	// the Antigravity CLI. Its conversations are each in a folder of its
	// own whatever the folder they were made in; its models are its
	// account's, asked of it, each at an effort of its own, and it is
	// started with an effort besides. Nothing typed into it is known to
	// switch it, so it is switched by being started again on its
	// conversation
	{Name: "agy", goOn: "--continue", has: []string{"-c", "--continue", "--conversation"}, home: agyFolder,
		resume:  fixed("agy --conversation %s"),
		talks:   transcripts{agent: "agy", files: agyFiles, find: agyFind, id: agyID, decode: agyLine, cwd: agyCwd},
		Efforts: []string{"low", "medium", "high", "xhigh", "max"},
		with:    words([]string{"--model", "%s"}, []string{"--effort", "%s"}), lists: agyModels{}.models, again: true},
	// its help has a word for a model and names none: it has those the config names
	{Name: "gemini", goOn: "--resume latest", has: []string{"-r", "--resume"},
		with: words([]string{"--model", "%s"}, nil)},
	{Name: "cursor-agent", goOn: "--continue", has: []string{"--continue", "--resume", "resume"}},
}

// agents is every agent: the built-in ones, then each an enabled plugin
// declares. A plugin cannot take a built-in one's name.
func (a *App) agents() []Agent {
	if a == nil {
		return builtin // nothing to ask a plugin with
	}
	a.agentsOnce.Do(func() {
		a.agentTable = slices.Clone(builtin)
		for i := range a.agentTable {
			if a.MiaDir == "" {
				continue
			}
			cache := filepath.Join(a.MiaDir, "cache", "conversations")
			switch t := a.agentTable[i].talks.(type) {
			case transcripts:
				t.cache = cache
				a.agentTable[i].talks = t
			case opencodeTalks:
				t.cache = cache
				a.agentTable[i].talks = t
				a.agentTable[i].lists = opencodeModels{cache: filepath.Join(a.MiaDir, "cache", "models")}.models
			}
			if a.agentTable[i].Name == "agy" {
				a.agentTable[i].lists = agyModels{cache: filepath.Join(a.MiaDir, "cache", "models")}.models
			}
		}
		for _, one := range a.pluginAgents() {
			if _, taken := agentNamed(a.agentTable, one.Name); !taken {
				a.agentTable = append(a.agentTable, one)
			}
		}
	})
	return a.agentTable
}

func agentNamed(table []Agent, name string) (Agent, bool) {
	for _, one := range table {
		if one.Name == name {
			return one, true
		}
	}
	return Agent{}, false
}

// agentOf is the agent a command line runs and what it was given, "" for
// anything else. ps shows some behind node, or with their launcher's own
// arguments; a window can be started with env before it.
func agentOf(table []Agent, command string) (string, []string) {
	words := strings.Fields(command)
	words = words[launcher(words):]
	if len(words) == 0 {
		return "", nil
	}
	first := filepath.Base(words[0])
	for _, one := range table {
		if first != one.Name && !slices.Contains(one.Commands, first) {
			continue
		}
		args := slices.Clone(words[1:])
		if one.Name == "cursor-agent" {
			args = slices.DeleteFunc(args, func(a string) bool { return a == "--use-system-ca" || strings.HasSuffix(a, "/index.js") })
		}
		return one.Name, args
	}
	return "", nil
}

// launcher is how many words of a command line come before the one that
// runs its agent: env, its flags and the variables it sets, variables set
// as a shell sets them, and node or bun, or the shell a script is run by.
func launcher(words []string) int {
	at := withoutEnv(words)
	if len(words)-at > 1 {
		by := filepath.Base(words[at])
		if by == "node" || by == "bun" || (shells[by] && !strings.HasPrefix(words[at+1], "-")) {
			at++
		}
	}
	return min(at, len(words))
}

// launched is the command a shell was started to run, as ps says it of a
// window mia started — <shell> -lc "<command>; exec <shell> -l", the
// command after its cd when it has one — each word without the quotes it
// was written in; any other command line is left as it is. Once the
// command ends the shell is its own again, and says so.
func launched(command string) string {
	words := strings.Fields(command)
	if len(words) < 3 || !shells[filepath.Base(words[0])] || (words[1] != "-lc" && words[1] != "-c") {
		return command
	}
	var said []string
	for _, word := range words[2:] {
		last := strings.HasSuffix(word, ";")
		if word = strings.Replace(strings.Trim(strings.TrimSuffix(word, ";"), "'"), "='", "=", 1); word == "&&" {
			said = nil // before it, the cd
			continue
		}
		said = append(said, word)
		if last {
			break
		}
	}
	return strings.Join(said, " ")
}

// agentBehind is agentIn of a window's first pane, as tmux says it, where
// tmux may name only what launched the agent: node or bun, or the shell
// mia starts an agent's window by. Of those front, the command line in
// front in each terminal, is asked, when there is one to ask.
func agentBehind(table []Agent, command, title string, kind session.Kind, pid int, front func(pid int) string) string {
	if (command == "node" || command == "bun" || (kind == session.KindAgent && shells[command])) && front != nil {
		command = launched(front(pid))
	}
	return agentIn(table, command, title)
}

// withoutEnv is where a command line's command is, after env and the
// variables set before it.
func withoutEnv(words []string) int {
	at := 0
	if len(words) > 0 && filepath.Base(words[0]) == "env" {
		for at++; at < len(words) && strings.HasPrefix(words[at], "-"); at++ {
			if words[at] == "-u" || words[at] == "--unset" {
				at++ // the name it unsets
			}
		}
	}
	for at < len(words) && assignment.MatchString(words[at]) {
		at++
	}
	return min(at, len(words))
}

// assignment is a variable set before a command, NAME=value.
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// agentIn is the agent a pane runs, by its command line and, failing that,
// by the title the agent gave the pane.
func agentIn(table []Agent, command, title string) string {
	if name, _ := agentOf(table, command); name != "" {
		return name
	}
	for _, one := range table {
		if one.Title != nil && one.Title.MatchString(title) {
			return one.Name
		}
	}
	return ""
}

// Head is one conversation in a list: who it was with, where, what was
// first asked and last said, and whether a window has it open now.
type Head struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	// Account is the agent's account whose folder it is in, default for its
	// usual one; left out for an agent mia keeps no accounts of.
	Account  string     `json:"account,omitempty"`
	Cwd      string     `json:"cwd"`
	Worktree string     `json:"worktree"` // its name; "" when the folder is none of this repository's
	Gone     bool       `json:"gone"`     // its folder is no more
	Asked    string     `json:"asked"`    // the first thing the person asked, on one line
	Said     string     `json:"said"`     // the last thing the agent said, on one line
	Started  time.Time  `json:"started,omitzero"`
	Active   time.Time  `json:"active,omitzero"` // when it was last written to
	Turns    int        `json:"turns"`           // how many things the person asked
	Running  bool       `json:"running"`
	Window   *WindowRef `json:"window,omitempty"` // the window that has it open, when running
	// Model and Effort are the window's, as mia started it or last switched
	// it; left out when mia did not choose them. Of opencode's, the model
	// is otherwise the one opencode says the conversation runs on.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	// Continues is the conversation, with another agent, this one was
	// begun from by a handoff, and ContinuedBy the one that went on from
	// this: one thread, to read on. Left out of one that is neither.
	Continues   *Talk `json:"continues,omitempty"`
	ContinuedBy *Talk `json:"continued_by,omitempty"`
}

// WindowRef names a window of a worktree's session.
type WindowRef struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}

// Turn is one thing that happened in a conversation, for a chat view:
//
//	asked    what the person said: text
//	said     what the agent said: text, markdown as written
//	thought  what the agent thought, of an agent whose transcript keeps it:
//	         text, and seconds, how long, when the transcript says
//	tool     what the agent did, in a line: text, name, and file when it is one file's
//	question the agent asked the person to choose: text, options, and answer once given
//	answer   what the person chose, when they did: text, and question, the turn it answers
//	commit   a commit the agent made: hash and subject
//	did      the main agent ran one of mia's verbs that change something: text,
//	         verb, and the repository, worktree and machine the command names
//	switch   a chat went on with another agent, its conversation carried over:
//	         from, to, and a note of what was handed over
type Turn struct {
	N        int       `json:"n"`
	Kind     string    `json:"kind"`
	At       time.Time `json:"at,omitzero"`
	Text     string    `json:"text,omitempty"`
	Name     string    `json:"name,omitempty"`
	File     string    `json:"file,omitempty"`
	Options  []string  `json:"options,omitempty"`
	Answer   string    `json:"answer,omitempty"`
	Question int       `json:"question,omitempty"`
	Hash     string    `json:"hash,omitempty"`
	Subject  string    `json:"subject,omitempty"`
	Seconds  int       `json:"seconds,omitempty"`

	Verb       string `json:"verb,omitempty"`
	Repository string `json:"repository,omitempty"`
	Worktree   string `json:"worktree,omitempty"`
	Machine    string `json:"machine,omitempty"`

	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Note string `json:"note,omitempty"`
	// Tone is warn on a switch whose handover is still to be sent.
	Tone string `json:"tone,omitempty"`

	// ref ties a question to its answer while a transcript is read: the id
	// the agent gave the asking.
	ref string
	// command is the whole of what a shell tool ran, and cwd the folder the
	// agent was in then, when its transcript says: what a did is read from.
	command, cwd string
}

const (
	turnAsked    = "asked"
	turnSaid     = "said"
	turnThought  = "thought"
	turnTool     = "tool"
	turnQuestion = "question"
	turnAnswer   = "answer"
	turnCommit   = "commit"
	turnDid      = "did"
	turnSwitch   = "switch"
)

// TurnKinds is every kind a turn is.
var TurnKinds = []string{turnAsked, turnSaid, turnThought, turnTool, turnQuestion, turnAnswer, turnCommit, turnDid, turnSwitch}

// oneLine is text on one line, cut to what a list shows.
func oneLine(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if r := []rune(text); len(r) > 200 {
		text = string(r[:200]) + "…"
	}
	return text
}
