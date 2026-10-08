package dev

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
