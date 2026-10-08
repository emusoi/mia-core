package mirror

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/emusoi/mia-core/internal/run"
	"strings"
)

type Mirror struct{ Binary string }

func (m Mirror) tool() run.Tool {
	return run.Tool{Binary: m.Binary, Detail: run.FirstLine}
}

func Find() (Mirror, error) {
	path, err := exec.LookPath("mutagen")
	if err != nil {
		return Mirror{}, fmt.Errorf("an environment on another machine needs mutagen to keep the tree in step: https://mutagen.io/documentation/introduction/installation")
	}
	return Mirror{Binary: path}, nil
}

var Ignores = []string{
	".git",
	"node_modules",
	".venv", "venv", ".tox",
	"__pycache__", "*.pyc",
	".pytest_cache", ".mypy_cache", ".ruff_cache",
	"target", "dist", "build",
	".next/cache", ".turbo", ".parcel-cache",
	".yarn/cache", ".yarn/unplugged", ".yarn/install-state.gz", ".pnpm-store",
	".gradle", ".m2",
	".DS_Store",
}

type Session struct {
	Name      string
	Alpha     string
	Beta      string
	Status    string
	Paused    bool
	Problems  []string
	Conflicts int
}

func (s Session) Healthy() bool {
	return !s.Paused && len(s.Problems) == 0 && strings.Contains(s.Status, "watching")
}

func (s Session) Describe() string {
	switch {
	case s.Paused:
		return "paused"
	case len(s.Problems) > 0:
		return "STALLED · " + strings.Join(s.Problems, "; ")
	case s.Conflicts > 0:
		return fmt.Sprintf("%s · %d conflict(s) — `mutagen sync list %s`", s.Status, s.Conflicts, s.Name)
	default:
		return s.Status
	}
}

func SessionName(worktree string) string { return "mia-" + worktree }

// Ensure has a session between local and machine:remote. An existing one for
// the same two ends is kept, paused or not: a manual worktree's session is
// paused on purpose, and only `mia env sync` changes that.
func (m Mirror) Ensure(worktree, local, machine, remote string) error {
	name := SessionName(worktree)
	beta := machine + ":" + remote

	if existing, found, err := m.Status(worktree); err == nil && found {
		if existing.Alpha == local && existing.Beta == beta {
			return nil
		}
		if err := m.Terminate(worktree); err != nil {
			return err
		}
	}

	// Mutagen creates files 0600 and directories 0700 on the side it writes
	// to, so a box's copy of a checkout was private where git's is 0644/0755.
	// That broke anything that hashes a folder: yarn's hash of a `file:`
	// dependency changed on the box, and `yarn install --immutable` refused
	// the lockfile. Give new files the modes a checkout has.
	args := []string{"sync", "create", "--name=" + name, "--sync-mode=two-way-safe", "--no-global-configuration",
		"--default-file-mode=0644", "--default-directory-mode=0755"}
	for _, ignore := range Ignores {
		args = append(args, "--ignore="+ignore)
	}
	args = append(args, local, beta)
	if _, err := m.tool().Combined(args...); err != nil {
		return fmt.Errorf("start the mirror for %s: %s", worktree, err)
	}
	return nil
}

func (m Mirror) Seed(worktree string) error {
	if _, err := m.tool().Combined("sync", "flush", SessionName(worktree)); err != nil {
		return fmt.Errorf("seed the mirror for %s: %s", worktree, err)
	}
	return nil
}

// Pause stops a session from following changes; each side keeps its own.
func (m Mirror) Pause(worktree string) error {
	if _, err := m.tool().Combined("sync", "pause", SessionName(worktree)); err != nil {
		return fmt.Errorf("pause the mirror for %s: %s", worktree, err)
	}
	return nil
}

// Resume makes a paused session follow changes again, both ways.
func (m Mirror) Resume(worktree string) error {
	if _, err := m.tool().Combined("sync", "resume", SessionName(worktree)); err != nil {
		return fmt.Errorf("resume the mirror for %s: %s", worktree, err)
	}
	return nil
}

func (m Mirror) Terminate(worktree string) error {
	out, err := m.tool().Combined("sync", "terminate", SessionName(worktree))
	if err != nil && !strings.Contains(out, "unable to select") {
		return fmt.Errorf("stop the mirror for %s: %s", worktree, err)
	}
	return nil
}

func (m Mirror) Status(worktree string) (Session, bool, error) {
	out, err := m.tool().Capture("sync", "list", "--template", "{{json .}}", SessionName(worktree))
	if err != nil {
		return Session{}, false, nil
	}
	return parse([]byte(out), SessionName(worktree))
}

type endpoint struct {
	Protocol     string `json:"protocol"`
	Host         string `json:"host"`
	Path         string `json:"path"`
	Connected    bool   `json:"connected"`
	ScanProblems []struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	} `json:"scanProblems"`
	ExcludedScanProblems int `json:"excludedScanProblems"`
}

func (e endpoint) address() string {
	if e.Protocol == "ssh" {
		return e.Host + ":" + e.Path
	}
	return e.Path
}

func parse(data []byte, name string) (Session, bool, error) {
	var listed []struct {
		Name      string     `json:"name"`
		Status    string     `json:"status"`
		Paused    bool       `json:"paused"`
		Alpha     endpoint   `json:"alpha"`
		Beta      endpoint   `json:"beta"`
		Conflicts []struct{} `json:"conflicts"`
	}
	if err := json.Unmarshal(data, &listed); err != nil {
		return Session{}, false, fmt.Errorf("read the mirror's state: %w", err)
	}
	for _, entry := range listed {
		if entry.Name != name {
			continue
		}
		session := Session{
			Name:      entry.Name,
			Alpha:     entry.Alpha.address(),
			Beta:      entry.Beta.address(),
			Status:    entry.Status,
			Paused:    entry.Paused,
			Conflicts: len(entry.Conflicts),
		}
		for _, side := range []struct {
			where string
			end   endpoint
		}{{"here", entry.Alpha}, {"there", entry.Beta}} {
			if !side.end.Connected {
				session.Problems = append(session.Problems, side.where+" is not connected")
			}
			for _, problem := range side.end.ScanProblems {
				session.Problems = append(session.Problems,
					fmt.Sprintf("%s: %s (%s)", side.where, problem.Path, problem.Error))
			}
			if side.end.ExcludedScanProblems > 0 {
				session.Problems = append(session.Problems,
					fmt.Sprintf("%s: %d more problems not listed", side.where, side.end.ExcludedScanProblems))
			}
		}
		return session, true, nil
	}
	return Session{}, false, nil
}
