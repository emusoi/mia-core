package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
)

type Host struct {
	SSH   string
	Shell string
}

func Here() Host { return Host{} }

func On(machine runtime.Machine) Host { return Host{SSH: machine.SSH} }

func Name(worktree string) string { return "mia-" + worktree }

func exact(name string) string { return "=" + name }

const (
	GeometryWidth  = 200
	GeometryHeight = 50
)

var trace = os.Getenv("MIA_TRACE") != ""

func (h Host) run(args ...string) (string, error) {
	if h.SSH == "" {
		return h.local(args...)
	}
	out, err := run.Tool{Binary: "tmux", SSH: h.SSH}.Combined(args...)
	return out, err
}

func (h Host) Exists(worktree string) bool {
	_, err := h.run("has-session", "-t", exact(Name(worktree)))
	return err == nil
}

func (h Host) Ensure(worktree, dir string) error {
	shell, err := h.shellPath()
	if err != nil {
		return err
	}
	if !h.Exists(worktree) {
		args := []string{"new-session", "-d", "-s", Name(worktree), "-c", dir,
			"-x", strconv.Itoa(GeometryWidth), "-y", strconv.Itoa(GeometryHeight)}
		if shell != "" {
			args = append(args, run.ShellJoin([]string{shell, "-l"}))
		}
		if out, err := h.run(args...); err != nil {
			return fmt.Errorf("start session for %s: %s", worktree, strings.TrimSpace(out))
		}
	}
	if shell != "" {
		if out, err := h.run("set-option", "-t", Target(worktree, ""), "default-shell", shell); err != nil {
			return fmt.Errorf("set shell for %s: %s", worktree, strings.TrimSpace(out))
		}
	}
	_ = h.Style(worktree)
	return nil
}

func (h Host) shellPath() (string, error) {
	if h.Shell == "" {
		return "", nil
	}
	path, err := (run.Tool{Binary: "sh", SSH: h.SSH}).Capture("-c", "command -v "+run.ShellJoin([]string{h.Shell}))
	if err != nil {
		return "", fmt.Errorf("find %s on the session host: %w", h.Shell, err)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("shell %s resolved to %q on the session host", h.Shell, path)
	}
	return path, nil
}

func (h Host) Attach(worktree string) error {
	name := Name(worktree)
	if h.SSH != "" {
		return runtime.Interactive(h.SSH, "tmux", "-u", "attach-session", "-t", exact(name)).Run()
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found — sessions need it: https://github.com/tmux/tmux/wiki/Installing")
	}

	if !isTerminal(os.Stdout) {
		return fmt.Errorf("there is no terminal here to attach %s to — `tmux attach -t %s` from one", worktree, name)
	}

	if tty := clientOfPane(os.Getenv("MIA_TMUX_PANE")); tty != "" {
		return syscall.Exec(path, []string{"tmux", "switch-client", "-c", tty, "-t", exact(name)}, os.Environ())
	}
	argv := []string{"tmux", "attach-session", "-t", exact(name)}
	if os.Getenv("TMUX") != "" && os.Getenv("TMUX_PANE") == "" {
		return syscall.Exec(path, []string{"tmux", "switch-client", "-t", exact(name)}, os.Environ())
	}
	if insideTmux() {
		argv = []string{"tmux", "switch-client", "-t", exact(name)}
		if tty := clientOfPane(os.Getenv("TMUX_PANE")); tty != "" {
			argv = append(argv, "-c", tty)
		}
	} else {
		return execWithout(path, argv, "TMUX", "TMUX_PANE")
	}
	return syscall.Exec(path, argv, os.Environ())
}

func (h Host) SelectWindow(worktree, window string) error {
	if out, err := h.run("select-window", "-t", Target(worktree, window)); err != nil {
		return fmt.Errorf("no window %s in %s: %s", window, worktree, strings.TrimSpace(out))
	}
	return nil
}

