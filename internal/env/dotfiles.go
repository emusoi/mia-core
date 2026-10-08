package env

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
)

// Dotfiles are a person's own setup, from the user-wide config
// (~/.config/mia/config.toml), so they follow them to every machine and
// environment whatever repository they are in:
//
//	[dotfiles]
//	machine = ["~/.tmux.conf"]      # into the home of the machine an environment runs on, where tmux runs
//	env = ["~/.zshrc", "~/.oh-my-zsh"]  # into every environment's home
//	shell = "zsh"                   # what env shell and new session windows run
//	packages = ["zsh", "fzf"]       # installed in the environment when missing
//	setup = ["command -v nvim || ..."]  # run in the environment after the files land
type Dotfiles struct {
	Machine  []string `toml:"machine"`
	Env      []string `toml:"env"`
	Shell    string   `toml:"shell"`
	Packages []string `toml:"packages"`
	// Setup runs each command with sh -lc in the environment on every env up,
	// after packages and files, for what a package manager cannot give (a
	// newer release, a plugin restore). They should be quick when already done.
	Setup []string `toml:"setup"`
}

// shellCommand is the login shell a window inside the environment opens: the
// configured one when the environment has it, the image's otherwise.
func (m Manager) shellCommand() []string {
	if m.Dotfiles.Shell == "" {
		return loginShell
	}
	sh := m.Dotfiles.Shell
	return []string{"/bin/sh", "-lc", fmt.Sprintf("command -v %s >/dev/null && exec %s -l || exec ${SHELL:-/bin/bash} -l", sh, sh)}
}

// ApplyDotfiles puts the person's setup in place for a running environment:
// the box files on the machine it runs on (reloading tmux there, which may
// already be running), the packages and env files in the container.
func (m Manager) ApplyDotfiles(record model.Record, out *os.File) error {
	where, name, err := m.running(record)
	if err != nil {
		return err
	}
	if err := m.CopyMachineDotfiles(record, out); err != nil {
		fmt.Fprintf(out, "⚠ dotfiles: %v\n", err)
	}
	if len(m.Dotfiles.Packages) > 0 {
		missing := []string{}
		for _, pkg := range m.Dotfiles.Packages {
			// The package, not a command of its name: ripgrep installs rg.
			if _, err := where.Engine.Exec(name, "sh", "-c", "dpkg -s "+pkg+" >/dev/null 2>&1 || command -v "+pkg); err != nil {
				missing = append(missing, pkg)
			}
		}
		if len(missing) > 0 {
			fmt.Fprintf(out, "installing %s\n", strings.Join(missing, " "))
			install := "command -v apt-get >/dev/null && [ \"$(id -u)\" = 0 ] && DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends " + strings.Join(missing, " ") + " >/dev/null"
			if _, err := where.Engine.Exec(name, "sh", "-c", install); err != nil {
				fmt.Fprintf(out, "⚠ packages: %v\n", err)
			}
		}
	}
	// A path in the home goes to the same place in the environment's home; an
	// absolute path outside it goes to the same absolute path, which is what a
	// config that sources /opt/homebrew/share/... needs to work unchanged.
	var inHome []string
	for _, entry := range m.Dotfiles.Env {
		if !outsideHome(entry) {
			inHome = append(inHome, entry)
			continue
		}
		// Listed by the person themselves, so follow its links wherever they
		// go (Homebrew's share/ links into its Cellar).
		source, err := filepath.EvalSymlinks(entry)
		if err == nil {
			err = m.copyInto(where, name, source, path.Clean(entry))
		}
		if err != nil {
			fmt.Fprintf(out, "⚠ %s: %v\n", entry, err)
			continue
		}
		fmt.Fprintf(out, "carried %s\n", entry)
	}
	if len(inHome) > 0 {
		if err := m.CarryTools(record, Tools{Copy: inHome}, false, out); err != nil {
			return err
		}
	}
	for _, command := range m.Dotfiles.Setup {
		if _, err := where.Engine.Exec(name, "sh", "-lc", command); err != nil {
			fmt.Fprintf(out, "⚠ dotfiles setup %q: %v\n", command, err)
		}
	}
	return nil
}

// CopyMachineDotfiles refreshes only files that belong on the runtime machine.
// It does not start or modify the container.
func (m Manager) CopyMachineDotfiles(record model.Record, out *os.File) error {
	where, err := m.Where(record)
	if err != nil {
		return err
	}
	if !where.Remote() || len(m.Dotfiles.Machine) == 0 {
		return nil
	}
	if err := copyToMachine(where.Machine.SSH, m.Dotfiles.Machine); err != nil {
		return fmt.Errorf("%s: %w", where.Machine.Name, err)
	}
	fmt.Fprintf(out, "dotfiles on %s: %s\n", where.Machine.Name, strings.Join(m.Dotfiles.Machine, " "))
	return nil
}

func outsideHome(entry string) bool {
	if !filepath.IsAbs(entry) {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}
	rel, err := filepath.Rel(home, entry)
	return err != nil || strings.HasPrefix(rel, "..")
}

// copyToMachine copies files under this machine's home into the same places
// under the machine's home over ssh, and has a running tmux reread its config.
func copyToMachine(ssh string, entries []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	var rel []string
	for _, entry := range entries {
		r := relativeToHome(entry)
		if _, err := os.Stat(filepath.Join(home, r)); err == nil {
			rel = append(rel, path.Clean(r))
		}
	}
	if len(rel) == 0 {
		return nil
	}
	create := exec.Command("tar", append([]string{"-C", home, "-chf", "-"}, rel...)...)
	create.Env = append(os.Environ(), "COPYFILE_DISABLE=1")
	extract := exec.Command("ssh", "-o", "BatchMode=yes", ssh, "tar -C \"$HOME\" -xf - && { tmux source-file \"$HOME/.tmux.conf\" 2>/dev/null || true; }")
	pipe, err := create.StdoutPipe()
	if err != nil {
		return err
	}
	extract.Stdin = pipe
	if err := create.Start(); err != nil {
		return err
	}
	out, err := extract.CombinedOutput()
	if werr := create.Wait(); err == nil && werr != nil {
		err = werr
	}
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
