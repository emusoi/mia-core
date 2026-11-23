package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
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
	"github.com/emusoi/mia-core/internal/gateway"
	"github.com/emusoi/mia-core/internal/git"
)

const serveUsage = "mia serve [install|uninstall|start|stop|status]"

// serveDir is where `mia serve` keeps its socket, beside the gateway's
// files in mia's config folder.
func serveDir() string { return filepath.Join(config.Dir(), "serve") }

func serveSocket() string { return filepath.Join(serveDir(), "serve.sock") }

func cmdServe(args []string) int {
	if len(args) == 0 {
		return serveHere()
	}
	if len(args) > 1 {
		return usageErr(serveUsage)
	}
	switch args[0] {
	case "install":
		return serveInstall()
	case "uninstall":
		return serveUninstall()
	case "start":
		return serveStart()
	case "stop":
		return serveStop()
	case "status":
		return serveStatus()
	}
	return usageErr(serveUsage)
}

// serveHere is `mia serve`: the server, in the foreground, until it is
// told to stop.
func serveHere() int {
	// it is in no tmux, whatever it was started from: it reads the tmux
	// server a plain terminal would, and watches it as a client of its own
	for _, name := range []string{"TMUX", "TMUX_PANE", "MIA_TMUX_PANE"} {
		os.Unsetenv(name)
	}
	// what it runs, a verb or a plugin, reads for itself: one that asked the
	// server back would wait on the repository its own question holds
	os.Setenv("MIA_NO_SERVE", "1")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := listenServe()
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(os.Stderr, "mia serve on %s\n", serveSocket())
	if err := newServer().serve(ctx, listener); err != nil {
		return fail(err)
	}
	return exitOK
}

// listenServe takes the socket: one a live server answers on is that
// server's, and one nobody answers on was left by a server that was killed.
// ponytail: two started in the same instant can both find nobody; the
// second takes the socket and the first serves no one. A lock file if seen.
func listenServe() (net.Listener, error) {
	if running, ok := serving(); ok {
		return nil, fmt.Errorf("mia serve runs already, pid %d — `kill %d` ends it", running.Pid, running.Pid)
	}
	if err := os.MkdirAll(serveDir(), 0o700); err != nil {
		return nil, err
	}
	if err := os.Remove(serveSocket()); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", serveSocket())
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(serveSocket(), 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

// serveVersion is what GET /version says.
type serveVersion struct {
	// Build is the program that serves: its path, its size and when it was
	// written. Another build of mia does not ask this one.
	Build   string `json:"build"`
	API     int    `json:"api"`
	Version string `json:"version"`
	Pid     int    `json:"pid"`
}

// serveClient asks the server on the socket, each ask given within.
func serveClient(within time.Duration) *http.Client {
	return &http.Client{Timeout: within, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", serveSocket())
		},
	}}
}

// serving is the server that answers on the socket now, if one does.
func serving() (serveVersion, bool) {
	response, err := serveClient(400 * time.Millisecond).Get("http://mia/version")
	if err != nil {
		return serveVersion{}, false
	}
	defer response.Body.Close()
	var version serveVersion
	if json.NewDecoder(response.Body).Decode(&version) != nil || version.Build == "" {
		return serveVersion{}, false
	}
	return version, true
}

// askServer is `mia api <query>` asked of the server, when one of this
// build answers: what it printed is printed, and what it exited with is
// given. Not asked: when none answers or it is another build of mia, which
// may answer what this one would not, when MIA_NO_SERVE is set, and for
// --follow; the query is then answered here, as it always was.
func askServer(rest []string) (code int, asked bool) {
	if len(rest) == 0 || slices.Contains(rest, "--follow") {
		return 0, false
	}
	here, err := os.Getwd()
	if err != nil || !ourServer() {
		return 0, false
	}
	response, err := serveClient(0).Get("http://mia/api/" + url.PathEscape(rest[0]) + "?" + url.Values{"dir": {here}, "arg": rest[1:]}.Encode())
	if err != nil {
		return 0, false
	}
	defer response.Body.Close()
	out, err := io.ReadAll(response.Body)
	code, noCode := strconv.Atoi(response.Header.Get("Mia-Exit"))
	said, unread := url.QueryUnescape(response.Header.Get("Mia-Stderr"))
	if err != nil || noCode != nil || unread != nil {
		return 0, false // it went as it answered: nothing of it was printed
	}
	fmt.Fprint(os.Stderr, said)
	os.Stdout.Write(out)
	return code, true
}

