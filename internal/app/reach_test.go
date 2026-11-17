package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/runtime"
)

// fakeSSH puts an ssh on PATH that answers each host as its line of a
// shell case says, and counts each time it is run in the file it names.
func fakeSSH(t *testing.T, hosts map[string]string) string {
	t.Helper()
	bin := t.TempDir()
	asked := filepath.Join(bin, "asked")
	script := "#!/bin/sh\necho asked >> " + asked + "\ncase \" $* \" in\n"
	for host, does := range hosts {
		script += `*" ` + host + ` "*) ` + does + " ;;\n"
	}
	script += "*) echo \"ssh: no such host in this test\" >&2; exit 255 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return asked
}

func times(file string) int {
	data, _ := os.ReadFile(file)
	return strings.Count(string(data), "asked")
}

// What ssh or a machine's mia said when an ask failed is read in one
// place, into a state and the step that fixes it.
func TestAFailedAskOfAMachineSaysWhatIsWrongAndTheFix(t *testing.T) {
	gittest.Isolate(t)
	fakeSSH(t, map[string]string{
		"down.invalid":   `echo "ssh: connect to host down.invalid port 22: Operation timed out" >&2; exit 255`,
		"dns.invalid":    `echo "ssh: Could not resolve hostname dns.invalid: nodename nor servname provided, or not known" >&2; exit 255`,
		"denied.invalid": `echo "user@denied.invalid: Permission denied (publickey,password)." >&2; exit 255`,
		"key.invalid":    `echo "Host key verification failed." >&2; exit 255`,
		"nomia.invalid":  `printf '%s\n' "sh: 1: /home/user/.local/bin/mia: not found" "@@mia " >&2; exit 127`,
		"old.invalid":    `printf '%s\n' "mia: not inside a git repository" "@@mia v0.1.0" >&2; exit 1`,
		"query.invalid":  `printf '%s\n' 'mia: no such query "standing" — try one of capabilities|worktrees' "@@mia v0.1.4" >&2; exit 3`,
		"broken.invalid": `printf '%s\n' "mia: read names.json: unexpected end of JSON input" "@@mia v0.3.0" >&2; exit 1`,
		"fine.invalid":   `echo fine`,
	})
	for _, c := range []struct {
		host         string
		inRepository bool
		state        ReachState
		detail, fix  string
	}{
		{"down", false, ReachUnreachable, "box cannot be reached over ssh: ssh: connect to host down.invalid port 22: Operation timed out", "turn box on, or check its address with ssh down.invalid"},
		{"dns", false, ReachUnreachable, "box cannot be reached over ssh: ssh: Could not resolve hostname dns.invalid: nodename nor servname provided, or not known", "turn box on, or check its address with ssh dns.invalid"},
		{"denied", false, ReachNeedsLogin, "box would not let ssh in: user@denied.invalid: Permission denied (publickey,password).", "ssh denied.invalid once in a terminal, to sign in and accept its key"},
		{"key", false, ReachNeedsLogin, "box would not let ssh in: Host key verification failed.", "ssh key.invalid once in a terminal, to sign in and accept its key"},
		{"nomia", false, ReachNoMia, "box has no mia at ~/.local/bin/mia", "mia machine ship box"},
		{"old", false, ReachOldMia, "box's mia (v0.1.0) is older than this one and does not answer this", "mia machine ship box"},
		{"query", false, ReachOldMia, "box's mia (v0.1.4) is older than this one and does not answer this", "mia machine ship box"},
		{"old", true, ReachNotSetUp, "box does not have this repository", "mia machine link box"},
		{"broken", false, ReachFailed, "box's mia (v0.3.0) said: read names.json: unexpected end of JSON input", ""},
		{"fine", false, ReachOK, "", ""},
	} {
		machine := runtime.Machine{Name: "box", SSH: c.host + ".invalid"}
		_, err := runtime.Run(machine.SSH, "sh", "-c", "true")
		if got := Diagnose(machine, err, c.inRepository); got.State != c.state || got.Detail != c.detail || got.Fix != c.fix || got.Warnings != nil {
			t.Errorf("%s, in a repository %v:\n got %+v\nwant %s, %q, %q", c.host, c.inRepository, got, c.state, c.detail, c.fix)
		}
	}
}

