package runtime_test

import (
	"os"
	"testing"

	"github.com/emusoi/mia-core/internal/runtime"
)

func TestProbeARealMachine(t *testing.T) {
	target := os.Getenv("MIA_TEST_SSH")
	if target == "" {
		t.Skip("set MIA_TEST_SSH to a machine to probe")
	}
	machine, err := runtime.Probe(target)
	if err != nil {
		t.Fatal(err)
	}
	if machine.Engine == "" {
		t.Fatal("probe found no engine and did not say so")
	}
	t.Logf("%s: %s", target, machine.Engine)
}
