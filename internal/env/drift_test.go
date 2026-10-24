package env_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/env"
)

func TestUpReportsDriftAndRecreatesWithoutDeletingVolumes(t *testing.T) {
	manager, record, log := environmentProbe(t, `case "$1" in
info) exit 0 ;;
inspect)
  case "$3" in
  *State.Status*) cat "$MIA_DRIFT_STATE" 2>/dev/null ;;
  *Config.Labels*) cat "$MIA_DRIFT_LABEL" 2>/dev/null ;;
  esac ;;
image) exit 0 ;;
run)
  printf running > "$MIA_DRIFT_STATE"
  for arg do
    case "$arg" in dev.mia.config=*) printf '%s' "${arg#*=}" > "$MIA_DRIFT_LABEL" ;; esac
  done ;;
rm) : > "$MIA_DRIFT_STATE" ;;
start) printf running > "$MIA_DRIFT_STATE" ;;
exec) exit 0 ;;
*) exit 99 ;;
esac
`)
	state := filepath.Join(record.Path, "state")
	label := filepath.Join(record.Path, "label")
	t.Setenv("MIA_DRIFT_STATE", state)
	t.Setenv("MIA_DRIFT_LABEL", label)
	manager.Settings = env.Settings{Image: "fixture/image:1", Setup: []string{"true"}}
	out, err := os.CreateTemp(record.Path, "up-output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	up := func(recreate bool) (string, string) {
		t.Helper()
		if err := out.Truncate(0); err != nil {
			t.Fatal(err)
		}
		if _, err := out.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(log, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Up(record, out, recreate); err != nil {
			t.Fatal(err)
		}
		if _, err := out.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		printed, err := io.ReadAll(out)
		if err != nil {
			t.Fatal(err)
		}
		calls, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		return string(printed), string(calls)
	}

	_, calls := up(false)
	if !strings.Contains(calls, "run -d --name mia-monduli") {
		t.Fatalf("fresh environment was not created: %s", calls)
	}
	printed, _ := up(false)
	if strings.Contains(printed, "configuration changed") {
		t.Fatalf("unchanged environment reported drift: %s", printed)
	}

	manager.Settings.Setup = []string{"true", "changed"}
	printed, calls = up(false)
	if !strings.Contains(printed, "configuration changed") || strings.Contains(calls, "rm -f mia-monduli") {
		t.Fatalf("ordinary up did not warn without recreating: %s %s", printed, calls)
	}
	printed, calls = up(true)
	if !strings.Contains(printed, "volumes and sync are kept") || !strings.Contains(calls, "rm -f mia-monduli") || !strings.Contains(calls, "run -d --name mia-monduli") {
		t.Fatalf("recreate did not replace the container: %s %s", printed, calls)
	}
	if strings.Contains(calls, "volume rm") {
		t.Fatalf("recreate deleted volumes: %s", calls)
	}
	printed, _ = up(false)
	if strings.Contains(printed, "configuration changed") {
		t.Fatalf("recreated environment still reported drift: %s", printed)
	}
}
