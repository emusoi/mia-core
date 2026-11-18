package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// What is pasted or typed into a session goes whole however long it is:
// tmux refuses a command line over about 16 KB, and a long prompt is more.
func TestTextOfMoreThanACommandLineArrivesWhole(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-paste-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	for _, s := range []string{"pasted", "typed"} {
		if out, err := exec.Command("tmux", "new-session", "-d", "-s", "mia-"+s, "stty -echo; cat > "+filepath.Join(dir, s)).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	text := strings.Repeat(strings.Repeat("é", 49)+"\n", 2048) // 200 KB; 4096 bytes in is half an é
	if err := Here().Paste("pasted", "", text); err != nil {
		t.Fatal(err)
	}
	if err := Here().Send("typed", "", text); err != nil {
		t.Fatal(err)
	}
	for s, want := range map[string]string{"pasted": text, "typed": text + "\n"} { // and the Enter after it
		deadline := time.Now().Add(10 * time.Second)
		for {
			data, _ := os.ReadFile(filepath.Join(dir, s))
			if string(data) == want {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: %d of %d bytes arrived", s, len(data), len(want))
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
