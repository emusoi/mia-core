package cli

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/session"
)

// quietFor says nothing is told for a while, ten of the server's looks.
func quietFor(t *testing.T, told <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-told:
		t.Fatalf("told with nothing changed: %s", what)
	case <-time.After(300 * time.Millisecond):
	}
}

func toldOf(t *testing.T, told <-chan struct{}, what string) {
	t.Helper()
	select {
	case _, open := <-told:
		if !open {
			t.Fatalf("the server went before it told %s", what)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("not told %s", what)
	}
}

// An app listens only to a server of its own build, and not when
// MIA_NO_SERVE is set: with none, it is not told and looks for itself as
// it always did.
func TestAnAppListensOnlyToAServerOfItsBuild(t *testing.T) {
	isolateServe(t)
	repo := gittest.New(t)
	question := asked("status", repo.Root)
	if told, hangUp := listenServer(question); told != nil {
		hangUp()
		t.Fatal("listening with no server")
	}
	listener, err := listenServe()
	if err != nil {
		t.Fatal(err)
	}
	other := newServer()
	other.build = "another build"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- other.serve(ctx, listener) }()
	told, hangUp := listenServer(question)
	cancel()
	<-done
	if told != nil {
		hangUp()
		t.Fatal("listening to a server of another build")
	}
	serveIn(t)
	t.Setenv("MIA_NO_SERVE", "1")
	if told, hangUp := listenServer(question); told != nil {
		hangUp()
		t.Fatal("listening with MIA_NO_SERVE set")
	}
	t.Setenv("MIA_NO_SERVE", "")
	told, hangUp = listenServer(question)
	if told == nil {
		t.Fatal("not listening to a server of this build")
	}
	hangUp()
	if _, open := <-told; open {
		t.Error("told after hanging up")
	}
}

// mia git and mia files are told when the worktree they show changes in
// git — a file written, written again, staged, a commit — and not while
// nothing does; when the server goes, they are told that, and look for
// themselves.
func TestGitAndFilesAreToldWhenTheWorktreeChanges(t *testing.T) {
	isolateServe(t)
	repo := gittest.New(t)
	listener, err := listenServe()
	if err != nil {
		t.Fatal(err)
	}
	s := newServer()
	s.tick, s.backstop = 30*time.Millisecond, time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.serve(ctx, listener) }()
	t.Cleanup(func() { cancel(); <-done })

	told, hangUp := listenServer(asked("status", repo.Root))
	if told == nil {
		t.Fatal("not listening")
	}
	defer hangUp()
	quietFor(t, told, "as it began")
	repo.Write("notes.md", "one\n")
	toldOf(t, told, "of a file written")
	quietFor(t, told, "after a file was written")
	repo.Write("notes.md", "one, and two\n")
	toldOf(t, told, "of a file written again")
	repo.Git("add", "notes.md")
	toldOf(t, told, "of a file staged")
	repo.Git("commit", "-qm", "notes")
	toldOf(t, told, "of a commit")
	quietFor(t, told, "after a commit")

	cancel()
	select {
	case _, open := <-told:
		if open {
			t.Fatal("told of a change as the server went")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("not told the server went")
	}
}

// mia chat is told when its window's agent goes to work, and looks then;
// not while nothing changes. With no server it is told nothing, and looks
// each second as it always did.
func TestAChatIsToldWhenItsWindowsAgentGoesToWork(t *testing.T) {
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
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	a.Plugins = pluginHost(config.Config{})
	record, err := a.RecordOrIn("quick", repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	window := ""
	for _, one := range a.Windows(record) {
		if one.Name == "first" {
			window = strconv.Itoa(one.Index)
		}
	}
	chat := func() *chatRun {
		run := &chatRun{a: a, record: record, view: &chatView{worktree: record.Name}, say: func(...[]byte) {}, done: make(chan struct{}), batches: make(chan chatBatch, 100), ended: make(chan chatEnd, 100)}
		run.aim(app.Head{}, window, nil)
		run.listened(<-run.listening) // asked as the chat is said
		t.Cleanup(func() {
			close(run.quit)
			if run.hangUp != nil {
				run.hangUp()
			}
		})
		return run
	}

	alone := chat()
	if alone.told != nil || !alone.gone {
		t.Fatal("a chat with no server listens")
	}

	serveIn(t, func(s *server) { s.tick, s.backstop, s.retry = 30*time.Millisecond, time.Hour, 50*time.Millisecond })
	run := chat()
	if run.told == nil || run.view.talk.working {
		t.Fatalf("a chat with a server: listening %v, working %v", run.told != nil, run.view.talk.working)
	}
	quietFor(t, run.told, "of a window at rest")
	if out, err := exec.Command("tmux", "select-pane", "-t", session.Target(record.Name, window), "-T", "⠋ at work").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	toldOf(t, run.told, "that the agent went to work")
	run.looks()
	if !run.view.talk.working {
		t.Error("told the agent works, the chat does not say so")
	}
	quietFor(t, run.told, "of an agent at work")
}
