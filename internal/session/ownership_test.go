package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInheritedTmuxEnvironmentDoesNotOwnANewTerminal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\nprintf '%s\\n' \"$MIA_TEST_TTY\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("TMUX", "inherited")
	t.Setenv("TMUX_PANE", "%1")
	stdout, err := os.Create(filepath.Join(dir, "child-terminal"))
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = stdout
	t.Cleanup(func() { os.Stdout = original; stdout.Close() })
	t.Setenv("MIA_TEST_TTY", stdout.Name())
	if !insideTmux() {
		t.Fatal("the pane owns this terminal")
	}
	other := filepath.Join(dir, "parent-terminal")
	if err := os.WriteFile(other, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIA_TEST_TTY", other)
	if insideTmux() {
		t.Fatal("inherited variables made a different terminal look like the parent pane")
	}
}
