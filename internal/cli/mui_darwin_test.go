package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/runtime"
)

// openPty is a new pty: its master, and the path of its slave.
func openPty(t *testing.T) (*os.File, string) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	var name [128]byte
	for _, ask := range []struct{ req, arg uintptr }{{syscall.TIOCPTYGRANT, 0}, {syscall.TIOCPTYUNLK, 0}, {syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))}} {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), ask.req, ask.arg); errno != 0 {
			t.Fatal(errno)
		}
	}
	return master, string(name[:bytes.IndexByte(name[:], 0)])
}

// TestMuiCommand is not a test: run by the test below as a process of its
// own, it is the mia command its environment names.
func TestMuiCommand(t *testing.T) {
	if command := os.Getenv("MIA_TEST_MUI"); command != "" {
		os.Exit(Run([]string{command}))
	}
}

// A UI drawn over a pane ends as soon as the pty it was started in is let
// go, also when what started it, as mia desk does, let it keep the pty's
// master: that copy is the UI's own, and it would keep the pty open, and
// the UI running, for nobody.
func TestAUIEndsWithThePtyItWasStartedIn(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	root, err := filepath.EvalSymlinks(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	said := filepath.Join(os.Getenv("HOME"), ".claude", "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(root, "-"), "c1.jsonl")
	if err := os.MkdirAll(filepath.Dir(said), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(said, []byte(`{"type":"user","cwd":"`+root+`","sessionId":"c1","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"hello"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"chat", "git", "files", "review"} {
		for _, kept := range []bool{false, true} {
			name := command
			if kept {
				name += " keeping the master"
			}
			t.Run(name, func(t *testing.T) {
				master, path := openPty(t)
				slave, err := os.OpenFile(path, os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				ui := exec.Command(os.Args[0], "-test.run=^TestMuiCommand$")
				ui.Dir, ui.Env = root, append(os.Environ(), "MIA_TEST_MUI="+command)
				ui.Stdin, ui.Stdout, ui.Stderr = slave, slave, slave
				ui.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
				if kept {
					ui.ExtraFiles = []*os.File{master}
				}
				if err := ui.Start(); err != nil {
					t.Fatal(err)
				}
				slave.Close()
				// what a terminal answers when asked its background, as each is at start
				if _, err := master.WriteString("\033]11;rgb:0000/0000/0000\033\\\033[1;1R"); err != nil {
					t.Fatal(err)
				}
				ended := make(chan error, 1)
				go func() { ended <- ui.Wait(); close(ended) }()
				t.Cleanup(func() {
					_ = ui.Process.Kill()
					<-ended
				})

				// the UI is up once its line on the plain terminal is written
				var out []byte
				data := make([]byte, 4096)
				for !bytes.Contains(out, []byte("the plain terminal")) {
					_ = master.SetReadDeadline(time.Now().Add(10 * time.Second))
					n, err := master.Read(data)
					if out = append(out, data[:n]...); err != nil {
						t.Fatalf("the UI never came up: %v, it said %q", err, out)
					}
				}
				master.Close()
				select {
				case <-ended:
				case <-time.After(3 * time.Second):
					t.Fatalf("mia %s still runs after its pty was let go; it said %q", command, strings.TrimSpace(string(out)))
				}
			})
		}
	}
}

// A chat of mia's lives where the main agent does: from another machine
// mia chat --chat is that machine's mia's, run there over ssh with this
// terminal as its own, told the pane of this machine's tmux it is drawn
// through, so that what it says reaches mia desk here as a UI run here
// does.
func TestAChatOnAnotherMachineIsDrawnFromThere(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())
	t.Setenv("MIA_MAIN_HERE", "")
	t.Setenv("TMUX", "/tmp/tmux-test/default,1,0")
	t.Setenv("TMUX_PANE", "%7")
	bin := t.TempDir()
	log := filepath.Join(bin, "asked")
	ssh := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	machines := runtime.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "runtimes.json")}
	if err := machines.Add(runtime.Machine{Name: "fedora", SSH: "fedora.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), []byte("[main]\nmachine = \"fedora\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	master, path := openPty(t)
	defer master.Close()
	slave, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	stdin := os.Stdin
	os.Stdin = slave
	defer func() { os.Stdin = stdin }()
	if code := Run([]string{"chat", "--chat", "goba"}); code != exitOK {
		t.Fatalf("chat --chat goba exited %d", code)
	}
	data, _ := os.ReadFile(log)
	for _, want := range []string{"-t fedora.invalid", "MIA_MAIN_HERE=1", `$HOME/.local/bin/mia chat --chat "$name"`, "'goba' 'TMUX_PANE=%7' 'MIA_MUI_TMUX=1'"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("fedora was not asked %s:\n%s", want, data)
		}
	}
	for _, bad := range [][]string{{"chat", "--chat"}, {"chat", "--chat", "goba", "--window", "1"}, {"chat", "--chat", "goba", "extra"}} {
		if code := Run(bad); code != exitUsage {
			t.Errorf("%v exited %d, want %d", bad, code, exitUsage)
		}
	}
}
