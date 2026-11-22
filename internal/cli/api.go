package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/resolve"
	"github.com/emusoi/mia-core/internal/stack"
)

// asker is who asks `mia api`: the folder it is asked from, where the
// answer and what is said beside it go, and how the repository there is
// opened: anew by the command line, the one kept by `mia serve`.
type asker struct {
	dir  string
	out  io.Writer
	errs io.Writer
	open func(dir string) (*app.App, error)
	// outside is mia in no repository, for machines and here asked from a
	// folder in none.
	outside func() *app.App
	// follow is a query's --follow, given what says the turns as they come.
	follow func(func(stop <-chan struct{}, say func([]app.Turn) error) error) int
}

// atTerminal is who asks from the command line: the folder mia is run in,
// and its own output.
func atTerminal() asker {
	who := asker{out: os.Stdout, errs: os.Stderr, open: openRepository, outside: app.Outside, follow: followTurns}
	var err error
	if who.dir, err = os.Getwd(); err != nil {
		who.open = func(string) (*app.App, error) { return nil, err }
	}
	return who
}

// openRepository is mia in the repository a folder is in, as a verb has it.
func openRepository(dir string) (*app.App, error) {
	a, err := app.Open(dir)
	if err != nil {
		return nil, err
	}
	a.Events = func(e app.Event) { sendEvent(a, e) }
	a.Plugins = pluginHost(a.Config)
	return a, nil
}

func (who asker) emit(value any) int {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return who.fail(err)
	}
	fmt.Fprintln(who.out, string(data))
	return exitOK
}

func (who asker) fail(err error) int {
	fmt.Fprintf(who.errs, "mia: %v\n", err)
	var notFound resolve.NotFound
	if errors.As(err, &notFound) {
		return exitNotFound
	}
	return exitFailed
}

func (who asker) usage(form string) int {
	fmt.Fprintf(who.errs, "usage: %s\n", form)
	return exitUsage
}

// answerAPI is `mia api <query>`: the queries that are the person's, asked
// from anywhere, then those of the repository the asker is in.
func answerAPI(who asker, rest []string) int {
	if len(rest) > 0 {
		switch rest[0] {
		case "main", "standing": // the person's, from anywhere
			return cmdAPIMain(who, rest)
		case "chats", "chat": // the person's, from anywhere
			return cmdAPIChats(who, rest)
		case "agents": // every agent, from anywhere: a repository's config too, when in one
			if len(rest) > 1 {
				return who.usage("mia api agents")
			}
			c := configAt(who.dir, who.errs)
			a := app.Outside()
			a.Config, a.Plugins = c, pluginHost(c)
			return who.emit(a.AgentModels())
		case "apps": // the person's, from anywhere
			if len(rest) > 1 {
				return who.usage("mia api apps")
			}
			c := configAt(who.dir, who.errs)
			a := app.Outside()
			a.Config, a.Plugins = c, pluginHost(c)
			apps, problems := a.Apps()
			for _, problem := range problems {
				fmt.Fprintln(who.errs, "mia:", problem) // the others are listed all the same
			}
			return who.emit(apps)
		case "accounts": // the person's, from anywhere
			projects := take(&rest, "--projects")
			if len(rest) > 1 {
				return who.usage("mia api accounts [--projects]")
			}
			a, err := accountsAt(who.dir)
			if err != nil {
				return who.fail(err)
			}
			if projects {
				return who.emit(a.Projects())
			}
			return who.emit(a.Accounts())
		case "setups": // the person's, from anywhere: a repository's by default, when in one
			return cmdAPISetups(who, rest[1:])
		case "status": // the worktree the folder is in, as git has it: no repository of mia's
			if len(rest) > 1 {
				return who.usage("mia api status")
			}
			status, err := statusIn(who.dir)
			if err != nil {
				return who.fail(err)
			}
			return who.emit(status)
		case "repos": // every repository, from anywhere
			if len(rest) == 1 {
				repos, err := app.Repositories()
				if err != nil {
					return who.fail(err)
				}
				return who.emit(repos)
			}
		}
	}
	if len(rest) == 0 || !slices.Contains(apiQueryList, rest[0]) || rest[0] == "capabilities" {
		return cmdAPI(who, nil, rest) // the usage, what the API speaks, a query there is not: no repository's
	}
	a, err := who.open(who.dir)
	if err != nil {
		if len(rest) == 1 && (rest[0] == "machines" || rest[0] == "here") && errors.Is(err, git.ErrNotARepository) {
			return cmdAPI(who, who.outside(), rest) // the machines are the same from anywhere
		}
		return who.fail(err)
	}
	return cmdAPI(who, a, rest)
}

