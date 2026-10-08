package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	exe "github.com/emusoi/mia-core/internal/run"
)

var trace = os.Getenv("MIA_TRACE") != "" || os.Getenv("MIA_TRACE_GIT") != ""

func gitIn(dir string) exe.Tool {
	return exe.Tool{Binary: "git", Dir: dir, Label: filepath.Base(dir), Verbose: trace}
}

func run(dir string, args ...string) (string, error) {
	out, err := gitIn(dir).Capture(args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), err)
	}
	return out, nil
}

func Toplevel(dir string) (string, error) {
	top, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return Resolve(top), nil
}

func Resolve(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func CommonDir(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return Resolve(out), nil
}

func CurrentBranch(dir string) string {
	branch, err := run(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return branch
}

type Worktree struct {
	Path   string
	Branch string
	Head   string
	Main   bool
}

func Worktrees(dir string) ([]Worktree, error) {
	out, err := run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var (
		list    []Worktree
		current Worktree
		open    bool
	)
	flush := func() {
		if open {
			current.Main = len(list) == 0
			list = append(list, current)
		}
		current, open = Worktree{}, false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			flush()
			current.Path = Resolve(strings.TrimPrefix(line, "worktree "))
			open = true
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
	return list, nil
}

func HeadsAt(common string) map[string]string {
	heads := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(common, "packed-refs")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if sha, ref, ok := strings.Cut(line, " "); ok && strings.HasPrefix(ref, "refs/heads/") {
				heads[strings.TrimPrefix(ref, "refs/heads/")] = sha
			}
		}
	}
	loose := filepath.Join(common, "refs", "heads")
	filepath.WalkDir(loose, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if data, err := os.ReadFile(path); err == nil {
			if rel, err := filepath.Rel(loose, path); err == nil {
				heads[filepath.ToSlash(rel)] = strings.TrimSpace(string(data))
			}
		}
		return nil
	})
	return heads
}

func WorktreesAt(root, common string, heads map[string]string) ([]Worktree, error) {
	read := func(gitdir string) (Worktree, bool) {
		data, err := os.ReadFile(filepath.Join(gitdir, "HEAD"))
		if err != nil {
			return Worktree{}, false
		}
		head := strings.TrimSpace(string(data))
		tree := Worktree{}
		if ref, ok := strings.CutPrefix(head, "ref: refs/heads/"); ok {
			tree.Branch = ref
			tree.Head = heads[ref]
		} else {
			tree.Head = head
		}
		return tree, true
	}
	main, ok := read(common)
	if !ok {
		return nil, fmt.Errorf("no HEAD in %s", common)
	}
	main.Path, main.Main = Resolve(root), true
	list := []Worktree{main}
	entries, _ := os.ReadDir(filepath.Join(common, "worktrees"))
	for _, entry := range entries {
		gitdir := filepath.Join(common, "worktrees", entry.Name())
		data, err := os.ReadFile(filepath.Join(gitdir, "gitdir"))
		if err != nil {
			continue
		}
		path := filepath.Dir(strings.TrimSpace(string(data)))
		if _, err := os.Stat(path); err != nil {
			continue
		}
		tree, ok := read(gitdir)
		if !ok {
			continue
		}
		tree.Path = Resolve(path)
		list = append(list, tree)
	}
	return list, nil
}

func AddFrom(dir, path, branch string, create bool, from string) error {
	_, _ = run(dir, "worktree", "prune")
	args := []string{"worktree", "add"}
	switch {
	case branch == "":
		args = append(args, "--detach", path)
	case create && from != "":
		args = append(args, "-b", branch, path, from)
	case create:
		args = append(args, "-b", branch, path)
	default:
		args = append(args, path, branch)
	}
	_, err := run(dir, args...)
	return err
}

func Remove(dir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := run(dir, append(args, path)...)
	return err
}

