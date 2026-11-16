package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/signal"
)

// Standing is where every agent stands, in every repository mia has
// worked in: which wait on the person, which work, and which finished
// with nobody having looked. It is read from the signals each repository
// keeps, as the radar reads them, and gives them no other meaning:
//
//	waiting      the worktree's best signal calls for you, and stays until answered
//	done_unread  it calls for you, and stays until the worktree is seen
//	working      it does not call, and an agent set it: claude's busy
type Standing struct {
	Waiting    []Stand `json:"waiting"`
	Working    []Stand `json:"working"`
	DoneUnread []Stand `json:"done_unread"`
	Counts     struct {
		Waiting    int `json:"waiting"`
		Working    int `json:"working"`
		DoneUnread int `json:"done_unread"`
	} `json:"counts"`
	// Machines is each machine asked, this one first, and whether it
	// answered: one that did not is named here and has no entries above.
	Machines []Answered `json:"machines"`
}

// Stand is one worktree's standing.
type Stand struct {
	Repository string `json:"repository"`
	// Root is the repository's main checkout on its machine: where a verb
	// about the worktree is run.
	Root     string `json:"root"`
	Worktree string `json:"worktree"`
	Machine  string `json:"machine"`
	// Agent is the agent that said it, "" when something else did: a test
	// watcher, a person.
	Agent string `json:"agent"`
	// Window is the window it was said from, where an answer is typed; left
	// out when it was said from none.
	Window *WindowRef `json:"window,omitempty"`
	// Kind and Line are the signal's: what its setter called it — asks,
	// done, failed, busy — and the line that says why.
	Kind string `json:"kind"`
	Line string `json:"line"`
	// Options are the numbered choices on the agent's screen, the first
	// being 1; of a waiting one only, and only when it shows any.
	Options []string `json:"options,omitempty"`
	// Yes is the keys its setter says approve it, pressed with no Enter.
	Yes   string    `json:"yes,omitempty"`
	Since time.Time `json:"since"`
}

// Answered is a machine that was asked where its agents stand.
type Answered struct {
	Name    string `json:"name"`
	Answers bool   `json:"answers"`
	Detail  string `json:"detail,omitempty"` // why it did not
}

// standingLanes is how many repositories are read at once.
const standingLanes = 8

// Standing is where the agents stand in every repository on this machine
// and, with machines, on every machine added: each is asked for its own,
// and one that does not answer is named. Nothing is written.
func (a *App) Standing(machines bool) (Standing, error) {
	repos, err := Repositories()
	if err != nil {
		return Standing{}, err
	}
	var others []runtime.Machine
	if machines {
		others, _ = a.Runtimes.All()
	}
	theirs := make([]Standing, len(others))
	asked := make([]Answered, len(others))
	var wg sync.WaitGroup
	for i, machine := range others {
		wg.Go(func() { theirs[i], asked[i] = a.standingOn(machine) })
	}
	panes := session.Here().AllPanes()
	found := make([][]stood, len(repos))
	lanes := make(chan struct{}, standingLanes)
	for i, repo := range repos {
		wg.Go(func() {
			lanes <- struct{}{}
			defer func() { <-lanes }()
			found[i] = a.standingIn(repo, panes)
		})
	}
	wg.Wait()

	all := Standing{Waiting: []Stand{}, Working: []Stand{}, DoneUnread: []Stand{}, Machines: []Answered{{Name: runtime.Local, Answers: true}}}
	for _, one := range slices.Concat(found...) {
		switch one.group {
		case groupWaiting:
			all.Waiting = append(all.Waiting, one.Stand)
		case groupWorking:
			all.Working = append(all.Working, one.Stand)
		case groupDone:
			all.DoneUnread = append(all.DoneUnread, one.Stand)
		}
	}
	for i, machine := range others {
		all.Machines = append(all.Machines, asked[i])
		for _, group := range []struct{ to, from *[]Stand }{{&all.Waiting, &theirs[i].Waiting}, {&all.Working, &theirs[i].Working}, {&all.DoneUnread, &theirs[i].DoneUnread}} {
			for _, one := range *group.from {
				one.Machine = machine.Name
				*group.to = append(*group.to, one)
			}
		}
	}
	// the longest wait first, as it is the one most owed an answer
	for _, group := range []*[]Stand{&all.Waiting, &all.Working, &all.DoneUnread} {
		slices.SortStableFunc(*group, func(x, y Stand) int { return x.Since.Compare(y.Since) })
	}
	all.Counts.Waiting, all.Counts.Working, all.Counts.DoneUnread = len(all.Waiting), len(all.Working), len(all.DoneUnread)
	return all, nil
}

const (
	groupWaiting = iota + 1
	groupWorking
	groupDone
)

type stood struct {
	Stand
	group int
}

