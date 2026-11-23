package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/signal"
)

// How often the server looks, while anyone listens.
const (
	serveTick     = time.Second      // at when mia's files and each worktree's HEAD and index were written
	serveBackstop = 30 * time.Second // at everything, whatever was seen to change
	serveSettle   = 150 * time.Millisecond
	serveRetry    = 5 * time.Second // for a tmux server to watch, while none runs
	serveRebuilt  = 2 * time.Second // at whether mia was rebuilt
	serveLetGo    = 5 * time.Second // for what is being asked again, as the server ends
	// how long a question read by GET /api is kept, its answer asked again
	// as a listener's is, once it was last read
	serveLinger = time.Minute
	// at the main agent once more, after tmux last said its panes changed:
	// one that gives its pane no title stops working 3 seconds after it
	// last wrote, and nothing says so
	serveQuiet = 4 * time.Second
)

// event is one answer on GET /events, a line of JSON after `data: `.
type event struct {
	// Q is the question as it was subscribed to.
	Q string `json:"q"`
	// Code is what `mia api` exits with for it.
	Code int `json:"code"`
	// Result is what it prints; left out when it prints nothing.
	Result json.RawMessage `json:"result,omitempty"`
	// Stderr is what it says beside that.
	Stderr string `json:"stderr,omitempty"`
}

func eventOf(q string, said *answer) []byte {
	one := event{Q: q, Code: said.code, Stderr: said.errs.String()}
	if json.Valid(said.out.Bytes()) {
		one.Result = said.out.Bytes()
	}
	data, err := json.Marshal(one)
	if err != nil { // nothing an answer holds does not marshal
		panic(err)
	}
	return data
}

// hub is what the listeners of GET /events hold: each question asked once
// for all who asked it, and asked again when something it could answer
// from has changed. A question nobody listens to is not there.
type hub struct {
	s       *server
	start   sync.Once
	mu      sync.Mutex
	topics  map[string]*topic
	seen    map[string]stamp // by git folder, "" the person's own
	repos   struct {         // the repositories, as Repositories said them when the registry was last written
		registry string
		all      []app.Repository
	}
	looked time.Time     // when the last look began, under the hub's lock
	kick   chan struct{} // a verb was run: everything is asked again
	tmux   chan struct{} // tmux says a session or a window changed
	main   chan struct{} // tmux says the main agent's panes changed, or a chat's
	lanes  chan struct{} // what is asked at once
	asking sync.WaitGroup
}

// stamp is what was seen of a repository's files, or of the person's: when
// mia's records and git's were written, and when the next of its signals
// goes by itself.
type stamp struct {
	records, git string
	expires      time.Time // zero when none will
}

// topic is one question and who listens to it, or reads it.
type topic struct {
	q        string
	question question
	common   string     // the git folder of its repository, "" when it is asked of none
	mu       sync.Mutex // held while it is asked
	said     *answer    // as it was last asked
	last     []byte     // the same, as an event
	held     time.Time  // under the hub's lock: when GET /api last read it; zero when only listened to
	// under the hub's lock: what its answer is read from beyond its
	// repository's files, and when those were written as it was asked
	read  []string
	stamp string
	// under the hub's lock: whether it is to be asked again for what it
	// is answered from changed, whether it is being asked, and again once
	// that is done
	stale, asking, again bool
	subs                 map[*subscriber]struct{} // under the hub's lock
}

// subscriber is one GET /events.
type subscriber struct {
	events chan []byte
	lost   chan struct{} // closed when it did not read as fast as it was told
	mu     sync.Mutex
	sent   map[string][]byte // by question, what it was last told
}

// tell sends a question's answer, unless it is what it was last told.
func (sub *subscriber) tell(q string, data []byte) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.sent == nil || bytes.Equal(sub.sent[q], data) {
		return
	}
	select {
	case sub.events <- data:
		sub.sent[q] = data
	default:
		sub.sent = nil // told no more: it reads again from a new GET
		close(sub.lost)
	}
}

// parseQ reads a q of GET /events: what follows /api/ in a read.
func parseQ(q string) (question, error) {
	query, rest, _ := strings.Cut(q, "?")
	values, err := url.ParseQuery(rest)
	if err != nil || query == "" {
		return question{}, fmt.Errorf("q=%s is not <query>?dir=<folder>&arg=<word>…", q)
	}
	return questionOf(query, values), nil
}

