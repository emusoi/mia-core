package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// managers puts a launchctl and a systemctl on PATH that only write down
// how they were asked, and says the service loaded when loaded is there:
// the real ones are never run. What they were asked is calls.
func managers(t *testing.T) (calls func() []string, loaded string) {
	t.Helper()
	bin := t.TempDir()
	written, loaded := filepath.Join(bin, "calls"), filepath.Join(bin, "loaded")
	for _, name := range []string{"launchctl", "systemctl"} {
		script := fmt.Sprintf("#!/bin/sh\necho \"%s $*\" >> %s\ncase \"$1 $2\" in print*|--user\\ is-active) [ -f %s ] ;; esac\n", name, written, loaded)
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		data, _ := os.ReadFile(written)
		os.Remove(written)
		return strings.FieldsFunc(string(data), func(r rune) bool { return r == '\n' })
	}, loaded
}

func sameCalls(t *testing.T, verb string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("mia serve %s asked\n%s\nnot\n%s", verb, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// mia serve install writes launchd's user agent, with the environment the
// server answers from carried as the shell has it, and loads it; start,
// stop, status and uninstall ask launchd of it as it stands.
func TestTheServiceIsALaunchdAgentOnAMac(t *testing.T) {
	isolateServe(t)
	defer func(was string) { serveOS = was }(serveOS)
	serveOS = "darwin"
	calls, loaded := managers(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "/home/user/.claude-work")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	plist := filepath.Join(home, "Library", "LaunchAgents", "com.emusoi.mia.serve.plist")
	job := "gui/" + strconv.Itoa(os.Getuid()) + "/com.emusoi.mia.serve"
	dir := t.TempDir()

	if got := capture(t, dir, "serve", "install"); got.code != exitOK || !strings.Contains(got.out, plist) {
		t.Fatalf("mia serve install = %+v", got)
	}
	sameCalls(t, "install", calls(), "launchctl print "+job, "launchctl bootstrap gui/"+strconv.Itoa(os.Getuid())+" "+plist)
	log := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "serve", "serve.log")
	want := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.emusoi.mia.serve</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + self + `</string>
		<string>serve</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>` + os.Getenv("PATH") + `</string>
		<key>TMUX_TMPDIR</key>
		<string>` + os.Getenv("TMUX_TMPDIR") + `</string>
		<key>XDG_CONFIG_HOME</key>
		<string>` + os.Getenv("XDG_CONFIG_HOME") + `</string>
		<key>CLAUDE_CONFIG_DIR</key>
		<string>/home/user/.claude-work</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>` + log + `</string>
	<key>StandardErrorPath</key>
	<string>` + log + `</string>
</dict>
</plist>
`
	if data, err := os.ReadFile(plist); err != nil || string(data) != want {
		t.Fatalf("the agent written is\n%s\n%v\nnot\n%s", data, err, want)
	}
	if info, err := os.Stat(filepath.Dir(log)); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the folder of its log is %v, %v", info, err)
	}

	if err := os.WriteFile(loaded, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := capture(t, dir, "serve", "install"); got.code != exitOK {
		t.Fatalf("mia serve install, loaded = %+v", got)
	}
	sameCalls(t, "install, loaded", calls(), "launchctl print "+job, "launchctl bootout "+job, "launchctl bootstrap gui/"+strconv.Itoa(os.Getuid())+" "+plist)
	if got := capture(t, dir, "serve", "start"); got.code != exitOK {
		t.Fatalf("mia serve start = %+v", got)
	}
	sameCalls(t, "start, loaded", calls(), "launchctl print "+job, "launchctl kickstart -k "+job)
	if got := capture(t, dir, "serve", "status"); got.code != exitOK || !strings.Contains(got.out, "service   running — "+plist) || !strings.Contains(got.out, "nobody answers") || !strings.Contains(got.out, log) {
		t.Errorf("mia serve status = %+v", got)
	}
	calls()
	if got := capture(t, dir, "serve", "stop"); got.code != exitOK {
		t.Fatalf("mia serve stop = %+v", got)
	}
	sameCalls(t, "stop", calls(), "launchctl print "+job, "launchctl bootout "+job)

	os.Remove(loaded)
	if got := capture(t, dir, "serve", "start"); got.code != exitOK {
		t.Fatalf("mia serve start = %+v", got)
	}
	sameCalls(t, "start, not loaded", calls(), "launchctl print "+job, "launchctl bootstrap gui/"+strconv.Itoa(os.Getuid())+" "+plist)
	if got := capture(t, dir, "serve", "uninstall"); got.code != exitOK {
		t.Fatalf("mia serve uninstall = %+v", got)
	}
	sameCalls(t, "uninstall, not loaded", calls(), "launchctl print "+job)
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Errorf("the agent is still there: %v", err)
	}
	if got := capture(t, dir, "serve", "start"); got.code == exitOK || !strings.Contains(got.errs, "mia serve install") {
		t.Errorf("mia serve start, none installed = %+v", got)
	}
	sameCalls(t, "start, none installed", calls())
}