// add gives the test's mia a machine at <name>.invalid.
func add(t *testing.T, names ...string) {
	t.Helper()
	machines := runtime.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "runtimes.json")}
	for _, name := range names {
		if err := machines.Add(runtime.Machine{Name: name, SSH: name + ".invalid"}); err != nil {
			t.Fatal(err)
		}
	}
}

// `mia api standing` says of each machine how it could be reached, beside
// whether it answered: the shape an app reads.
func TestStandingSaysHowEachMachineIsReached(t *testing.T) {
	gittest.Isolate(t)
	asked := fakeSSH(t, map[string]string{
		"box.invalid": `echo "user@box.invalid: Permission denied (publickey)." >&2; exit 255`,
		// a Linux machine whose own mia warns that it does not linger
		"fedora.invalid": `echo '{"waiting":[],"working":[],"done_unread":[],"counts":{"waiting":0,"working":0,"done_unread":0},"machines":[{"name":"local","answers":true,"reach":{"state":"ok","warnings":[{"name":"linger","detail":"user services stop at logout, and the agents with them","fix":"loginctl enable-linger"}]}}]}'`,
		// one whose mia is older than reach: it says nothing of it
		"old.invalid": `echo '{"waiting":[],"working":[],"done_unread":[],"counts":{"waiting":0,"working":0,"done_unread":0},"machines":[{"name":"local","answers":true}]}'`,
	})
	add(t, "box", "fedora", "old")
	a, err := AtMain()
	if err != nil {
		t.Fatal(err)
	}
	standing, err := a.Standing(true)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(standing.Machines)
	if err != nil {
		t.Fatal(err)
	}
	const want = `[{"name":"local","answers":true,"reach":{"state":"ok"}},` +
		`{"name":"box","answers":false,"detail":"box.invalid: user@box.invalid: Permission denied (publickey).","reach":{"state":"needs_login","detail":"box would not let ssh in: user@box.invalid: Permission denied (publickey).","fix":"ssh box.invalid once in a terminal, to sign in and accept its key"}},` +
		`{"name":"fedora","answers":true,"reach":{"state":"ok","warnings":[{"name":"linger","detail":"user services stop at logout, and the agents with them","fix":"loginctl enable-linger"}]}},` +
		`{"name":"old","answers":true,"reach":{"state":"ok"}}]`
	if string(data) != want {
		t.Errorf("machines =\n%s\nwant\n%s", data, want)
	}
	// asked again at once, the one that did not answer is not, and says the same
	again, err := a.Standing(true)
	if err != nil || again.Machines[1].Reach.State != ReachNeedsLogin || again.Machines[1].Reach.Fix == "" {
		t.Errorf("asked again = %+v, %v", again.Machines, err)
	}
	if got := times(asked); got != 5 {
		t.Errorf("ssh ran %d times, want 5", got)
	}
	// and a listing reads it without asking
	if reach, at, ok := a.LastReach("box"); !ok || reach.State != ReachNeedsLogin || at.IsZero() {
		t.Errorf("box as last asked = %+v, %v, %v", reach, at, ok)
	}
	if _, _, ok := a.LastReach("nobody"); ok {
		t.Error("a machine never asked has a last answer")
	}
}

