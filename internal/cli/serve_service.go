package cli

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/gateway"
	"github.com/emusoi/mia-core/internal/run"
)

// serveLabel is the service's name to launchd, in mia's own namespace;
// systemd's is serveUnit.
const (
	serveLabel = "com.emusoi.mia.serve"
	serveUnit  = "mia-serve"
)

// serveOS is the system the service is written for: launchd's on darwin,
// systemd's on linux.
var serveOS = runtime.GOOS

// serveCarried is what the server's answers depend on of the environment
// it runs in, carried into the service from the shell that installs it
// where that sets it: where plugins and agents are found, the tmux server,
// mia's config folder with the socket in it, and where the agents keep
// their conversations.
var serveCarried = []string{"PATH", "TMUX_TMPDIR", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "CLAUDE_CONFIG_DIR", "CODEX_HOME", "PI_CODING_AGENT_DIR"}

// serveLog is where the service writes what the server says, under launchd.
func serveLog() string { return filepath.Join(serveDir(), "serve.log") }

// serveService is the file the service is written in: launchd's user
// agent, or systemd's user unit.
func serveService() (string, error) {
	switch serveOS {
	case "darwin":
		home, err := os.UserHomeDir()
		return filepath.Join(home, "Library", "LaunchAgents", serveLabel+".plist"), err
	case "linux":
		config, err := os.UserConfigDir()
		return filepath.Join(config, "systemd", "user", serveUnit+".service"), err
	}
	return "", fmt.Errorf("mia serve runs as a service of launchd or systemd, which %s has neither of — run `mia serve` itself", serveOS)
}

// serveServiceBody is what the service file says: run mia serve, the
// program given, with the environment carried, at login and again
// whenever it ends.
func serveServiceBody(self string) string {
	var env [][2]string
	for _, name := range serveCarried {
		if value := os.Getenv(name); value != "" {
			env = append(env, [2]string{name, value})
		}
	}
	var body strings.Builder
	if serveOS == "linux" {
		fmt.Fprintf(&body, "[Unit]\nDescription=mia serve\n\n[Service]\nExecStart=%s serve\nRestart=always\nRestartSec=2\n", self)
		for _, one := range env {
			fmt.Fprintf(&body, "Environment=%s\n", strconv.Quote(one[0]+"="+one[1]))
		}
		body.WriteString("\n[Install]\nWantedBy=default.target\n")
		return body.String()
	}
	text := func(s string) string {
		var escaped strings.Builder
		xml.EscapeText(&escaped, []byte(s))
		return escaped.String()
	}
	fmt.Fprintf(&body, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>serve</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
`, serveLabel, text(self))
	for _, one := range env {
		fmt.Fprintf(&body, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", one[0], text(one[1]))
	}
	fmt.Fprintf(&body, `	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, text(serveLog()), text(serveLog()))
	return body.String()
}

// launchctl asks launchd, as the person.
func launchctl(args ...string) error {
	out, err := run.Local("launchctl").Combined(args...)
	if err != nil {
		return fmt.Errorf("launchctl %s: %s", strings.Join(args, " "), strings.TrimSpace(out))
	}
	return nil
}

// launchdDomain is the person's own launchd, and serveJob the service in it.
func launchdDomain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func serveJob() string { return launchdDomain() + "/" + serveLabel }

// serveLoaded says whether the service manager has the service: launchd
// has it loaded, systemd has it running.
func serveLoaded() bool {
	if serveOS == "darwin" {
		return launchctl("print", serveJob()) == nil
	}
	return systemctl("is-active", "--quiet", serveUnit) == nil
}

