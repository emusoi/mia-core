package env

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func supervise(t *testing.T, service Service) (dir string, loop *exec.Cmd) {
	t.Helper()
	dir = t.TempDir()
	script := strings.ReplaceAll(service.Script(), runtimeDir, dir)
	loop = exec.Command("sh", "-c", script)
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if loop.Process != nil {
			loop.Process.Signal(syscall.SIGTERM)
			loop.Wait()
		}
	})
	return dir, loop
}

func logOf(dir, id string) string {
	data, _ := os.ReadFile(filepath.Join(dir, id+".log"))
	return string(data)
}

func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("not within %s: %s", within, what)
}

func TestNoRestartIsTheExecFormItAlwaysWas(t *testing.T) {
	s := Service{ID: "plain", Run: []string{"echo", "hi"}}
	got := s.Script()
	want := "mkdir -p /run/mia && echo $$ > '/run/mia/plain.pid' && exec 'echo' 'hi' >> '/run/mia/plain.log' 2>&1"
	if got != want {
		t.Fatalf("a service without restart must not change shape:\n got %q\nwant %q", got, want)
	}
}

func TestOnFailureRestartsAFailingCommand(t *testing.T) {
	s := Service{ID: "flaky", Restart: RestartOnFailure,
		Run: []string{"sh", "-c", "echo tick; exit 1"}}
	dir, _ := supervise(t, s)
	eventually(t, 8*time.Second, "at least two ticks and a restart line", func() bool {
		out := logOf(dir, "flaky")
		return strings.Count(out, "tick") >= 2 && strings.Contains(out, "mia: exited 1, restarting")
	})
}

func TestOnFailureLetsACleanExitStop(t *testing.T) {
	s := Service{ID: "once", Restart: RestartOnFailure,
		Run: []string{"sh", "-c", "echo done; exit 0"}}
	dir, loop := supervise(t, s)
	finished := make(chan error, 1)
	go func() { finished <- loop.Wait() }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("a command that exited 0 must end the loop, not be restarted")
	}
	if strings.Contains(logOf(dir, "once"), "restarting") {
		t.Fatal("a clean exit was restarted")
	}
}

func TestOnUnhealthyRestartsWhenHealthKeepsFailing(t *testing.T) {
	old := healthEvery
	healthEvery = 1
	t.Cleanup(func() { healthEvery = old })
	s := Service{ID: "stuck", Restart: RestartUnhealthy, Grace: 1,
		Run:    []string{"sh", "-c", "echo up; sleep 60"},
		Health: []string{"false"}}
	dir, _ := supervise(t, s)
	eventually(t, 15*time.Second, "an unhealthy restart and a second start", func() bool {
		out := logOf(dir, "stuck")
		return strings.Contains(out, "not answering 3 times, restarting") && strings.Count(out, "up") >= 2
	})
}

func TestTermToTheLoopTakesTheChildWithIt(t *testing.T) {
	s := Service{ID: "held", Restart: RestartOnFailure,
		Run: []string{"sh", "-c", "echo $$ > " + "$MIA_TEST_CHILD; sleep 60"}}
	dir := t.TempDir()
	childPid := filepath.Join(dir, "child")
	script := strings.ReplaceAll(s.Script(), runtimeDir, dir)
	loop := exec.Command("sh", "-c", script)
	loop.Env = append(os.Environ(), "MIA_TEST_CHILD="+childPid)
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	eventually(t, 5*time.Second, "the child wrote its pid", func() bool {
		_, err := os.Stat(childPid)
		return err == nil
	})
	data, _ := os.ReadFile(childPid)
	pid := strings.TrimSpace(string(data))
	loop.Process.Signal(syscall.SIGTERM)
	loop.Wait()
	eventually(t, 5*time.Second, "the child is gone", func() bool {
		return exec.Command("kill", "-0", pid).Run() != nil
	})
}

func TestUnhealthyNeedsAHealthCommand(t *testing.T) {
	err := Service{ID: "x", Restart: RestartUnhealthy, Run: []string{"true"}}.Validate()
	if err == nil || !strings.Contains(err.Error(), "needs a `health` command") {
		t.Fatalf("Validate = %v", err)
	}
	if err := (Service{ID: "x", Restart: "sometimes"}).Validate(); err == nil {
		t.Fatal("an unknown restart mode must be refused")
	}
}
