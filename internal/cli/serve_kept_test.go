package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/model"
)

// A question read by GET /api is answered once and given again from what
// is kept, however often it is read, until something it is answered from
// says it changed: a record of mia's written, read again at once, though
// the server would look only an hour later, or a verb run by the server.
// What another program keeps in mia's folder, mia desk's, is none of it;
// and nothing is asked again before it is read again.
func TestAReadIsKeptUntilWhatItIsAnsweredFromChanges(t *testing.T) {
	isolateServe(t)
	t.Setenv("MIA_TEST_IS_MIA", "1")
	repo := aWorktree(t)
	s := serveIn(t, func(s *server) { s.tick, s.backstop = time.Hour, time.Hour })
	key := questionKey(question{dir: repo.Root, args: []string{"worktrees"}})
	kept := func() *answer {
		t.Helper()
		s.hub.mu.Lock()
		defer s.hub.mu.Unlock()
		if s.hub.topics[key] == nil {
			t.Fatalf("%s is not kept: %v", key, s.hub.topics)
		}
		return s.hub.topics[key].said
	}
	first := served(t, repo.Root, "worktrees")
	if first != byHand(t, repo.Root, "worktrees") {
		t.Fatalf("first = %+v", first)
	}
	was := kept()
	if again := served(t, repo.Root, "worktrees"); again != first || kept() != was {
		t.Fatalf("read again with nothing written, it was asked again: %+v", again)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "desk-state.json"), []byte(`{"tab":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if served(t, repo.Root, "worktrees"); kept() != was {
		t.Error("mia desk's own file written, and the answer was asked again")
	}

	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "kijenge", Path: repo.Worktree("kijenge", "other")}); err != nil {
		t.Fatal(err)
	}
	if got := served(t, repo.Root, "worktrees"); !strings.Contains(got.out, `"name": "kijenge"`) || kept() == was {
		t.Fatalf("a worktree made since is not listed: %+v", got)
	}
	was = kept()
	if ran, _ := posted(t, verbAsked{Args: []string{"api", "base"}, Dir: repo.Root}); ran.Code != exitOK {
		t.Fatalf("POST /run = %+v", ran)
	}
	if served(t, repo.Root, "worktrees"); kept() == was {
		t.Error("a verb was run by the server, and the answer kept was given")
	}

	// looked at often, a change is seen, and only marked until it is read
	s.hub.mu.Lock()
	s.tick = 10 * time.Millisecond
	s.hub.mu.Unlock()
	was = kept()
	if err := a.Store.Put(model.Record{Name: "kigoma", Path: repo.Worktree("kigoma", "third")}); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(300 * time.Millisecond); time.Now().Before(end); {
		s.hub.look()
		time.Sleep(10 * time.Millisecond)
	}
	s.hub.mu.Lock()
	stale := s.hub.topics[key].stale
	s.hub.mu.Unlock()
	if !stale || kept() != was {
		t.Errorf("nobody listens: it is marked, stale %v, and not asked until read", stale)
	}
	if got := served(t, repo.Root, "worktrees"); !strings.Contains(got.out, `"name": "kigoma"`) {
		t.Errorf("read, it is asked: %+v", got)
	}
}

// What is in a folder is read again only once the folder was written: a
// glob of the same patterns is filepath.Glob's, and a file made or taken
// away since is seen.
func TestAFolderIsReadAgainOnlyOnceWritten(t *testing.T) {
	dir := t.TempDir()
	for _, file := range []string{"a.json", "b.json", "c.toml", ".hidden.json", "apps/one/app.toml", "apps/two/app.toml", "apps/two/other"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, file)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	long := time.Now().Add(-time.Hour) // written long ago: what was read of it is what is there
	for _, folder := range []string{dir, filepath.Join(dir, "apps"), filepath.Join(dir, "apps", "one"), filepath.Join(dir, "apps", "two")} {
		if err := os.Chtimes(folder, long, long); err != nil {
			t.Fatal(err)
		}
	}
	patterns := []string{"*.json", "*.toml", "apps/*/app.toml", "apps/*", "nowhere/*.json", "a.json/*"}
	same := func() {
		t.Helper()
		for _, pattern := range patterns {
			want, _ := filepath.Glob(filepath.Join(dir, pattern))
			if got := globbed(filepath.Join(dir, pattern)); !slices.Equal(got, want) {
				t.Errorf("%s = %v, filepath.Glob says %v", pattern, got, want)
			}
		}
	}
	same()
	same() // and from what was read
	if err := os.WriteFile(filepath.Join(dir, "d.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "apps", "one", "app.toml")); err != nil {
		t.Fatal(err)
	}
	same()
}
