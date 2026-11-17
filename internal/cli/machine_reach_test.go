package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/runtime"
)

// twoMachines is an ssh for the test that reaches fedora, a Linux machine
// that does not linger, and is refused by box.
func twoMachines(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	ssh := `#!/bin/sh
case " $* " in
*" box.invalid "*) echo "user@box.invalid: Permission denied (publickey)." >&2; exit 255 ;;
*"'api' 'standing'"*) echo '{"waiting":[],"working":[],"done_unread":[],"counts":{"waiting":0,"working":0,"done_unread":0},"machines":[{"name":"local","answers":true,"reach":{"state":"ok","warnings":[{"name":"linger","detail":"user services stop at logout, and the agents with them","fix":"loginctl enable-linger"}]}}]}' ;;
*) printf '%s\n' "Linux 6.11.4" /usr/bin/podman "@@mia v0.3.0" Linger=no ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	machines := runtime.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "runtimes.json")}
	for _, name := range []string{"box", "fedora"} {
		if err := machines.Add(runtime.Machine{Name: name, SSH: name + ".invalid", Engine: "podman"}); err != nil {
			t.Fatal(err)
		}
	}
}

// `mia machine ls` says how each machine stood when standing last asked
// it, and asks none itself.
func TestMachineListSaysHowEachStoodWhenLastAsked(t *testing.T) {
	gittest.Isolate(t)
	twoMachines(t)

	data, code := printed(t, "machine", "ls")
	if want := "local        this computer            \nbox          box.invalid              podman\nfedora       fedora.invalid           podman\n"; code != exitOK || string(data) != want {
		t.Fatalf("never asked, a listing is as it was: %q, exit %d", data, code)
	}
	if _, code := printed(t, "api", "standing"); code != exitOK {
		t.Fatalf("standing exited %d", code)
	}
	data, code = printed(t, "machine", "ls")
	for _, line := range []string{
		"box          box.invalid              podman   needs_login — box would not let ssh in: user@box.invalid: Permission denied (publickey). — ssh box.invalid once in a terminal, to sign in and accept its key\n",
		"fedora       fedora.invalid           podman   ok — user services stop at logout, and the agents with them: loginctl enable-linger\n",
	} {
		if code != exitOK || !strings.Contains(string(data), line) {
			t.Errorf("the listing lacks %q:\n%s", line, data)
		}
	}
	data, code = printed(t, "machine", "ls", "--json")
	var listed []listedMachine
	if err := json.Unmarshal(data, &listed); err != nil || code != exitOK || len(listed) != 3 {
		t.Fatalf("as JSON: %s, exit %d, %v", data, code, err)
	}
	if listed[0].Name != "local" || listed[0].Reach != nil || listed[0].Asked != nil {
		t.Errorf("this computer = %+v", listed[0])
	}
	if box := listed[1]; box.Name != "box" || box.SSH != "box.invalid" || box.Reach == nil || box.Reach.State != app.ReachNeedsLogin || box.Reach.Fix == "" || box.Asked == nil {
		t.Errorf("box = %+v", box)
	}
	if fedora := listed[2]; fedora.Reach == nil || fedora.Reach.State != app.ReachOK || len(fedora.Reach.Warnings) != 1 {
		t.Errorf("fedora = %+v", fedora)
	}
}

// `mia doctor` says of each machine what is wrong and the fix, and warns
// of a Linux machine whose user services stop at logout.
func TestDoctorSaysWhatIsWrongWithAMachineAndTheFix(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	twoMachines(t)

	data, _ := printed(t, "doctor", "--json")
	var report doctorReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("doctor was not JSON: %v\n%s", err, data)
	}
	checks := map[string]doctorCheck{}
	for _, check := range report.Checks {
		checks[check.Name] = check
	}
	if box := checks["machine box"]; box.Status != "fail" || box.State != app.ReachNeedsLogin || !strings.Contains(box.Detail, "Permission denied") || box.Fix != "ssh box.invalid once in a terminal, to sign in and accept its key" {
		t.Errorf("box = %+v", box)
	}
	if fedora := checks["machine fedora"]; fedora.Status != "ok" || fedora.State != app.ReachOK || fedora.Detail != "Linux 6.11.4 · /usr/bin/podman" {
		t.Errorf("fedora = %+v", fedora)
	}
	if linger := checks["linger on fedora"]; linger.Status != "warn" || linger.Fix != "loginctl enable-linger" {
		t.Errorf("linger on fedora = %+v", linger)
	}
	if _, asked := checks["tmux on box"]; asked || report.OK {
		t.Errorf("a machine that would not let ssh in was asked more, or doctor passed: %s", data)
	}
	text, _ := printed(t, "doctor")
	if want := "FAIL  machine box              box would not let ssh in: user@box.invalid: Permission denied (publickey). — ssh box.invalid once in a terminal, to sign in and accept its key\n"; !strings.Contains(string(text), want) {
		t.Errorf("doctor lacks %q:\n%s", want, text)
	}
}
