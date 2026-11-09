package session

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// launchd and systemd start mia with no locale, and a tmux client without
// one prints a tab in a format as "_": without -u mia found no session.
func TestPanesAreFoundWithoutALocale(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mia-locale-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	saved := os.Environ()
	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range saved {
			k, v, _ := strings.Cut(kv, "=")
			os.Setenv(k, v)
		}
	})
	home := os.Getenv("HOME")
	os.Clearenv()
	os.Setenv("HOME", home)
	os.Setenv("PATH", "/usr/bin:/bin:"+strings.TrimSuffix(tmux, "/tmux"))
	os.Setenv("TMUX_TMPDIR", dir)
	t.Cleanup(func() { exec.Command(tmux, "kill-server").Run() })
	// the server, as yours, has a UTF-8 locale; only mia runs without one
	server := exec.Command(tmux, "-f", "/dev/null", "new-session", "-d", "-s", Name("ilala"))
	server.Env = append(os.Environ(), "LANG=C.UTF-8")
	if out, err := server.CombinedOutput(); err != nil {
		t.Fatalf("tmux: %s", out)
	}
	if panes := Here().AllPanes()[Name("ilala")]; len(panes) != 1 {
		t.Fatalf("the session's panes without a locale: %+v", Here().AllPanes())
	}
	if len(Here().Panes("ilala")) != 1 {
		t.Fatal("the session's own panes without a locale")
	}
}
