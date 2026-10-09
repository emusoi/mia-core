package cli

import (
	"slices"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
)

func TestDashSideTakesOneToFour(t *testing.T) {
	t.Cleanup(func() { startSide = "" })
	if code := cmdDash(&app.App{}, []string{"--side", "5"}); code != exitUsage {
		t.Errorf("--side 5 is a usage error: %d", code)
	}
	startSide = "3"
	if argv := dashArgv("/bin/mia"); !slices.Equal(argv, []string{"/bin/mia", "dash", "--side", "3"}) {
		t.Errorf("the popup passes --side on: %v", argv)
	}
}