func Dirty(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func DirtyTracked(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain", "-uno")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func ValidBranch(dir, branch string) bool {
	_, err := run(dir, "check-ref-format", "--branch", branch)
	return err == nil
}

func BranchExists(dir, branch string) bool {
	_, err := run(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

var ErrNotARepository = errors.New("not inside a git repository")

func Repository(dir string) (top, common string, err error) {
	if top, common, ok := repositoryFromFiles(dir); ok {
		return top, common, nil
	}
	top, err = Toplevel(dir)
	if err != nil {
		return "", "", ErrNotARepository
	}
	common, err = CommonDir(dir)
	if err != nil {
		return "", "", err
	}
	return top, common, nil
}

func repositoryFromFiles(dir string) (top, common string, ok bool) {
	dir = Resolve(dir)
	for {
		dotgit := filepath.Join(dir, ".git")
		info, err := os.Stat(dotgit)
		if err == nil {
			if info.IsDir() {
				return dir, dotgit, true
			}
			data, err := os.ReadFile(dotgit)
			if err != nil {
				return "", "", false
			}
			gitdir, found := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
			if !found {
				return "", "", false
			}
			if !filepath.IsAbs(gitdir) {
				gitdir = filepath.Join(dir, gitdir)
			}
			rel, err := os.ReadFile(filepath.Join(gitdir, "commondir"))
			if err != nil {
				return "", "", false
			}
			return dir, filepath.Clean(filepath.Join(gitdir, strings.TrimSpace(string(rel)))), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

func Head(dir string) string {
	out, err := run(dir, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func SubjectsSince(dir, span string) []string {
	if !strings.Contains(span, "..") {
		span += "..HEAD"
	}
	out, err := run(dir, "log", "--format=%s", span)
	if err != nil {
		return nil
	}
	var subjects []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			subjects = append(subjects, line)
		}
	}
	return subjects
}

func DefaultBranch(dir string) string {
	if out, err := run(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if name := strings.TrimSpace(out); name != "" {
			return name
		}
	}
	for _, candidate := range []string{"main", "master"} {
		if _, err := run(dir, "rev-parse", "--verify", "--quiet", candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func IsAncestor(dir, ancestor, descendant string) bool {
	_, err := run(dir, "merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}

func Heads(dir string) map[string]string {
	out, err := run(dir, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads")
	heads := map[string]string{}
	if err != nil {
		return heads
	}
	for _, line := range strings.Split(out, "\n") {
		if name, sha, ok := strings.Cut(line, " "); ok {
			heads[name] = sha
		}
	}
	return heads
}

func MergedInto(dir, ref string) map[string]bool {
	out, err := run(dir, "branch", "--format=%(refname:short)", "--merged", ref)
	merged := map[string]bool{}
	if err != nil {
		return merged
	}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			merged[line] = true
		}
	}
	return merged
}

func BehindAhead(dir, base, head string) (behind, ahead int) {
	out, err := run(dir, "rev-list", "--left-right", "--count", base+"..."+head)
	if err != nil {
		return 0, 0
	}
	fmt.Sscanf(out, "%d %d", &behind, &ahead)
	return behind, ahead
}

func HasRemote(dir, name string) bool {
	out, err := run(dir, "remote")
	return err == nil && slices.Contains(strings.Fields(out), name)
}

func FetchPR(dir string, number int, branch string) error {
	out, err := run(dir, "remote")
	if err != nil {
		return err
	}
	remotes := strings.Fields(out)
	if len(remotes) == 0 {
		return fmt.Errorf("this repository has no remote to fetch pull request %d from", number)
	}
	remote := remotes[0]
	if slices.Contains(remotes, "origin") {
		remote = "origin"
	}
	_, err = run(dir, "fetch", remote, fmt.Sprintf("pull/%d/head:refs/heads/%s", number, branch))
	return err
}

type FileStat struct {
	Path    string
	Added   int
	Deleted int
}

func ChangedFiles(dir, base, head string) []FileStat {
	out, err := run(dir, "diff", "--numstat", base+"..."+head)
	if err != nil {
		return nil
	}
	var files []FileStat
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		file := FileStat{Path: fields[2]}
		fmt.Sscanf(fields[0], "%d", &file.Added)
		fmt.Sscanf(fields[1], "%d", &file.Deleted)
		files = append(files, file)
	}
	return files
}

func DiffStat(dir, base, head string) (files, added, deleted int) {
	for _, file := range ChangedFiles(dir, base, head) {
		files++
		added += file.Added
		deleted += file.Deleted
	}
	return files, added, deleted
}

type Commit struct {
	SHA     string
	Subject string
}

func HasBranch(dir, branch string) bool {
	_, err := run(dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func RevParse(dir, ref string) string {
	out, err := run(dir, "rev-parse", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func Checkout(dir, branch string) error {
	out, err := runCombined(dir, "checkout", branch)
	if err != nil {
		return fmt.Errorf("move to %s: %s", branch, firstLine(strings.TrimSpace(out)))
	}
	return nil
}

func BranchAt(dir, branch, from string) error {
	out, err := runCombined(dir, "branch", branch, from)
	if err != nil {
		return fmt.Errorf("create %s: %s", branch, firstLine(strings.TrimSpace(out)))
	}
	return nil
}

func CreateBranch(dir, branch string) error {
	out, err := runCombined(dir, "checkout", "-b", branch)
	if err != nil {
		return fmt.Errorf("create %s: %s", branch, firstLine(strings.TrimSpace(out)))
	}
	return nil
}

func Rebase(dir, branch, onto, oldBase string) error {
	if err := Checkout(dir, branch); err != nil {
		return err
	}
	out, err := runCombined(dir, "rebase", "--onto", onto, oldBase, branch)
	if err != nil {
		return fmt.Errorf("restack %s onto %s: %s", branch, onto, firstLine(strings.TrimSpace(out)))
	}
	return nil
}

func FastForward(dir, to string) error {
	out, err := runCombined(dir, "merge", "--ff-only", to)
	if err != nil {
		return fmt.Errorf("fast-forward to %s: %s", to, firstLine(strings.TrimSpace(out)))
	}
	return nil
}

func DeleteBranch(dir, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	out, err := runCombined(dir, "branch", flag, branch)
	if err != nil {
		return fmt.Errorf("delete %s: %s", branch, firstLine(strings.TrimSpace(out)))
	}
	return nil
}

func RebaseInProgress(dir string) bool {
	common, err := run(dir, "rev-parse", "--git-path", "rebase-merge")
	if err != nil {
		return false
	}
	if _, statErr := os.Stat(strings.TrimSpace(common)); statErr == nil {
		return true
	}
	apply, err := run(dir, "rev-parse", "--git-path", "rebase-apply")
	if err != nil {
		return false
	}
	_, statErr := os.Stat(strings.TrimSpace(apply))
	return statErr == nil
}

func runCombined(dir string, args ...string) (string, error) {
	out, err := gitIn(dir).Combined(args...)
	return out, err
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}

func ForkPoint(dir, parent, branch string) string {
	if out, err := run(dir, "merge-base", "--fork-point", parent, branch); err == nil {
		return strings.TrimSpace(out)
	}
	return MergeBase(dir, parent, branch)
}

func MergeBase(dir, a, b string) string {
	out, err := run(dir, "merge-base", a, b)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func UpdateRef(dir, ref, sha string) error {
	_, err := runCombined(dir, "update-ref", ref, sha)
	return err
}

func LastCommit(dir string) (time.Time, string) {
	out, err := run(dir, "log", "-1", "--format=%ct%x00%s", "HEAD")
	if err != nil {
		return time.Time{}, ""
	}
	when, subject, _ := strings.Cut(strings.TrimSpace(out), "\x00")
	secs, err := strconv.ParseInt(when, 10, 64)
	if err != nil || secs == 0 {
		return time.Time{}, subject
	}
	return time.Unix(secs, 0), subject
}

func LastCommitTime(dir string) time.Time {
	out, err := run(dir, "log", "-1", "--format=%ct", "HEAD")
	if err != nil {
		return time.Time{}
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(secs, 0)
}

// CommitTimes maps each tracked file to the time of the last commit that touched it.
func CommitTimes(dir string) (map[string]time.Time, error) {
	listed, err := run(dir, "-c", "core.quotepath=off", "ls-files")
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, name := range strings.Split(listed, "\n") {
		if name != "" {
			want[name] = true
		}
	}
	// Newest first: the first time a path appears is its last change.
	history, err := run(dir, "-c", "core.quotepath=off", "log", "--format=@%ct", "--name-only", "--no-renames", "HEAD")
	if err != nil {
		return nil, err
	}
	times := make(map[string]time.Time, len(want))
	var at time.Time
	for _, line := range strings.Split(history, "\n") {
		if len(times) == len(want) {
			break
		}
		if stamp, ok := strings.CutPrefix(line, "@"); ok {
			secs, err := strconv.ParseInt(stamp, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("git log gave a time %q: %w", stamp, err)
			}
			at = time.Unix(secs, 0)
			continue
		}
		if want[line] {
			if _, seen := times[line]; !seen {
				times[line] = at
			}
		}
	}
	return times, nil
}
