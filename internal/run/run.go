package run

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/emusoi/mia-core/internal/paths"
)

var trace = os.Getenv("MIA_TRACE") != ""

func SSHBinary() string {
	if path, err := exec.LookPath("ssh"); err == nil && !strings.Contains(path, " ") {
		return path
	}
	return "/usr/bin/ssh"
}

func SSHOptions() []string {
	control := paths.In("ssh", "%C")
	os.MkdirAll(filepath.Dir(control), 0o700)
	return []string{
		"-o", "BatchMode=yes",
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=5m",
		"-o", "ControlPath=" + control,
		"-o", "ServerAliveInterval=15",
		"-o", "ConnectTimeout=6",
	}
}

func ShellJoin(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

func Local(binary string) Tool { return Tool{Binary: binary} }

func On(target, binary string) Tool { return Tool{Binary: binary, SSH: target} }

func Shell(target string) *exec.Cmd {
	command := exec.Command(SSHBinary(), append(SSHOptions(), "-t", target)...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command
}

type Tool struct {
	Binary  string
	SSH     string
	Dir     string
	Label   string
	Env     []string
	Detail  func(string) string
	Tty     bool
	Verbose bool
}

type Failure struct {
	Args   []string
	Stderr string
	Err    error

	detail string
}

func (f *Failure) Error() string { return f.detail }

func (f *Failure) Unwrap() error { return f.Err }

func ExitCode(err error) int {
	var exit *exec.ExitError
	if err == nil {
		return 0
	}
	for err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			exit = e
			break
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		err = unwrapped.Unwrap()
	}
	if exit == nil {
		return 1
	}
	return exit.ExitCode()
}

func (t Tool) Command(args ...string) *exec.Cmd {
	var command *exec.Cmd
	if t.SSH == "" {
		command = exec.Command(t.Binary, args...)
		command.Dir = t.Dir
	} else {
		options := SSHOptions()
		if t.Tty && isTerminal(os.Stdin) {
			options = append(options, "-t")
		}
		options = append(options, t.SSH, "--", ShellJoin(append([]string{t.Binary}, args...)))
		command = exec.Command(SSHBinary(), options...)
	}
	if len(t.Env) > 0 {
		command.Env = append(os.Environ(), t.Env...)
	}
	return command
}

func (t Tool) fail(args []string, stderr string, err error) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		stderr = err.Error()
	}
	detail := stderr
	if t.Detail != nil {
		detail = t.Detail(stderr)
	}
	return &Failure{Args: args, Stderr: stderr, Err: err, detail: detail}
}

func (t Tool) tick(args []string, started time.Time) {
	if !trace && !t.Verbose {
		return
	}
	label := t.Label
	fmt.Fprintf(os.Stderr, "%-7s %6dms %-26s %s\n", t.Binary, time.Since(started).Milliseconds(), label, strings.Join(args, " "))
}

func (t Tool) Capture(args ...string) (string, error) {
	command := t.Command(args...)
	var stderr strings.Builder
	command.Stderr = &stderr
	started := time.Now()
	out, err := command.Output()
	t.tick(args, started)
	if err != nil {
		return "", t.fail(args, stderr.String(), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (t Tool) Combined(args ...string) (string, error) {
	started := time.Now()
	out, err := t.Command(args...).CombinedOutput()
	t.tick(args, started)
	if err != nil {
		return string(out), t.fail(args, string(out), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (t Tool) Status(args ...string) (int, string) {
	started := time.Now()
	out, err := t.Command(args...).CombinedOutput()
	t.tick(args, started)
	return ExitCode(err), string(out)
}

func (t Tool) Interactive(args ...string) error {
	command := t.Command(args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func (t Tool) Stream(to io.Writer, args ...string) error {
	command := t.Command(args...)
	command.Stdout, command.Stderr = to, to
	return command.Run()
}

func (t Tool) Detached(to io.Writer, args ...string) (*exec.Cmd, error) {
	command := t.Command(args...)
	command.Stdout, command.Stderr = to, to
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return command, command.Start()
}

func (t Tool) Pipe(args ...string) (io.WriteCloser, io.ReadCloser, *exec.Cmd, error) {
	command := t.Command(args...)
	in, err := command.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	out, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := command.Start(); err != nil {
		return nil, nil, nil, err
	}
	return in, out, command, nil
}

func (t Tool) Replace(drop []string, args ...string) error {
	path, err := exec.LookPath(t.Binary)
	if err != nil {
		return err
	}
	environment := os.Environ()
	if len(drop) > 0 {
		kept := environment[:0]
		for _, entry := range environment {
			name, _, _ := strings.Cut(entry, "=")
			if !contains(drop, name) {
				kept = append(kept, entry)
			}
		}
		environment = kept
	}
	return syscall.Exec(path, append([]string{t.Binary}, args...), environment)
}

func contains(list []string, want string) bool {
	for _, one := range list {
		if one == want {
			return true
		}
	}
	return false
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func FirstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}
