package env

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
)

type Tools struct {
	Copy []string `toml:"copy"`

	Credentials []string `toml:"credentials"`

	// CarryCredentials places the credential files on every `mia env up`, as
	// `mia env tools --credentials` does. Off unless a repository asks for it:
	// those files can act as you, wherever the environment runs.
	CarryCredentials bool `toml:"carry_credentials"`
}

const marker = ".placed-by-mia"

func (m Manager) CarryTools(record model.Record, tools Tools, withCredentials bool, out *os.File) error {
	where, name, err := m.running(record)
	if err != nil {
		return err
	}
	home, err := where.Engine.Exec(name, "sh", "-c", "echo $HOME")
	if err != nil {
		return err
	}
	home = strings.TrimSpace(home)

	for _, entry := range tools.Copy {
		source, err := resolveOneHop(entry)
		if err != nil {
			fmt.Fprintf(out, "⚠ %v\n", err)
			continue
		}
		target := path.Join(home, relativeToHome(entry))
		if err := m.copyInto(where, name, source, target); err != nil {
			fmt.Fprintf(out, "⚠ %s: %v\n", entry, err)
			continue
		}
		fmt.Fprintf(out, "carried %s\n", entry)
	}

	if !withCredentials {
		if len(tools.Credentials) > 0 {
			fmt.Fprintf(out, "%d credential(s) left behind — `--credentials` moves them\n", len(tools.Credentials))
		}
		return nil
	}
	fmt.Fprintf(out, "these files may hold tokens for services other than the tool — anything in them can act as you:\n")
	for _, entry := range tools.Credentials {
		fmt.Fprintf(out, "  %s\n", entry)
	}

	for _, entry := range tools.Credentials {
		if err := m.placeCredential(where, name, home, entry); err != nil {
			fmt.Fprintf(out, "⚠ %s: %v\n", entry, err)
			continue
		}
		fmt.Fprintf(out, "placed %s (0600)\n", entry)
	}
	return nil
}

func (m Manager) copyInto(where Location, container, source, target string) error {
	if err := where.Engine.Exec2(container, "mkdir", "-p", path.Dir(target)); err != nil {
		return err
	}
	create := run.Local("tar").Command("-C", filepath.Dir(source), "-c", "-h", "-f", "-", filepath.Base(source))
	create.Env = append(os.Environ(), "COPYFILE_DISABLE=1")
	pipe, err := create.StdoutPipe()
	if err != nil {
		return err
	}
	if err := create.Start(); err != nil {
		return err
	}
	defer create.Wait()
	return where.Engine.ExecStdin(container, path.Dir(target), []string{"tar", "-x", "-f", "-"}, pipe)
}

func (m Manager) placeCredential(where Location, container, home, entry string) error {
	source, err := resolveOneHop(entry)
	if err != nil {
		return err
	}
	target := path.Join(home, relativeToHome(entry))

	present := where.Engine.Exec2(container, "test", "-e", target) == nil
	ours := where.Engine.Exec2(container, "test", "-e", target+marker) == nil
	if present && !ours {
		return fmt.Errorf("there is already a login there that mia did not place — remove it inside the environment first, or leave it alone")
	}

	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := where.Engine.Exec2(container, "mkdir", "-p", path.Dir(target)); err != nil {
		return err
	}
	if err := where.Engine.ExecStdin(container, path.Dir(target),
		[]string{"sh", "-c", "umask 077 && cat > " + shellJoin([]string{target})}, strings.NewReader(string(data))); err != nil {
		return err
	}
	if err := where.Engine.Exec2(container, "chmod", "600", target); err != nil {
		return err
	}
	return where.Engine.Exec2(container, "sh", "-c", "date > "+shellJoin([]string{target + marker}))
}

func ResolveOneHop(entry string) (string, error) { return resolveOneHop(entry) }

func resolveOneHop(entry string) (string, error) {
	expanded, err := expandHome(entry)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(expanded)
	if err != nil {
		return "", fmt.Errorf("%s is not there", entry)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return expanded, nil
	}
	target, err := os.Readlink(expanded)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(expanded), target)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(target, home) {
		return "", fmt.Errorf("%s points outside your home directory, at %s — mia will not carry that", entry, target)
	}
	return target, nil
}

func expandHome(entry string) (string, error) {
	if !strings.HasPrefix(entry, "~") {
		return entry, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(entry, "~")), nil
}

func relativeToHome(entry string) string {
	if strings.HasPrefix(entry, "~/") {
		return strings.TrimPrefix(entry, "~/")
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, entry); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return filepath.Base(entry)
}

func runHere(dir string, argv []string) (string, error) {
	out, err := run.Tool{Binary: argv[0], Dir: dir}.Combined(argv[1:]...)
	return out, err
}

func exitOf(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return 127
}
