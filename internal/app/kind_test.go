package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/session"
)

func TestWhatAPaneIsForIsReadFromItsCommand(t *testing.T) {
	a := &app.App{}
	a.Config.Services = []env.Service{{ID: "api", Run: []string{"./bin/api", "--port", "9000"}}}
	for command, want := range map[string]session.Kind{
		"":                         session.KindShell,
		"htop":                     session.KindShell,
		"claude --resume 1234":     session.KindAgent,
		"env FOO=1 codex":          session.KindAgent,
		"EDITOR=x nvim main.go":    session.KindEditor,
		"/usr/local/bin/hx .":      session.KindEditor,
		"go test ./...":            session.KindTests,
		"pnpm test --watch":        session.KindTests,
		"tail -f /var/log/system":  session.KindLogs,
		"npm run dev":              session.KindServer,
		"/srv/bin/api --port 9000": session.KindServer,
		"go build ./...":           session.KindShell,
	} {
		if got := a.WhatOf(command).Kind; got != want {
			t.Errorf("%q is for %q, want %q", command, got, want)
		}
	}
}

// windowKinds is what each window of a worktree's session is for, by name.
func windowKinds(worktree string) map[string]session.Kind {
	all := map[string]session.Kind{}
	for _, w := range session.Here().WindowsOf(worktree) {
		all[w.Name] = w.Kind
	}
	return all
}

func TestEachPaneMiaStartsIsMarkedWithWhatItIsForAndKeepsItWhenItComesBack(t *testing.T) {
	lab(t, map[string]string{"nvim": "exec sleep 300", "claude": "exec sleep 300"})
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("marked")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "first", session.What{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "notes", session.What{}, []string{"nvim", "notes.md"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "claude", session.What{Kind: session.KindAgent}, []string{"claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "follow", session.What{}, []string{"tail", "-f", "/dev/null"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWindow(record, "told", session.What{Kind: session.KindServer}, []string{"sleep", "300"}); err != nil {
		t.Fatal(err)
	}
	want := map[string]session.Kind{"first": session.KindShell, "notes": session.KindEditor, "claude": session.KindAgent,
		"follow": session.KindLogs, "told": session.KindServer}
	same := func(got map[string]session.Kind) bool {
		for name, kind := range want {
			if got[name] != kind {
				return false
			}
		}
		return true
	}
	if got := windowKinds(record.Name); !same(got) {
		t.Fatalf("each window is marked with what it is for: %v, want %v", got, want)
	}
	listed, err := a.List(record.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || len(listed[0].Windows) < len(want) {
		t.Fatalf("the worktree is listed with its windows: %+v", listed)
	}
	for _, l := range listed {
		for _, w := range l.Windows {
			if l.Name == record.Name && w.Kind != want[w.Name] {
				t.Errorf("the worktree's listing has window %s %q, want %q", w.Name, w.Kind, want[w.Name])
			}
		}
	}

	// tmux goes down: what each pane was for is remembered, and marked again
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(a.MiaDir, "sessions.json")); !strings.Contains(string(data), `"Kind": "editor"`) {
		t.Errorf("sessions.json keeps each pane's kind:\n%s", data)
	}
	exec.Command("tmux", "kill-server").Run()
	if _, err := a.Resurrect(false); err != nil {
		t.Fatal(err)
	}
	var got map[string]session.Kind
	for range 100 {
		if got = windowKinds(record.Name); same(got) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !same(got) {
		t.Errorf("each window comes back marked as it was: %v, want %v", got, want)
	}
}