// ourServer says whether a server of this build answers on the socket, to
// be asked: another build may answer what this one would not, and none is
// asked when MIA_NO_SERVE is set.
func ourServer() bool {
	if os.Getenv("MIA_NO_SERVE") != "" {
		return false
	}
	running, ok := serving()
	return ok && running.Build == gateway.Build()
}

// asked is a question as GET /events takes it, what follows /api/ in a
// read: a query typed in a folder with the words after it.
func asked(query, dir string, args ...string) string {
	return query + "?" + url.Values{"dir": {dir}, "arg": args}.Encode()
}

// listenServer is GET /events for the questions, of a server of this
// build: told has a value each time an answer of theirs is another than it
// was, and is closed when the server goes; hangUp ends the listening. It
// returns once their answers as they are now came, which are not told.
// With no server to ask, told is nil.
func listenServer(questions ...string) (told <-chan struct{}, hangUp func()) {
	if !ourServer() {
		return nil, func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://mia/events?"+url.Values{"q": questions}.Encode(), nil)
	if err != nil {
		cancel()
		return nil, func() {}
	}
	response, err := serveClient(0).Do(request)
	if err != nil {
		cancel()
		return nil, func() {}
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		cancel()
		return nil, func() {}
	}
	changed, now := make(chan struct{}, 1), make(chan struct{})
	go func() {
		defer close(changed)
		defer response.Body.Close()
		lines := bufio.NewScanner(response.Body)
		lines.Buffer(nil, 1<<24)
		heard := map[string]bool{}
		for lines.Scan() {
			data, ok := strings.CutPrefix(lines.Text(), "data: ")
			var one event
			if !ok || json.Unmarshal([]byte(data), &one) != nil {
				continue
			}
			if !heard[one.Q] {
				if heard[one.Q] = true; len(heard) == len(questions) {
					close(now)
				}
				continue
			}
			select {
			case changed <- struct{}{}:
			default: // told already, and not yet heard
			}
		}
	}()
	select {
	case <-now:
		return changed, cancel
	case <-changed: // closed: it went before it answered
		cancel()
		return nil, func() {}
	}
}

// server is `mia serve`. It holds no truth: each repository is kept open
// only so that what mia keeps in memory about it outlives one question.
type server struct {
	build string
	mu    sync.Mutex
	kept  map[string]*kept
	// outside is mia in no repository, held as one is: what it keeps in
	// memory of a machine that did not answer outlives one question
	outside *app.App
	hub     *hub
	ctx     context.Context // done when the server is told to stop
	// how often it looks, while anyone listens, and whether mia was rebuilt
	tick, backstop, settle, retry, rebuilt, quiet time.Duration
	// how long, as it ends, it waits for what is being asked again
	letGo time.Duration
	// how long what GET /api read is kept once it was last read
	linger time.Duration
}

// kept is a repository held open, asked one question at a time.
type kept struct {
	mu    sync.Mutex
	app   *app.App
	stamp string
	since time.Time
}

func newServer() *server {
	s := &server{build: gateway.Build(), kept: map[string]*kept{}, outside: app.Outside(),
		tick: serveTick, backstop: serveBackstop, settle: serveSettle, retry: serveRetry, rebuilt: serveRebuilt, letGo: serveLetGo, quiet: serveQuiet, linger: serveLinger}
	s.hub = &hub{s: s, topics: map[string]*topic{}, seen: map[string]stamp{},
		kick: make(chan struct{}, 1), tmux: make(chan struct{}, 1), main: make(chan struct{}, 1), lanes: make(chan struct{}, 4)}
	return s
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(serveVersion{Build: s.build, API: apiVersion, Version: versionOf(version), Pid: os.Getpid()})
	})
	mux.HandleFunc("GET /api/{query}", s.read)
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("POST /run", s.run)
	// (Where the server spends its time, for `go tool pprof` over the socket: it is
	// as private as the socket is.)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	return mux
}