// ofARepository are the queries answered from the repository they are
// asked in; the others are answered from them all, or from none.
var ofARepository = []string{"worktrees", "machines", "here", "env", "stack", "base", "conversations", "conversation", "status", "window", "sessions", "signals"}

// eachLook are the queries asked again each time the server looks, as no
// file says when what they are answered from changed: a worktree's files,
// and a window's screen and whether its agent is at work.
var eachLook = []string{"status", "window"}

// ofMachines are the queries answered from how busy a machine is and what
// another machine says over ssh: no tmux, and of no repository but the one
// they are asked in. What changes them is time, which the backstop is the
// clock of; tmux saying something, or another repository written, does not.
var ofMachines = []string{"here", "machines"}

// ofChats are the queries answered from the main agent's panes and the
// chats': asked again each time tmux says those changed.
var ofChats = []string{"main", "chats", "chat"}

// topicOf is the topic of a question, made when there is none, with
// what is done to it under the hub's lock.
func (h *hub) topicOf(q string, asked question, with func(*topic)) *topic {
	h.start.Do(func() {
		go h.run()
		go h.watchTmux()
	})
	h.mu.Lock()
	t := h.topics[q]
	if t == nil {
		h.mu.Unlock()
		common := ""
		if slices.Contains(ofARepository, asked.args[0]) && asked.dir != "" {
			_, common, _ = git.Repository(asked.dir)
		}
		h.mu.Lock()
		if t = h.topics[q]; t == nil {
			t = &topic{q: q, question: asked, common: common, subs: map[*subscriber]struct{}{}}
			h.topics[q] = t
		}
	}
	with(t)
	now := time.Now() // a look begun from now looks at what it is answered from
	h.mu.Unlock()
	h.lookSince(now) // from now, a change to what it is answered from is seen
	return t
}

// join adds a listener to a question, and gives it the answer as it is now.
func (h *hub) join(q string, asked question, sub *subscriber) *topic {
	t := h.topicOf(q, asked, func(t *topic) { t.subs[sub] = struct{}{} })
	t.mu.Lock()
	if t.last == nil {
		h.answer(t)
	} else {
		sub.tell(q, t.last)
	}
	t.mu.Unlock()
	return t
}

func (h *hub) leave(t *topic, sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(t.subs, sub)
	if h.unheld(t, time.Now()) && h.topics[t.q] == t {
		delete(h.topics, t.q)
	}
}

// unheld is whether nobody holds a topic: none listens to it, and GET
// /api has not read it a while. Under the hub's lock.
func (h *hub) unheld(t *topic, now time.Time) bool {
	return len(t.subs) == 0 && now.Sub(t.held) >= h.s.linger
}

// read is a question as GET /api asks it: answered once and kept as a
// listener's is, asked again as soon as something it is answered from
// says it changed, and given from what is kept until then. What changes
// it by itself each time the server looks, a worktree's files or a
// window's screen, is asked each time, as is a stream.
func (h *hub) read(asked question) *answer {
	if slices.Contains(eachLook, asked.args[0]) || slices.Contains(asked.args, "--follow") {
		return h.s.ask(asked)
	}
	t := h.topicOf(questionKey(asked), asked, func(t *topic) { t.held = time.Now() })
	t.mu.Lock()
	defer t.mu.Unlock()
	h.mu.Lock()
	due := t.said == nil || t.stale
	t.stale = false
	h.mu.Unlock()
	if due {
		h.answer(t)
	}
	return t.said
}

// questionKey is a question as GET /events takes it, as mia itself writes one.
func questionKey(q question) string { return asked(q.args[0], q.dir, q.args[1:]...) }

// answer asks a question, its lock held, and tells each listener whose
// answer it is not. What it is read from beyond its repository is stamped
// before it is asked: written while it is, it is asked again.
func (h *hub) answer(t *topic) {
	read := h.s.beyond(t.question)
	stamp := stamped(read)
	t.said = h.s.ask(t.question)
	t.last = eventOf(t.q, t.said)
	h.mu.Lock()
	t.read, t.stamp = read, stamp
	subs := make([]*subscriber, 0, len(t.subs))
	for sub := range t.subs {
		subs = append(subs, sub)
	}
	h.mu.Unlock()
	for _, sub := range subs {
		sub.tell(t.q, t.last)
	}
}

