package app

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/plugin"
)

// Agent is everything mia knows that is one coding agent's own: how to tell
// it in a window, how it goes on with a conversation, where it keeps them
// and how they cross to another machine. claude, codex and pi are compiled
// in — a move runs from the binary on the other machine, and listing has to
// be fast — and a plugin adds one by declaring it in its manifest.
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
}

// start is the word that runs it.
func (a Agent) start() string {
	if len(a.Commands) > 0 {
		return a.Commands[0]
	}
	return a.Name
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

// builtin is each coding agent mia knows without a plugin. The first three
// have conversations mia reads; of the others it knows only how to start
// them again.
var builtin = []Agent{
	{Name: "claude", goOn: "--continue", has: []string{"-c", "--continue", "-r", "--resume"},
		resume: fixed("claude --resume %s"), byFolder: true, carrier: carriers["claude"], notes: "CLAUDE.md",
		// claude writes the session of each of its processes to sessions/<pid>.json
		held:  `pid="$(ps -o tpgid= -p "$1" | tr -d ' ')"; cat "${CLAUDE_CONFIG_DIR:-$HOME/.claude}/sessions/$pid.json" 2>/dev/null`,
		talks: transcripts{agent: "claude", files: claudeFiles, find: claudeFind, id: claudeID, decode: claudeLine},
		// the aliases it takes for the latest of each, and /model and /effort in it
		Models:  []plugin.Model{{ID: "opus", Label: "Opus"}, {ID: "sonnet", Label: "Sonnet"}, {ID: "haiku", Label: "Haiku"}, {ID: "fable", Label: "Fable"}},
		Efforts: []string{"low", "medium", "high", "xhigh", "max"},
		with:    words([]string{"--model", "%s"}, []string{"--effort", "%s"}), to: typing("/model %s", "/effort %s")},
	{Name: "codex", goOn: "resume --last", has: []string{"resume"},
		resume: fixed("codex resume %s"), carrier: carriers["codex"],
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
	{Name: "pi", goOn: "--continue", has: []string{"-c", "--continue", "-r", "--resume", "--session"},
		resume: fixed("pi --session %s"), byFolder: true, carrier: carriers["pi"],
		talks: transcripts{agent: "pi", files: piFiles, find: piFind, id: piID, decode: piLine},
		// provider/id, as /model in it takes one exactly; its effort is its thinking level
		Models: []plugin.Model{{ID: "anthropic/claude-opus-5", Label: "Claude Opus 5"}, {ID: "anthropic/claude-sonnet-5", Label: "Claude Sonnet 5"},
			{ID: "openai-codex/gpt-5.6-sol", Label: "GPT-5.6 Sol"}, {ID: "openai-codex/gpt-5.6-luna", Label: "GPT-5.6 Luna"}},
		Efforts: []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"},
		with:    words([]string{"--model", "%s"}, []string{"--thinking", "%s"}), to: typing("/model %s", "/thinking %s")},
	{Name: "opencode", goOn: "--continue", has: []string{"-c", "--continue", "-s", "--session"}},
	{Name: "gemini", goOn: "--resume latest", has: []string{"-r", "--resume"}},
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
			if t, ok := a.agentTable[i].talks.(transcripts); ok && a.MiaDir != "" {
				t.cache = filepath.Join(a.MiaDir, "cache", "conversations")
				a.agentTable[i].talks = t
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
// arguments.
func agentOf(table []Agent, command string) (string, []string) {
	words := strings.Fields(command)
	if len(words) > 1 && (filepath.Base(words[0]) == "node" || filepath.Base(words[0]) == "bun") {
		words = words[1:]
	}
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
	ID       string     `json:"id"`
	Agent    string     `json:"agent"`
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
	// it; left out when mia did not choose them.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
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
//	tool     what the agent did, in a line: text, name, and file when it is one file's
//	question the agent asked the person to choose: text, options, and answer once given
//	answer   what the person chose, when they did: text, and question, the turn it answers
//	commit   a commit the agent made: hash and subject
//	did      the main agent ran one of mia's verbs that change something: text,
//	         verb, and the repository, worktree and machine the command names
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

	Verb       string `json:"verb,omitempty"`
	Repository string `json:"repository,omitempty"`
	Worktree   string `json:"worktree,omitempty"`
	Machine    string `json:"machine,omitempty"`

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
	turnTool     = "tool"
	turnQuestion = "question"
	turnAnswer   = "answer"
	turnCommit   = "commit"
	turnDid      = "did"
)

// oneLine is text on one line, cut to what a list shows.
func oneLine(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if r := []rune(text); len(r) > 200 {
		text = string(r[:200]) + "…"
	}
	return text
}