// On Linux it is a systemd user unit, as the gateway's is.
func TestTheServiceIsASystemdUserUnitOnLinux(t *testing.T) {
	isolateServe(t)
	defer func(was string) { serveOS = was }(serveOS)
	serveOS = "linux"
	calls, loaded := managers(t)
	t.Setenv("CODEX_HOME", "/home/user/codex home")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	config, _ := os.UserConfigDir()
	unit := filepath.Join(config, "systemd", "user", "mia-serve.service")
	dir := t.TempDir()

	if got := capture(t, dir, "serve", "install"); got.code != exitOK || !strings.Contains(got.out, unit) {
		t.Fatalf("mia serve install = %+v", got)
	}
	sameCalls(t, "install", calls(), "systemctl --user daemon-reload", "systemctl --user enable mia-serve", "systemctl --user restart mia-serve")
	want := "[Unit]\nDescription=mia serve\n\n[Service]\nExecStart=" + self + " serve\nRestart=always\nRestartSec=2\n" +
		`Environment="PATH=` + os.Getenv("PATH") + "\"\n" +
		`Environment="TMUX_TMPDIR=` + os.Getenv("TMUX_TMPDIR") + "\"\n" +
		`Environment="XDG_CONFIG_HOME=` + os.Getenv("XDG_CONFIG_HOME") + "\"\n" +
		`Environment="CODEX_HOME=/home/user/codex home"` + "\n" +
		"\n[Install]\nWantedBy=default.target\n"
	if data, err := os.ReadFile(unit); err != nil || string(data) != want {
		t.Fatalf("the unit written is\n%s\n%v\nnot\n%s", data, err, want)
	}
	if got := capture(t, dir, "serve", "start"); got.code != exitOK {
		t.Fatalf("mia serve start = %+v", got)
	}
	sameCalls(t, "start", calls(), "systemctl --user restart mia-serve")
	if got := capture(t, dir, "serve", "stop"); got.code != exitOK || !strings.Contains(got.out, "not running") {
		t.Fatalf("mia serve stop, not running = %+v", got)
	}
	sameCalls(t, "stop, not running", calls(), "systemctl --user is-active --quiet mia-serve")
	if err := os.WriteFile(loaded, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := capture(t, dir, "serve", "stop"); got.code != exitOK {
		t.Fatalf("mia serve stop = %+v", got)
	}
	sameCalls(t, "stop", calls(), "systemctl --user is-active --quiet mia-serve", "systemctl --user stop mia-serve")
	if got := capture(t, dir, "serve", "uninstall"); got.code != exitOK {
		t.Fatalf("mia serve uninstall = %+v", got)
	}
	sameCalls(t, "uninstall", calls(), "systemctl --user disable --now mia-serve", "systemctl --user daemon-reload")
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Errorf("the unit is still there: %v", err)
	}
}

// mia serve status says which server answers on the socket: its process
// and whether it is this build.
func TestTheStatusSaysWhoAnswersOnTheSocket(t *testing.T) {
	isolateServe(t)
	defer func(was string) { serveOS = was }(serveOS)
	serveOS = "darwin"
	calls, _ := managers(t)
	serveIn(t)
	got := capture(t, t.TempDir(), "serve", "status")
	if got.code != exitOK || !strings.Contains(got.out, "service   not installed") || !strings.Contains(got.out, fmt.Sprintf("answers — pid %d, this build", os.Getpid())) {
		t.Errorf("mia serve status = %+v", got)
	}
	sameCalls(t, "status, none installed", calls())
}

// A server whose program was rebuilt since it started ends, for the
// service to start the new build.
func TestAServerEndsWhenMiaIsRebuilt(t *testing.T) {
	isolateServe(t)
	listener, err := listenServe()
	if err != nil {
		t.Fatal(err)
	}
	s := newServer()
	s.build, s.rebuilt = "an older build", 20*time.Millisecond
	done := make(chan error, 1)
	go func() { done <- s.serve(context.Background(), listener) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("it ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a server of another build than its program's still serves")
	}
}

// One ends so even while a question it asks again never finishes, as a
// fetch from a machine that answered ssh and then hung once held it.
func TestAServerEndsWhenMiaIsRebuiltWhileAQuestionHangs(t *testing.T) {
	isolateServe(t)
	listener, err := listenServe()
	if err != nil {
		t.Fatal(err)
	}
	s := newServer()
	s.build, s.rebuilt, s.letGo = "an older build", 20*time.Millisecond, 50*time.Millisecond
	s.hub.asking.Add(1) // never done
	done := make(chan error, 1)
	go func() { done <- s.serve(context.Background(), listener) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("it ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a question that hangs keeps a server of another build serving")
	}
}