// A machine that is off, or wants a login, is asked again after 30
// seconds once and then once a minute; one whose mia fails stays at 30.
func TestAMachineThatStaysOffIsAskedOnceAMinute(t *testing.T) {
	for _, c := range []struct {
		state    ReachState
		failures int
		want     time.Duration
	}{
		{ReachUnreachable, 1, 30 * time.Second}, {ReachUnreachable, 2, time.Minute}, {ReachUnreachable, 9, time.Minute},
		{ReachNeedsLogin, 1, 30 * time.Second}, {ReachNeedsLogin, 2, time.Minute},
		{ReachOldMia, 5, 30 * time.Second}, {ReachNoMia, 5, 30 * time.Second}, {ReachFailed, 5, 30 * time.Second}, {"", 3, 30 * time.Second},
	} {
		if got := askAgainAfter(c.state, c.failures); got != c.want {
			t.Errorf("%s after %d failures is asked again after %s, want %s", c.state, c.failures, got, c.want)
		}
	}

	gittest.Isolate(t)
	asked := fakeSSH(t, map[string]string{
		"box.invalid": `echo "ssh: connect to host box.invalid port 22: Operation timed out" >&2; exit 255`,
		"old.invalid": `printf '%s\n' "mia: not inside a git repository" "@@mia v0.1.0" >&2; exit 1`,
	})
	add(t, "box")
	a, err := AtMain()
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return at }
	for _, step := range []struct {
		after time.Duration
		runs  int // of ssh, in all
	}{
		{0, 1},                // asked, and off
		{29 * time.Second, 1}, // too soon
		{2 * time.Second, 2},  // 31s on: asked again, off again
		{31 * time.Second, 2}, // 30s is no longer enough
		{28 * time.Second, 2}, // 59s
		{2 * time.Second, 3},  // 61s: once a minute
		{59 * time.Second, 3},
		{2 * time.Second, 4},
	} {
		at = at.Add(step.after)
		standing, err := a.Standing(true)
		if err != nil || standing.Machines[1].Reach.State != ReachUnreachable {
			t.Fatalf("standing = %+v, %v", standing.Machines, err)
		}
		if got := times(asked); got != step.runs {
			t.Errorf("at %s ssh had run %d times, want %d", at.Format("15:04:05"), got, step.runs)
		}
	}

	// one whose mia is old is no slower to notice a new one
	add(t, "old")
	if err := a.Runtimes.Remove("box"); err != nil {
		t.Fatal(err)
	}
	before := times(asked)
	for i := range 4 {
		at = at.Add(31 * time.Second)
		if standing, _ := a.Standing(true); standing.Machines[1].Reach.State != ReachOldMia {
			t.Fatalf("standing = %+v", standing.Machines)
		}
		if got := times(asked) - before; got != i+1 {
			t.Errorf("an old mia was asked %d times in %d, 31s apart", got, i+1)
		}
	}
}

