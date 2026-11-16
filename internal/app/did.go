package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/git"
)

// What the main agent did is not logged anywhere: it is read from its own
// conversation. A shell command it ran that is one of mia's verbs, and
// changes something, is said as a line — "Told olasiti: rebase onto main" —
// with the verb and what the command names. A verb that only reads stays
// the quiet tool line it was, and so does any command that is more than
// `mia …` or `cd <folder> && mia …`.

// shellCommands splits a command line into its commands, each into words,
// as a shell does for the plain case: words, quotes, backslashes, && between
// commands, ~ for home. ok is false for anything a shell would do more
// with — a pipe, a redirection, a variable, a substitution, a glob, a
// second line — since what such a command ran is not what its words say.
func shellCommands(line string) (commands [][]string, ok bool) {
	var words []string
	var word strings.Builder
	open := false // a word is being read, an empty quoted one too
	endWord := func() {
		if open {
			words = append(words, word.String())
			word.Reset()
			open = false
		}
	}
	text := []rune(strings.TrimSpace(line))
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == ' ' || c == '\t':
			endWord()
		case c == '\\':
			if i+1 >= len(text) || text[i+1] == '\n' {
				return nil, false
			}
			i++
			word.WriteRune(text[i])
			open = true
		case c == '\'':
			end := slices.Index(text[i+1:], '\'')
			if end < 0 {
				return nil, false
			}
			word.WriteString(string(text[i+1 : i+1+end]))
			i += end + 1
			open = true
		case c == '"':
			open = true
			for i++; ; i++ {
				if i >= len(text) || text[i] == '$' || text[i] == '`' {
					return nil, false
				}
				if text[i] == '"' {
					break
				}
				if text[i] == '\\' && i+1 < len(text) && strings.ContainsRune("\"\\$`", text[i+1]) {
					i++
				}
				word.WriteRune(text[i])
			}
		case c == '&' && i+1 < len(text) && text[i+1] == '&':
			endWord()
			if len(words) == 0 {
				return nil, false
			}
			commands, words = append(commands, words), nil
			i++
		case c == '~' && !open && (i+1 == len(text) || text[i+1] == '/' || text[i+1] == ' '):
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, false
			}
			word.WriteString(home)
			open = true
		case strings.ContainsRune("|&;<>()$`*?[]{}!#~\n", c):
			return nil, false
		default:
			word.WriteRune(c)
			open = true
		}
	}
	endWord()
	if len(words) == 0 {
		return nil, false
	}
	return append(commands, words), true
}

// deed is one of mia's verbs as it was typed, and what it says it did.
type deed struct {
	verb     string
	worktree string
	machine  string
	// say makes the line, given the worktree as it is now known and " on
	// <repository>" or nothing.
	say func(worktree, on string) string
}

// arguments is a verb's arguments taken apart: the words that are not
// flags, the values of the flags that take one, the flags that take none,
// and what came after --. ok is false for a flag the verb does not have.
func arguments(args []string, valued, bare []string) (words []string, values map[string]string, set map[string]bool, after []string, ok bool) {
	values, set = map[string]string{}, map[string]bool{}
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--":
			return words, values, set, args[i+1:], true
		case slices.Contains(valued, arg) && i+1 < len(args):
			values[arg] = args[i+1]
			i++
		case slices.Contains(bare, arg):
			set[arg] = true
		case strings.HasPrefix(arg, "-"):
			return nil, nil, nil, nil, false
		default:
			words = append(words, arg)
		}
	}
	return words, values, set, nil, true
}

// onMachine is a worktree named with its machine, as <worktree>@<machine>.
func onMachine(name string) (worktree, machine string) {
	worktree, machine, _ = strings.Cut(name, "@")
	return worktree, machine
}

// told says what was typed to an agent: a number is an answer to what it
// asked, anything else something it was told.
func told(text string) func(worktree, on string) string {
	text = oneLine(text)
	answer := text != "" && len(text) <= 2 && strings.Trim(text, "0123456789") == ""
	return func(worktree, _ string) string {
		if answer {
			return "Answered " + worktree + ": " + text
		}
		return "Told " + worktree + ": " + text
	}
}

