package app_test

import (
	"os"
	"os/exec"
	"slices"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/session"
)

func TestWhatWentDownWithTmuxComesBackAndWhatWasClosedStaysClosed(t *testing.T) {
	gittest.Isolate(t)
	socket, err := os.MkdirTemp("/tmp", "mia-resurrect-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := a.New("kept")
	if err != nil {
		t.Fatal(err)
	}
	closed, err := a.New("closed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(kept, "agent", []string{"sleep", "300"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(closed, "shell", nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	exec.Command("tmux", "kill-session", "-t", "="+session.Name(closed.Name)).Run()
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Resurrect(true); len(got) != 0 {
		t.Fatalf("tmux is still up: nothing to bring back, got %+v", got)
	}

	exec.Command("tmux", "kill-server").Run()
	got, err := a.Resurrect(true)
	if err != nil || len(got) != 1 || got[0].Name != kept.Name {
		t.Fatalf("only the session that went down with tmux comes back: %+v %v", got, err)
	}
	if i := slices.IndexFunc(got[0].Windows, func(w app.Window) bool { return w.Name == "agent" }); i < 0 || got[0].Windows[i].Command != "sleep 300" {
		t.Errorf("its program runs again: %+v", got[0].Windows)
	}
	if _, err := a.Resurrect(false); err != nil {
		t.Fatal(err)
	}
	if !session.Here().HasWindow(kept.Name, "agent") || session.Here().Exists(closed.Name) {
		t.Errorf("kept is back with its agent window, closed is not: %v", session.Here().Windows(kept.Name))
	}
	if again, _ := a.Resurrect(true); len(again) != 0 {
		t.Errorf("once back, it is not brought back twice: %+v", again)
	}
}