// run asks again: at once when a verb was run, tmux says something or the
// main agent's panes changed, on a short tick when a file was written or a
// signal goes, and of everything now and then.
func (h *hub) run() {
	tick, backstop := time.NewTicker(h.s.tick), time.NewTicker(h.s.backstop)
	defer tick.Stop()
	defer backstop.Stop()
	everything := func(*topic) bool { return true }
	var heard time.Time // when tmux last said the main agent's panes changed, or a chat's
	for {
		select {
		case <-h.s.ctx.Done():
			return
		case <-h.kick:
			h.settle(h.kick)
			h.round(everything)
		case <-h.tmux:
			h.settle(h.tmux)
			h.round(notOfMachines)
		case <-h.main:
			heard = time.Now()
			h.round(ofTheChats)
		case <-backstop.C:
			h.round(everything)
		case <-tick.C:
			h.look()
			// an agent that gives its pane no title is at work while it
			// writes: once tmux has said nothing of it a while, it is not
			quiet := !heard.IsZero() && time.Since(heard) >= h.s.quiet
			if quiet {
				heard = time.Time{}
			}
			h.round(func(t *topic) bool {
				return t.stale || slices.Contains(eachLook, t.question.args[0]) || quiet && ofTheChats(t)
			})
		}
	}
}

func ofTheChats(t *topic) bool    { return slices.Contains(ofChats, t.question.args[0]) }
func notOfMachines(t *topic) bool { return !slices.Contains(ofMachines, t.question.args[0]) }

// mark has the topics due asked again: at the next round, and by GET /api
// at once.
func (h *hub) mark(due func(*topic) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, t := range h.topics {
		if due(t) {
			t.stale = true
		}
	}
}

// settle waits out a burst on a channel, which is one change.
func (h *hub) settle(burst chan struct{}) {
	select {
	case <-h.s.ctx.Done():
	case <-time.After(h.s.settle):
	}
	select {
	case <-burst:
	default:
	}
}

// again has every question asked again, soon, and at once of GET /api.
func (h *hub) again() {
	h.mark(func(*topic) bool { return true })
	wake(h.kick)
}

func wake(one chan struct{}) {
	select {
	case one <- struct{}{}:
	default:
	}
}

// look notes each repository whose files were written since it last
// looked, or one of whose signals has gone by itself since, "" when the
// person's own were, and has each question answered from one of them, or
// from files written since, asked again: at the next tick, and by GET
// /api at once. A repository looked at for the first time is only noted.
// A topic nobody holds any more goes.
func (h *hub) look() { h.lookSince(time.Now()) }

// lookSince looks, unless a look begun since then was done already: many
// asking at once are looked for once.
func (h *hub) lookSince(then time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.looked.Before(then) {
		return
	}
	now := time.Now()
	h.looked = now
	watched := map[string]bool{}
	for q, t := range h.topics {
		if h.unheld(t, now) {
			delete(h.topics, q)
			continue
		}
		watched[t.common] = true
	}
	if len(watched) == 0 {
		h.seen = map[string]stamp{} // nobody listens: nothing is looked at
		return
	}
	if watched[""] { // a question of no repository is answered from them all
		for _, repo := range h.repositories() {
			watched[filepath.Join(repo.Path, ".git")] = true
		}
	}
	watched[""] = true
	for common := range h.seen {
		if !watched[common] {
			delete(h.seen, common)
		}
	}
	records, gits := map[string]bool{}, map[string]bool{} // what changed, of each
	for common := range watched {
		was, looked := h.seen[common]
		is := stamp{expires: was.expires}
		is.records, is.git = writtenIn(common)
		if expired := !was.expires.IsZero() && !now.Before(was.expires); !looked || is.records != was.records || expired {
			records[common] = looked
			is.expires = nextGoes(common)
		}
		gits[common] = looked && is.git != was.git
		h.seen[common] = is
	}
	for common, changed := range records {
		if !changed {
			delete(records, common)
		}
	}
	for _, t := range h.topics {
		// a question of no repository is answered from mia's records in
		// every one, and of none's git; how busy a machine is, of none's
		if records[""] || records[t.common] || t.common == "" && len(records) > 0 && notOfMachines(t) ||
			t.common != "" && gits[t.common] && notOfMachines(t) {
			t.stale = true
		}
		if t.read == nil {
			continue
		}
		if is := stamped(t.read); is != t.stamp {
			t.stamp, t.stale = is, true
		}
	}
}

