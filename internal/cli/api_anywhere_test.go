package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// The machines, this computer and what the API speaks are not about one repository: the desk
// and the phone ask from wherever they were started.
func TestAPIAnswersOutsideARepositoryWhatIsNotAboutOne(t *testing.T) {
	gittest.Isolate(t)
	outside := t.TempDir()
	t.Chdir(outside)
	answer := func(query string) []byte {
		t.Helper()
		out, err := os.CreateTemp(t.TempDir(), "out")
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		stdout := os.Stdout
		os.Stdout = out
		code := Run([]string{"api", query})
		os.Stdout = stdout
		if code != exitOK {
			t.Fatalf("mia api %s outside a repository exited %d", query, code)
		}
		data, err := os.ReadFile(out.Name())
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var machines []app.MachineState
	if err := json.Unmarshal(answer("machines"), &machines); err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 || machines[0].Name != "local" || !machines[0].Answers || machines[0].Health == nil {
		t.Fatalf("machines = %+v", machines)
	}
	var here app.HereState
	if err := json.Unmarshal(answer("here"), &here); err != nil {
		t.Fatal(err)
	}
	if here.Health == nil || here.Machines == nil {
		t.Fatalf("here = %+v", here)
	}
	var caps capabilities
	if err := json.Unmarshal(answer("capabilities"), &caps); err != nil || caps.Version != apiVersion {
		t.Fatalf("capabilities = %+v, %v", caps, err)
	}
	if left, _ := filepath.Glob(filepath.Join(outside, "*")); len(left) != 0 {
		t.Fatalf("asking left files where it was asked: %v", left)
	}
	// what is about a repository still says there is none; a query there is not is still not found
	if code := Run([]string{"api", "worktrees"}); code != exitFailed {
		t.Fatalf("api worktrees outside a repository exited %d", code)
	}
	if code := Run([]string{"api", "nosuch"}); code != exitNotFound {
		t.Fatalf("an unknown query outside a repository exited %d", code)
	}
}
