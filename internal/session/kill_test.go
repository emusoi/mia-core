package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestKillingAWorktreesSessionClosesItsPopupsAndNothingElse(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-kill-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	for _, args := range [][]string{
		{"-f", "/dev/null", "new-session", "-d", "-s", Name("ilala")},
		{"new-session", "-d", "-s", Name("sinza")},
		{"new-session", "-d", "-s", "mia-popup-1-2"},
		{"set-option", "-t", "mia-popup-1-2:", "@mia_worktree", "ilala"},
	} {
		if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
			t.Fatalf("tmux %v: %s", args, out)
		}
	}
	real, _ := exec.LookPath("tmux")
	bin := t.TempDir()
	wrapper := "#!/bin/sh\nout=$(" + real + " \"$@\" 2>&1); code=$?\n[ -n \"$out\" ] && printf '%s\\n' \"$out\" | tr '\\t' _\nexit $code\n"
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := Here().Kill("ilala"); err != nil {
		t.Fatal(err)
	}
	if !Here().Exists("sinza") {
		t.Fatal("another worktree's session went with it")
	}
	if exec.Command("tmux", "has-session", "-t", "=mia-popup-1-2").Run() == nil {
		t.Error("the worktree's popup session outlived it")
	}
}
