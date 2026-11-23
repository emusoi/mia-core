package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/signal"
)

// `mia api sessions`, `signals` and `plugins` print what `mia resurrect
// --list --json`, `mia signal ls --json` and `mia plugin ls --json` print,
// byte for byte, typed by hand and asked of the server.
func TestTheListsOfVerbsAreQueriesThatSayTheSame(t *testing.T) {
	isolateServe(t)
	repo := aWorktree(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	seen := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if err := os.WriteFile(filepath.Join(a.MiaDir, "sessions.json"), []byte(`{"longido":{"server":"1","windows":[{"Command":"claude","Dir":"`+repo.Root+`.longido"}],"seen":"`+seen+`"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Signals.Set(signal.Signal{Worktree: "longido", Kind: "asks", Text: "which one?", Rank: signal.DefaultRank, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	plugins := t.TempDir()
	if err := os.WriteFile(filepath.Join(plugins, "mia-hello"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", plugins+string(os.PathListSeparator)+os.Getenv("PATH"))
	serveIn(t)
	for query, verb := range map[string][]string{
		"sessions": {"resurrect", "--list", "--json"},
		"signals":  {"signal", "ls", "--json"},
		"plugins":  {"plugin", "ls", "--json"},
	} {
		was := capture(t, repo.Root, verb...)
		if was.code != exitOK || !strings.Contains(was.out, map[string]string{"sessions": "longido", "signals": "which one?", "plugins": "hello"}[query]) {
			t.Fatalf("mia %s = %+v", strings.Join(verb, " "), was)
		}
		for _, got := range []reply{byHand(t, repo.Root, query), served(t, repo.Root, query)} {
			if got != was {
				t.Errorf("mia api %s = %+v\nmia %s = %+v", query, got, strings.Join(verb, " "), was)
			}
		}
	}
}