// standingIn is one repository's worktrees that stand somewhere, each by
// its best signal. A signal whose setter is gone is passed over by the
// rule a listing passes over it by, and left for a listing to forget.
func (a *App) standingIn(repo Repository, panes map[string][]session.Pane) []stood {
	signals, err := signal.Store{Dir: filepath.Join(repo.Path, ".git", "mia")}.Load()
	if err != nil {
		return nil
	}
	from, running := panesByID(panes), setters(signals)
	in := map[string]string{} // a pane's id → its session
	for name, its := range panes {
		for _, pane := range its {
			in[pane.ID] = name
		}
	}
	table := a.agents()
	var found []stood
	seen := map[string]bool{}
	for _, one := range signals { // best first
		if seen[one.Worktree] || setterGone(one, from, running) {
			continue
		}
		pane, live := from[one.Pane]
		seen[one.Worktree] = true
		stand := stood{Stand: Stand{Repository: repo.Name, Root: repo.Path, Worktree: one.Worktree, Machine: runtime.Local,
			Kind: one.Kind, Line: one.Text, Yes: one.Yes, Since: one.At.UTC()}}
		if _, known := agentNamed(table, one.From); known {
			stand.Agent = one.From
		}
		if live {
			stand.Window = &WindowRef{Index: pane.Index, Name: pane.Window}
			if stand.Agent == "" {
				stand.Agent = agentIn(table, pane.Command, pane.Title)
			}
		}
		switch {
		case one.Calls() && one.UntilSeen:
			stand.group = groupDone
		case one.Calls():
			stand.group = groupWaiting
			if live && in[one.Pane] == session.Name(one.Worktree) {
				screen, _ := session.Here().Capture(one.Worktree, strconv.Itoa(pane.Index), false)
				stand.Options = optionsOn(screen)
			}
		case stand.Agent != "":
			stand.group = groupWorking
		default:
			continue // a note on its row
		}
		found = append(found, stand)
	}
	return found
}

// option is a numbered choice as an agent draws one: "❯ 1. Yes", "  2. No".
var option = regexp.MustCompile(`^[\s│|]*(?:[❯›>▶]\s*)?(\d+)[.)]\s+(\S.*?)[\s│|]*$`)

// optionsOn is the numbered choices a screen ends with: the last run of
// lines numbered from 1, whatever is drawn between them.
func optionsOn(screen string) []string {
	var options []string
	for _, line := range strings.Split(screen, "\n") {
		m := option.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch n, _ := strconv.Atoi(m[1]); {
		case n == 1:
			options = []string{m[2]}
		case n == len(options)+1:
			options = append(options, m[2])
		}
	}
	if len(options) < 2 {
		return nil // a list of one is prose that starts with a number
	}
	return options
}

// unreachableFor is how long a machine that did not answer is not asked
// again, so one that is off costs one ssh timeout in that time and not
// one each asking.
const unreachableFor = 30 * time.Second

// unreachable is why a machine did not answer, when it was asked and did
// not within unreachableFor.
func (a *App) unreachable(machine string) (string, bool) {
	data, err := os.ReadFile(a.answerPath(machine))
	if err != nil {
		return "", false
	}
	var last answer
	if json.Unmarshal(data, &last) != nil || last.Answers || time.Since(last.At) >= unreachableFor {
		return "", false
	}
	return last.Detail, true
}

// askMia runs a machine's own mia with args, from no folder, and gives what
// it printed: how one machine asks another what is a person's and no
// repository's, where agents stand and the main agent. before is put
// before the command, for its environment.
func askMia(machine runtime.Machine, before string, args ...string) (string, error) {
	return runtime.Run(machine.SSH, append([]string{"sh", "-c", utf8 + before + remoteMia + ` "$@"`, "sh"}, args...)...)
}

// standingOn asks a machine's own mia where its agents stand.
func (a *App) standingOn(machine runtime.Machine) (Standing, Answered) {
	asked := Answered{Name: machine.Name}
	if detail, off := a.unreachable(machine.Name); off {
		asked.Detail = detail
		return Standing{}, asked
	}
	out, err := askMia(machine, "", "api", "standing", "--here")
	var theirs Standing
	if err == nil {
		if err = json.Unmarshal([]byte(out), &theirs); err != nil {
			err = fmt.Errorf("its mia did not say where its agents stand: %w", err)
		}
	}
	if err != nil {
		asked.Detail = run.FirstLine(err.Error())
		a.keepAnswer(machine.Name, false, asked.Detail)
		return Standing{}, asked
	}
	a.keepAnswer(machine.Name, true, "")
	asked.Answers = true
	return theirs, asked
}

// SeeStanding marks a worktree seen, as switching to it does: what it
// finished is unread no more. which is <repository>/<worktree>, the
// repository by its name or its main checkout's path, and the worktree
// with @<machine> after it when it is another machine's.
func (a *App) SeeStanding(which string) error {
	which, on, _ := strings.Cut(which, "@")
	cut := strings.LastIndex(which, "/")
	if cut <= 0 || cut == len(which)-1 {
		return fmt.Errorf("--seen takes <repository>/<worktree>, not %q", which)
	}
	if on != "" && on != runtime.Local {
		machine, err := a.Runtimes.Get(on)
		if err != nil {
			return err
		}
		_, err = runtime.Run(machine.SSH, "sh", "-c", utf8+remoteMia+` api standing --here --seen "$1" >/dev/null`, "sh", which)
		return err
	}
	repos, err := Repositories()
	if err != nil {
		return err
	}
	for _, repo := range repos {
		if repo.Name != which[:cut] && repo.Path != which[:cut] {
			continue
		}
		in, err := Open(repo.Path)
		if err != nil {
			return err
		}
		return in.MarkSeen(which[cut+1:])
	}
	return fmt.Errorf("mia has no repository called %s here — `mia api repos` lists them", which[:cut])
}