func cmdAPI(who asker, a *app.App, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(who.errs, "usage: mia api <"+apiQueries+"> [worktree]")
		return exitUsage
	}
	target := ""
	if len(args) > 1 {
		target = args[1]
	}
	switch args[0] {
	case "capabilities":
		return who.emit(apiCapabilities())

	case "worktrees":
		listings, err := a.List(args[1:]...)
		if err != nil {
			return who.fail(err)
		}
		return who.emit(listings)

	case "machines":
		return who.emit(a.Machines())

	case "here":
		return who.emit(a.Here())

	case "env":
		record, err := a.RecordOrIn(target, who.dir)
		if err != nil {
			return who.fail(err)
		}
		manager, err := managerFor(a)
		if err != nil {
			return who.fail(err)
		}
		return who.emit(manager.Snapshot(record))

	case "stack":
		record, err := a.RecordOrIn(target, who.dir)
		if err != nil {
			return who.fail(err)
		}
		branch := git.CurrentBranch(record.Path)
		if branch == "" {
			return who.emit([]stack.Layer{})
		}
		// as `mia stack show --json` says them: each with the worktree it is in
		layers, err := a.Layers(record.Path, branch)
		if err != nil {
			return who.fail(err)
		}
		return who.emit(append([]stack.Layer{}, layers...))

	case "conversations":
		rest := args[1:]
		want := app.Wanted{Worktree: takeValue(&rest, "--worktree"), Agent: takeValue(&rest, "--agent")}
		if limit := takeValue(&rest, "--limit"); limit != "" {
			n, err := strconv.Atoi(limit)
			if err != nil || n < 0 {
				return who.usage(conversationsUsage)
			}
			want.Limit = n
		}
		if len(rest) > 0 {
			return who.usage(conversationsUsage)
		}
		heads, problems := a.Conversations(want)
		for _, problem := range problems {
			fmt.Fprintln(who.errs, "mia:", problem) // the others are listed all the same
		}
		return who.emit(append([]app.Head{}, heads...))

	case "conversation":
		rest := args[1:]
		after, worktree, follow := 0, takeValue(&rest, "--worktree"), take(&rest, "--follow")
		if value := takeValue(&rest, "--after"); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return who.usage(conversationUsage + followNote)
			}
			after = n
		}
		if len(rest) != 1 {
			return who.usage(conversationUsage + followNote)
		}
		if follow {
			return who.follow(func(stop <-chan struct{}, say func([]app.Turn) error) error {
				return a.FollowConversation(rest[0], worktree, after, stop, say)
			})
		}
		turns, err := a.ConversationIn(rest[0], worktree, after)
		if err != nil {
			return who.fail(err)
		}
		return who.emit(turns)

	case "base":
		return who.emit(baseBranch{Branch: a.BaseBranch(a.Root)})

	case "window":
		rest := args[1:]
		worktree := takeValue(&rest, "--worktree")
		if len(rest) != 1 {
			return who.usage(windowQueryUsage)
		}
		index, err := strconv.Atoi(rest[0])
		if err != nil || index < 0 {
			return who.usage(windowQueryUsage)
		}
		record, err := a.RecordOrIn(worktree, who.dir)
		if err != nil {
			return who.fail(err)
		}
		standing := windowStanding{Working: a.AtWork(record, index)}
		if asks, on := a.AsksInWindow(record, rest[0]); on {
			standing.Asks = &asks
		}
		return who.emit(standing)

	default:
		fmt.Fprintf(who.errs, "mia: no such query %q — try one of %s\n", args[0], apiQueries)
		return exitNotFound
	}
}

// baseBranch is what `mia api base` says.
type baseBranch struct {
	Branch string `json:"branch"`
}

// windowStanding is what `mia api window` says: whether the agent in a
// worktree's window is at work, as mia chat says it, and what its screen
// asks, when it asks something.
type windowStanding struct {
	Working bool      `json:"working"`
	Asks    *app.Asks `json:"asks,omitempty"`
}

// worktreeStatus is what `mia api status` says: how the worktree a folder
// is in stands in git, as mia git shows it — its branch, the commit it is
// at, what it follows and how far apart they are, its files staged,
// changed and untracked — and how many stashes the repository keeps.
type worktreeStatus struct {
	Branch    string       `json:"branch"`
	Head      string       `json:"head"`
	Upstream  string       `json:"upstream"`
	Gone      bool         `json:"gone"`
	ToPush    int          `json:"to_push"`
	ToPull    int          `json:"to_pull"`
	Staged    []statusFile `json:"staged"`
	Changed   []statusFile `json:"changed"`
	Untracked []statusFile `json:"untracked"`
	Stashes   int          `json:"stashes"`
}

// statusFile is a file of a status, with how long it is and when it was
// last written in the worktree, which say it was changed again; one that
// is not there, deleted, has neither.
type statusFile struct {
	Path    string    `json:"path"`
	From    string    `json:"from,omitempty"`
	State   string    `json:"state,omitempty"`
	Size    int64     `json:"size"`
	Written time.Time `json:"written,omitzero"`
}