// serveInstall is `mia serve install`: the service written, with the
// environment of the shell it is typed in, and started.
func serveInstall() int {
	file, err := serveService()
	if err != nil {
		return fail(err)
	}
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(serveDir(), 0o700); err != nil { // its log is there
		return fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(file, []byte(serveServiceBody(self)), 0o644); err != nil {
		return fail(err)
	}
	var steps [][]string
	if serveOS == "darwin" {
		if serveLoaded() { // written again: loaded again
			steps = append(steps, []string{"bootout", serveJob()})
		}
		steps = append(steps, []string{"bootstrap", launchdDomain(), file})
	} else {
		steps = [][]string{{"daemon-reload"}, {"enable", serveUnit}, {"restart", serveUnit}}
	}
	if err := serveSteps(steps); err != nil {
		return fail(err)
	}
	fmt.Printf("mia serve installed as a service — %s\nit starts at login, and again whenever it ends: after `make install` too, as a server that sees mia rebuilt ends itself; `mia serve status` says whether it answers\n", file)
	return exitOK
}

// serveSteps runs each step with the service manager, until one fails.
func serveSteps(steps [][]string) error {
	for _, step := range steps {
		ask := systemctl
		if serveOS == "darwin" {
			ask = launchctl
		}
		if err := ask(step...); err != nil {
			return err
		}
	}
	return nil
}

// serveInstalled is the service's file, when it is written.
func serveInstalled() (string, error) {
	file, err := serveService()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(file); err != nil {
		return "", errors.New("no mia serve service is installed — `mia serve install`")
	}
	return file, nil
}

// serveUninstall is `mia serve uninstall`: the service stopped, let go of
// and its file taken away.
func serveUninstall() int {
	file, err := serveInstalled()
	if err != nil {
		return fail(err)
	}
	var steps [][]string
	if serveOS == "darwin" {
		if serveLoaded() {
			steps = [][]string{{"bootout", serveJob()}}
		}
	} else {
		steps = [][]string{{"disable", "--now", serveUnit}}
	}
	if err := serveSteps(steps); err != nil {
		return fail(err)
	}
	if err := os.Remove(file); err != nil {
		return fail(err)
	}
	if serveOS == "linux" {
		if err := systemctl("daemon-reload"); err != nil {
			return fail(err)
		}
	}
	fmt.Println("mia serve service removed")
	return exitOK
}

// serveStart is `mia serve start`: the service started, or started again
// when it runs.
func serveStart() int {
	file, err := serveInstalled()
	if err != nil {
		return fail(err)
	}
	steps := [][]string{{"restart", serveUnit}}
	if serveOS == "darwin" {
		steps = [][]string{{"bootstrap", launchdDomain(), file}}
		if serveLoaded() {
			steps = [][]string{{"kickstart", "-k", serveJob()}}
		}
	}
	if err := serveSteps(steps); err != nil {
		return fail(err)
	}
	fmt.Println("mia serve started")
	return exitOK
}

// serveStop is `mia serve stop`: the service stopped, until it is started
// again or the person logs in again. launchd starts again a job it keeps
// alive that ends, so there it is let go of, its file kept.
func serveStop() int {
	if _, err := serveInstalled(); err != nil {
		return fail(err)
	}
	if !serveLoaded() {
		fmt.Println("mia serve is not running as a service")
		return exitOK
	}
	steps := [][]string{{"stop", serveUnit}}
	if serveOS == "darwin" {
		steps = [][]string{{"bootout", serveJob()}}
	}
	if err := serveSteps(steps); err != nil {
		return fail(err)
	}
	fmt.Println("mia serve stopped — `mia serve start` starts it again, and so does the next login")
	return exitOK
}

// serveStatus is `mia serve status`: whether the service is installed and
// runs, and whether a server answers on the socket, which and of what
// build.
func serveStatus() int {
	file, err := serveService()
	if err != nil {
		return fail(err)
	}
	switch _, err := os.Stat(file); {
	case err != nil:
		fmt.Println("service   not installed — `mia serve install`")
	case serveLoaded():
		fmt.Printf("service   running — %s\n", file)
	default:
		fmt.Printf("service   installed, not running — `mia serve start`; %s\n", file)
	}
	running, ok := serving()
	switch {
	case !ok:
		fmt.Printf("socket    nobody answers on %s\n", serveSocket())
	case running.Build == gateway.Build():
		fmt.Printf("socket    answers — pid %d, this build: %s\n", running.Pid, running.Build)
	default:
		fmt.Printf("socket    answers — pid %d, another build, which mia api does not ask: %s\n", running.Pid, running.Build)
	}
	if serveOS == "darwin" {
		fmt.Printf("log       %s\n", serveLog())
	} else {
		fmt.Printf("log       journalctl --user -u %s\n", serveUnit)
	}
	return exitOK
}
