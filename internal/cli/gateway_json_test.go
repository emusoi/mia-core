package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/emusoi/mia-core/internal/gateway"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestGatewayStatusHasStableJSON(t *testing.T) {
	gittest.Isolate(t)
	out, err := os.CreateTemp(t.TempDir(), "gateway-status")
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = out
	code := Run([]string{"gateway", "status", "--json"})
	os.Stdout = stdout
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if code != exitOK {
		t.Fatalf("gateway status exited %d", code)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	var got gatewayReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Running != gateway.Running() || got.Address != gateway.Addr || got.Routes == nil {
		t.Fatalf("gateway status = %+v", got)
	}
}