// serve answers on the listener until it is told to stop, or until the
// program it was started from is another build: it ends then, for the
// service that keeps it to start the new one, as `mia api` asks only a
// server of its own build.
func (s *server) serve(ctx context.Context, listener net.Listener) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	s.ctx = ctx
	web := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		web.Close()
	}()
	go func() {
		look := time.NewTicker(s.rebuilt)
		defer look.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-look.C:
				if gateway.Build() != s.build {
					fmt.Fprintln(os.Stderr, "mia serve: mia was rebuilt — this build ends, for the new one to serve")
					stop()
					return
				}
			}
		}
	}()
	err := web.Serve(listener)
	stop()
	// what is being asked again is done before what it reads may go; once
	// the hub's lock is had, the context done, no round starts another. One
	// that hangs (a machine that answered ssh, then not) is let go: the
	// server ends all the same, for the new build or the service to serve.
	asked := make(chan struct{})
	go func() {
		s.hub.mu.Lock()
		s.hub.mu.Unlock()
		s.hub.asking.Wait()
		close(asked)
	}()
	select {
	case <-asked:
	case <-time.After(s.letGo):
		fmt.Fprintln(os.Stderr, "mia serve: what was still being asked is let go")
	}
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// question is one query as it is asked of the server: what follows /api/,
// <query>?dir=<folder>&arg=<word>&arg=<word>, the words after the query as
// `mia api` takes them and the folder it would have been typed in.
type question struct {
	dir  string
	args []string // the query, then its words
}

func questionOf(query string, values url.Values) question {
	return question{dir: values.Get("dir"), args: append([]string{query}, values["arg"]...)}
}

// answer is what `mia api` prints for a question, and exits with.
type answer struct {
	out, errs bytes.Buffer
	code      int
}

// ask answers a question as `mia api` does, by the same functions, of the
// repository kept open.
func (s *server) ask(q question) *answer {
	var said answer
	var held *kept
	who := asker{dir: q.dir, out: &said.out, errs: &said.errs,
		outside: func() *app.App { return s.outside },
		open: func(dir string) (*app.App, error) {
			one, err := s.repository(dir)
			if err != nil {
				return nil, err
			}
			held = one
			return one.app, nil
		},
		follow: func(func(stop <-chan struct{}, say func([]app.Turn) error) error) int {
			fmt.Fprintln(&said.errs, "mia: --follow is a stream — GET /events?q=… gives it")
			return exitUsage
		},
	}
	said.code = answerAPI(who, q.args)
	if held != nil {
		held.stamp = held.read()
		held.mu.Unlock()
	}
	return &said
}

// keptFor is how long a repository is held open at most: what it read once
// that no file of mia's says has changed, a plugin's manifest, is read
// again after it.
const keptFor = time.Minute

// repository is the repository a folder is in, held open and locked: its
// caller unlocks it. It is opened again when anything it read once, its
// configuration and what another mia kept in the cache, has been written
// by someone else since.
func (s *server) repository(dir string) (*kept, error) {
	top, common, err := git.Repository(dir)
	if err != nil {
		return nil, err
	}
	key := common
	if filepath.Base(common) != ".git" {
		key = top
	}
	s.mu.Lock()
	one := s.kept[key]
	if one == nil {
		one = &kept{}
		s.kept[key] = one
	}
	s.mu.Unlock()
	one.mu.Lock()
	if one.app == nil || time.Since(one.since) > keptFor || one.read() != one.stamp {
		opened, err := openRepository(dir)
		if err != nil {
			one.app = nil
			one.mu.Unlock()
			return nil, err
		}
		one.app, one.since = opened, time.Now()
	}
	return one, nil
}

// read is when each file a repository held open has read once was written.
func (k *kept) read() string {
	return written(
		filepath.Join(config.Dir(), "*.toml"),
		filepath.Join(config.Dir(), "*.json"),
		filepath.Join(config.Dir(), "apps", "*", "app.toml"),
		filepath.Join(k.app.MiaDir, "config.toml"),
		filepath.Join(k.app.Root, ".mia.toml"),
		filepath.Join(k.app.MiaDir, "cache", "*.json"),
	)
}

// written is when each file of the patterns was last written and how long
// it is: it changes when one of them does, comes or goes. A pattern of no
// wildcard is the one file, looked at once.
func written(patterns ...string) string {
	var all strings.Builder
	for _, pattern := range patterns {
		files := []string{pattern}
		if wild(pattern) {
			files = globbed(pattern)
		}
		for _, file := range files {
			if othersIn(file) {
				continue
			}
			if info, err := os.Stat(file); err == nil {
				fmt.Fprintf(&all, "%s %d %d\n", file, info.ModTime().UnixNano(), info.Size())
			}
		}
	}
	return all.String()
}

// othersIn is whether a file in mia's own folder is one another program
// keeps there, which nothing mia answers is read from: mia desk's state and
// settings, written as it is used.
func othersIn(file string) bool {
	return filepath.Dir(file) == config.Dir() && strings.HasPrefix(filepath.Base(file), "desk")
}

