package app

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/paths"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/store"
)

// An account is one login of a coding agent: a folder of its own that the
// agent keeps its sign-in, its settings and its conversations in, named to
// it by a variable — claude's CLAUDE_CONFIG_DIR, codex's CODEX_HOME, pi's
// PI_CODING_AGENT_DIR, or the account_env a plugin's agent declares. The
// agent's usual folder is the account called default, which mia leaves as
// it is; every other one is a folder of mia's, accounts/<agent>/<name> in
// the user-wide config folder. mia makes the folder and opens a window for
// the person to sign in with the agent's own login: it never reads, copies
// or moves a sign-in. What of the agent's own folder is the person's setup,
// and no sign-in or state, each of mia's links to, so a session on any
// account has it.

// DefaultAccount is the agent's usual folder, which no variable names.
const DefaultAccount = "default"

// Account is one account of an agent.
type Account struct {
	Agent string `json:"agent"`
	Name  string `json:"name"`
	// Home is its folder; "" for the default one of an agent mia does not
	// know the folder of.
	Home    string `json:"home"`
	Default bool   `json:"default"`
	// LoggedIn is yes when the file the agent writes once someone signs in
	// is there, no when it is not and the agent keeps a sign-in nowhere
	// else — or nothing at all is in the folder — and unknown when mia
	// cannot tell. Only whether a file is there is looked at, never what
	// is in it.
	LoggedIn string `json:"logged_in"`
	// Sessions is the worktrees with a window that runs the agent with it,
	// main for the main agent: this computer's.
	Sessions []string `json:"sessions"`
	// UserDefault is true of the account the user-wide config starts the
	// agent's sessions on, [agents.<name>] account, or of default when it
	// names none: one agent's are all false when it names one the agent
	// has not. A repository's config or `mia account use` can say another
	// for its own sessions.
	UserDefault bool `json:"user_default"`
	// Shared is what of the agent's own folder the account links to, by
	// name; none for the default one, which is that folder.
	Shared []string `json:"shared"`
}

const (
	loggedYes     = "yes"
	loggedNo      = "no"
	loggedUnknown = "unknown"
)

// accountsSession is the session an account is signed in to when no
// worktree is named: mia-accounts, held in the name registry as main is.
const accountsSession = "accounts"

// accountName is what an account can be called: a folder's name anywhere.
var accountName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// accountsDir is where mia keeps the accounts it makes, a folder for each
// agent.
func accountsDir() string { return paths.In("accounts") }

// homeOf is the folder an agent keeps an account's things in. The default
// one is what the agent's variable says, or its own folder when nothing
// does; "" for an agent mia does not know that of.
func homeOf(agent Agent, account string) string {
	if account != "" && account != DefaultAccount {
		return filepath.Join(accountsDir(), agent.Name, account)
	}
	if set := os.Getenv(agent.AccountEnv); agent.AccountEnv != "" && set != "" && !inAccounts(set) {
		return set
	}
	if agent.home == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, filepath.FromSlash(agent.home))
}

// inAccounts reports whether a folder is one of mia's accounts: an agent
// started with one hands its variable on to what it runs, mia too, and
// that is not the agent's usual folder.
func inAccounts(dir string) bool {
	root := accountsDir()
	return root != "" && strings.HasPrefix(filepath.Clean(dir)+string(filepath.Separator), root+string(filepath.Separator))
}

