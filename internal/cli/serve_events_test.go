package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// q is a question as GET /events takes it: what follows /api/ in a read.
func q(query, dir string, args ...string) string { return asked(query, dir, args...) }

// listen is GET /events for the questions, until the test ends: each event
// as it comes.
func listen(t *testing.T, questions ...string) <-chan event {
	t.Helper()
	events, _ := listenUntil(t, questions...)
	return events
}

// listenUntil is listen, and what ends the listening before the test does.
func listenUntil(t *testing.T, questions ...string) (<-chan event, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://mia/events?"+url.Values{"q": questions}.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := serveClient(0).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("GET /events: %s, %s", response.Status, response.Header.Get("Content-Type"))
	}
	events, ended := make(chan event, 256), make(chan struct{})
	go func() {
		defer close(ended)
		defer close(events)
		lines := bufio.NewScanner(response.Body)
		lines.Buffer(nil, 1<<24)
		name := ""
		for lines.Scan() {
			if kind, ok := strings.CutPrefix(lines.Text(), "event: "); ok {
				name = kind
			}
			data, ok := strings.CutPrefix(lines.Text(), "data: ")
			if !ok {
				continue
			}
			var one event
			if err := json.Unmarshal([]byte(data), &one); err != nil || name != "result" {
				t.Errorf("event %q, data %q: %v", name, data, err)
			}
			events <- one
		}
	}()
	hangUp := func() {
		cancel()
		response.Body.Close()
		<-ended
	}
	t.Cleanup(hangUp)
	return events, hangUp
}

// told is the next event, which has to come.
func told(t *testing.T, events <-chan event) event {
	t.Helper()
	select {
	case one, more := <-events:
		if !more {
			t.Fatal("the stream ended")
		}
		return one
	case <-time.After(10 * time.Second):
		t.Fatal("no event came")
		return event{}
	}
}

// A listener is told each question's answer once, and again only when it
// is another: nothing while nothing changes, however often the server
// looks, and after a worktree is made only the question it changes.
func TestAListenerHearsAnAnswerOnceAndAgainWhenItIsAnother(t *testing.T) {
	isolateServe(t)
	repo := aWorktree(t)
	s := serveIn(t, func(s *server) {
		s.tick, s.backstop, s.retry = 20*time.Millisecond, 100*time.Millisecond, 50*time.Millisecond
	})
	worktrees, base := q("worktrees", repo.Root), q("base", repo.Root+".longido")
	events := listen(t, worktrees, base)
	first := map[string]event{}
	for range 2 {
		one := told(t, events)
		first[one.Q] = one
	}
	want := byHand(t, repo.Root, "worktrees")
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(want.out)); err != nil {
		t.Fatal(err)
	}
	if got := first[worktrees]; got.Code != exitOK || string(got.Result) != compact.String() || got.Stderr != "" {
		t.Fatalf("worktrees, first = %+v\nmia api worktrees prints %s", got, want.out)
	}
	if got := first[base]; string(got.Result) != `{"branch":"main"}` {
		t.Fatalf("base, first = %+v", got)
	}
	select {
	case one := <-events:
		t.Fatalf("nothing changed, and an event came: %+v", one)
	case <-time.After(700 * time.Millisecond): // several of its looks at everything
	}

	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.New("other"); err != nil {
		t.Fatal(err)
	}
	for made := false; !made; { // git has it before mia names it: that is an answer of its own
		one := told(t, events)
		var listings []app.Listing
		if err := json.Unmarshal(one.Result, &listings); err != nil || one.Q != worktrees {
			t.Fatalf("a worktree was made, and the event is %+v: %v", one, err)
		}
		for _, listing := range listings {
			made = made || listing.Branch == "other" && listing.Adopted
		}
	}
	select {
	case one := <-events:
		t.Fatalf("after the one change, another event came: %+v", one)
	case <-time.After(400 * time.Millisecond):
	}

	// a question mistyped is an answer too, with its exit code; and once
	// its listener has gone it is held no more
	mistyped, hangUp := listenUntil(t, q("stack", repo.Root, "nowhere"))
	if one := told(t, mistyped); one.Code != exitNotFound || one.Result != nil || !strings.Contains(one.Stderr, "nowhere") {
		t.Errorf("a stack of no worktree = %+v", one)
	}
	held := func() int {
		s.hub.mu.Lock()
		defer s.hub.mu.Unlock()
		return len(s.hub.topics)
	}
	if held() != 3 {
		t.Fatalf("%d questions are held, of three listened to", held())
	}
	hangUp()
	for deadline := time.Now().Add(5 * time.Second); held() != 2; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("its listener gone, %d questions are still held", held())
		}
	}
	if response, err := serveClient(time.Second).Get("http://mia/events"); err != nil || response.StatusCode != http.StatusBadRequest {
		t.Errorf("GET /events with no question: %v, %v", response, err)
	}
}

