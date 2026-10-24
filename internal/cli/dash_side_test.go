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

func TestSideTakesANumberOrAPlace(t *testing.T) {
	for _, args := range [][]string{nil, {"5"}, {"middle"}, {"1", "2"}} {
		if code := cmdSide(&app.App{}, args); code != exitUsage {
			t.Errorf("mia side %v is a usage error: %d", args, code)
		}
	}
}
