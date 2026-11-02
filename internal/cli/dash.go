package cli

import "github.com/emusoi/mia-core/internal/app"

// cmdDash is the radar under its old name, exactly: an editor that asks for
// a worktree with --pick-to, and a key bound to an old mia dash flag, both
// keep working.
func cmdDash(a *app.App, args []string) int {
	takeValue(&args, "--side")
	take(&args, "--popup")
	take(&args, "--switch")
	radarArgs := []string{}
	if pick := takeValue(&args, "--pick-to"); pick != "" {
		radarArgs = append(radarArgs, "--pick-to", pick)
	}
	if do := takeValue(&args, "--do"); do == "new" || do == "branch" {
		radarArgs = append(radarArgs, "--new")
	}
	return cmdRadar(a, radarArgs)
}
