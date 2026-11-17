package app

import (
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
)

// ReachState is what asking a machine came to: a closed set an app can
// switch on.
type ReachState string

const (
	ReachOK          ReachState = "ok"
	ReachUnreachable ReachState = "unreachable" // ssh could not connect: it is off, its name does not resolve, it timed out
	ReachNeedsLogin  ReachState = "needs_login" // ssh connected and was refused, or wanted a key accepted or a password typed
	ReachNoMia       ReachState = "no_mia"      // it answers and has no mia where mia ships it
	ReachOldMia      ReachState = "old_mia"     // its mia answers and does not know what was asked
	ReachNotSetUp    ReachState = "not_set_up"  // its mia runs and the repository asked about is not there
	ReachFailed      ReachState = "failed"      // its mia answered with an error that is none of the above
)

// ReachStates is every state, for what lists them.
var ReachStates = []ReachState{ReachOK, ReachUnreachable, ReachNeedsLogin, ReachNoMia, ReachOldMia, ReachNotSetUp, ReachFailed}

// Reach is whether a machine could be asked, and when not, what is wrong
// in a line and the one step that puts it right.
type Reach struct {
	State  ReachState `json:"state"`
	Detail string     `json:"detail,omitempty"`
	Fix    string     `json:"fix,omitempty"` // left out when mia knows none
	// Warnings are what works now and will not go on working.
	Warnings []Warning `json:"warnings,omitempty"`
}

// Warning is one thing about a machine that answers which wants fixing.
type Warning struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
}

// versionMark is before the version a machine's mia says it is, on the
// line a failed ask ends with.
const versionMark = "@@mia"

// sayVersionWhenFailed is put after a command of a machine's mia: when it
// fails, the mia there says which it is, in the same round trip.
const sayVersionWhenFailed = ` || { code=$?; echo "` + versionMark + ` $(` + remoteMia + ` version 2>/dev/null)" >&2; exit $code; }`

// loginSigns are what ssh says, in lower case, when it reached the machine
// and was not let in. BatchMode keeps it from asking, so it says so and
// exits.
var loginSigns = []string{
	"permission denied", "host key verification failed", "too many authentication failures",
	"authenticity of host", "no supported authentication methods", "passphrase", "password",
	"agent refused operation", "keyring", "sign_and_send_pubkey", "remote host identification has changed",
}

// Diagnose is the one place a failed ask of a machine is read: what err
// means, and the step that fixes it. inRepository is whether what was
// asked is about a repository there; what mia answers from any folder
// needs none, so a mia that wants one for it is an older mia.
func Diagnose(machine runtime.Machine, err error, inRepository bool) Reach {
	if err == nil {
		return Reach{State: ReachOK}
	}
	said := err.Error()
	var failure *run.Failure
	if errors.As(err, &failure) {
		said = failure.Stderr
	}
	var lines []string
	version := ""
	for _, line := range strings.Split(said, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, versionMark); ok {
			version = strings.TrimSpace(rest)
		} else if line != "" {
			lines = append(lines, line)
		}
	}
	first := ""
	if len(lines) > 0 {
		first = strings.TrimPrefix(strings.TrimPrefix(lines[0], machine.SSH+": "), "mia: ")
	}
	low := strings.ToLower(strings.Join(lines, "\n"))
	has := func(signs ...string) bool {
		for _, sign := range signs {
			if strings.Contains(low, sign) {
				return true
			}
		}
		return false
	}
	name, code := machine.Name, run.ExitCode(err)
	its := name + "'s mia"
	if version != "" {
		its += " (" + version + ")"
	}
	switch {
	case code == sshFailed && has(loginSigns...):
		return Reach{State: ReachNeedsLogin, Detail: name + " would not let ssh in: " + first,
			Fix: "ssh " + machine.SSH + " once in a terminal, to sign in and accept its key"}
	case code == sshFailed:
		return Reach{State: ReachUnreachable, Detail: name + " cannot be reached over ssh: " + first,
			Fix: "turn " + name + " on, or check its address with ssh " + machine.SSH}
	case (code == 126 || code == 127) && has("mia"), has("mia: not found", "mia: no such file", "mia: command not found"):
		return noMia(name)
	case has("no such query", "did not say"), !inRepository && has(git404):
		return Reach{State: ReachOldMia, Detail: its + " is older than this one and does not answer this", Fix: "mia machine ship " + name}
	case inRepository && has(git404, "no such file or directory"):
		return Reach{State: ReachNotSetUp, Detail: name + " does not have this repository", Fix: "mia machine link " + name}
	}
	return Reach{State: ReachFailed, Detail: its + " said: " + first}
}

// git404 is how mia says, in lower case, that it was run in no repository.
const git404 = "not inside a git repository"

// askAgainAfter is how long a machine that did not answer is left alone.
// One that is off or wants a login stays so until a person does something:
// after failing twice running it is asked once a minute.
func askAgainAfter(state ReachState, failures int) time.Duration {
	if failures > 1 && (state == ReachUnreachable || state == ReachNeedsLogin) {
		return time.Minute
	}
	return unreachableFor
}

