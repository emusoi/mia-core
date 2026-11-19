package dev

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/emusoi/mia-core/internal/run"
)

type Lent struct {
	Worktree string `json:"worktree"`
	Path     string `json:"path"`
	Stashed  bool   `json:"stashed"`
}

type Main struct {
	Path  string
	State string
}

const stashMessage = "mia dev: main's own work"

func (m Main) Current() (Lent, bool, error) {
	raw, err := os.ReadFile(m.State)
	if errors.Is(err, os.ErrNotExist) {
		return Lent{}, false, nil
	}
	if err != nil {
		return Lent{}, false, err
	}
	var current Lent
	if err := json.Unmarshal(raw, &current); err != nil {
		return Lent{}, false, fmt.Errorf("%s: %w", m.State, err)
	}
	return current, true, nil
}

func (m Main) Lend(name, path string) error {
	current, ok, err := m.Current()
	if err != nil {
		return err
	}
	if ok && current.Worktree != name {
		if _, err := m.Off(); err != nil {
			return err
		}
		ok = false
	}
	if !ok {
		current = Lent{Worktree: name, Path: path}
		dirty, err := git(m.Path, "status", "--porcelain")
		if err != nil {
			return err
		}
		if dirty != "" {
			if _, err := git(m.Path, "stash", "push", "--include-untracked", "-m", stashMessage); err != nil {
				return fmt.Errorf("could not put main's own work aside: %w", err)
			}
			current.Stashed = true
		}
		raw, err := json.Marshal(current)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(m.State), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(m.State, raw, 0o644); err != nil {
			return err
		}
	}
	_, err = Mirror(path, m.Path)
	return err
}

func (m Main) Off() (Lent, error) {
	current, ok, err := m.Current()
	if err != nil || !ok {
		return Lent{}, err
	}
	if _, err := git(m.Path, "reset", "-q", "--hard"); err != nil {
		return current, err
	}
	if _, err := git(m.Path, "clean", "-fdq"); err != nil {
		return current, err
	}
	if current.Stashed {
		ref, err := stashRef(m.Path)
		if err != nil {
			return current, err
		}
		if ref == "" {
			return current, fmt.Errorf("main's own work was put aside but its stash is gone — look in `git stash list`")
		}
		if _, err := git(m.Path, "stash", "pop", "-q", ref); err != nil {
			return current, fmt.Errorf("main's own work is still in %s: %w", ref, err)
		}
	}
	if err := os.Remove(m.State); err != nil && !errors.Is(err, os.ErrNotExist) {
		return current, err
	}
	return current, nil
}

func stashRef(dir string) (string, error) {
	list, err := git(dir, "stash", "list", "--format=%gd %s")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(list, "\n") {
		ref, subject, _ := strings.Cut(line, " ")
		if strings.HasSuffix(subject, stashMessage) {
			return ref, nil
		}
	}
	return "", nil
}

func Mirror(from, to string) (int, error) {
	want, err := files(from)
	if err != nil {
		return 0, err
	}
	have, err := files(to)
	if err != nil {
		return 0, err
	}
	changed := 0
	for rel := range want {
		did, err := copyIfDifferent(filepath.Join(from, rel), filepath.Join(to, rel))
		if err != nil {
			return changed, err
		}
		if did {
			changed++
		}
	}
	for rel := range have {
		if want[rel] {
			continue
		}
		target := filepath.Join(to, rel)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return changed, err
		}
		changed++
		removeEmptyParents(filepath.Dir(target), to)
	}
	return changed, nil
}

// Follower keeps main's files the worktree's as they change. Each look
// stats what it copied last, and copies only a file whose stat changed;
// the worktree is listed again, which walks it, only when a folder of it
// changed — a file came or went — or relistEvery has passed.
type Follower struct {
	from, to string
	files    map[string]stamp // each file of the worktree's, as last copied
	dirs     map[string]stamp // the folders they are in
	listed   time.Time
	looked   time.Time // when every file was last looked at
}

// relistEvery is how often the worktree is listed though no folder of it
// changed: a file can come into one that holds only ignored files.
const relistEvery = 30 * time.Second

// lookEvery is how often every file is looked at while no folder changed. A
// file saved by renaming another over it (most editors), added or removed
// changes its folder and is seen at the next step; one written in place
// changes none, and waits for this. Stat-ing a big worktree every second is
// most of what following costs.
const lookEvery = 5 * time.Second

