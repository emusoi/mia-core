package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/signal"
)

// restFixture is many repositories and worktrees as a person has them, a
// tmux server with a session for some, a machine that is off and an agent's
// conversations, each repository's first folder returned.
func restFixture(t *testing.T, worktrees []int) []string {
	t.Helper()
	isolateServe(t)
	t.Chdir(t.TempDir()) // the server's own folder is in no repository, as a service's is
	socket, err := os.MkdirTemp("/tmp", "mia-rest-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	machines := runtime.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "runtimes.json")}
	if err := machines.Add(runtime.Machine{Name: "fedora", SSH: "fedora.invalid"}); err != nil {
		t.Fatal(err)
	}
	var roots []string
	sessions := 0
	for i, n := range worktrees {
		repo := gittest.New(t)
		roots = append(roots, repo.Root)
		a, err := app.Open(repo.Root)
		if err != nil {
			t.Fatal(err)
		}
		for j := 1; j < n; j++ {
			record, err := a.New(fmt.Sprintf("r%dw%d", i, j))
			if err != nil {
				t.Fatal(err)
			}
			if sessions < 30 && j%2 == 1 {
				sessions++
				if out, err := exec.Command("tmux", "new-session", "-d", "-s", session.Name(record.Name), "-c", record.Path, "sleep", "3600").CombinedOutput(); err != nil {
					t.Fatalf("tmux new-session: %s", out)
				}
			}
			if i == 0 && j <= 20 { // a conversation in each of the first
				project := filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(record.Path, "-"))
				if err := os.MkdirAll(project, 0o755); err != nil {
					t.Fatal(err)
				}
				var lines strings.Builder
				for k := range 200 {
					fmt.Fprintf(&lines, `{"type":"user","cwd":%q,"timestamp":"2026-11-02T10:%02d:%02dZ","message":{"content":"turn %d of a long conversation about the list"}}`+"\n", record.Path, k/60, k%60, k)
				}
				if err := os.WriteFile(filepath.Join(project, fmt.Sprintf("c%d.jsonl", j)), []byte(lines.String()), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return roots
}

// shimmed is a folder of programs that each note their name and first
// word in the file returned, then run the program they stand for, and a
// folder of ssh alone, which notes nothing; ssh stands for a machine that
// is off, which answers nothing for a while.
func shimmed(t *testing.T) (dir, sshOnly, said string) {
	t.Helper()
	// as ConnectTimeout=6 does to a machine that is off
	offline := "/bin/sleep 6\necho 'ssh: connect to host fedora.invalid port 22: Operation timed out' >&2\nexit 255\n"
	dir, sshOnly = t.TempDir(), t.TempDir()
	said = filepath.Join(dir, "said")
	if err := os.WriteFile(filepath.Join(sshOnly, "ssh"), []byte("#!/bin/sh\n"+offline), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "tmux", "sh", "lsof", "ps", "ssh", "uptime", "vm_stat", "df", "sysctl", "awk", "sed"} {
		script := "#!/bin/sh\necho \"" + name + " $1\" >> " + said + "\n"
		if name == "ssh" {
			script += offline
		} else {
			real, err := exec.LookPath(name)
			if err != nil {
				continue
			}
			script += "exec " + real + " \"$@\"\n"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir, sshOnly, said
}

// drain reads GET /events for the questions until the test ends, counting
// the events.
func drain(t *testing.T, questions []string, count *atomic.Int64) {
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
	done := make(chan struct{})
	go func() {
		defer close(done)
		lines := bufio.NewScanner(response.Body)
		lines.Buffer(nil, 1<<24)
		for lines.Scan() {
			if strings.HasPrefix(lines.Text(), "data: ") {
				count.Add(1)
			}
		}
	}()
	t.Cleanup(func() { cancel(); response.Body.Close(); <-done })
}

func cpu(who int) time.Duration {
	var usage syscall.Rusage
	syscall.Getrusage(who, &usage)
	return time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
}

// The server at rest, as a person's machine has it: ~23 repositories, ~120
// worktrees, the phone server's questions and mia desk's, for a minute in
// which nothing changes. It says what it cost, the CPU of the server and of
// what it ran, and what it ran by kind, and writes a CPU profile of the
// server to the folder MIA_SERVE_PROFILE names. A minute each, so only then.
func TestTheServerAtRest(t *testing.T) {
	out := os.Getenv("MIA_SERVE_PROFILE")
	if out == "" {
		t.Skip("MIA_SERVE_PROFILE=<folder> measures the server at rest, over two minutes")
	}
	roots := restFixture(t, []int{40, 15, 10, 8, 6, 5, 4, 4, 3, 3, 2, 2, 2, 2, 2, 2, 2, 1, 1, 1, 1, 1, 1})
	shims, sshOnly, said := shimmed(t)
	plain := os.Getenv("PATH")
	t.Setenv("PATH", sshOnly+string(os.PathListSeparator)+plain) // the machine that is off, from the first ask
	serveIn(t)
	var phone, desk []string
	for _, root := range roots {
		phone = append(phone, q("worktrees", root), q("here", root))
	}
	phone = append(phone, q("repos", ""), q("machines", ""))
	desk = append(desk, phone...)
	for _, args := range [][]string{{"main"}, {"standing"}, {"conversations"}, {"main", "conversations"}, {"agents"}, {"setups"}, {"accounts"}, {"accounts", "--projects"}, {"apps"}} {
		desk = append(desk, q(args[0], roots[0], args[1:]...))
	}
	var phoneTold, deskTold atomic.Int64
	drain(t, phone, &phoneTold)
	drain(t, desk, &deskTold)
	settle := 20 * time.Second // every first answer, the machine that is off asked once
	if wait, err := time.ParseDuration(os.Getenv("MIA_SERVE_SETTLE")); err == nil {
		settle = wait
	}
	settled := time.Now().Add(settle)
	for time.Now().Before(settled) {
		time.Sleep(time.Second)
	}
	minute := func(name string) (self, children time.Duration, told int64) {
		fmt.Fprintf(os.Stderr, "MARK %s start %s\n", time.Now().Format("15:04:05.000"), name)
		defer func() { fmt.Fprintf(os.Stderr, "MARK %s end %s\n", time.Now().Format("15:04:05.000"), name) }()
		self0, children0, told0 := cpu(syscall.RUSAGE_SELF), cpu(syscall.RUSAGE_CHILDREN), phoneTold.Load()+deskTold.Load()
		for end := time.Now().Add(time.Minute); time.Now().Before(end); {
			time.Sleep(time.Second)
		}
		self, children = cpu(syscall.RUSAGE_SELF)-self0, cpu(syscall.RUSAGE_CHILDREN)-children0
		told = phoneTold.Load() + deskTold.Load() - told0
		t.Logf("%s: the server %.2f CPU-s, what it ran %.2f CPU-s, %d events told", name, self.Seconds(), children.Seconds(), told)
		return self, children, told
	}

	// a minute as it is, its CPU and a profile of the server; then another
	// with what it runs noted, which costs a shell more each
	profile, err := os.Create(filepath.Join(out, "serve.cpu.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(profile); err != nil {
		t.Fatal(err)
	}
	minute("a minute at rest")
	pprof.StopCPUProfile()
	profile.Close()
	t.Setenv("PATH", shims+string(os.PathListSeparator)+plain)
	minute("a minute noting what it runs")
	data, _ := os.ReadFile(said)
	kinds := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			kinds[line]++
		}
	}
	type kind struct {
		name string
		n    int
	}
	var all []kind
	total := 0
	for name, n := range kinds {
		all = append(all, kind{name, n})
		if !strings.HasPrefix(name, "awk") && !strings.HasPrefix(name, "sed") {
			total += n
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n || all[i].n == all[j].n && all[i].name < all[j].name })
	var lines strings.Builder
	for _, one := range all {
		fmt.Fprintf(&lines, "  %5d  %s\n", one.n, one.name)
	}
	t.Logf("what it ran in the minute, %d programs less awk and sed, by program and first word:\n%s", total, lines.String())
	t.Logf("the profile: go tool pprof -top %s", filepath.Join(out, "serve.cpu.pprof"))
}

// shim puts programs of these names first on PATH, each a script that
// notes it ran in the file returned and does what the script says.
func shim(t *testing.T, scripts map[string]string) (said string) {
	t.Helper()
	dir := t.TempDir()
	said = filepath.Join(dir, "said")
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\necho "+name+" >> "+said+"\n"+script+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return said
}

func shimRan(said, name string) int {
	data, _ := os.ReadFile(said)
	return strings.Count(string(data), name+"\n")
}

// How busy this machine is, asked of many repositories, is measured once
// for them all, and a machine that is off is asked once and not again at
// each of the server's looks at everything: they are what costs at rest.
func TestManyAskingHowBusyMeasureTheMachineOnceAndOneThatIsOffOnce(t *testing.T) {
	isolateServe(t)
	t.Chdir(t.TempDir()) // the server's own folder is in no repository, as a service's is
	machines := runtime.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "runtimes.json")}
	if err := machines.Add(runtime.Machine{Name: "fedora", SSH: "fedora.invalid"}); err != nil {
		t.Fatal(err)
	}
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		t.Skip("no lsof here")
	}
	said := shim(t, map[string]string{
		"lsof": `exec ` + lsof + ` "$@"`,
		"ssh":  `echo "ssh: connect to host fedora.invalid port 22: Operation timed out" >&2; exit 255`,
	})
	var questions []string
	for range 4 {
		questions = append(questions, q("here", gittest.New(t).Root))
	}
	questions = append(questions, q("machines", ""))
	serveIn(t, func(s *server) {
		s.tick, s.backstop, s.retry = 20*time.Millisecond, 100*time.Millisecond, 50*time.Millisecond
	})
	events := listen(t, questions...)
	for range questions {
		if one := told(t, events); one.Code != exitOK {
			t.Fatalf("first = %+v", one)
		}
	}
	for end := time.Now().Add(time.Second); time.Now().Before(end); { // ten looks at everything
		select {
		case <-events:
		case <-time.After(50 * time.Millisecond):
		}
	}
	if measured := shimRan(said, "lsof"); measured < 1 || measured > 2 { // lsof twice when anything listens on a port
		t.Errorf("lsof ran %d times, of five questions asked ten times: this machine is measured once", measured)
	}
	if asked := shimRan(said, "ssh"); asked != 1 {
		t.Errorf("the machine that is off was asked %d times", asked)
	}
}

// A signal said to go at a time goes then: those who listen are told,
// with no file written and before the server looks at everything.
func TestASignalThatGoesByItselfIsToldAsItGoes(t *testing.T) {
	isolateServe(t)
	repo := aWorktree(t)
	if err := (signal.Store{Dir: repo.MiaDir()}).Set(signal.Signal{Worktree: "longido", Kind: "note", Text: "for a moment", From: "test", At: time.Now(), Until: time.Now().Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	serveIn(t, func(s *server) { s.tick, s.backstop, s.retry = 20*time.Millisecond, time.Hour, 50*time.Millisecond })
	events := listen(t, q("worktrees", repo.Root, "longido"))
	if one := told(t, events); !strings.Contains(string(one.Result), "for a moment") {
		t.Fatalf("first = %+v", one)
	}
	if one := told(t, events); strings.Contains(string(one.Result), "for a moment") {
		t.Fatalf("after it went = %+v", one)
	}
}

// A turn written to a conversation is told to whoever listens to the
// conversations, and so is a sign-in to those of the accounts: no file the
// server stamps of a repository says either, and it does not wait for its
// look at everything.
func TestATurnWrittenAndASignInAreToldAsTheyAreWritten(t *testing.T) {
	isolateServe(t)
	repo := gittest.New(t)
	claude, codex := t.TempDir(), t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", codex)
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	project := filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(repo.Root, "-"))
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	root, _ := json.Marshal(repo.Root)
	line := func(n int) string {
		return `{"type":"user","cwd":` + string(root) + `,"timestamp":"2026-11-02T10:00:0` + strconv.Itoa(n) + `Z","message":{"content":"ask ` + strconv.Itoa(n) + `"}}` + "\n"
	}
	transcript := filepath.Join(project, "c1.jsonl")
	if err := os.WriteFile(transcript, []byte(line(1)), 0o644); err != nil {
		t.Fatal(err)
	}
	serveIn(t, func(s *server) { s.tick, s.backstop, s.retry = 20*time.Millisecond, time.Hour, 50*time.Millisecond })
	conversations, accounts := q("conversations", repo.Root), q("accounts", repo.Root)
	events := listen(t, conversations, accounts)
	turns := func(one event) int {
		var heads []app.Head
		if err := json.Unmarshal(one.Result, &heads); err != nil || len(heads) != 1 {
			t.Fatalf("conversations = %+v: %v", one, err)
		}
		return heads[0].Turns
	}
	codexIn := func(one event) string {
		var all []app.Account
		if err := json.Unmarshal(one.Result, &all); err != nil {
			t.Fatalf("accounts = %+v: %v", one, err)
		}
		for _, account := range all {
			if account.Agent == "codex" && account.Default {
				return account.LoggedIn
			}
		}
		t.Fatalf("no codex account in %s", one.Result)
		return ""
	}
	for range 2 {
		switch one := told(t, events); one.Q {
		case conversations:
			if got := turns(one); got != 1 {
				t.Fatalf("first, %d turns", got)
			}
		case accounts:
			if got := codexIn(one); got != "no" {
				t.Fatalf("first, codex logged in %q", got)
			}
		}
	}
	f, err := os.OpenFile(transcript, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line(2)); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if one := told(t, events); one.Q != conversations || turns(one) != 2 {
		t.Fatalf("after a turn was written = %+v", one)
	}
	// the file codex writes once someone signs in, empty: mia looks only at whether it is there
	if err := os.WriteFile(filepath.Join(codex, "auth.json"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if one := told(t, events); one.Q != accounts || codexIn(one) != "yes" {
		t.Fatalf("after a sign-in = %+v", one)
	}
}

// The main agent going to work on its own is told as tmux says its pane's
// title changed, which is no session or window coming or going.
func TestTheMainAgentGoingToWorkIsToldAsItsPaneSaysSo(t *testing.T) {
	isolateServe(t)
	t.Chdir(t.TempDir())
	socket, err := os.MkdirTemp("/tmp", "mia-serve-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", session.Name("main"), "-n", "claude", "sleep", "600").CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %s", out)
	}
	title := func(title string) {
		t.Helper()
		if out, err := exec.Command("tmux", "select-pane", "-t", session.Target("main", "claude"), "-T", title).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	title("✳ Claude Code") // claude's own, at rest
	serveIn(t, func(s *server) { s.tick, s.backstop, s.retry = 20*time.Millisecond, time.Hour, 50*time.Millisecond })
	events := listen(t, q("main", ""))
	state := func(one event) app.MainState {
		var state app.MainState
		if err := json.Unmarshal(one.Result, &state); err != nil {
			t.Fatalf("main = %+v: %v", one, err)
		}
		return state
	}
	if first := state(told(t, events)); !first.Running || first.Working {
		t.Fatalf("first = %+v", first)
	}
	for watching := false; !watching; time.Sleep(25 * time.Millisecond) {
		out, _ := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
		watching = strings.Contains(string(out), "_mia-serve-")
	}
	select { // what its coming made tmux say is said, and nothing changed
	case one := <-events:
		if state(one).Working {
			t.Fatalf("at work before its title says so: %+v", one)
		}
	case <-time.After(500 * time.Millisecond):
	}
	title("⠋ at work")
	for {
		if state(told(t, events)).Working {
			break
		}
	}
}
