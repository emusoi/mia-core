package container

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAnTabInAContainerIsToldWhatTerminalItIsOn(t *testing.T) {
	argv := Engine{Binary: "podman"}.ExecArgv("mia-longido", "/work", []string{"helper"})

	if argv[len(argv)-1] != "helper" {
		t.Fatalf("the command must come last: %v", argv)
	}
	if !slices.Contains(argv, "TERM=xterm-256color") {
		t.Fatalf("without TERM the TUI draws in eight colours: %v", argv)
	}
}

func TestTheEngineRepeatsWhatPodmanActuallySaid(t *testing.T) {
	said := `Cannot connect to Podman. Please verify your connection to the Linux system using ` + "`podman system connection list`" + `
Error: unable to connect to Podman socket: failed to connect: dial tcp 127.0.0.1:51881: connect: connection refused`

	if got := whatWentWrong(said); got != "unable to connect to Podman socket: failed to connect: dial tcp 127.0.0.1:51881: connect: connection refused" {
		t.Fatalf("the line naming the socket is the one that helps: %q", got)
	}
	if got := whatWentWrong("no such container\nand nothing else"); got != "no such container" {
		t.Fatalf("without an Error: line the first one stands: %q", got)
	}
}

func TestRunningAsksTheEngineOnceForEveryName(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "podman")
	script := "#!/bin/sh\necho \"$@\" >> " + filepath.Join(dir, "calls") + "\nprintf 'mia-longido\\nmia-monduli\\n'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	up := Engine{Binary: fake}.Running()

	if !up["mia-longido"] || !up["mia-monduli"] || up["mia-nope"] {
		t.Fatalf("running = %v", up)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if got := strings.TrimSpace(string(calls)); got != "ps --format {{.Names}}" {
		t.Fatalf("asked the engine %q", got)
	}
}
