package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
)

type Repo struct {
	T    *testing.T
	Root string
}

func New(t *testing.T) *Repo {
	t.Helper()
	root := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := &Repo{T: t, Root: root}
	repo.Git("init", "-q", "-b", "main")
	repo.Git("config", "user.email", "test@example.com")
	repo.Git("config", "user.name", "test")
	repo.Write("README.md", "fixture\n")
	repo.Git("add", "-A")
	repo.Git("commit", "-qm", "first")
	repo.Root = git.Resolve(root)
	return repo
}

func (r *Repo) Git(args ...string) string {
	r.T.Helper()
	return r.GitIn(r.Root, args...)
}

func (r *Repo) GitIn(dir string, args ...string) string {
	r.T.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	if err != nil {
		r.T.Fatalf("git %s in %s: %s", strings.Join(args, " "), dir, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *Repo) Write(name, content string) {
	r.T.Helper()
	path := filepath.Join(r.Root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.T.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.T.Fatal(err)
	}
}

func (r *Repo) Commit(message string) {
	r.T.Helper()
	r.Git("add", "-A")
	r.Git("commit", "-qm", message)
}

func (r *Repo) Worktree(suffix, branch string) string {
	r.T.Helper()
	path := r.Root + "." + suffix
	r.Git("worktree", "add", "-q", "-b", branch, path)
	return git.Resolve(path)
}

func (r *Repo) Detached(suffix string) string {
	r.T.Helper()
	path := r.Root + "." + suffix
	r.Git("worktree", "add", "-q", "--detach", path)
	return git.Resolve(path)
}

func (r *Repo) MiaDir() string {
	r.T.Helper()
	common, err := git.CommonDir(r.Root)
	if err != nil {
		r.T.Fatal(err)
	}
	return filepath.Join(common, "mia")
}

func Isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}
