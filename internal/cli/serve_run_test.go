package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// The server runs a verb by running mia itself, and starts itself in the
// background the same way. In a test mia is the test's own program, which
// is mia when it is told so.
func TestMain(m *testing.M) {
	if os.Getenv("MIA_TEST_IS_MIA") != "" {
		os.Exit(Run(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// posted is POST /run: what the server answered, and with which status.
func posted(t *testing.T, asked verbAsked) (verbRan, int) {
	t.Helper()
	body, err := json.Marshal(asked)
	if err != nil {
		t.Fatal(err)
	}
	response, err := serveClient(time.Minute).Post("http://mia/run", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var answered verbRan
	if err := json.NewDecoder(response.Body).Decode(&answered); err != nil {
		t.Fatalf("POST /run %v answered no JSON: %v", asked.Args, err)
	}
	return answered, response.StatusCode
}

// A verb asked of the server is run in the folder given, what it printed
// and exited with is the answer, and whoever listens hears what it changed
// then: the server looks at nothing else here.
func TestAVerbRunByTheServerIsHeardByItsListeners(t *testing.T) {
	isolateServe(t)
	socket, err := os.MkdirTemp("/tmp", "mia-serve-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	t.Setenv("MIA_TEST_IS_MIA", "1")
	repo := gittest.New(t)
	serveIn(t, func(s *server) { s.tick, s.backstop = time.Hour, time.Hour })
	events := listen(t, q("worktrees", repo.Root))
	if one := told(t, events); strings.Contains(string(one.Result), "quick") {
		t.Fatalf("first = %+v", one)
	}

	made, status := posted(t, verbAsked{Args: []string{"new", "--name", "quick", "quick"}, Dir: repo.Root})
	if status != http.StatusOK || made.Code != exitOK || !strings.Contains(made.Stdout, "quick") {
		t.Fatalf("mia new = %+v, %d", made, status)
	}
	one := told(t, events)
	var listings []app.Listing
	if err := json.Unmarshal(one.Result, &listings); err != nil || len(listings) != 2 || listings[1].Name != "quick" || !listings[1].Adopted {
		t.Fatalf("a worktree was made by the server, and its listeners heard %+v: %v", one, err)
	}

	if read, _ := posted(t, verbAsked{Args: []string{"run", "quick", "cat"}, Dir: repo.Root, Stdin: "what it reads\n"}); read.Code != exitOK || read.Stdout != "what it reads\n" {
		t.Errorf("a verb that reads = %+v", read)
	}
	if missing, status := posted(t, verbAsked{Args: []string{"rm", "nowhere"}, Dir: repo.Root}); status != http.StatusOK || missing.Code != exitNotFound || !strings.Contains(missing.Stderr, "nowhere") {
		t.Errorf("a verb that fails = %+v, %d", missing, status)
	}
	if nowhere, _ := posted(t, verbAsked{Args: []string{"p", "ls"}, Dir: repo.Root + ".gone"}); nowhere.Code != exitFailed || nowhere.Stderr == "" {
		t.Errorf("a verb in a folder there is not = %+v", nowhere)
	}
	response, err := serveClient(time.Second).Post("http://mia/run", "application/json", strings.NewReader("new quick"))
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /run with no JSON: %v, %v", response, err)
	}
}

// The server has no terminal: a verb that needs the one it is typed in is
// refused, saying so, and nothing of it is done.
func TestAVerbThatNeedsATerminalIsNotRunByTheServer(t *testing.T) {
	isolateServe(t)
	t.Setenv("MIA_TEST_IS_MIA", "1")
	repo := gittest.New(t)
	serveIn(t)
	for _, args := range [][]string{
		{"shell", "quick"}, {"away"}, {"chat"}, {"git"}, {"files"}, {"review"},
		{"window", "open", "quick"}, {"windows", "open"}, {"machine", "attach", "quick@fedora"}, {"machine", "shell", "fedora"},
		{"env", "shell"}, {"new", "--shell", "quick"}, {"tmux", "next", "x"}, {"config"},
		{"gc", "--others", "--ask"}, {"resurrect", "--ask"}, {"gateway", "trust"}, {"gateway", "serve"},
		{"api", "conversation", "c1", "--follow"}, {"dev", "quick", "--follow"}, {"serve"}, {"serve", "stop"}, {},
	} {
		refused, status := posted(t, verbAsked{Args: args, Dir: repo.Root})
		if status != http.StatusForbidden || refused.Code != exitUsage || refused.Stdout != "" || !strings.Contains(refused.Stderr, "mia serve has no terminal") {
			t.Errorf("mia %s = %+v, %d", strings.Join(args, " "), refused, status)
		}
	}
	if out := repo.Git("branch", "--format=%(refname:short)"); out != "main" {
		t.Errorf("a verb refused made a branch: %q", out)
	}
	for _, args := range [][]string{
		{"new", "quick"}, {"window", "new", "quick"}, {"window", "send", "quick", "--", "open"}, {"config", "show"}, {"config", "base", "main"},
		{"api", "worktrees"}, {"gc", "--merged"}, {"main", "say", "shell"}, {"env", "up"}, {"gateway", "start"}, {"tmux", "install"},
	} {
		if why := notServed(args); why != "" {
			t.Errorf("mia %s is refused: it %s", strings.Join(args, " "), why)
		}
	}
}