func (h Host) AttachWindow(worktree, window string) error {
	if err := h.SelectWindow(worktree, window); err != nil {
		return err
	}
	return h.Attach(worktree)
}

func Popup(argv []string, beside, title, width, height string) error {
	if beside == "" {
		return fmt.Errorf("a popup needs tmux — ⏎ lands there instead")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	script := viewScript(strconv.Itoa(os.Getpid()))
	body := "#!/bin/sh\nexec env -u TMUX -u MIA_TMUX_PANE " + run.ShellJoin(append([]string{self}, argv...)) + "\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		return err
	}
	defer os.Remove(script)
	out, err := run.Local("tmux").Combined("display-popup", "-E", "-t", beside, "-w", width, "-h", height, "-T", title, script)
	if err != nil {
		return fmt.Errorf("popup: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (h Host) CloseHover(beside string) {
	if tty := clientOfPane(beside); tty != "" {
		_, _ = run.Local("tmux").Combined("display-popup", "-C", "-c", tty)
	}
}

func viewScript(worktree string) string {
	return filepath.Join(os.TempDir(), "mia-hover-"+worktree+".sh")
}

func clientOfPane(pane string) string {
	if pane == "" {
		return ""
	}
	out, err := run.Local("tmux").Capture("list-clients", "-F", "#{client_tty} #{pane_id}")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if tty, id, ok := strings.Cut(line, " "); ok && id == pane {
			return tty
		}
	}
	return ""
}

func Current() string {
	pane := os.Getenv("MIA_TMUX_PANE")
	if pane == "" {
		pane = os.Getenv("TMUX_PANE")
	}
	if pane == "" {
		return ""
	}
	out, err := Here().run("display-message", "-p", "-t", pane, "#{session_name}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func insideTmux() bool {
	if os.Getenv("TMUX") == "" || os.Getenv("TMUX_PANE") == "" {
		return false
	}
	out, err := run.Local("tmux").Capture("display-message", "-p", "-t", os.Getenv("TMUX_PANE"), "#{pane_tty}")
	if err != nil {
		return false
	}
	pane, err := os.Stat(strings.TrimSpace(string(out)))
	if err != nil {
		return false
	}
	current, err := os.Stdout.Stat()
	return err == nil && os.SameFile(pane, current)
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func execWithout(path string, argv []string, drop ...string) error {
	unwanted := map[string]bool{}
	for _, name := range drop {
		unwanted[name] = true
	}
	var kept []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !unwanted[name] {
			kept = append(kept, entry)
		}
	}
	return syscall.Exec(path, argv, kept)
}

func (h Host) Rename(worktree, name string) error {
	if worktree == name || !h.Exists(worktree) {
		return nil
	}
	if out, err := h.run("rename-session", "-t", exact(Name(worktree)), Name(name)); err != nil {
		return fmt.Errorf("rename session for %s: %s", worktree, strings.TrimSpace(out))
	}
	return nil
}

func (h Host) Kill(worktree string) error {
	if !h.Exists(worktree) {
		return nil
	}
	group, err := h.run("display-message", "-p", "-t", Target(worktree, ""), "#{session_group}")
	if err != nil {
		return err
	}
	group = strings.TrimSpace(group)
	sessions, err := h.run("list-sessions", "-F", "#{session_name}|#{session_group}|#{@mia_worktree}")
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(strings.TrimSpace(sessions), "\n") {
		fields := strings.SplitN(line, "|", 3)
		if len(fields) != 3 {
			continue
		}
		name, memberOf, owner := fields[0], fields[1], fields[2]
		legacyView := group != "" && memberOf == group
		if strings.HasPrefix(name, "mia-popup-") && (owner == worktree || legacyView) {
			if out, err := h.run("kill-session", "-t", exact(name)); err != nil {
				if _, exists := h.run("has-session", "-t", exact(name)); exists == nil {
					return fmt.Errorf("close popup for %s: %s", worktree, strings.TrimSpace(out))
				}
			}
		}
	}
	out, err := h.run("kill-session", "-t", exact(Name(worktree)))
	if err != nil {
		return fmt.Errorf("kill session for %s: %s", worktree, strings.TrimSpace(out))
	}
	return nil
}

func (h Host) Available() bool {
	if h.SSH == "" {
		_, err := exec.LookPath("tmux")
		return err == nil
	}
	_, err := runtime.Run(h.SSH, "sh", "-c", "command -v tmux")
	return err == nil
}

func Target(worktree, window string) string {
	if window == "" {
		return exact(Name(worktree)) + ":"
	}
	return exact(Name(worktree)) + ":" + window
}

func (h Host) Capture(worktree, window string, withEscapes bool) (string, error) {
	return h.CaptureBack(worktree, window, withEscapes, 0)
}

func (h Host) Send(worktree, window, text string) error {
	if out, err := h.run("send-keys", "-t", Target(worktree, window), "-l", text); err != nil {
		return fmt.Errorf("type into %s: %s", worktree, strings.TrimSpace(out))
	}
	if out, err := h.run("send-keys", "-t", Target(worktree, window), "Enter"); err != nil {
		return fmt.Errorf("type into %s: %s", worktree, strings.TrimSpace(out))
	}
	return nil
}

func (h Host) CaptureBack(worktree, window string, withEscapes bool, back int) (string, error) {
	args := []string{"capture-pane", "-p", "-t", Target(worktree, window)}
	if withEscapes {
		args = append(args, "-e")
	}
	if back > 0 {
		args = append(args, "-S", "-"+strconv.Itoa(back))
	}
	out, err := h.run(args...)
	if err != nil {
		return "", fmt.Errorf("read the session for %s: %s", worktree, strings.TrimSpace(out))
	}
	return out, nil
}

func (h Host) Paste(worktree, window, text string) error {
	if !h.Exists(worktree) {
		return fmt.Errorf("%s has no session", worktree)
	}
	buffer := fmt.Sprintf("mia-send-%d-%d", os.Getpid(), time.Now().UnixNano())
	direct := h.run
	if h.SSH == "" {
		direct = run.Tool{Binary: "tmux"}.Combined
	}
	if out, err := direct("set-buffer", "-b", buffer, "--", text); err != nil {
		return fmt.Errorf("paste to %s: %s", worktree, strings.TrimSpace(out))
	}
	if out, err := direct("paste-buffer", "-p", "-d", "-b", buffer, "-t", Target(worktree, window)); err != nil {
		return fmt.Errorf("paste to %s: %s", worktree, strings.TrimSpace(out))
	}
	return nil
}

type Pane struct {
	Index    int
	Window   string
	PID      int
	Command  string
	Activity time.Time
	Active   bool
}

const paneFormat = "#{window_index} #{window_name} #{pane_pid} #{pane_current_command} #{window_activity} #{window_active}"

func parsePane(fields []string) (Pane, bool) {
	if len(fields) < 3 {
		return Pane{}, false
	}
	index, err := strconv.Atoi(fields[0])
	if err != nil {
		return Pane{}, false
	}
	n, err := strconv.Atoi(fields[2])
	if err != nil {
		return Pane{}, false
	}
	pane := Pane{Index: index, Window: fields[1], PID: n}
	if len(fields) > 3 {
		pane.Command = fields[3]
	}
	if len(fields) > 4 {
		if seconds, err := strconv.ParseInt(fields[4], 10, 64); err == nil {
			pane.Activity = time.Unix(seconds, 0)
		}
	}
	if len(fields) > 5 {
		pane.Active = fields[5] == "1"
	}
	return pane, true
}

func (h Host) Panes(worktree string) []Pane {
	out, err := h.run("list-panes", "-s", "-t", exact(Name(worktree)), "-F", paneFormat)
	if err != nil {
		return nil
	}
	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if pane, ok := parsePane(strings.Fields(line)); ok {
			panes = append(panes, pane)
		}
	}
	return panes
}

func (h Host) AllPanes() map[string][]Pane {
	out, err := h.run("list-panes", "-a", "-F", "#{session_name} "+paneFormat)
	if err != nil {
		return nil
	}
	all := map[string][]Pane{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if pane, ok := parsePane(fields[1:]); ok {
			all[fields[0]] = append(all[fields[0]], pane)
		}
	}
	return all
}

func (p Pane) Ref() string {
	if strings.ContainsAny(p.Window, ".:") {
		return strconv.Itoa(p.Index)
	}
	return p.Window
}

func (p Pane) Quiet() time.Duration {
	if p.Activity.IsZero() {
		return 0
	}
	return time.Since(p.Activity)
}

type Window struct {
	Index   int           `json:"index"`
	Name    string        `json:"name"`
	Command string        `json:"command"`
	PID     int           `json:"pid,omitempty"`
	Quiet   time.Duration `json:"-"`
	Still   string        `json:"quiet"`
	Screen  string        `json:"-"`
	Active  bool          `json:"active"`
}

func (h Host) WindowsOf(worktree string) []Window {
	return WindowsFrom(h.Panes(worktree))
}

func WindowsFrom(panes []Pane) []Window {
	seen := map[int]bool{}
	var windows []Window
	for _, pane := range panes {
		if seen[pane.Index] {
			continue
		}
		seen[pane.Index] = true
		quiet := pane.Quiet()
		windows = append(windows, Window{Index: pane.Index, Name: pane.Window, Command: pane.Command, PID: pane.PID, Quiet: quiet, Still: quiet.Round(time.Second).String(), Active: pane.Active})
	}
	return windows
}

type Process struct {
	PID  int
	PPID int
	Comm string
	Args string
}

func ParseProcesses(table string) []Process {
	var procs []Process
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		procs = append(procs, Process{PID: pid, PPID: ppid, Comm: fields[2], Args: strings.Join(fields[3:], " ")})
	}
	return procs
}

