package app

import (
	"path/filepath"
	"slices"
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
func (a *App) WhatOf(command string) session.What {
	words := strings.Fields(command)
	words = words[withoutEnv(words):]
	if len(words) == 0 {
		return session.What{Kind: session.KindShell}
	}
	words[0] = filepath.Base(words[0])
	if name, _ := agentOf(a.agents(), command); name != "" {
		return session.What{Kind: session.KindAgent}
	}
	if slices.Contains(editors, words[0]) {
		return session.What{Kind: session.KindEditor}
	}
	if startsWith(words, testRunners) {
		return session.What{Kind: session.KindTests}
	}
	if startsWith(words, logTailers) {
		return session.What{Kind: session.KindLogs}
	}
	var servers [][]string
	for _, server := range devServers {
		servers = append(servers, strings.Fields(server))
	}
	if a != nil {
		for _, service := range a.Config.Services {
			if run := slices.Clone(service.Run); len(run) > 0 {
				run[0] = filepath.Base(run[0])
				servers = append(servers, run)
			}
		}
	}
	if startsWith(words, servers) {
		return session.What{Kind: session.KindServer}
	}
	return session.What{Kind: session.KindShell}
}

// startsWith reports whether words start with any of the lines.
func startsWith(words []string, lines [][]string) bool {
	return slices.ContainsFunc(lines, func(line []string) bool {
		return len(line) > 0 && len(words) >= len(line) && slices.Equal(words[:len(line)], line)
	})
}
