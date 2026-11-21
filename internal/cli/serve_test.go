package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

// isolateServe is gittest.Isolate with mia's config folder under a short
// path: a socket's path is at most a hundred bytes or so, and a test's
// temporary folder alone is most of that.
func isolateServe(t *testing.T) {
	t.Helper()
	gittest.Isolate(t)
	short, err := os.MkdirTemp("/tmp", "mia")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(short) })
	t.Setenv("XDG_CONFIG_HOME", short)
}

// serveIn starts the server in this test, on the test's own socket, until
// the test ends; set is what is changed of it before it serves.
func serveIn(t *testing.T, set ...func(*server)) *server {
	t.Helper()
	listener, err := listenServe()
	if err != nil {
		t.Fatal(err)
	}
	s := newServer()
	for _, set := range set {
		set(s)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("the server ended with %v", err)
		}
	})
	return s
}

// reply is what a query printed, said beside it and exited with.
type reply struct {
	out, errs string
	code      int
}

// byHand is `mia api` typed in a folder, by this process and no server.
func byHand(t *testing.T, dir string, args ...string) reply {
	t.Helper()
	t.Setenv("MIA_NO_SERVE", "1")
	return capture(t, dir, append([]string{"api"}, args...)...)
}

// capture is mia typed in a folder: what it printed, said and exited with.
func capture(t *testing.T, dir string, args ...string) reply {
	t.Helper()
	t.Chdir(dir)
	files := [2]*os.File{}
	for i := range files {
		file, err := os.CreateTemp(t.TempDir(), "said")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		files[i] = file
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = files[0], files[1]
	code := Run(args)
	os.Stdout, os.Stderr = stdout, stderr
	out, _ := os.ReadFile(files[0].Name())
	errs, _ := os.ReadFile(files[1].Name())
	return reply{string(out), string(errs), code}
}

// served is the same query asked of the server on the socket.
func served(t *testing.T, dir string, args ...string) reply {
	t.Helper()
	values := url.Values{"dir": {dir}, "arg": args[1:]}
	response, err := serveClient(time.Minute).Get("http://mia/api/" + args[0] + "?" + values.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	out, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	code, err := strconv.Atoi(response.Header.Get("Mia-Exit"))
	if err != nil {
		t.Fatalf("GET /api/%s said no exit code: %v", args[0], err)
	}
	errs, err := url.QueryUnescape(response.Header.Get("Mia-Stderr"))
	if err != nil {
		t.Fatal(err)
	}
	return reply{string(out), errs, code}
}

// withoutHealth is an answer less what is measured as it is asked, the
// load and the memory of the machine: two askings never measure the same.
func withoutHealth(t *testing.T, answer string) string {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(answer), &value); err != nil {
		t.Fatalf("%v: %s", err, answer)
	}
	var strip func(any)
	strip = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			delete(value, "health")
			for _, inner := range value {
				strip(inner)
			}
		case []any:
			for _, inner := range value {
				strip(inner)
			}
		}
	}
	strip(value)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// aWorktree is a repository with a worktree mia keeps, longido.
func aWorktree(t *testing.T) *gittest.Repo {
	t.Helper()
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "longido", Path: repo.Worktree("longido", "feature")}); err != nil {
		t.Fatal(err)
	}
	return repo
}