// tmux says when a window opens, and the listeners are told then, not at
// the server's next look: it looks at nothing here.
func TestAListenerHearsAWindowOpenAsTmuxSaysIt(t *testing.T) {
	isolateServe(t)
	socket, err := os.MkdirTemp("/tmp", "mia-serve-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	if code := Run([]string{"new", "--bg", "--name", "quick", "quick"}); code != exitOK {
		t.Fatalf("mia new exited %d", code)
	}
	if code := Run([]string{"window", "new", "quick", "first", "--", "sleep", "600"}); code != exitOK {
		t.Fatalf("mia window new exited %d", code)
	}
	serveIn(t, func(s *server) { s.tick, s.backstop, s.retry = time.Hour, time.Hour, 50*time.Millisecond })
	events := listen(t, q("worktrees", repo.Root, "quick"))
	if one := told(t, events); !strings.Contains(string(one.Result), `"name":"first"`) || strings.Contains(string(one.Result), `"name":"second"`) {
		t.Fatalf("first = %+v", one)
	}
	watching := false
	for range 200 {
		out, _ := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
		if watching = strings.Contains(string(out), "_mia-serve-"); watching {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !watching {
		t.Fatal("the server does not watch tmux")
	}
	if out, err := exec.Command("tmux", "new-window", "-d", "-t", "=mia-quick:", "-n", "second", "sleep", "600").CombinedOutput(); err != nil {
		t.Fatalf("tmux new-window: %s", out)
	}
	for {
		if one := told(t, events); strings.Contains(string(one.Result), `"name":"second"`) {
			break
		}
	}
}

// A question with --follow is the stream `mia api … --follow` prints: the
// turns once, then those written since.
func TestAListenerFollowsAConversationAsItIsWritten(t *testing.T) {
	isolateServe(t)
	repo := gittest.New(t)
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	project := filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(repo.Root, "-"))
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	root, _ := json.Marshal(repo.Root)
	transcript := filepath.Join(project, "c1.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"type":"user","cwd":`+string(root)+`,"timestamp":"2026-11-02T10:00:00Z","message":{"content":"make the list faster"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	serveIn(t)
	events := listen(t, q("conversation", repo.Root, "c1", "--follow"))
	turns := func() []app.Turn {
		t.Helper()
		one := told(t, events)
		var turns []app.Turn
		if err := json.Unmarshal(one.Result, &turns); err != nil || one.Code != exitOK {
			t.Fatalf("%+v is not turns: %v", one, err)
		}
		return turns
	}
	if got := turns(); len(got) != 1 || got[0].N != 1 || got[0].Text != "make the list faster" {
		t.Fatalf("first = %+v", got)
	}
	// one asked without --follow is asked again once the transcript is
	// written, at the server's next look, not its backstop
	asked := listen(t, q("conversation", repo.Root, "c1"))
	if one := told(t, asked); !strings.Contains(string(one.Result), "make the list faster") {
		t.Fatalf("asked = %+v", one)
	}
	f, err := os.OpenFile(transcript, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"assistant","cwd":` + string(root) + `,"timestamp":"2026-11-02T10:00:05Z","message":{"content":[{"type":"text","text":"It is faster."}]}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got := turns(); len(got) != 1 || got[0].N != 2 || got[0].Text != "It is faster." {
		t.Fatalf("second = %+v: only the new turn", got)
	}
	heard := time.Now()
	if one := told(t, asked); !strings.Contains(string(one.Result), "It is faster.") || time.Since(heard) > 3*time.Second {
		t.Errorf("asked again = %+v, %s after", one, time.Since(heard))
	}
	// one there is not ends at once, as the command does, with what it says
	if one := told(t, listen(t, q("conversation", repo.Root, "c9", "--follow"))); one.Code != exitFailed || one.Stderr == "" {
		t.Errorf("following a conversation there is not = %+v", one)
	}
}

// A chat is told as the main agent is: tmux says when its agent's pane
// changes, and a listener to the chats hears it go to work, with nothing
// looked at in between.
func TestAListenerHearsAChatGoToWorkAsTmuxSaysIt(t *testing.T) {
	isolateServe(t)
	socket, err := os.MkdirTemp("/tmp", "mia-serve-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	t.Chdir(t.TempDir())
	main, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}
	c, err := main.NewChat("claude", "", "", "look at the logs")
	if err != nil {
		t.Fatal(err)
	}
	name := c.Main().Name
	// claude's title at rest, and at work once told to: set by the agent, as claude sets it
	gone := filepath.Join(t.TempDir(), "go")
	agent := `printf '\033]2;✳ Claude Code\033\\'; while [ ! -f ` + gone + ` ]; do sleep 0.05; done; printf '\033]2;⠂ Looking at the logs\033\\'; sleep 600`
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", "mia-chat-"+name, "-n", "claude", "sh", "-c", agent).CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %s", out)
	}
	time.Sleep(4 * time.Second) // what it last wrote is no sign it works
	serveIn(t, func(s *server) { s.tick, s.backstop, s.retry = time.Hour, time.Hour, 50*time.Millisecond })
	events := listen(t, q("chats", ""), q("chat", "", name))
	chat := func(one event) app.ChatState {
		t.Helper()
		var chats []app.ChatState
		if err := json.Unmarshal(one.Result, &chats); err != nil || len(chats) != 2 {
			t.Fatalf("chats = %s: %v", one.Result, err)
		}
		return chats[1]
	}
	for range 2 {
		one := told(t, events)
		if one.Q == q("chats", "") {
			if got := chat(one); got.Name != name || !got.Running || got.Working || got.Title != "look at the logs" {
				t.Fatalf("first = %+v", got)
			}
		}
	}
	for range 200 {
		out, _ := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
		if strings.Contains(string(out), "_mia-serve-") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	time.Sleep(1500 * time.Millisecond) // its subscription asked for, and said once as it is
	if err := os.WriteFile(gone, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	heard := map[string]bool{}
	for !heard["chats"] || !heard["chat"] {
		one := told(t, events)
		if one.Q == q("chats", "") && chat(one).Working {
			heard["chats"] = true
		}
		var state app.MainState
		if one.Q == q("chat", "", name) && json.Unmarshal(one.Result, &state) == nil && state.Working {
			heard["chat"] = true
		}
	}
}
