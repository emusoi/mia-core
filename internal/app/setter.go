package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/signal"
)

// process is one process as ps lists it.
type process struct {
	parent  int
	started time.Time
	command string
}

// processes is the processes with these ids, or every one when none is
// named; nil when ps did not answer or says nothing mia can read, as a ps
// with no lstart does.
func processes(pids ...int) map[int]process {
	const columns = "pid=,ppid=,lstart=,args="
	args := []string{"-axo", columns}
	if len(pids) > 0 {
		list := make([]string, len(pids))
		for i, pid := range pids {
			list[i] = strconv.Itoa(pid)
		}
		args = []string{"-o", columns, "-p", strings.Join(list, ",")}
	}
	ps := run.Local("ps").Command(args...)
	ps.Env = append(os.Environ(), "LC_ALL=C") // lstart in the one language it is read in
	out, err := ps.Output()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || len(exit.Stderr) > 0) {
		return nil // ps exits 1 and says nothing when none of the ids is running
	}
	all := map[int]process{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		parent, err2 := strconv.Atoi(fields[1])
		started, err3 := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(fields[2:7], " "), time.Local)
		if err1 != nil || err2 != nil || err3 != nil {
			return nil
		}
		all[pid] = process{parent: parent, started: started, command: strings.Join(fields[7:], " ")}
	}
	return all
}

// sameStart reports whether two readings of when a process began are of one
// process: a second apart is, as a ps that reckons it from the uptime may say.
func sameStart(a, b time.Time) bool {
	return a.Sub(b).Abs() <= time.Second
}

// setters is the processes recorded with the signals that go with them,
// those of them still running; nil when none is recorded or ps did not
// answer.
func setters(signals []signal.Signal) map[int]process {
	var pids []int
	for _, one := range signals {
		if one.Pid != 0 && !one.UntilSeen && !slices.Contains(pids, one.Pid) {
			pids = append(pids, one.Pid)
		}
	}
	if len(pids) == 0 {
		return nil
	}
	return processes(pids...)
}

// passes are the commands between a hook and the agent that ran it: the
// shells a hook is run by, and mia itself.
var passes = []string{"sh", "bash", "zsh", "dash", "ash", "ksh", "fish", "csh", "tcsh", "mia"}

// Setter is the process that stands behind a signal this process sets, and
// when it began: of the processes this one descends from the nearest that
// is an agent, as the agent table tells one by its command line, or with no
// agent among them the nearest that is not a shell or mia. This process and
// a hook's shell are gone at once; that one lasts as long as what asked.
// Zero when none can be told.
func (a *App) Setter() (int, time.Time) {
	return setterOf(a.agents(), processes(), os.Getppid())
}

func setterOf(table []Agent, all map[int]process, pid int) (int, time.Time) {
	nearest := 0
	for hops := 0; pid > 1 && hops < 64; hops++ {
		one, there := all[pid]
		if !there {
			break
		}
		if name, _ := agentOf(table, one.command); name != "" {
			return pid, one.started
		}
		if first, _, _ := strings.Cut(one.command, " "); nearest == 0 && !slices.Contains(passes, strings.TrimPrefix(filepath.Base(first), "-")) {
			nearest = pid
		}
		pid = one.parent
	}
	if nearest == 0 {
		return 0, time.Time{}
	}
	return nearest, all[nearest].started
}