func (h Host) Windows(worktree string) []string {
	out, err := h.run("list-windows", "-t", exact(Name(worktree)), "-F", "#{window_name}")
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}

func (h Host) StartWindow(worktree, dir, window string, argv []string) error {
	if err := h.Ensure(worktree, dir); err != nil {
		return err
	}
	if h.HasWindow(worktree, window) {
		return nil
	}
	args := append([]string{"new-window", "-d", "-t", exact(Name(worktree)), "-n", window, "-c", dir, "-e", "MIA_WORKTREE=" + worktree, "--"}, argv...)
	if out, err := h.run(args...); err != nil {
		return fmt.Errorf("start %s in %s: %s", window, worktree, strings.TrimSpace(out))
	}
	_ = h.styleOptions(Target(worktree, window), true, miaWindowStyle)
	return nil
}

func (h Host) HasWindow(worktree, window string) bool {
	for _, name := range h.Windows(worktree) {
		if name == window {
			return true
		}
	}
	return false
}

func (h Host) NameFirstWindow(worktree, window string) error {
	if out, err := h.run("rename-window", "-t", exact(Name(worktree))+":^", window); err != nil {
		return fmt.Errorf("name %s's first window: %s", worktree, strings.TrimSpace(out))
	}
	return nil
}

func (h Host) KillWindow(worktree, window string) error {
	exists := h.HasWindow(worktree, window)
	if index, err := strconv.Atoi(window); err == nil {
		for _, one := range h.WindowsOf(worktree) {
			if one.Index == index {
				exists = true
				break
			}
		}
	}
	if !exists {
		return nil
	}
	out, err := h.run("kill-window", "-t", Target(worktree, window))
	if err != nil {
		return fmt.Errorf("stop %s in %s: %s", window, worktree, strings.TrimSpace(out))
	}
	return nil
}

func Exists(worktree string) bool { return Here().Exists(worktree) }