// accountsOf is an agent's accounts by name, default first; none for an
// agent mia keeps no accounts of.
func accountsOf(agent Agent) []string {
	if agent.AccountEnv == "" {
		return nil
	}
	names := []string{DefaultAccount}
	entries, _ := os.ReadDir(filepath.Join(accountsDir(), agent.Name))
	for _, e := range entries {
		if e.IsDir() && e.Name() != DefaultAccount && accountName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names
}

// home is one account's folder.
type home struct{ account, dir string }

// homesOf is every account's folder of the agent of that name, default
// first: where its conversations are; of an agent mia keeps no accounts
// of, its one folder, under no account's name. One whose folder mia does
// not know is left out.
func homesOf(name string) []home {
	agent, known := agentNamed(builtin, name)
	if !known {
		return nil
	}
	accounts := accountsOf(agent)
	if agent.AccountEnv == "" {
		accounts = []string{""} // one folder, of no account mia keeps
	}
	var all []home
	for _, account := range accounts {
		if dir := homeOf(agent, account); dir != "" {
			all = append(all, home{account, dir})
		}
	}
	return all
}

// accountIn is the account of an agent a file is in, by its folder; ""
// when it is in none of them.
func accountIn(homes []home, file string) string {
	for _, h := range slices.Backward(homes) { // a named one is under none of the others
		if strings.HasPrefix(file, h.dir+string(filepath.Separator)) {
			return h.account
		}
	}
	return ""
}

// loggedIn says whether someone has signed in to an agent in a folder, by
// whether a file is there; never by what is in it.
func loggedIn(agent Agent, dir string) string {
	if dir == "" {
		return loggedUnknown
	}
	if agent.signedIn != "" {
		if _, err := os.Stat(filepath.Join(dir, agent.signedIn)); err == nil {
			return loggedYes
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || !slices.ContainsFunc(entries, func(e fs.DirEntry) bool {
		return e.Type()&fs.ModeSymlink == 0 || !slices.Contains(agent.shared, e.Name())
	}) {
		return loggedNo // the agent has never run with it: nothing, or only the setup it shares
	}
	if agent.signedIn == "" || slices.Contains(agent.keychain, goruntime.GOOS) {
		return loggedUnknown
	}
	return loggedNo
}

// AccountFolders is what logged_in is told from, by their stamps alone:
// the folder of each agent's accounts, where one is added, each account's
// folder of every agent mia keeps accounts of, and the file in it that is
// there once someone signed in.
func (a *App) AccountFolders() []string {
	var paths []string
	for _, agent := range a.agents() {
		if agent.AccountEnv == "" {
			continue
		}
		paths = append(paths, filepath.Join(accountsDir(), agent.Name))
		for _, name := range accountsOf(agent) {
			dir := homeOf(agent, name)
			paths = append(paths, dir)
			if agent.signedIn != "" {
				paths = append(paths, filepath.Join(dir, agent.signedIn))
			}
		}
	}
	return paths
}

// withAccount is a command line that runs an agent, with the variable
// that names the account's folder set before it; as it is for the default
// account.
func withAccount(agent Agent, account, command string) string {
	if account == "" || account == DefaultAccount || agent.AccountEnv == "" || strings.HasPrefix(command, agent.AccountEnv+"=") {
		return command // the default, or one the command line says already
	}
	return agent.AccountEnv + "=" + shellQuote(homeOf(agent, account)) + " " + command
}

// account is the account of an agent a start names, or the one the config
// gives it; "" for the default. One the agent has not is refused, with
// those it has.
func (a *App) account(agent Agent, named string, set ...string) (string, error) {
	for _, one := range set {
		named = cmp.Or(named, one)
	}
	if named == "" || named == DefaultAccount {
		return "", nil
	}
	if agent.AccountEnv == "" {
		return "", fmt.Errorf("mia keeps no accounts of %s — an agent has them when mia knows the variable that names its folder", agent.Name)
	}
	have := accountsOf(agent)
	if !slices.Contains(have, named) {
		return "", fmt.Errorf("%s has no account %s — it has %s; `mia account add %s %s` makes it", agent.Name, named, strings.Join(have, ", "), agent.Name, named)
	}
	return named, nil
}

// Accounts is every account of every agent mia keeps accounts of, by
// agent, the default one first, with the worktrees whose windows use each.
func (a *App) Accounts() []Account {
	using := a.accountWindows()
	users, _ := config.Load("") // unreadable, it says none
	all := []Account{}
	for _, agent := range a.agents() {
		chosen := cmp.Or(users.Agents[agent.Name].Account, DefaultAccount)
		for _, name := range accountsOf(agent) {
			dir := homeOf(agent, name)
			all = append(all, Account{Agent: agent.Name, Name: name, Home: dir, Default: name == DefaultAccount,
				LoggedIn: loggedIn(agent, dir), Sessions: append([]string{}, using[agent.Name+"/"+name]...), UserDefault: name == chosen,
				Shared: sharedIn(agent, name)})
		}
	}
	return all
}

// keptIn is the folders whose chosen.json says what windows were started
// with: this repository's, every one mia has named a worktree in, and the
// main agent's.
func (a *App) keptIn() []string {
	var dirs []string
	add := func(dir string) {
		if dir != "" && !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	add(a.MiaDir)
	if repos, err := Repositories(); err == nil {
		for _, r := range repos {
			add(filepath.Join(r.Path, ".git", "mia"))
		}
	}
	if main := paths.In(mainName); main != "" {
		add(filepath.Join(main, ".mia"))
	}
	add(accountsDir()) // the windows signing in to one
	return dirs
}

// accountWindows is the worktrees, by agent/account, with a window that
// runs that agent with that account: what mia started it with, of a window
// that runs it still. Only this computer's sessions are looked at.
func (a *App) accountWindows() map[string][]string {
	panes := session.Here().AllPanes()
	kept := map[string]Chosen{}
	for _, dir := range a.keptIn() {
		maps.Copy(kept, chosenIn(dir))
	}
	table := a.agents()
	behind := sync.OnceValue(foreground)
	using := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(panes)) {
		worktree, ours := strings.CutPrefix(name, "mia-")
		if !ours {
			continue
		}
		titles := map[int]string{}
		for _, pane := range slices.Backward(panes[name]) {
			titles[pane.Index] = pane.Title // a window is its first pane
		}
		for _, window := range session.WindowsFrom(panes[name]) {
			agent, known := agentNamed(table, agentBehind(table, window.Command, titles[window.Index], window.Kind, window.PID, behind))
			if !known || agent.AccountEnv == "" {
				continue
			}
			key := agent.Name + "/" + cmp.Or(kept[chosenKey(name, window.PID)].Account, DefaultAccount)
			if !slices.Contains(using[key], worktree) {
				using[key] = append(using[key], worktree)
			}
		}
	}
	return using
}

// AccountOpened is what adding an account made.
type AccountOpened struct {
	Account  Account `json:"account"`
	Worktree string  `json:"worktree"` // the session's: accounts for mia's own
	Session  string  `json:"session"`
	Window   string  `json:"window"`
	Command  string  `json:"command"`
	// Shared is what of the agent's own folder the account links to, made
	// before the agent starts on it.
	Shared Sharing `json:"shared"`
}

// accountAgent is the agent of that name, refused when mia keeps no
// accounts of it.
func (a *App) accountAgent(name string) (Agent, error) {
	agent, known := agentNamed(a.agents(), name)
	if !known {
		return Agent{}, fmt.Errorf("mia knows no agent %s — `mia api agents` lists those it does", name)
	}
	if agent.AccountEnv == "" {
		return Agent{}, fmt.Errorf("mia keeps no accounts of %s — an agent has them when mia knows the variable that names its folder", name)
	}
	return agent, nil
}

func checkAccountName(name string) error {
	if name == DefaultAccount {
		return fmt.Errorf("%s is the agent's own folder, which is always there", DefaultAccount)
	}
	if !accountName.MatchString(name) {
		return fmt.Errorf("%q is not an account's name — lowercase letters, digits, - and _, starting with a letter or a digit", name)
	}
	return nil
}

// AddAccount makes a folder for a new account of an agent and opens a
// window running the agent with it, in mia's own session or in a
// worktree's, for the person to sign in with the agent's own login. mia
// signs nothing in.
func (a *App) AddAccount(agentName, name, in string) (AccountOpened, error) {
	agent, err := a.accountAgent(agentName)
	if err != nil {
		return AccountOpened{}, err
	}
	if err := checkAccountName(name); err != nil {
		return AccountOpened{}, err
	}
	dir := homeOf(agent, name)
	if dir == "" {
		return AccountOpened{}, errors.New("cannot find a home directory to keep accounts in")
	}
	if _, err := os.Stat(dir); err == nil {
		return AccountOpened{}, fmt.Errorf("%s has an account %s already, in %s — `mia account login %s %s` signs in to it again", agent.Name, name, dir, agent.Name, name)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return AccountOpened{}, err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return AccountOpened{}, err
	}
	shared, err := share(agent, name)
	if err != nil {
		return AccountOpened{}, err
	}
	opened, err := a.signInWindow(agent, name, dir, in)
	opened.Shared = shared
	return opened, err
}

// LoginAccount opens a window running the agent on an account it has
// already, default too, for the person to sign in to it again with the
// agent's own login: one signed out, or one whose sign-in has run out.
// mia signs nothing in, and nothing out.
func (a *App) LoginAccount(agentName, name, in string) (AccountOpened, error) {
	agent, err := a.existingAccount(agentName, name)
	if err != nil {
		return AccountOpened{}, err
	}
	shared, err := share(agent, name)
	if err != nil {
		return AccountOpened{}, err
	}
	opened, err := a.signInWindow(agent, name, homeOf(agent, name), in)
	opened.Shared = shared
	return opened, err
}

// Sharing is what of an agent's own folder one of its accounts links to.
type Sharing struct {
	Agent string `json:"agent"`
	Name  string `json:"name"`
	// From is the agent's own folder, which the links lead to.
	From string `json:"from"`
	// Linked is what was linked now: missing from the account's folder,
	// or a link there that led nowhere.
	Linked []string `json:"linked"`
	// Already is what was linked before.
	Already []string `json:"already"`
	// Kept is what the account's folder has of its own under that name, a
	// file, a folder or a link elsewhere: mia links nothing over it.
	Kept []string `json:"kept"`
}

// share links, in the folder of one of an agent's accounts, what of the
// agent's own folder the agent shares and that folder has, each to it
// there: what is missing, or a link that leads nowhere. Nothing of the
// account's own is touched, and nothing of the agent's own folder but
// those names is looked at. The default account is that folder: nothing.
func share(agent Agent, name string) (Sharing, error) {
	shared := Sharing{Agent: agent.Name, Name: name, Linked: []string{}, Already: []string{}, Kept: []string{}}
	from, dir := homeOf(agent, DefaultAccount), homeOf(agent, name)
	if name == DefaultAccount || from == "" || len(agent.shared) == 0 {
		return shared, nil
	}
	shared.From = from
	for _, entry := range agent.shared {
		source, link := filepath.Join(from, entry), filepath.Join(dir, entry)
		if _, err := os.Lstat(source); err != nil {
			continue // the agent's own folder has none
		}
		there, err := os.Lstat(link)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return shared, err
		case there.Mode()&fs.ModeSymlink == 0:
			shared.Kept = append(shared.Kept, entry)
			continue
		default:
			if to, _ := os.Readlink(link); to == source {
				shared.Already = append(shared.Already, entry)
				continue
			}
			if _, err := os.Stat(link); err == nil {
				shared.Kept = append(shared.Kept, entry) // a link of its own, that leads somewhere
				continue
			}
			if err := os.Remove(link); err != nil { // one that leads nowhere
				return shared, err
			}
		}
		if err := os.Symlink(source, link); err != nil {
			return shared, err
		}
		shared.Linked = append(shared.Linked, entry)
	}
	return shared, nil
}

// sharedIn is what of an agent's own folder one of its accounts links to.
func sharedIn(agent Agent, name string) []string {
	linked := []string{}
	from, dir := homeOf(agent, DefaultAccount), homeOf(agent, name)
	if name == DefaultAccount || from == "" {
		return linked
	}
	for _, entry := range agent.shared {
		if to, err := os.Readlink(filepath.Join(dir, entry)); err == nil && to == filepath.Join(from, entry) {
			linked = append(linked, entry)
		}
	}
	return linked
}

// ShareAccount links, in the folder of an account of an agent, the setup
// of the agent's own folder that is missing there; again, it links only
// what is missing still. One named, or every one of mia's when name is "".
func (a *App) ShareAccount(agentName, name string) ([]Sharing, error) {
	agent, err := a.accountAgent(agentName)
	if err != nil {
		return nil, err
	}
	if len(agent.shared) == 0 {
		return nil, fmt.Errorf("mia shares nothing of %s's own folder yet: only claude's setup is named, one by one, as safe to share", agent.Name)
	}
	names := []string{name}
	if name == "" {
		names = slices.DeleteFunc(accountsOf(agent), func(n string) bool { return n == DefaultAccount })
	} else if name == DefaultAccount {
		return nil, fmt.Errorf("%s is %s's own folder, the one each account shares from", DefaultAccount, agent.Name)
	} else if _, err := a.existingAccount(agentName, name); err != nil {
		return nil, err
	}
	all := []Sharing{}
	for _, one := range names {
		shared, err := share(agent, one)
		if err != nil {
			return all, fmt.Errorf("share %s's setup with %s: %w", agent.Name, one, err)
		}
		all = append(all, shared)
	}
	return all, nil
}

// existingAccount is the agent of that name, refused when it has no
// account of that name, with those it has.
func (a *App) existingAccount(agentName, name string) (Agent, error) {
	agent, err := a.accountAgent(agentName)
	if err != nil {
		return Agent{}, err
	}
	if have := accountsOf(agent); !slices.Contains(have, name) {
		return Agent{}, fmt.Errorf("%s has no account %s — it has %s; `mia account add %s %s` makes it", agent.Name, name, strings.Join(have, ", "), agent.Name, name)
	}
	return agent, nil
}

// HasAccount refuses an account an agent has not, or an agent mia keeps
// no accounts of.
func (a *App) HasAccount(agentName, name string) error {
	_, err := a.existingAccount(agentName, name)
	return err
}

// signInWindow opens a window running an agent on one of its accounts, in
// mia's own session or in a worktree's, for the person to sign in to.
func (a *App) signInWindow(agent Agent, name, dir, in string) (AccountOpened, error) {
	command := withAccount(agent, name, agent.start())
	opened := AccountOpened{Account: Account{Agent: agent.Name, Name: name, Home: dir, Default: name == DefaultAccount,
		LoggedIn: loggedIn(agent, dir), Sessions: []string{}, Shared: sharedIn(agent, name)}, Command: command}
	if in != "" {
		record, err := a.RecordOrHere(in)
		if err != nil {
			return opened, err
		}
		if a.sessionOf(record).SSH != "" {
			return opened, fmt.Errorf("%s's session is on a machine, and the account is this computer's — sign in on that machine with its own mia", record.Name)
		}
		argv, err := a.LaunchArgv(record, command)
		if err != nil {
			return opened, err
		}
		window := freeWindow(a.sessionOf(record), record.Name, agent.Name+"-"+name)
		if window, err = a.NewWindow(record, window, session.What{Kind: session.KindAgent, Label: agent.Name}, argv); err != nil {
			return opened, err
		}
		opened.Worktree, opened.Window = record.Name, window
		if err := a.keepAccount(a.sessionOf(record), a, record.Name, window, name); err != nil {
			return opened, err
		}
	} else {
		host := a.mainHost()
		if !host.Available() {
			return opened, errors.New("tmux not found — an account is signed in to in a window: https://github.com/tmux/tmux/wiki/Installing")
		}
		if err := os.MkdirAll(accountsDir(), 0o700); err != nil { // not there before one is added
			return opened, err
		}
		root := git.Resolve(accountsDir())
		if err := a.Names.Claim(accountsSession, root); err != nil {
			return opened, fmt.Errorf("mia signs accounts in in the session mia-%s: %w", accountsSession, err)
		}
		shell := cmp.Or(a.Config.Dotfiles.Shell, os.Getenv("SHELL"), "/bin/sh")
		window := freeWindow(host, accountsSession, agent.Name+"-"+name)
		env := agent.AccountEnv + "=" + dir
		if name == DefaultAccount {
			env = "" // the agent finds its own folder as it always does: a variable set to it is another to claude's Keychain
		}
		// the window is the agent, and goes when the person has signed in and left it
		if err := host.StartWindowWith(env, accountsSession, root, window, session.What{Kind: session.KindAgent, Label: agent.Name}, []string{shell, "-lc", command}); err != nil {
			return opened, err
		}
		opened.Worktree, opened.Window = accountsSession, window
		// kept beside the accounts, as the session is no repository's
		if err := a.keepAccount(host, &App{MiaDir: accountsDir()}, accountsSession, window, name); err != nil {
			return opened, err
		}
	}
	opened.Session = session.Name(opened.Worktree)
	return opened, nil
}

// keepAccount keeps, in the folder of kept's, the account a window of a
// session was started on: no setup is written for it.
func (a *App) keepAccount(host session.Host, kept *App, worktree, window, account string) error {
	for _, one := range host.WindowsOf(worktree) {
		if one.Name == window {
			return kept.keep(host, worktree, one.PID, Chosen{Account: account})
		}
	}
	return nil
}

// freeWindow is a window name a session has not: the one asked, or it with
// a number after.
func freeWindow(host session.Host, worktree, window string) string {
	name := window
	for i := 2; host.HasWindow(worktree, name); i++ {
		name = fmt.Sprintf("%s-%d", window, i)
	}
	return name
}

// inUse refuses an account a window runs the agent with.
func (a *App) inUse(agent Agent, name, doing string) error {
	if worktrees := a.accountWindows()[agent.Name+"/"+name]; len(worktrees) > 0 {
		return fmt.Errorf("%s runs with the account %s in %s — stop it there first, then %s", agent.Name, name, strings.Join(worktrees, ", "), doing)
	}
	return nil
}

// RenameAccount gives an account of an agent another name: its folder is
// moved. An agent that keeps a sign-in under the folder's path elsewhere —
// claude in the macOS Keychain — does not find it under the new one.
func (a *App) RenameAccount(agentName, from, to string) (Account, error) {
	agent, err := a.accountAgent(agentName)
	if err != nil {
		return Account{}, err
	}
	if from == DefaultAccount {
		return Account{}, fmt.Errorf("%s is the agent's own folder: it keeps its name", DefaultAccount)
	}
	if err := checkAccountName(to); err != nil {
		return Account{}, err
	}
	was, now := homeOf(agent, from), homeOf(agent, to)
	if _, err := os.Stat(was); err != nil {
		return Account{}, fmt.Errorf("%s has no account %s — `mia account ls` lists them", agent.Name, from)
	}
	if _, err := os.Stat(now); err == nil {
		return Account{}, fmt.Errorf("%s has an account %s already", agent.Name, to)
	}
	if err := a.inUse(agent, from, "rename it"); err != nil {
		return Account{}, err
	}
	if err := os.Rename(was, now); err != nil {
		return Account{}, err
	}
	err = changePlaces(func(said map[string]map[string]string) {
		for _, agents := range said {
			if agents[agent.Name] == from {
				agents[agent.Name] = to
			}
		}
	})
	if err != nil {
		return Account{}, err
	}
	return Account{Agent: agent.Name, Name: to, Home: now, LoggedIn: loggedIn(agent, now), Sessions: []string{}, Shared: sharedIn(agent, to)}, nil
}

// Conversations a folder holds: its transcripts, as every agent mia reads
// writes them.
func transcriptsUnder(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.HasSuffix(path, ".jsonl") {
			n++
		}
		return nil
	})
	return n
}

