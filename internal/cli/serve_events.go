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
)

// How often the server looks, while anyone listens.
const (
	serveTick     = time.Second      // at when mia's files and each worktree's HEAD and index were written
	serveBackstop = 30 * time.Second // at everything, whatever was seen to change
	serveSettle   = 150 * time.Millisecond
	serveRetry    = 5 * time.Second // for a tmux server to watch, while none runs
	serveRebuilt  = 2 * time.Second // at whether mia was rebuilt
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
	s      *server
	start  sync.Once
	mu     sync.Mutex
	topics map[string]*topic
	seen   map[string]string // by git folder, "" the person's own: when its files were written
	kick   chan struct{}
}

// topic is one question and who listens to it.
type topic struct {
	q        string
	question question
	common   string // the git folder of its repository, "" when it is asked of none
	mu       sync.Mutex
	last     []byte
	subs     map[*subscriber]struct{} // under the hub's lock
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
var ofARepository = []string{"worktrees", "machines", "here", "env", "stack", "base", "conversations", "conversation", "status", "window"}

// eachLook are the queries asked again each time the server looks, as no
// file says when what they are answered from changed: a worktree's files,
// and a window's screen and whether its agent is at work.
var eachLook = []string{"status", "window"}

// join adds a listener to a question, and gives it the answer as it is now.
func (h *hub) join(q string, asked question, sub *subscriber) *topic {
	h.start.Do(func() {
		go h.run()
		go h.watchTmux()
	})
	common := ""
	if slices.Contains(ofARepository, asked.args[0]) && asked.dir != "" {
		_, common, _ = git.Repository(asked.dir)
	}
	h.mu.Lock()
	t := h.topics[q]
	if t == nil {
		t = &topic{q: q, question: asked, common: common, subs: map[*subscriber]struct{}{}}
		h.topics[q] = t
	}
	t.subs[sub] = struct{}{}
	h.mu.Unlock()
	if changed := h.look(); len(changed) > 0 { // from now, a change to what it is answered from is seen
		h.round(changed)
	}
	t.mu.Lock()
	if t.last == nil {
		t.last = eventOf(q, h.s.ask(asked))
	}
	sub.tell(q, t.last)
	t.mu.Unlock()
	return t
}

func (h *hub) leave(t *topic, sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(t.subs, sub)
	if len(t.subs) == 0 {
		delete(h.topics, t.q)
	}
}

// run asks again: at once when tmux says something or a verb was run, on a
// short tick when a file was written, and of everything now and then.
func (h *hub) run() {
	tick, backstop := time.NewTicker(h.s.tick), time.NewTicker(h.s.backstop)
	defer tick.Stop()
	defer backstop.Stop()
	for {
		select {
		case <-h.s.ctx.Done():
			return
		case <-h.kick:
			select { // what comes in a burst is one change
			case <-h.s.ctx.Done():
				return
			case <-time.After(h.s.settle):
			}
			select {
			case <-h.kick:
			default:
			}
			h.round(nil)
		case <-backstop.C:
			h.round(nil)
		case <-tick.C:
			h.round(h.look())
		}
	}
}

// again has every question asked again, soon.
func (h *hub) again() {
	select {
	case h.kick <- struct{}{}:
	default:
	}
}

// look is each repository whose files were written since it last looked,
// "" when the person's own were: their questions are to be asked again. A
// repository looked at for the first time is only noted.
func (h *hub) look() map[string]bool {
	h.mu.Lock()
	watched := map[string]bool{}
	for _, t := range h.topics {
		watched[t.common] = true
	}
	if len(watched) == 0 {
		h.seen = map[string]string{} // nobody listens: nothing is looked at
		h.mu.Unlock()
		return map[string]bool{}
	}
	if watched[""] { // a question of no repository is answered from them all
		repos, _ := app.Repositories()
		for _, repo := range repos {
			watched[filepath.Join(repo.Path, ".git")] = true
		}
	}
	watched[""] = true
	for common := range h.seen {
		if !watched[common] {
			delete(h.seen, common)
		}
	}
	changed := map[string]bool{}
	for common := range watched {
		now := writtenIn(common)
		if was, looked := h.seen[common]; looked && was != now {
			changed[common] = true
		}
		h.seen[common] = now
	}
	h.mu.Unlock()
	return changed
}

// writtenIn is when the files a repository's answers change with were
// written: mia's own records and each worktree's HEAD and index; for "",
// the person's: mia's config folder and the main agent's records.
func writtenIn(common string) string {
	if common == "" {
		return written(
			filepath.Join(config.Dir(), "*.json"),
			filepath.Join(config.Dir(), "*.toml"),
			filepath.Join(config.Dir(), "main", ".mia", "*.json"),
		)
	}
	return written(
		filepath.Join(common, "mia", "*.json"),
		filepath.Join(common, "mia", "config.toml"),
		filepath.Join(filepath.Dir(common), ".mia.toml"),
		filepath.Join(common, "HEAD"),
		filepath.Join(common, "index"),
		filepath.Join(common, "worktrees", "*", "HEAD"),
		filepath.Join(common, "worktrees", "*", "index"),
	)
}

// round asks the questions again, those of the repositories changed, those
// of no repository when any is and those asked each look, or all of them
// when changed is nil, and tells the listeners of each whose answer is not
// what it was.
func (h *hub) round(changed map[string]bool) {
	h.mu.Lock()
	var due []*topic
	for _, t := range h.topics {
		written := len(changed) > 0 && (changed[""] || t.common == "" || changed[t.common])
		if changed == nil || written || slices.Contains(eachLook, t.question.args[0]) {
			due = append(due, t)
		}
	}
	h.mu.Unlock()
	var wg sync.WaitGroup
	lanes := make(chan struct{}, 4)
	for _, t := range due {
		wg.Go(func() {
			lanes <- struct{}{}
			defer func() { <-lanes }()
			t.mu.Lock()
			defer t.mu.Unlock()
			t.last = eventOf(t.q, h.s.ask(t.question))
			h.mu.Lock()
			subs := make([]*subscriber, 0, len(t.subs))
			for sub := range t.subs {
				subs = append(subs, sub)
			}
			h.mu.Unlock()
			for _, sub := range subs {
				sub.tell(t.q, t.last)
			}
		})
	}
	wg.Wait()
}

// tmuxSays is what tmux says when a session or a window comes, goes, is
// renamed or put in front.
var tmuxSays = []string{"%sessions-changed", "%session-renamed", "%session-window-changed", "%window-pane-changed", "%layout-change", "%window-add", "%window-close", "%window-renamed", "%unlinked-window-add", "%unlinked-window-close", "%unlinked-window-renamed"}

// watchTmux listens to the tmux server as a control-mode client on a
// session of its own, and has everything asked again each time tmux says
// a session or a window changed. With no tmux server running none is
// started to watch (-N): one is looked for again every while. The session
// goes with the client, and its one pane only sleeps.
func (h *hub) watchTmux() {
	name := fmt.Sprintf("_mia-serve-%d", os.Getpid())
	for h.s.ctx.Err() == nil {
		tmux := exec.CommandContext(h.s.ctx, "tmux", "-N", "-C", "new-session", "-s", name, "sleep", "2147483647")
		in, _ := tmux.StdinPipe()
		out, _ := tmux.StdoutPipe()
		if in != nil && out != nil && tmux.Start() == nil {
			fmt.Fprintf(in, "set-option -t '=%s:' destroy-unattached on\nrefresh-client -f no-output,ignore-size\n", name)
			lines := bufio.NewReader(out)
			for {
				line, err := lines.ReadString('\n')
				if err != nil {
					break
				}
				if slices.ContainsFunc(tmuxSays, func(says string) bool { return strings.HasPrefix(line, says) }) {
					h.again()
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
	said.code = answerAPI(asker{dir: asked.dir, out: io.Discard, errs: &said.errs, open: openRepository,
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
