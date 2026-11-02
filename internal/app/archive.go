package app

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
)

const archiveRefs = "refs/mia/archive/"

// Archived is a worktree put away: its name, branch, and when.
type Archived struct {
	Name   string    `json:"name"`
	Branch string    `json:"branch"`
	At     time.Time `json:"at"`
	Dirty  bool      `json:"dirty"` // it kept uncommitted work
}

// Archive puts a worktree away: everything in it, uncommitted and untracked
// work too, is saved as a commit under refs/mia/archive/<name>, and the
// worktree goes. Its branch stays. Unarchive brings it back as it was.
func (a *App) Archive(query string, stopRunning bool) (Archived, error) {
	removal, err := a.PlanRemoval(query)
	if err != nil {
		return Archived{}, err
	}
	record := removal.Record
	if git.Resolve(record.Path) == git.Resolve(a.Root) {
		return Archived{}, fmt.Errorf("the main checkout is not archived")
	}
	branch := git.CurrentBranch(record.Path)
	if branch == "" {
		return Archived{}, fmt.Errorf("%s is on no branch — archiving keeps one to come back to", record.Name)
	}
	if git.RevParse(a.Root, archiveRefs+record.Name) != "" {
		return Archived{}, fmt.Errorf("an archive named %s is already kept — `mia unarchive %s` first", record.Name, record.Name)
	}
	if removal.Session && !stopRunning {
		return Archived{}, fmt.Errorf("%s has a session running — `mia shell %s` to see what, or `mia archive --stop-running %s`", record.Name, record.Name, record.Name)
	}
	saved, err := snapshot(record.Path, fmt.Sprintf("mia archive %s\n\nBranch: %s\n", record.Name, branch))
	if err != nil {
		return Archived{}, err
	}
	if _, err := (run.Tool{Binary: "git", Dir: a.Root}).Capture("update-ref", archiveRefs+record.Name, saved); err != nil {
		return Archived{}, err
	}
	removal.Dirty = false // saved: removing loses nothing
	if err := a.Remove(removal, true, true); err != nil {
		return Archived{}, err
	}
	return Archived{Name: record.Name, Branch: branch, At: time.Now(), Dirty: saved != git.RevParse(a.Root, branch)}, nil
}

// snapshot is a commit of everything in the worktree that git does not
// ignore, on top of HEAD; HEAD itself when nothing differs. The worktree,
// its index and its stash are left as they are.
func snapshot(dir, message string) (string, error) {
	head := git.RevParse(dir, "HEAD")
	index, err := os.CreateTemp("", "mia-archive-index-*")
	if err != nil {
		return "", err
	}
	index.Close()
	os.Remove(index.Name()) // git makes it afresh
	defer os.Remove(index.Name())
	g := run.Tool{Binary: "git", Dir: dir, Env: []string{"GIT_INDEX_FILE=" + index.Name()}}
	if _, err := g.Capture("read-tree", "HEAD"); err != nil {
		return "", err
	}
	if _, err := g.Capture("add", "-A"); err != nil {
		return "", err
	}
	tree, err := g.Capture("write-tree")
	if err != nil {
		return "", err
	}
	if headTree := git.RevParse(dir, "HEAD^{tree}"); tree == headTree {
		return head, nil
	}
	return g.Capture("commit-tree", tree, "-p", head, "-m", message)
}

// Archives is every archive kept, newest first.
func (a *App) Archives() ([]Archived, error) {
	out, err := (run.Tool{Binary: "git", Dir: a.Root}).Capture("for-each-ref", "--sort=-creatordate",
		"--format=%(refname:lstrip=3)%09%(creatordate:unix)%09%(objectname)", archiveRefs)
	if err != nil || out == "" {
		return nil, err
	}
	var all []Archived
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			continue
		}
		var secs int64
		fmt.Sscan(f[1], &secs)
		branch, dirty := archivedBranch(a.Root, f[2])
		all = append(all, Archived{Name: f[0], Branch: branch, At: time.Unix(secs, 0), Dirty: dirty})
	}
	return all, nil
}