// RemoveAccount removes an account of an agent, its folder with it. One a
// window runs the agent with is refused, and so is one that holds
// conversations unless forced. Signing out is the agent's own: mia signs
// nothing out, so a sign-in the agent keeps elsewhere stays there.
func (a *App) RemoveAccount(agentName, name string, force bool) (Account, error) {
	agent, err := a.accountAgent(agentName)
	if err != nil {
		return Account{}, err
	}
	if name == DefaultAccount {
		return Account{}, fmt.Errorf("%s is the agent's own folder, which mia leaves as it is", DefaultAccount)
	}
	dir := homeOf(agent, name)
	if name == "" || !accountName.MatchString(name) {
		return Account{}, fmt.Errorf("%s has no account %q", agent.Name, name)
	}
	if _, err := os.Stat(dir); err != nil {
		return Account{}, fmt.Errorf("%s has no account %s — `mia account ls` lists them", agent.Name, name)
	}
	if err := a.inUse(agent, name, "remove it"); err != nil {
		return Account{}, err
	}
	if n := transcriptsUnder(dir); n > 0 && !force {
		return Account{}, fmt.Errorf("the account %s of %s holds %d conversation(s), which go with its folder — `mia account rm --force %s %s` removes them too", name, agent.Name, n, agent.Name, name)
	}
	gone := Account{Agent: agent.Name, Name: name, Home: dir, LoggedIn: loggedIn(agent, dir), Sessions: []string{}, Shared: []string{}}
	if err := os.RemoveAll(dir); err != nil {
		return gone, err
	}
	// a repository that started the agent on it starts it on what its config says
	return gone, changePlaces(func(said map[string]map[string]string) {
		for _, agents := range said {
			if agents[agent.Name] == name {
				delete(agents, agent.Name)
			}
		}
	})
}

