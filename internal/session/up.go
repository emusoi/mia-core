package session

import (
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"
	"slices"
	"strings"
	"time"
)

// upSession is the first session of a server mia starts: it holds the server up until the
// session it was started for is there, and ends by itself.
const upSession = "_mia-up"

// up starts this Mac's tmux server through launchd when none is running, before mia starts a
// session in it. macOS ends every process an app started when the app quits, a tmux server
// that left it too: started under mia.app, every session and agent would go with the app, at
// a quit or an update. A server launchd started is no app's, and outlives whoever asked.
// The job is removed once the server is up, so launchd does not start it again.
func up() {
	if goruntime.GOOS != "darwin" || exec.Command("tmux", "list-sessions").Run() == nil {
		return
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return
	}
	// (Its environment is the caller's, as a server started here would have had: launchd's
	// own is bare. Less what mia hands a plugin, as every tmux call here leaves out.)
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(pluginCall, name)
	})
	label := fmt.Sprintf("com.emusoi.mia.tmux.%d", os.Getpid())
	args := append(append([]string{"submit", "-l", label, "--", "/usr/bin/env", "-i"}, env...), tmux, "-u", "new-session", "-d", "-s", upSession, "sleep 10")
	if exec.Command("/bin/launchctl", args...).Run() != nil {
		return
	}
	defer exec.Command("/bin/launchctl", "remove", label).Run()
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); time.Sleep(20 * time.Millisecond) {
		if exec.Command("tmux", "has-session", "-t", exact(upSession)).Run() == nil {
			return
		}
	}
}