// repositories is every repository, read again only when the registry
// they are found from has been written.
func (h *hub) repositories() []app.Repository {
	if registry := written(config.NamesPath()); registry != h.repos.registry || h.repos.all == nil {
		h.repos.all, _ = app.Repositories()
		h.repos.registry = registry
	}
	return h.repos.all
}

// writtenIn is when the files a repository's answers change with were
// written: mia's own records, and git's, each worktree's HEAD and index;
// for "", the person's records: mia's config folder, the apps in it, the
// main agent's records and each chat's, and the folder chats are made in
// and taken from.
func writtenIn(common string) (records, git string) {
	if common == "" {
		return written(
			filepath.Join(config.Dir(), "*.json"),
			filepath.Join(config.Dir(), "*.toml"),
			filepath.Join(config.Dir(), "apps", "*", "app.toml"),
			filepath.Join(signalsOf(""), "*.json"),
			filepath.Join(config.Dir(), "chats"),
			filepath.Join(config.Dir(), "chats", "*", ".mia", "*.json"),
		), ""
	}
	files := []string{filepath.Join(common, "HEAD"), filepath.Join(common, "index")}
	for _, worktree := range globbed(filepath.Join(common, "worktrees", "*")) {
		files = append(files, filepath.Join(worktree, "HEAD"), filepath.Join(worktree, "index"))
	}
	return written(
		filepath.Join(signalsOf(common), "*.json"),
		filepath.Join(common, "mia", "config.toml"),
		filepath.Join(filepath.Dir(common), ".mia.toml"),
	), written(files...)
}

// signalsOf is the folder a repository keeps its records and signals in,
// for "" the main agent's.
func signalsOf(common string) string {
	if common == "" {
		return filepath.Join(config.Dir(), "main", ".mia")
	}
	return filepath.Join(common, "mia")
}

// nextGoes is when the next of a repository's signals goes by itself,
// for "" the main agent's; zero when none will.
func nextGoes(common string) time.Time {
	signals, _ := signal.Store{Dir: signalsOf(common)}.Load()
	var next time.Time
	for _, one := range signals {
		if !one.Until.IsZero() && (next.IsZero() || one.Until.Before(next)) {
			next = one.Until
		}
	}
	return next
}

// round has the questions due asked again, each that is not being asked
// already, which is asked once more when it is done; what is due is told
// under the hub's lock. It does not wait for them. One nobody listens to,
// only read by GET /api, is not asked until it is read again.
func (h *hub) round(due func(*topic) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.s.ctx.Err() != nil {
		return
	}
	for _, t := range h.topics {
		if !due(t) {
			continue
		}
		if t.stale = len(t.subs) == 0; t.stale {
			continue
		}
		if t.asking {
			t.again = true
			continue
		}
		t.asking = true
		h.asking.Go(func() { h.askAgain(t) })
	}
}

// askAgain asks a question again, as often as it was due while it was.
func (h *hub) askAgain(t *topic) {
	for again := true; again && h.s.ctx.Err() == nil; {
		h.lanes <- struct{}{}
		t.mu.Lock()
		h.answer(t)
		t.mu.Unlock()
		<-h.lanes
		h.mu.Lock()
		again, t.again = t.again, false
		h.mu.Unlock()
	}
	h.mu.Lock()
	t.asking = false
	h.mu.Unlock()
}

// tmuxSays is what tmux says when a session or a window comes, goes, is
// renamed or put in front.
var tmuxSays = []string{"%sessions-changed", "%session-renamed", "%session-window-changed", "%window-pane-changed", "%layout-change", "%window-add", "%window-close", "%window-renamed", "%unlinked-window-add", "%unlinked-window-close", "%unlinked-window-renamed"}

// mainPanes is what tmux is asked to say of the main agent's panes and
// each chat's each time it changes, at most once a second: each pane's
// title, where claude says it is at work, and when its window last wrote,
// which is how another agent is. Its loops carry no comma, which would end
// them.
var mainPanes = "#{S:#{?#{||:#{==:#{session_name}," + session.Name("main") + "},#{m:" + session.Name("chat-") + "*,#{session_name}}},#{W:#{P:#{pane_id} #{window_activity} #{pane_title};}},}}"

