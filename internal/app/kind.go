package app

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/session"
)

// editors are the programs a pane is an editor for, by name.
var editors = []string{"nvim", "vim", "vi", "hx", "emacs"}

// testRunners and logTailers start a command line, by their first words,
// that runs tests or follows logs.
var (
	testRunners = [][]string{{"go", "test"}, {"gotestsum"}, {"cargo", "test"}, {"npm", "test"}, {"npm", "run", "test"},
		{"pnpm", "test"}, {"pnpm", "run", "test"}, {"yarn", "test"}, {"bun", "test"}, {"pytest"}, {"jest"}, {"vitest"},
		{"make", "test"}, {"swift", "test"}}
	logTailers = [][]string{{"tail", "-f"}, {"tail", "-F"}, {"journalctl"}, {"kubectl", "logs"}, {"docker", "logs"}, {"stern"}}
)

// WhatOf is what a pane that runs a command line is for: an agent of the
// agent table; an editor; a server, as one of the dev servers mia keeps or
// a [[service]] of the config runs; tests; logs; and a shell for anything
// else, or nothing.
func (a *App) WhatOf(command string) session.What { return a.WhatAs("", command) }

// WhatAs is WhatOf as kind says, when it says one: the kind, and the title
// tmux shows for such a pane running the command — the agent's name, the
// editor and its file, the server's name and port, tests, logs — or ""
// for a shell, whose title is its folder as it goes.
func (a *App) WhatAs(kind session.Kind, command string) session.What {
	all := strings.Fields(command)
	words := all[withoutEnv(all):]
	if kind == "" {
		kind = a.kindOf(command, words)
	}
	what := session.What{Kind: kind}
	if len(words) == 0 {
		return what
	}
	first := filepath.Base(words[0])
	switch kind {
	case session.KindAgent:
		what.Label, _ = agentOf(a.agents(), command)
		what.Label = cmp.Or(what.Label, first)
	case session.KindEditor:
		// a file it was opened on; on a folder, the folder it is in says it
		if file := lastArg(words[1:]); file != "" && file != "." {
			what.Label = first + " · " + filepath.Base(file)
		}
	case session.KindServer:
		what.Label = cmp.Or(a.serviceOf(words), first)
		if port := portOf(all); port != "" {
			what.Label += " :" + port
		}
	case session.KindTests, session.KindLogs:
		what.Label = string(kind)
	}
	return what
}

func (a *App) kindOf(command string, words []string) session.Kind {
	if len(words) == 0 {
		return session.KindShell
	}
	words = slices.Clone(words)
	words[0] = filepath.Base(words[0])
	switch {
	case a.AgentOf(command) != "":
		return session.KindAgent
	case slices.Contains(editors, words[0]):
		return session.KindEditor
	case startsWith(words, testRunners):
		return session.KindTests
	case startsWith(words, logTailers):
		return session.KindLogs
	case a.serviceOf(words) != "":
		return session.KindServer
	}
	for _, server := range devServers {
		if startsWith(words, [][]string{strings.Fields(server)}) {
			return session.KindServer
		}
	}
	return session.KindShell
}

// serviceOf is the id of the [[service]] of the config whose command the
// words run; "" for none.
func (a *App) serviceOf(words []string) string {
	if a == nil || len(words) == 0 {
		return ""
	}
	words = slices.Clone(words)
	words[0] = filepath.Base(words[0])
	for _, service := range a.Config.Services {
		if run := slices.Clone(service.Run); len(run) > 0 {
			run[0] = filepath.Base(run[0])
			if startsWith(words, [][]string{run}) {
				return service.ID
			}
		}
	}
	return ""
}

// portOf is the port a command line says it listens on, by PORT=, --port
// or -p; "" when it says none.
func portOf(words []string) string {
	for i, word := range words {
		value, said := "", false
		switch {
		case strings.HasPrefix(word, "PORT="):
			value, said = strings.TrimPrefix(word, "PORT="), true
		case strings.HasPrefix(word, "--port="):
			value, said = strings.TrimPrefix(word, "--port="), true
		case (word == "--port" || word == "-p") && i+1 < len(words):
			value, said = words[i+1], true
		}
		if _, err := strconv.Atoi(value); said && err == nil {
			return value
		}
	}
	return ""
}

// lastArg is the last of the words that is no flag; "" when all are.
func lastArg(words []string) string {
	for i := len(words) - 1; i >= 0; i-- {
		if !strings.HasPrefix(words[i], "-") && !strings.HasPrefix(words[i], "+") {
			return words[i]
		}
	}
	return ""
}

// startsWith reports whether words start with any of the lines.
func startsWith(words []string, lines [][]string) bool {
	return slices.ContainsFunc(lines, func(line []string) bool {
		return len(line) > 0 && len(words) >= len(line) && slices.Equal(words[:len(line)], line)
	})
}