// The server is asked what `mia api` is, and answers by the same functions:
// every query there is gets the bytes, the words beside them and the exit
// code the command line gives, the ones that fail and the ones mistyped too.
func TestTheServerAnswersEveryQueryAsTheCommandLineDoes(t *testing.T) {
	isolateServe(t)
	repo := aWorktree(t)
	serveIn(t)
	outside := t.TempDir()
	asked := map[string]bool{}
	for _, args := range [][]string{
		{"capabilities"}, {"worktrees"}, {"worktrees", "longido"}, {"worktrees", "long"}, {"machines"}, {"here"}, {"repos"},
		{"env"}, {"env", "longido"}, {"stack"}, {"stack", "longido"}, {"stack", "nowhere"}, {"base"},
		{"conversations"}, {"conversations", "--worktree", "longido", "--limit", "2"}, {"conversations", "--limit", "x"},
		{"conversation", "c1"}, {"conversation"}, {"conversation", "c1", "--follow"},
		{"main"}, {"main", "conversations"}, {"main", "turns"}, {"main", "turns", "--after", "x"},
		{"standing"}, {"standing", "--here"}, {"agents"}, {"agents", "x"}, {"setups"}, {"setups", "--all"},
		{"accounts"}, {"accounts", "--projects"}, {"apps"}, {"status"}, {"status", "x"},
		{"window", "0"}, {"window", "0", "--worktree", "longido"}, {"window"}, {"window", "x"}, {"nosuchquery"},
	} {
		asked[args[0]] = true
		for _, dir := range []string{repo.Root, repo.Root + ".longido", outside} {
			if len(args) > 1 && args[len(args)-1] == "--follow" {
				// a stream, of the server's own: here it only says where it is
				if got := served(t, dir, args...); dir != outside && (got.code != exitUsage || !strings.Contains(got.errs, "/events")) {
					t.Errorf("GET /api of %v in %s = %+v", args, dir, got)
				}
				continue
			}
			want, got := byHand(t, dir, args...), served(t, dir, args...)
			if args[0] == "machines" || args[0] == "here" {
				want.out, got.out = withoutHealth(t, want.out), withoutHealth(t, got.out)
			}
			if got != want {
				t.Errorf("mia api %s in %s\nthe server: %+v\nthe command line: %+v", strings.Join(args, " "), dir, got, want)
			}
		}
	}
	for _, query := range apiQueryList {
		if !asked[query] {
			t.Errorf("mia api %s is not asked of the server here", query)
		}
	}
}

// A repository is held open between questions, and not past what it read
// once: a worktree made since is listed, and a configuration written since
// is the one answered from.
func TestARepositoryHeldOpenAnswersFromWhatIsThereNow(t *testing.T) {
	isolateServe(t)
	repo := aWorktree(t)
	s := serveIn(t)
	if got := served(t, repo.Root, "base"); got.out != "{\n  \"branch\": \"main\"\n}\n" {
		t.Fatalf("base = %+v", got)
	}
	held := s.kept[filepath.Join(repo.Root, ".git")]
	if held == nil || held.app == nil {
		t.Fatalf("the repository is not held open: %+v", s.kept)
	}
	first := held.app
	if served(t, repo.Root+".longido", "worktrees"); held.app != first {
		t.Error("asked again with nothing written, the repository was opened again")
	}
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Put(model.Record{Name: "kijenge", Path: repo.Worktree("kijenge", "other")}); err != nil {
		t.Fatal(err)
	}
	if got := served(t, repo.Root, "worktrees", "kijenge"); !strings.Contains(got.out, `"name": "kijenge"`) {
		t.Errorf("a worktree made since is not listed: %+v", got)
	}
	if err := os.WriteFile(filepath.Join(repo.MiaDir(), "config.toml"), []byte("base = \"feature\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := served(t, repo.Root, "base"); got.out != "{\n  \"branch\": \"feature\"\n}\n" {
		t.Errorf("base, its configuration written since = %+v", got)
	}
}

func TestTheServerSaysItsBuildAndWhatTheAPISpeaks(t *testing.T) {
	isolateServe(t)
	s := serveIn(t)
	running, ok := serving()
	if !ok || running.Build != s.build || running.API != apiVersion || running.Pid != os.Getpid() {
		t.Fatalf("GET /version = %+v, %v", running, ok)
	}
}

// One server a machine: a second is refused while the first answers, and a
// socket nobody answers on, left by a server that was killed, is taken.
func TestASecondServerIsRefusedAndASocketLeftBehindIsTaken(t *testing.T) {
	isolateServe(t)
	leaveASocket(t)
	serveIn(t)
	if _, ok := serving(); !ok {
		t.Fatal("the server does not answer on the socket it took")
	}
	if info, err := os.Stat(serveSocket()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the socket is %v, %v: it is the person's alone", info.Mode().Perm(), err)
	}
	second, err := listenServe()
	if err == nil {
		second.Close()
		t.Fatal("a second server took the socket of one that answers")
	}
	if !strings.Contains(err.Error(), "runs already") {
		t.Errorf("the second was refused with %q", err)
	}
	if _, ok := serving(); !ok {
		t.Fatal("refusing the second took the first's socket away")
	}
	response, err := serveClient(time.Second).Get("http://mia/nothing")
	if err != nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("a path there is not: %v, %v", response, err)
	}
	response.Body.Close()
}
