package cli

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/emusoi/mia-core/internal/gateway"
)

// heldOpen is how many repositories the server holds open: those it has
// answered a question of.
func heldOpen(s *server) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := 0
	for _, one := range s.kept {
		one.mu.Lock()
		if one.app != nil {
			held++
		}
		one.mu.Unlock()
	}
	return held
}

// leaveASocket leaves the socket a server that was killed leaves: there,
// and nobody answering on it.
func leaveASocket(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(serveDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	left, err := net.ListenUnix("unix", &net.UnixAddr{Name: serveSocket(), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	left.SetUnlinkOnClose(false)
	left.Close()
	if _, err := os.Stat(serveSocket()); err != nil {
		t.Fatalf("no socket was left behind: %v", err)
	}
}

// mia api asks the server when one of its build answers, and prints what
// it says: the same bytes, words and exit code as when it reads for itself.
func TestMiaAPIAsksTheServerThatRuns(t *testing.T) {
	isolateServe(t)
	repo := aWorktree(t)
	queries := [][]string{{"api", "worktrees"}, {"api", "stack", "nowhere"}, {"api", "conversations", "--limit", "x"}, {"api", "nosuchquery"}, {"api"}}
	var alone []reply
	for _, query := range queries { // none runs: it reads for itself
		alone = append(alone, capture(t, repo.Root+".longido", query...))
	}
	if alone[0].code != exitOK || alone[1].code != exitNotFound || alone[2].code != exitUsage {
		t.Fatalf("with no server: %+v", alone)
	}
	s := serveIn(t)
	for i, query := range queries {
		if got := capture(t, repo.Root+".longido", query...); got != alone[i] {
			t.Errorf("mia %v\nof the server: %+v\nalone: %+v", query, got, alone[i])
		}
	}
	if heldOpen(s) != 1 {
		t.Error("a server ran, and mia api did not ask it")
	}
}

// What mia api prints is what the server answered, its words and exit code
// too, asked with the folder it is typed in and the words after the query.
func TestWhatMiaAPIPrintsIsTheServers(t *testing.T) {
	isolateServe(t)
	repo := aWorktree(t)
	listener, err := listenServe()
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(serveVersion{Build: gateway.Build(), API: apiVersion})
	})
	asked := func(r *http.Request) map[string]any {
		return map[string]any{"query": r.PathValue("query"), "dir": r.URL.Query().Get("dir"), "args": r.URL.Query()["arg"]}
	}
	mux.HandleFunc("GET /api/{query}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Mia-Exit", "3")
		w.Header().Set("Mia-Stderr", url.QueryEscape("mia: said by the server\n"))
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(asked(r))
	})
	web := &http.Server{Handler: mux}
	go web.Serve(listener)
	t.Cleanup(func() { web.Close() })
	got := capture(t, repo.Root, "api", "worktrees", "longido")
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(map[string]any{"query": "worktrees", "dir": here, "args": []string{"longido"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != (reply{string(want) + "\n", "mia: said by the server\n", exitNotFound}) {
		t.Errorf("mia api worktrees longido = %+v\nthe server answered %s", got, want)
	}
}

// It reads for itself when no server answers, the socket left by one that
// was killed too, when the server is another build of mia, which may answer
// what this one would not, and when told to.
func TestMiaAPIReadsForItselfWhenNoServerOfItsBuildIsThere(t *testing.T) {
	for name, set := range map[string]func(*testing.T) *server{
		"no server":            func(*testing.T) *server { return nil },
		"a socket left behind": func(t *testing.T) *server { leaveASocket(t); return nil },
		"another build": func(t *testing.T) *server {
			return serveIn(t, func(s *server) { s.build = "another build" })
		},
		"MIA_NO_SERVE": func(t *testing.T) *server {
			t.Setenv("MIA_NO_SERVE", "1")
			return serveIn(t)
		},
	} {
		t.Run(name, func(t *testing.T) {
			isolateServe(t)
			repo := aWorktree(t)
			s := set(t)
			if got := capture(t, repo.Root, "api", "base"); got != (reply{"{\n  \"branch\": \"main\"\n}\n", "", exitOK}) {
				t.Errorf("mia api base = %+v", got)
			}
			if s != nil && heldOpen(s) != 0 {
				t.Error("the server was asked")
			}
		})
	}
}