// archivedBranch is the branch an archive came from, and whether it saved
// work beyond the branch's commit: a commit of mia's names it in its
// message; a clean archive is the branch's commit itself.
func archivedBranch(root, sha string) (string, bool) {
	message, _ := (run.Tool{Binary: "git", Dir: root}).Capture("log", "-1", "--format=%B", sha)
	if strings.HasPrefix(message, "mia archive ") {
		for _, line := range strings.Split(message, "\n") {
			if b, ok := strings.CutPrefix(line, "Branch: "); ok {
				return strings.TrimSpace(b), true
			}
		}
	}
	names, _ := (run.Tool{Binary: "git", Dir: root}).Capture("branch", "--points-at", sha, "--format=%(refname:short)")
	branch, _, _ := strings.Cut(names, "\n")
	return branch, false
}

// Unarchive brings an archived worktree back: on its branch, under its old
// name when that is free, with the uncommitted and untracked work it had.
func (a *App) Unarchive(name string) (model.Record, error) {
	saved := git.RevParse(a.Root, archiveRefs+name)
	if saved == "" {
		return model.Record{}, fmt.Errorf("no archive named %s — `mia archive ls` lists them", name)
	}
	branch, dirty := archivedBranch(a.Root, saved)
	if branch == "" || !git.BranchExists(a.Root, branch) {
		return model.Record{}, fmt.Errorf("the branch %s was archived on is gone; its work is still at %s%s", branch, archiveRefs, name)
	}
	base := saved
	if dirty {
		base = git.RevParse(a.Root, saved+"^")
	}
	if tip := git.RevParse(a.Root, branch); tip != base {
		return model.Record{}, fmt.Errorf("%s has moved since %s was archived; its work is still at %s%s", branch, name, archiveRefs, name)
	}
	record, err := a.New(branch)
	if err != nil {
		return record, err
	}
	if dirty, _ := git.Dirty(record.Path); dirty {
		return record, fmt.Errorf("%s already has %s checked out with changes; its archived work is still at %s%s", record.Name, branch, archiveRefs, name)
	}
	if record.Name != name {
		if renamed, err := a.Rename(record.Name, name); err == nil {
			record = renamed
		}
	}
	if saved != base {
		if err := putBack(record.Path, saved); err != nil {
			return record, err
		}
	}
	_, err = (run.Tool{Binary: "git", Dir: a.Root}).Capture("update-ref", "-d", archiveRefs+name)
	return record, err
}

// putBack makes a clean worktree hold a snapshot's work, uncommitted: the
// files go from HEAD to the snapshot, then the index back to HEAD.
func putBack(dir, saved string) error {
	g := run.Tool{Binary: "git", Dir: dir}
	if _, err := g.Capture("read-tree", "-m", "-u", "HEAD", saved); err != nil {
		return err
	}
	_, err := g.Capture("reset", "-q")
	return err
}

// Fork makes a worktree on a new branch starting where another worktree
// is, with a copy of its uncommitted and untracked work: a second try that
// leaves the first as it is.
func (a *App) Fork(query, branch string) (model.Record, error) {
	from, err := a.RecordOrHere(query)
	if err != nil {
		return model.Record{}, err
	}
	if a.Config.Prefix != "" && !strings.HasPrefix(branch, a.Config.Prefix) {
		branch = a.Config.Prefix + branch
	}
	if git.BranchExists(a.Root, branch) {
		return model.Record{}, fmt.Errorf("%s already exists — a fork starts a new branch", branch)
	}
	head := git.RevParse(from.Path, "HEAD")
	saved, err := snapshot(from.Path, "mia fork of "+from.Name)
	if err != nil {
		return model.Record{}, err
	}
	before, _ := a.Store.Load()
	record, err := a.newWorktree(branch, head)
	if record.Path != "" && !slices.ContainsFunc(before, func(one model.Record) bool { return one.Path == record.Path }) {
		a.Emit("worktree.created", record, nil)
	}
	if err != nil {
		return record, err
	}
	if saved != head {
		return record, putBack(record.Path, saved)
	}
	return record, nil
}