// stamp is what a stat says that changes when a file is written, replaced
// or made another kind.
type stamp struct {
	size, mtime int64
	mode        os.FileMode
	ino         uint64
}

func stampOf(info os.FileInfo) stamp {
	s := stamp{size: info.Size(), mtime: info.ModTime().UnixNano(), mode: info.Mode()}
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		s.ino = uint64(sys.Ino)
	}
	return s
}

// Follow is a Follower from the worktree at from into main at to. Its
// first Step compares every file, as Mirror does.
func Follow(from, to string) *Follower { return &Follower{from: from, to: to} }

// Step brings main up to the worktree, and says how many files it changed.
func (f *Follower) Step(now time.Time) (int, error) {
	relist := f.files == nil || now.Sub(f.listed) >= relistEvery
	for dir, was := range f.dirs {
		if relist {
			break
		}
		info, err := os.Lstat(filepath.Join(f.from, dir))
		relist = err != nil || stampOf(info) != was
	}
	if !relist && now.Sub(f.looked) < lookEvery {
		return 0, nil
	}
	f.looked = now
	names := f.files
	if relist {
		listed, err := files(f.from)
		if err != nil {
			return 0, err
		}
		names, f.listed = map[string]stamp{}, now
		f.dirs = map[string]stamp{}
		for rel := range listed {
			names[rel] = stamp{}
			for dir := filepath.Dir(rel); ; dir = filepath.Dir(dir) {
				if _, ok := f.dirs[dir]; ok {
					break
				}
				if info, err := os.Lstat(filepath.Join(f.from, dir)); err == nil {
					f.dirs[dir] = stampOf(info)
				}
				if dir == "." {
					break
				}
			}
		}
	}
	changed := 0
	next := make(map[string]stamp, len(names))
	for rel := range names {
		info, err := os.Lstat(filepath.Join(f.from, rel))
		if err != nil {
			continue // gone: taken from main below
		}
		// stat before copying: a write after it is copied at the next look
		now := stampOf(info)
		if was, ok := f.files[rel]; !ok || was != now {
			did, err := copyIfDifferent(filepath.Join(f.from, rel), filepath.Join(f.to, rel))
			if err != nil {
				return changed, err
			}
			if did {
				changed++
			}
		}
		next[rel] = now
	}
	for rel := range f.files {
		if _, ok := next[rel]; ok {
			continue
		}
		if _, err := os.Lstat(filepath.Join(f.from, rel)); err == nil {
			continue // there still, and now ignored: main keeps it
		}
		target := filepath.Join(f.to, rel)
		if err := os.Remove(target); err == nil {
			changed++
			removeEmptyParents(filepath.Dir(target), f.to)
		} else if !os.IsNotExist(err) {
			return changed, err
		}
	}
	f.files = next
	return changed, nil
}

func files(dir string) (map[string]bool, error) {
	out, err := git(dir, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	found := map[string]bool{}
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(dir, rel)); err == nil {
			found[rel] = true
		}
	}
	return found, nil
}

func copyIfDifferent(source, target string) (bool, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return false, err
	}
	if existing, err := os.Lstat(target); err == nil && existing.IsDir() {
		if err := os.RemoveAll(target); err != nil {
			return false, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(source)
		if err != nil {
			return false, err
		}
		if current, err := os.Readlink(target); err == nil && current == link {
			return false, nil
		}
		os.Remove(target)
		return true, os.Symlink(link, target)
	}
	body, err := os.ReadFile(source)
	if err != nil {
		return false, err
	}
	if current, err := os.Lstat(target); err == nil && current.Mode().IsRegular() && current.Mode().Perm() == info.Mode().Perm() {
		if existing, err := os.ReadFile(target); err == nil && bytes.Equal(existing, body) {
			return false, nil
		}
	}
	if err := os.WriteFile(target, body, info.Mode().Perm()); err != nil {
		return false, err
	}
	return true, os.Chmod(target, info.Mode().Perm())
}

func removeEmptyParents(dir, root string) {
	for dir != root && os.Remove(dir) == nil {
		dir = filepath.Dir(dir)
	}
}

func git(dir string, args ...string) (string, error) {
	return run.Local("git").Capture(append([]string{"-C", dir}, args...)...)
}
