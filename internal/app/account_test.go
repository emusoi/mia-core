package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/session"
)

// ownFolders makes every agent's folder its own one under the test's home:
// none is named by a variable.
func ownFolders(t *testing.T) (home, accounts string) {
	t.Helper()
	for _, name := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "PI_CODING_AGENT_DIR"} {
		t.Setenv(name, "")
	}
	return os.Getenv("HOME"), filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "accounts")
}

func accountOf(all []app.Account, agent, name string) app.Account {
	for _, one := range all {
		if one.Agent == agent && one.Name == name {
			return one
		}
	}
	return app.Account{}
}

// An agent's default account is its own folder, and every other one a
// folder of mia's; whether someone signed in is read from whether a file
// is there, and is unknown where the agent may keep it elsewhere.
func TestAnAgentsAccountsAreItsOwnFolderAndEachOfMias(t *testing.T) {
	gittest.Isolate(t)
	home, accounts := ownFolders(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(home, ".claude", "settings.json"), "{}")
	put(t, filepath.Join(accounts, "claude", "work", ".credentials.json"), "")
	if err := os.MkdirAll(filepath.Join(accounts, "codex", "spare"), 0o700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(home, ".pi", "agent", "auth.json"), "")

	all := a.Accounts()
	own := accountOf(all, "claude", "default")
	wantOwn := "no"
	if goruntime.GOOS == "darwin" {
		wantOwn = "unknown" // in the Keychain, where mia does not look
	}
	if !own.Default || own.Home != filepath.Join(home, ".claude") || own.LoggedIn != wantOwn || own.Sessions == nil {
		t.Errorf("claude's own = %+v", own)
	}
	if work := accountOf(all, "claude", "work"); work.Default || work.Home != filepath.Join(accounts, "claude", "work") || work.LoggedIn != "yes" {
		t.Errorf("claude's work = %+v", work)
	}
	if spare := accountOf(all, "codex", "spare"); spare.LoggedIn != "no" {
		t.Errorf("an account the agent never ran with = %+v", spare)
	}
	if never := accountOf(all, "codex", "default"); never.Home != filepath.Join(home, ".codex") || never.LoggedIn != "no" {
		t.Errorf("codex's own, never run = %+v", never)
	}
	if pi := accountOf(all, "pi", "default"); pi.LoggedIn != "yes" {
		t.Errorf("pi's own = %+v", pi)
	}
	if opencode := accountOf(all, "opencode", "default"); opencode.Agent != "" {
		t.Errorf("an agent mia keeps no accounts of has none: %+v", opencode)
	}

	// an agent started on one of mia's hands its variable on to mia: that is not its own folder
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(accounts, "claude", "work"))
	if own := accountOf(a.Accounts(), "claude", "default"); own.Home != filepath.Join(home, ".claude") {
		t.Errorf("claude's own, from inside an account = %s", own.Home)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	// the repository's sessions start on the account mia was told, and once on another
	repository := filepath.Base(repo.Root)
	if _, err := a.UseAccount("claude", "nobody", repository); err == nil || !strings.Contains(err.Error(), "default, work") {
		t.Errorf("an account there is not: %v", err)
	}
	if used, err := a.UseAccount("claude", "work", repository); err != nil || used.Account != "work" || used.From != "mia" {
		t.Fatalf("use = %+v, %v", used, err)
	}
	command, chosen, err := a.CommandWith("claude", "", "", "")
	if want := "CLAUDE_CONFIG_DIR='" + filepath.Join(accounts, "claude", "work") + "' claude"; err != nil || command != want || chosen.Account != "work" {
		t.Errorf("started where the repository starts it: %q %+v %v", command, chosen, err)
	}
	if command, chosen, err := a.CommandWith("claude", "", "", "default"); err != nil || command != "claude" || chosen.Account != "" {
		t.Errorf("this once on its own: %q %+v %v", command, chosen, err)
	}
	if argv, _, err := a.ArgvWith([]string{"claude", "hi"}, "", "", ""); err != nil || len(argv) != 4 || argv[0] != "env" || argv[2] != "claude" {
		t.Errorf("a command given word by word is started behind env: %q %v", argv, err)
	}
	if _, _, err := a.CommandWith("make", "", "", "work"); err == nil {
		t.Error("an account for a window of no agent")
	}
	projects := a.Projects()
	if !slices.Contains(projects, app.Project{Repository: repository, Agent: "claude", Account: "work", From: "mia"}) ||
		!slices.Contains(projects, app.Project{Repository: repository, Agent: "codex", Account: "default", From: "default"}) {
		t.Errorf("projects = %+v", projects)
	}

	// renamed, the repository follows it; removed, it goes back to its config's
	if _, err := a.RenameAccount("claude", "default", "mine"); err == nil {
		t.Error("the agent's own folder was renamed")
	}
	if renamed, err := a.RenameAccount("claude", "work", "team"); err != nil || renamed.Home != filepath.Join(accounts, "claude", "team") {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	if !slices.Contains(a.Projects(), app.Project{Repository: repository, Agent: "claude", Account: "team", From: "mia"}) {
		t.Errorf("after a rename: %+v", a.Projects())
	}
	if _, err := a.RemoveAccount("claude", "default", true); err == nil {
		t.Error("the agent's own folder was removed")
	}
	put(t, filepath.Join(accounts, "claude", "team", "projects", "-x", "c1.jsonl"), "{}")
	if _, err := a.RemoveAccount("claude", "team", false); err == nil || !strings.Contains(err.Error(), "1 conversation") {
		t.Errorf("one that holds conversations: %v", err)
	}
	if _, err := a.RemoveAccount("claude", "team", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(accounts, "claude", "team")); err == nil {
		t.Error("its folder is there still")
	}
	if !slices.Contains(a.Projects(), app.Project{Repository: repository, Agent: "claude", Account: "default", From: "default"}) {
		t.Errorf("after a removal: %+v", a.Projects())
	}
}

// Every account's folder of an agent is looked in for conversations, and
// each says which it is in.
func TestConversationsAreFoundInEveryAccountOfAnAgent(t *testing.T) {
	gittest.Isolate(t)
	home, accounts := ownFolders(t)
	repo := gittest.New(t)
	folder := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(repo.Root, "-")
	put(t, filepath.Join(home, ".claude", "projects", folder, "c1.jsonl"), claudeSays(repo.Root, "user", `"the login page"`, ""))
	put(t, filepath.Join(accounts, "claude", "work", "projects", folder, "c2.jsonl"), claudeSays(repo.Root, "user", `"the login form"`, ""))
	put(t, filepath.Join(accounts, "codex", "work", "sessions", "2026", "11", "02", "rollout-2026-11-02T10-00-00-x1.jsonl"),
		`{"timestamp":"2026-11-02T10:00:00Z","type":"session_meta","payload":{"cwd":`+quoted(repo.Root)+`}}`,
		`{"timestamp":"2026-11-02T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"the login api"}]}}`)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	heads, problems := a.Conversations(app.Wanted{})
	accountOf := map[string]string{}
	for _, h := range heads {
		accountOf[h.ID] = h.Agent + "/" + h.Account
	}
	if len(problems) > 0 || accountOf["c1"] != "claude/default" || accountOf["c2"] != "claude/work" || accountOf["x1"] != "codex/work" {
		t.Fatalf("conversations = %v %v", accountOf, problems)
	}
	if turns, err := a.ConversationTurns("c2", 0); err != nil || len(turns) != 1 || turns[0].Text != "the login form" {
		t.Errorf("turns of one in an account: %+v %v", turns, err)
	}
	found := map[string]string{}
	for _, r := range a.Recall("login", 0) {
		found[r.Session] = r.Account
	}
	if found["c1"] != "default" || found["c2"] != "work" || found["x1"] != "work" {
		t.Errorf("recall = %v", found)
	}
}

// labOnly fails the test unless each program a window's login shell would
// run is the lab's own: a real agent must never start.
func labOnly(t *testing.T, programs ...string) {
	t.Helper()
	bin := filepath.Join(os.Getenv("TMUX_TMPDIR"), "bin")
	tmux, err := exec.LookPath("tmux") // beside real agents, perhaps: the lab has a link to it alone
	if err != nil {
		t.Skip("no tmux")
	}
	if err := os.Symlink(tmux, filepath.Join(bin, "tmux")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	for _, program := range programs {
		out, err := exec.Command("/bin/sh", "-lc", "command -v "+program).Output()
		if got := strings.TrimSpace(string(out)); err != nil || filepath.Dir(got) != bin {
			t.Fatalf("a window's shell finds %s at %q, not the lab's: no window is opened", program, got)
		}
	}
}

// waiting builds a program that only waits, at path: a process with the
// name of its file, as a real agent's is.
func waiting(t *testing.T, path, cache string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "wait.go")
	if err := os.WriteFile(source, []byte("package main\n\nimport \"time\"\n\nfunc main() { time.Sleep(5 * time.Minute) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", path, source)
	build.Env = append(os.Environ(), "GOCACHE="+cache, "GOFLAGS=", "GO111MODULE=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build a stand-in: %v %s", err, out)
	}
}

// Taken up on another account, a conversation is copied into that one's
// folder, stays where it was too, and the agent is started on it.
func TestResumingOnAnotherAccountCopiesTheConversationThere(t *testing.T) {
	cache, err := exec.Command("go", "env", "GOCACHE").Output()
	if err != nil {
		t.Skip("no go to build a stand-in with")
	}
	// it says what ran, and on what, then waits as a process named claude
	lab(t, map[string]string{"claude": `echo "ran $0 on $CLAUDE_CONFIG_DIR" >> "$TMUX_TMPDIR/ran"; exec "$TMUX_TMPDIR/hold/claude"`})
	waiting(t, filepath.Join(os.Getenv("TMUX_TMPDIR"), "hold", "claude"), strings.TrimSpace(string(cache)))
	labOnly(t, "claude")
	home, accounts := ownFolders(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("goba")
	if err != nil {
		t.Fatal(err)
	}
	folder := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(record.Path, "-")
	own := filepath.Join(home, ".claude", "projects", folder, "c1.jsonl")
	put(t, own, claudeSays(record.Path, "user", `"start it here"`, ""))
	work := filepath.Join(accounts, "claude", "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}

	done, err := a.ResumeConversation("c1", "", "", "", "", "work")
	if err != nil {
		t.Fatal(err)
	}
	if !done.Copied || done.Account != "work" || done.Command != "CLAUDE_CONFIG_DIR='"+work+"' claude --resume c1" {
		t.Fatalf("resumed = %+v", done)
	}
	if _, err := os.Stat(filepath.Join(work, "projects", folder, "c1.jsonl")); err != nil {
		t.Errorf("not copied into the account's folder: %v", err)
	}
	if _, err := os.Stat(own); err != nil {
		t.Errorf("not left where it was: %v", err)
	}
	var ran []byte
	for range 250 {
		if ran, _ = os.ReadFile(filepath.Join(os.Getenv("TMUX_TMPDIR"), "ran")); len(ran) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if want := "ran " + filepath.Join(os.Getenv("TMUX_TMPDIR"), "bin", "claude") + " on " + work + "\n"; string(ran) != want {
		t.Fatalf("what ran: %q, wanted %q", ran, want)
	}
	// a window started by its shell has the shell in front, to tmux: one
	// that runs the agent itself, on the account, is the one looked for
	direct := filepath.Join(os.Getenv("TMUX_TMPDIR"), "hold", "claude")
	if _, err := a.NewWindow(record, "direct", []string{"env", "CLAUDE_CONFIG_DIR=" + work, direct}); err != nil {
		t.Fatal(err)
	}
	if err := a.KeepChosen(record, "direct", "claude", app.Chosen{Account: "work"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 250 && !slices.ContainsFunc(session.Here().WindowsOf(record.Name), func(w session.Window) bool { return w.Command == "claude" }); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	heads, _ := a.Conversations(app.Wanted{Worktree: record.Name})
	var running []string
	for _, h := range heads {
		if h.Running {
			running = append(running, h.ID+"/"+h.Account+"/"+h.Window.Name)
		}
	}
	if !slices.Equal(running, []string{"c1/work/direct"}) {
		t.Errorf("running = %v: the window's is the copy on its account", running)
	}
	if again, err := a.ResumeConversation("c1", "", "", "", "", "default"); err == nil || again.Already {
		t.Errorf("one a window has open on another account, asked on its own: %+v %v", again, err)
	}
	if err := a.AccountOfWindow(record, "direct", "default"); err == nil || !strings.Contains(err.Error(), "mia conversation resume c1 --account default") {
		t.Errorf("a running agent is put on another account by a restart, which mia says and does not do: %v", err)
	}
	if !session.Here().HasWindow(record.Name, "direct") {
		t.Error("the window was closed")
	}
}