// On Linux, a machine whose user services stop at logout warns of it,
// and loginctl is not run again each time it is asked.
func TestALinuxMachineWarnsThatItDoesNotLinger(t *testing.T) {
	gittest.Isolate(t)
	bin := t.TempDir()
	asked, says := filepath.Join(bin, "asked"), filepath.Join(bin, "says")
	if err := os.WriteFile(filepath.Join(bin, "loginctl"), []byte("#!/bin/sh\necho asked >> "+asked+"\n[ \"$1 $3 $4\" = \"show-user -p Linger\" ] && cat "+says+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	say := func(linger string) {
		t.Helper()
		if err := os.WriteFile(says, []byte("Linger="+linger+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, err := AtMain()
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return at }

	say("no")
	if got := a.Linger(); got != nil || times(asked) != 0 {
		t.Fatalf("off Linux there is nothing to ask: %+v, loginctl ran %d times", got, times(asked))
	}
	was := hostOS
	hostOS = "linux"
	t.Cleanup(func() { hostOS = was })

	standing, err := a.Standing(false)
	if err != nil {
		t.Fatal(err)
	}
	want := Warning{Name: "linger", Detail: "user services stop at logout, and the agents with them", Fix: "loginctl enable-linger"}
	if got := standing.Machines[0].Reach; got.State != ReachOK || len(got.Warnings) != 1 || got.Warnings[0] != want {
		t.Fatalf("this machine's reach = %+v", got)
	}
	at = at.Add(9 * time.Minute)
	if got := a.Linger(); len(got) != 1 || times(asked) != 1 {
		t.Errorf("9 minutes on: %+v, loginctl ran %d times, want 1", got, times(asked))
	}
	// the fix shows within ten minutes, and a yes is believed for a day
	say("yes")
	at = at.Add(2 * time.Minute)
	if got := a.Linger(); got != nil || times(asked) != 2 {
		t.Errorf("fixed: %+v, loginctl ran %d times, want 2", got, times(asked))
	}
	say("no")
	at = at.Add(23 * time.Hour)
	if got := a.Linger(); got != nil || times(asked) != 2 {
		t.Errorf("23 hours on: %+v, loginctl ran %d times, want 2", got, times(asked))
	}
	at = at.Add(2 * time.Hour)
	if got := a.Linger(); len(got) != 1 || times(asked) != 3 {
		t.Errorf("a day on: %+v, loginctl ran %d times, want 3", got, times(asked))
	}
}

// Doctor asks a machine once: its system, the mia it has, whether it lingers.
func TestExaminingAMachine(t *testing.T) {
	gittest.Isolate(t)
	asked := fakeSSH(t, map[string]string{
		"fedora.invalid": `printf '%s\n' "Linux 6.11.4" /usr/bin/podman "@@mia v0.3.0" Linger=no`,
		"bare.invalid":   `printf '%s\n' "Linux 6.11.4" /usr/bin/docker "@@mia " Linger=yes`,
		"empty.invalid":  `printf '%s\n' "Linux 6.11.4" "@@mia v0.3.0"; exit 1`,
		"off.invalid":    `echo "ssh: connect to host off.invalid port 22: Connection refused" >&2; exit 255`,
	})
	system, reach := Examine(runtime.Machine{Name: "fedora", SSH: "fedora.invalid"})
	if system != "Linux 6.11.4 · /usr/bin/podman" || reach.State != ReachOK || len(reach.Warnings) != 1 || reach.Warnings[0].Fix != "loginctl enable-linger" {
		t.Errorf("fedora = %q, %+v", system, reach)
	}
	system, reach = Examine(runtime.Machine{Name: "bare", SSH: "bare.invalid"})
	if system != "Linux 6.11.4 · /usr/bin/docker" || reach.State != ReachNoMia || reach.Fix != "mia machine ship bare" {
		t.Errorf("bare = %q, %+v", system, reach)
	}
	if _, reach = Examine(runtime.Machine{Name: "empty", SSH: "empty.invalid"}); reach.State != ReachFailed || reach.Fix != "install podman or docker on empty" {
		t.Errorf("empty = %+v", reach)
	}
	if _, reach = Examine(runtime.Machine{Name: "off", SSH: "off.invalid"}); reach.State != ReachUnreachable {
		t.Errorf("off = %+v", reach)
	}
	if got := times(asked); got != 4 {
		t.Errorf("ssh ran %d times, want one a machine", got)
	}
}

// `mia api machines` asks a machine linked to the repository about it: one
// that answers and no longer has it says so, and one that is off is asked
// no oftener than standing asks it.
func TestAMachineWithoutTheRepositoryIsNotSetUp(t *testing.T) {
	gittest.Isolate(t)
	asked := fakeSSH(t, map[string]string{
		"fedora.invalid": `printf '%s\n' "cores 8" "@@worktrees" "@@nofolder"`,
		"box.invalid":    `echo "ssh: connect to host box.invalid port 22: No route to host" >&2; exit 255`,
	})
	a, err := AtMain()
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return at }

	fedora := a.askMachine(runtime.Machine{Name: "fedora", SSH: "fedora.invalid"}, "src/shop", true)
	want := Reach{State: ReachNotSetUp, Detail: "fedora does not have this repository at ~/src/shop", Fix: "mia machine link fedora"}
	if !fedora.Answers || fedora.Health == nil || fedora.Health.Cores != 8 || fedora.Reach.State != want.State || fedora.Reach.Detail != want.Detail || fedora.Reach.Fix != want.Fix || len(fedora.Worktrees) != 0 {
		t.Errorf("fedora = %+v", fedora)
	}
	data, err := json.Marshal(fedora)
	if err != nil || !strings.Contains(string(data), `"answers":true,"reach":{"state":"not_set_up","detail":"fedora does not have this repository at ~/src/shop","fix":"mia machine link fedora"}`) {
		t.Errorf("as JSON: %s, %v", data, err)
	}

	box := runtime.Machine{Name: "box", SSH: "box.invalid"}
	before := times(asked)
	for _, step := range []struct {
		after time.Duration
		runs  int
	}{{0, 1}, {29 * time.Second, 1}, {2 * time.Second, 2}, {59 * time.Second, 2}, {2 * time.Second, 3}} {
		at = at.Add(step.after)
		if state := a.askMachine(box, ".", false); state.Answers || state.Reach.State != ReachUnreachable || state.Detail == "" {
			t.Fatalf("box = %+v", state)
		}
		if got := times(asked) - before; got != step.runs {
			t.Errorf("at %s ssh had run %d times, want %d", at.Format("15:04:05"), got, step.runs)
		}
	}
}