// statusIn is how the worktree a folder is in stands in git.
func statusIn(dir string) (worktreeStatus, error) {
	top, common, err := git.Repository(dir)
	if err != nil {
		return worktreeStatus{}, err
	}
	out, err := git.Status(dir)
	if err != nil {
		return worktreeStatus{}, err
	}
	s := git.ParseStatus(out)
	files := func(changes []git.Change) []statusFile {
		listed := make([]statusFile, len(changes))
		for i, change := range changes {
			listed[i] = statusFile{Path: change.Path, From: change.From, State: change.State}
			if info, err := os.Lstat(filepath.Join(top, change.Path)); err == nil {
				listed[i].Size, listed[i].Written = info.Size(), info.ModTime()
			}
		}
		return listed
	}
	stashes, _ := os.ReadFile(filepath.Join(common, "logs", "refs", "stash"))
	return worktreeStatus{Branch: s.Branch, Head: s.Head, Upstream: s.Upstream, Gone: s.Gone, ToPush: s.ToPush, ToPull: s.ToPull,
		Staged: files(s.Staged), Changed: files(s.Changed), Untracked: files(s.Untracked), Stashes: bytes.Count(stashes, []byte("\n"))}, nil
}

var apiQueryList = []string{"capabilities", "worktrees", "machines", "here", "repos", "env", "stack", "base", "conversations", "conversation", "main", "chats", "chat", "standing", "agents", "setups", "accounts", "apps", "status", "window"}

const (
	setupsUsage        = "mia api setups [--repository <name> | --all]"
	conversationsUsage = "mia api conversations [--worktree <name>] [--agent <agent>] [--limit <n>]"
	conversationUsage  = "mia api conversation <id> [--worktree <name>] [--after <n>] [--follow]"
	windowQueryUsage   = "mia api window <index> [--worktree <name>]"
	// followNote is what --follow does, said under a usage that takes it.
	followNote = "\n--follow: the turns in one line of JSON, then a line of only the new ones each time there are" +
		"\n  more; a turn that grew, a reply still being written, comes again under the same n: replace by n." +
		"\n  Ends, exit 0, when stdin ends, the reader goes, or on SIGINT or SIGTERM."
)

// followTurns is a query's --follow: what follow hands say, each as a line
// of compact JSON, until stdin ends, the line cannot be written, or mia is
// told to stop. Each ends it with exit 0.
func followTurns(follow func(stop <-chan struct{}, say func([]app.Turn) error) error) int {
	stop := make(chan struct{})
	var once sync.Once
	end := func() { once.Do(func() { close(stop) }) }
	// SIGPIPE too: a line written to a reader that has gone then fails, and
	// does not kill mia
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			end()
		case <-stop:
		}
	}()
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		end()
	}()
	err := follow(stop, func(turns []app.Turn) error {
		data, err := json.Marshal(turns)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(append(data, '\n'))
		return err
	})
	end()
	if err != nil {
		return fail(err)
	}
	return exitOK
}

var apiQueries = strings.Join(apiQueryList, "|")

// apiVersion is what `mia api capabilities` says the JSON speaks; it goes
// up when a field changes meaning or goes, not when one is added.
const apiVersion = 1

type capabilities struct {
	Version  int      `json:"version"`
	Queries  []string `json:"queries"`
	Features []string `json:"features"`
}

func apiCapabilities() capabilities {
	return capabilities{
		Version: apiVersion,
		Queries: apiQueryList,
		Features: []string{
			"direct-http", "project-config", "doctor", "environment-readiness", "environment-drift",
			"local-sessions", "conversations", "main-agent", "standing", "remote-sync-preview", "sync-conflicts", "safe-cleanup", "stack-sync", "merged-gc", "compose-environments", "agent-models", "setups", "resume-model", "main-start-agent", "accounts", "main-asks", "main-start-new", "main-conversations", "machine-reach", "roles", "account-login", "account-user-default", "account-signals", "conversation-continue", "conversation-thread", "apps", "conversation-follow", "chat", "git-app", "files-app", "review-app", "agent-again", "chats", "chat-switch",
		},
	}
}

// cmdAPISetups is the setups the person has used, from any folder: in a
// repository, that repository's unless --all or --repository says
// otherwise; outside one, every repository's.
func cmdAPISetups(who asker, args []string) int {
	all, repository := take(&args, "--all"), takeValue(&args, "--repository")
	if len(args) > 0 || (all && repository != "") {
		return who.usage(setupsUsage)
	}
	a := app.Outside()
	if in, err := app.Open(who.dir); err == nil {
		a = in
		if !all && repository == "" {
			repository = filepath.Base(in.Root)
		}
	} else if a.Config, err = config.Load(""); err != nil {
		return who.fail(err)
	}
	a.Plugins = pluginHost(a.Config)
	return who.emit(a.Setups(repository))
}
