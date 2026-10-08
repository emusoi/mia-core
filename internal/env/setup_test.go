package env

import (
	"os"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/model"
)

func TestEnvSetupNamesTheKeyThatConfiguresIt(t *testing.T) {
	err := Manager{}.Setup(model.Record{Name: "monduli", Path: t.TempDir()}, os.Stderr)
	if err == nil || !strings.Contains(err.Error(), "[env] setup") {
		t.Fatalf("an unconfigured environment setup must name the key that configures it, got %v", err)
	}
}
