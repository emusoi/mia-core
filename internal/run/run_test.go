package run

import (
	"strings"
	"testing"
)

func TestCaptureTrimsAndKeepsTheToolsOwnWords(t *testing.T) {
	tool := Local("sh")
	out, err := tool.Capture("-c", "printf ' hello \\n'")
	if err != nil || out != "hello" {
		t.Fatalf("Capture = %q, %v — want trimmed output", out, err)
	}

	tool.Detail = func(stderr string) string { return "SEEN: " + FirstLine(stderr) }
	_, err = tool.Capture("-c", "echo 'first line' >&2; echo 'second' >&2; exit 3")
	if err == nil {
		t.Fatal("a non-zero exit must be an error")
	}
	if err.Error() != "SEEN: first line" {
		t.Errorf("Detail must shape the message the caller prints, got %q", err.Error())
	}
	var failure *Failure
	if !asFailure(err, &failure) {
		t.Fatal("the error must carry the raw stderr for callers that want it")
	}
	if !strings.Contains(failure.Stderr, "second") {
		t.Errorf("Failure.Stderr lost the rest of stderr: %q", failure.Stderr)
	}
	if ExitCode(err) != 3 {
		t.Errorf("ExitCode = %d, want 3", ExitCode(err))
	}
}

func TestAToolWithNoDetailReportsStderrAsItIs(t *testing.T) {
	_, err := Local("sh").Capture("-c", "echo 'plain words' >&2; exit 1")
	if err == nil || err.Error() != "plain words" {
		t.Fatalf("err = %v, want the stderr unchanged", err)
	}
}

func TestStatusSeparatesExitCodeFromFailure(t *testing.T) {
	code, out := Local("sh").Status("-c", "echo out; exit 7")
	if code != 7 {
		t.Errorf("code = %d, want 7", code)
	}
	if !strings.Contains(out, "out") {
		t.Errorf("Status must return what the command said, got %q", out)
	}
	if code, _ := Local("sh").Status("-c", "true"); code != 0 {
		t.Errorf("a passing command must report 0, got %d", code)
	}
}

func TestShellJoinSurvivesQuotes(t *testing.T) {
	got := ShellJoin([]string{"sh", "-c", "echo 'it's there'"})
	if !strings.Contains(got, `'\''`) {
		t.Errorf("a single quote must be escaped for the remote shell: %s", got)
	}
}

func TestAnSSHToolShellJoinsTheWholeCommand(t *testing.T) {
	command := On("box", "podman").Command("ps", "-a")
	line := strings.Join(command.Args, " ")
	if !strings.Contains(line, "box -- 'podman' 'ps' '-a'") {
		t.Errorf("the remote argv must be quoted as one word: %s", line)
	}
	if !strings.Contains(line, "ControlMaster=auto") {
		t.Errorf("ssh options are missing: %s", line)
	}
}

func asFailure(err error, into **Failure) bool {
	for err != nil {
		if f, ok := err.(*Failure); ok {
			*into = f
			return true
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}