// KeepsSignInElsewhere reports whether an agent may keep a sign-in out of
// its folder on this computer: the macOS Keychain, a keyring.
func (a *App) KeepsSignInElsewhere(agentName string) bool {
	agent, _ := agentNamed(a.agents(), agentName)
	return slices.Contains(agent.keychain, goruntime.GOOS)
}

// A repository starts each agent's sessions on one account: the one the
// person told mia to with `mia account use`, kept in mia's own state, or
// failing that the one the config gives the agent, [agents.<name>]
// account, or failing that the default. One window can be started on
// another, that once.

// Project is the account a repository starts an agent's sessions on, and
// what says so: mia, when the person said it with `mia account use`; the
// config; or nothing, for the default.
type Project struct {
	Repository string `json:"repository"`
	Agent      string `json:"agent"`
	Account    string `json:"account"`
	From       string `json:"from"`
}

const (
	fromMia     = "mia"
	fromConfig  = "config"
	fromDefault = "default"
)

// placesPath is where mia keeps what the person told it: by repository,
// its main checkout's folder name, the account of each agent.
func placesPath() string { return paths.In("accounts.json") }

func readPlaces() map[string]map[string]string {
	said := map[string]map[string]string{}
	if data, err := os.ReadFile(placesPath()); err == nil {
		_ = json.Unmarshal(data, &said) // unreadable is as nothing said
	}
	return said
}