func wild(pattern string) bool { return strings.ContainsAny(pattern, `*?[\`) }

// globbed is filepath.Glob of a pattern, each folder it reads read again
// only once it has been written since: what is in a folder comes, goes or
// is renamed only as its own time of writing changes. One written in the
// last moments is read again all the same, as a time can be coarser than
// two writes apart.
func globbed(pattern string) []string {
	dir, name := filepath.Split(pattern)
	dir = filepath.Clean(dir)
	dirs := []string{dir}
	if wild(dir) {
		dirs = globbed(dir)
	}
	var matches []string
	for _, dir := range dirs {
		for _, entry := range entriesOf(dir) {
			if matched, _ := filepath.Match(name, entry); matched {
				matches = append(matches, filepath.Join(dir, entry))
			}
		}
	}
	return matches
}

// folders is what is in each folder globbed read, by the folder, and when
// the folder was written as it was read.
var folders struct {
	sync.Mutex
	read map[string]folderRead
}

type folderRead struct {
	written time.Time
	names   []string
}

// entriesOf is the names in a folder, in order; none of a folder that is
// not there or is no folder.
func entriesOf(dir string) []string {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	recent := time.Since(info.ModTime()) < 2*time.Second
	folders.Lock()
	was, ok := folders.read[dir]
	folders.Unlock()
	if ok && !recent && was.written.Equal(info.ModTime()) {
		return was.names
	}
	file, err := os.Open(dir)
	if err != nil {
		return nil
	}
	names, _ := file.Readdirnames(-1)
	file.Close()
	slices.Sort(names)
	folders.Lock()
	if folders.read == nil {
		folders.read = map[string]folderRead{}
	}
	folders.read[dir] = folderRead{written: info.ModTime(), names: names}
	folders.Unlock()
	return names
}

// stamped is when each of the paths was last written and how long it is,
// and which are not there: no pattern, each path the one it names.
func stamped(paths []string) string {
	var all strings.Builder
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil {
			fmt.Fprintf(&all, "%d %d\n", info.ModTime().UnixNano(), info.Size())
		} else {
			all.WriteString("-\n")
		}
	}
	return all.String()
}

// beyond is what a question's answer is read from that no file the hub
// stamps of its repository says changed: where the agents write the
// conversations it is read from, and the folders of the agents' accounts,
// whose sign-in files say logged_in by being there. Each is looked at each
// second, by its stamp alone.
func (s *server) beyond(q question) []string {
	switch {
	case q.args[0] == "conversations" || q.args[0] == "conversation":
		held, err := s.repository(q.dir)
		if err != nil {
			return nil
		}
		defer held.mu.Unlock()
		return held.app.ConversationsWritten()
	case q.args[0] == "main" && (slices.Contains(q.args[1:], "conversations") || slices.Contains(q.args[1:], "turns")):
		a, err := app.AtMain()
		if err != nil {
			return nil
		}
		return a.ConversationsWritten()
	case q.args[0] == "chats":
		a, err := app.AtMain()
		if err != nil {
			return nil
		}
		return a.ChatsWritten()
	case q.args[0] == "chat":
		// chat [turns] <name> [--after <n>]
		rest := slices.Clone(q.args[1:])
		if len(rest) > 0 && rest[0] == "turns" {
			rest = rest[1:]
		}
		takeValue(&rest, "--after")
		if len(rest) != 1 {
			return nil
		}
		a, err := app.AtChat(rest[0])
		if err != nil {
			return nil
		}
		return a.ConversationsWritten()
	case q.args[0] == "accounts":
		a, err := app.AtMain()
		if err != nil {
			return nil
		}
		return a.AccountFolders()
	}
	return nil
}

// read is GET /api/<query>: the bytes `mia api <query>` prints as the body,
// its exit code and what it said beside them in headers; kept, and asked
// again once what it is answered from changed, as a listener's answer is.
func (s *server) read(w http.ResponseWriter, r *http.Request) {
	said := s.hub.read(questionOf(r.PathValue("query"), r.URL.Query()))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Mia-Exit", strconv.Itoa(said.code))
	if said.errs.Len() > 0 {
		w.Header().Set("Mia-Stderr", url.QueryEscape(said.errs.String()))
	}
	switch said.code {
	case exitOK:
	case exitNotFound:
		w.WriteHeader(http.StatusNotFound)
	case exitUsage:
		w.WriteHeader(http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Write(said.out.Bytes())
}