// deedOf reads mia's arguments as one of the verbs that change something.
// ok is false for a verb that only reads, one mia does not know the shape
// of, and one typed in a way its usage does not have.
func deedOf(table []Agent, args []string) (deed, bool) {
	if len(args) == 0 {
		return deed{}, false
	}
	verb, rest := args[0], args[1:]
	if (verb == "agent" || verb == "window") && len(rest) > 0 {
		verb, rest = verb+" "+rest[0], rest[1:]
	}
	isAgent := func(name string) bool { _, known := agentNamed(table, name); return known }
	switch verb {
	case "new":
		words, values, set, _, ok := arguments(rest, []string{"--on", "--from", "--name", "--in"}, []string{"--stack", "--worktree", "--shell", "--bg", "--json"})
		if !ok || len(words) != 1 || set["--stack"] {
			return deed{}, false
		}
		branch, machine := words[0], values["--on"]
		return deed{verb: verb, worktree: values["--name"], machine: machine, say: func(_, on string) string {
			if machine != "" {
				on += ", on " + machine
			}
			return "Made a worktree for " + branch + on
		}}, true
	case "agent run":
		words, _, _, _, ok := arguments(rest, []string{"--task"}, nil) // the task is the agent's to say
		if !ok || len(words) > 2 {
			return deed{}, false
		}
		var worktree, agent string
		for _, word := range words {
			if agent == "" && isAgent(word) {
				agent = word
			} else {
				worktree = word
			}
		}
		return deed{verb: verb, worktree: worktree, say: started(agent)}, true
	case "window new":
		words, _, _, command, ok := arguments(rest, nil, []string{"--editor", "--json"})
		if !ok || len(words) != 2 || len(command) == 0 {
			return deed{}, false
		}
		worktree, machine := onMachine(words[0])
		agent, _ := agentOf(table, strings.Join(command, " "))
		if agent == "" {
			return deed{}, false // a shell, a build: not an agent started
		}
		return deed{verb: verb, worktree: worktree, machine: machine, say: started(agent)}, true
	case "agent send", "window send", "window press":
		words, _, _, text, ok := arguments(rest, nil, nil)
		if !ok || len(words) > 2 || len(text) == 0 || (verb != "agent send" && len(words) != 2) {
			return deed{}, false
		}
		var worktree, machine string
		// one word is a worktree, unless it is an agent's window here: claude, claude-2
		if len(words) == 2 || (len(words) == 1 && !isAgent(strings.SplitN(words[0], "-", 2)[0])) {
			worktree, machine = onMachine(words[0])
		}
		return deed{verb: verb, worktree: worktree, machine: machine, say: told(strings.Join(text, " "))}, true
	case "agent stop":
		words, _, _, _, ok := arguments(rest, nil, nil)
		if !ok || len(words) == 0 || len(words) > 2 {
			return deed{}, false
		}
		return deed{verb: verb, worktree: words[0], say: func(worktree, _ string) string { return "Stopped the agent in " + worktree }}, true
	case "move":
		words, _, _, _, ok := arguments(rest, nil, nil) // --dry-run moves nothing
		if !ok || len(words) != 2 {
			return deed{}, false
		}
		worktree, _ := onMachine(words[0])
		to := words[1]
		return deed{verb: verb, worktree: worktree, machine: to, say: func(worktree, _ string) string {
			if to == "local" {
				return "Moved " + worktree + " back here"
			}
			return "Moved " + worktree + " to " + to
		}}, true
	case "rm", "remove", "archive":
		words, _, _, _, ok := arguments(rest, nil, []string{"--force", "--stop-running", "--branch"})
		if !ok || len(words) != 1 || words[0] == "ls" || words[0] == "list" {
			return deed{}, false
		}
		done := "Removed "
		if verb == "archive" {
			done = "Archived "
		} else {
			verb = "rm"
		}
		return deed{verb: verb, worktree: words[0], say: func(worktree, on string) string { return done + worktree + on }}, true
	}
	return deed{}, false
}

// started says an agent was started in a worktree: "Started goba on
// mia-desk · claude".
func started(agent string) func(worktree, on string) string {
	return func(worktree, on string) string {
		line := "Started " + worktree + on
		if agent != "" {
			line += " · " + agent
		}
		return line
	}
}

// place is where a folder is: its repository's name, and the name of the
// worktree it is or is in.
type place struct{ repository, worktree string }

// places says where a folder is, asking git once for each: worktrees by
// the name registry, which holds each one's path.
func (a *App) places() func(dir string) place {
	owners, _ := a.Names.All()
	known := map[string]place{}
	return func(dir string) place {
		if dir == "" || dir == a.Root {
			return place{}
		}
		if at, ok := known[dir]; ok {
			return at
		}
		var at place
		resolved := git.Resolve(dir)
		for name, path := range owners {
			if resolved == path || strings.HasPrefix(resolved, path+string(filepath.Separator)) {
				at.worktree = name
			}
		}
		if _, common, err := git.Repository(resolved); err == nil && filepath.Base(common) == ".git" {
			at.repository = filepath.Base(filepath.Dir(common))
		}
		known[dir] = at
		return at
	}
}

// did is a turn as what the main agent did, when it is a shell command
// that ran one of mia's verbs that change something; any other turn is
// given back as it is.
func (a *App) did(turn Turn, where func(dir string) place) Turn {
	command := turn.command
	if command == "" {
		// an agent a plugin adds gives the line only: whole when not cut
		ran, ok := strings.CutPrefix(turn.Text, "Ran ")
		if turn.Kind != turnTool || !ok || strings.HasSuffix(ran, "…") {
			return turn
		}
		command = ran
	}
	commands, ok := shellCommands(command)
	if !ok || len(commands) > 2 {
		return turn
	}
	dir := turn.cwd
	if len(commands) == 2 {
		if cd := commands[0]; len(cd) == 2 && cd[0] == "cd" {
			dir = cd[1]
		} else {
			return turn
		}
	}
	if !filepath.IsAbs(dir) {
		if turn.cwd == "" {
			dir = "" // under a folder the transcript does not say
		} else {
			dir = filepath.Join(turn.cwd, dir)
		}
	}
	mia := commands[len(commands)-1]
	if filepath.Base(mia[0]) != "mia" {
		return turn
	}
	done, ok := deedOf(a.agents(), mia[1:])
	if !ok {
		return turn
	}
	at := where(dir)
	worktree := done.worktree
	if done.verb != "new" { // which names its worktree only when told to
		if worktree == "" {
			worktree = at.worktree // the one the command was typed in
		}
		if worktree == "" {
			return turn // which one is not known: the line as it was says more
		}
	}
	on := ""
	if at.repository != "" && at.repository != worktree {
		on = " on " + at.repository
	}
	turn.Kind, turn.Text = turnDid, done.say(worktree, on)
	turn.Verb, turn.Repository, turn.Worktree, turn.Machine = done.verb, at.repository, worktree, done.machine
	return turn
}