// placeOf is the account a repository starts an agent's sessions on, ""
// for the default, and what says so.
func placeOf(said map[string]map[string]string, repository string, agent Agent, cfg config.Config) (string, string) {
	if name := said[repository][agent.Name]; name != "" {
		return name, fromMia
	}
	if name := cfg.Agents[agent.Name].Account; name != "" && name != DefaultAccount {
		return name, fromConfig
	}
	return "", fromDefault
}

// placeAccount is the account this repository starts an agent's sessions
// on; "" for the default.
func (a *App) placeAccount(agent Agent) string {
	if a.Root == "" {
		name, _ := placeOf(nil, "", agent, a.Config)
		return name
	}
	name, _ := placeOf(readPlaces(), filepath.Base(a.Root), agent, a.Config)
	return name
}

// changePlaces changes what mia keeps of which account each repository
// starts an agent on, under its lock.
func changePlaces(change func(map[string]map[string]string)) error {
	path := placesPath()
	if path == "" {
		return errors.New("cannot find a home directory to keep accounts in")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	unlock, err := store.Lock(filepath.Dir(path), "accounts.lock")
	if err != nil {
		return err
	}
	defer unlock()
	said := readPlaces()
	change(said)
	for repository, agents := range said {
		if len(agents) == 0 {
			delete(said, repository)
		}
	}
	data, err := json.MarshalIndent(said, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// UseAccount makes a repository start an agent's sessions on an account
// from now on; default gives it back to what the config says. A window
// that runs already stays on the one it was started on.
func (a *App) UseAccount(agentName, name, repository string) (Project, error) {
	agent, err := a.accountAgent(agentName)
	if err != nil {
		return Project{}, err
	}
	if name != DefaultAccount && !slices.Contains(accountsOf(agent), name) {
		return Project{}, fmt.Errorf("%s has no account %s — it has %s", agent.Name, name, strings.Join(accountsOf(agent), ", "))
	}
	err = changePlaces(func(said map[string]map[string]string) {
		if name == DefaultAccount {
			delete(said[repository], agent.Name)
			return
		}
		if said[repository] == nil {
			said[repository] = map[string]string{}
		}
		said[repository][agent.Name] = name
	})
	if err != nil {
		return Project{}, err
	}
	cfg := a.Config
	if filepath.Base(a.Root) != repository {
		cfg = configOf(repository)
	}
	now, from := placeOf(readPlaces(), repository, agent, cfg)
	return Project{Repository: repository, Agent: agent.Name, Account: cmp.Or(now, DefaultAccount), From: from}, nil
}

// configOf is a repository's config, by its main checkout's folder name:
// the user-wide one with its own over it, or the user-wide one alone for a
// repository mia cannot find.
func configOf(repository string) config.Config {
	if repos, err := Repositories(); err == nil {
		for _, r := range repos {
			if r.Name == repository {
				if cfg, err := config.Load(filepath.Join(r.Path, ".git", "mia")); err == nil {
					return cfg
				}
			}
		}
	}
	cfg, _ := config.Load("")
	return cfg
}

// Projects is the account each repository starts each agent's sessions
// on, for every repository mia has named a worktree in, this one, and any
// mia was told of, by repository and then agent.
func (a *App) Projects() []Project {
	said := readPlaces()
	configs := map[string]config.Config{}
	if a.MiaDir != "" && filepath.Base(a.MiaDir) == "mia" {
		configs[filepath.Base(a.Root)] = a.Config
	}
	if repos, err := Repositories(); err == nil {
		for _, r := range repos {
			if _, seen := configs[r.Name]; !seen {
				if cfg, err := config.Load(filepath.Join(r.Path, ".git", "mia")); err == nil {
					configs[r.Name] = cfg
				}
			}
		}
	}
	for repository := range said {
		if _, seen := configs[repository]; !seen {
			configs[repository], _ = config.Load("")
		}
	}
	all := []Project{}
	for _, repository := range slices.Sorted(maps.Keys(configs)) {
		for _, agent := range a.agents() {
			if agent.AccountEnv == "" {
				continue
			}
			name, from := placeOf(said, repository, agent, configs[repository])
			all = append(all, Project{Repository: repository, Agent: agent.Name, Account: cmp.Or(name, DefaultAccount), From: from})
		}
	}
	return all
}

// AccountOfWindow would put the agent in a window on another account. An
// agent takes its account when it starts, so it is refused, with how: stop
// the agent, then take its conversation up again on the other account,
// which copies the conversation there. mia stops nothing itself.
func (a *App) AccountOfWindow(record model.Record, window, name string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	var pane session.Pane
	for _, one := range session.WindowsFrom(host.Panes(record.Name)) {
		if one.Name == window || strconv.Itoa(one.Index) == window {
			pane = session.Pane{Index: one.Index, Window: one.Name, PID: one.PID, Command: one.Command, Kind: one.Kind}
		}
	}
	for _, p := range host.Panes(record.Name) {
		if p.Index == pane.Index && pane.Title == "" {
			pane.Title = p.Title
		}
	}
	var front func() map[int]string
	if host.SSH == "" {
		front = foreground
	}
	agent, known := agentNamed(a.agents(), agentBehind(a.agents(), pane.Command, pane.Title, pane.Kind, pane.PID, front))
	if !known {
		return fmt.Errorf("window %s of %s runs no agent mia knows", window, record.Name)
	}
	if _, err := a.account(agent, name); err != nil {
		return err
	}
	resume := "`mia conversation resume <id> --account " + name + "`"
	heads, _ := a.Conversations(Wanted{Worktree: record.Name, Agent: agent.Name})
	for _, h := range heads {
		if h.Running && h.Window != nil && h.Window.Index == pane.Index {
			resume = "`mia conversation resume " + h.ID + " --account " + name + "`"
		}
	}
	return fmt.Errorf("%s takes its account when it starts, so window %s of %s stays on the one it has — end %s there, then %s takes its conversation up on %s, copied there",
		agent.Name, pane.Window, record.Name, agent.Name, resume, name)
}

// accountShell is a line of shell that sets home to the folder of an
// agent's account on the computer it runs on: a named one where mia keeps
// it there, the default one as the agent finds it there. A move runs it on
// both sides.
func accountShell(agent Agent, account string) string {
	accounts := `"${XDG_CONFIG_HOME:-$HOME/.config}/mia/accounts/"`
	if account != "" && account != DefaultAccount {
		return `home=` + accounts + shellQuote(agent.Name+"/"+account)
	}
	if agent.AccountEnv == "" || agent.home == "" {
		return `home=`
	}
	own := `"$HOME/"` + shellQuote(agent.home)
	return `home="${` + agent.AccountEnv + `:-}"; case "$home" in "" | ` + accounts + `*) home=` + own + ` ;; esac`
}

// keepAccounts keeps, for a session of this computer just opened with
// windows, the account each window's agent was started on, as a window
// started by mia window new keeps it.
func (a *App) keepAccounts(worktree string, windows []Window) {
	host := session.Here()
	opened := session.WindowsFrom(host.Panes(worktree))
	slices.SortFunc(opened, func(x, y session.Window) int { return x.Index - y.Index })
	for i, w := range windows {
		if w.Account != "" && i < len(opened) {
			_ = a.keep(host, worktree, opened[i].PID, Chosen{Account: w.Account}) // what could not be kept is the default's
		}
	}
}