// clock is the time, which a test may set.
func (a *App) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// LastReach is what a machine came to when it was last asked, and when,
// without asking it; false when it never was.
func (a *App) LastReach(machine string) (Reach, time.Time, bool) {
	last, ok := a.lastAnswer(machine)
	if !ok {
		return Reach{}, time.Time{}, false
	}
	return last.reach(), last.At, true
}

func (a *App) lastAnswer(machine string) (answer, bool) {
	var last answer
	data, err := os.ReadFile(a.answerPath(machine))
	if err != nil || json.Unmarshal(data, &last) != nil {
		return answer{}, false
	}
	return last, true
}

// reach is the answer's, also of one kept by a mia that kept none.
func (last answer) reach() Reach {
	switch {
	case last.Reach.State != "":
		return last.Reach
	case last.Answers:
		return Reach{State: ReachOK}
	}
	return Reach{State: ReachFailed, Detail: last.Detail}
}

// hostOS is the system this mia runs on, which a test may set.
var hostOS = goruntime.GOOS

// How long what loginctl said is believed: a no is asked again soon, for
// the fix to show.
const (
	lingerKept   = 24 * time.Hour
	lingerNoKept = 10 * time.Minute
)

type lingerAnswer struct {
	Linger string    `json:"linger"`
	At     time.Time `json:"at"`
}

// Linger is the warning that this machine's user services stop when the
// person logs out, taking the agents with them: on Linux, when loginctl
// says lingering is off. What loginctl said is kept, so that a poll does
// not run it.
func (a *App) Linger() []Warning {
	if hostOS != "linux" {
		return nil
	}
	cache := ""
	if a.MiaDir != "" {
		cache = filepath.Join(a.MiaDir, "cache", "linger.json")
	}
	var last lingerAnswer
	if data, err := os.ReadFile(cache); err == nil && json.Unmarshal(data, &last) == nil {
		kept := lingerKept
		if last.Linger == "no" {
			kept = lingerNoKept
		}
		if a.clock().Sub(last.At) < kept {
			return LingerWarning(last.Linger)
		}
	}
	last = lingerAnswer{At: a.clock()}
	if me, err := user.Current(); err == nil {
		out, _ := run.Local("loginctl").Capture("show-user", me.Username, "-p", "Linger")
		last.Linger = LingerSaid(out)
	}
	if data, err := json.Marshal(last); err == nil && cache != "" && os.MkdirAll(filepath.Dir(cache), 0o755) == nil {
		_ = os.WriteFile(cache, data, 0o644)
	}
	return LingerWarning(last.Linger)
}

// lingerScript prints what loginctl says of lingering, on Linux, and
// nothing elsewhere: for a command that is run on a machine anyway.
const lingerScript = `[ "$(uname -s)" != Linux ] || loginctl show-user "$(id -un)" -p Linger 2>/dev/null`

// LingerSaid is the yes or no in what `loginctl show-user -p Linger`
// printed, "" when it printed neither.
func LingerSaid(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "Linger="); ok {
			return value
		}
	}
	return ""
}

// LingerWarning is the warning for what loginctl said: one for a no.
func LingerWarning(said string) []Warning {
	if said != "no" {
		return nil
	}
	return []Warning{{Name: "linger", Detail: "user services stop at logout, and the agents with them", Fix: "loginctl enable-linger"}}
}

// examineScript says, in one command, what doctor wants of a machine: its
// system and container engine, the mia it has, and whether it lingers.
const examineScript = `uname -sr; command -v podman || command -v docker; code=$?
echo "` + versionMark + ` $(` + remoteMia + ` version 2>/dev/null)"
` + lingerScript + `
exit $code`

// Examine asks a machine once and says its system and container engine,
// and how it stands: reachable, with a mia, and staying up after logout.
func Examine(machine runtime.Machine) (string, Reach) {
	out, err := runtime.Run(machine.SSH, "sh", "-c", examineScript)
	if run.ExitCode(err) == 1 { // ssh got there and the script's one test failed
		return "", Reach{State: ReachFailed, Detail: machine.Name + " has no container engine", Fix: "install podman or docker on " + machine.Name}
	}
	if err != nil {
		return "", Diagnose(machine, err, false)
	}
	var system []string
	version := ""
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, versionMark); ok {
			version = strings.TrimSpace(rest)
		} else if line != "" && !strings.HasPrefix(line, "Linger=") {
			system = append(system, line)
		}
	}
	reach := Reach{State: ReachOK, Warnings: LingerWarning(LingerSaid(out))}
	if version == "" {
		reach = noMia(machine.Name)
	}
	return strings.Join(system, " · "), reach
}

func noMia(name string) Reach {
	return Reach{State: ReachNoMia, Detail: name + " has no mia at ~/.local/bin/mia", Fix: "mia machine ship " + name}
}