// watchTmux listens to the tmux server as a control-mode client on a
// session of its own: everything but how busy a machine is is asked again
// each time tmux says a session or a window changed, and the main agent
// each time its panes do. With no tmux server running none is started to
// watch (-N): one is looked for again every while. The session goes with
// the client, and its one pane only sleeps.
func (h *hub) watchTmux() {
	name := fmt.Sprintf("_mia-serve-%d", os.Getpid())
	for h.s.ctx.Err() == nil {
		tmux := exec.CommandContext(h.s.ctx, "tmux", "-N", "-C", "new-session", "-s", name, "sleep", "2147483647")
		in, _ := tmux.StdinPipe()
		out, _ := tmux.StdoutPipe()
		if in != nil && out != nil && tmux.Start() == nil {
			fmt.Fprintf(in, "set-option -t '=%s:' destroy-unattached on\nrefresh-client -f no-output,ignore-size\nrefresh-client -B 'main::%s'\n", name, mainPanes)
			lines := bufio.NewReader(out)
			for {
				line, err := lines.ReadString('\n')
				if err != nil {
					break
				}
				switch {
				case strings.HasPrefix(line, "%subscription-changed main "):
					h.mark(ofTheChats)
					wake(h.main)
				case slices.ContainsFunc(tmuxSays, func(says string) bool { return strings.HasPrefix(line, says) }):
					h.mark(notOfMachines)
					wake(h.tmux)
				}
			}
			in.Close()
			tmux.Wait()
		}
		select {
		case <-h.s.ctx.Done():
		case <-time.After(h.s.retry):
		}
	}
}

// events is GET /events?q=…&q=…: for each question, its answer now and
// then again each time it is another, as server-sent events. A question
// with --follow is the stream `mia api … --follow` prints, a line an event.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	asked := r.URL.Query()["q"]
	if len(asked) == 0 {
		http.Error(w, "GET /events?q=<query>?dir=<folder>&arg=<word>… — each q is what follows /api/ in a read, escaped", http.StatusBadRequest)
		return
	}
	questions := make([]question, len(asked))
	for i, q := range asked {
		var err error
		if questions[i], err = parseQ(q); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flush := http.NewResponseController(w).Flush
	flush()
	sub := &subscriber{events: make(chan []byte, 256), lost: make(chan struct{}), sent: map[string][]byte{}}
	stop := make(chan struct{})
	defer close(stop)
	for i, q := range asked {
		if slices.Contains(questions[i].args, "--follow") {
			go s.follow(q, questions[i], sub, stop)
			continue
		}
		go func() {
			t := s.hub.join(q, questions[i], sub)
			<-stop
			s.hub.leave(t, sub)
		}()
	}
	for {
		select {
		case data := <-sub.events:
			if _, err := fmt.Fprintf(w, "event: result\ndata: %s\n\n", data); err != nil {
				return
			}
			flush()
		case <-sub.lost:
			return
		case <-r.Context().Done():
			return
		}
	}
}

// follow is a question with --follow on GET /events: the turns as
// `mia api … --follow` prints them, all of them once and then those that
// are new or grew, until the listener goes. What ends it early, a
// conversation there is not, is one event with its exit code.
func (s *server) follow(q string, asked question, sub *subscriber, stop <-chan struct{}) {
	var said answer
	said.code = answerAPI(asker{dir: asked.dir, out: io.Discard, errs: &said.errs, open: openRepository, outside: app.Outside,
		follow: func(follow func(stop <-chan struct{}, say func([]app.Turn) error) error) int {
			err := follow(stop, func(turns []app.Turn) error {
				data, err := json.Marshal(turns)
				if err != nil {
					return err
				}
				var one answer
				one.out.Write(data)
				select {
				case sub.events <- eventOf(q, &one):
					return nil
				case <-stop:
					return context.Canceled
				}
			})
			if err != nil {
				return asker{errs: &said.errs}.fail(err)
			}
			return exitOK
		},
	}, asked.args)
	if said.code == exitOK {
		return // it was followed until the listener went
	}
	select {
	case sub.events <- eventOf(q, &said):
	case <-stop:
	}
}
