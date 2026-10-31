package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/signal"
)

const signalUsage = "mia signal <worktree|.> <kind> [text] [--style <style>] [--rank <n>] [--from <source>] [--for <duration>|--until seen]  ·  mia signal <worktree> clear [--from <source>]  ·  mia signal ls"

// cmdSignal is mia signal: what anything outside mia says about a worktree,
// kept and shown as its setter says, with no meaning of mia's own.
func cmdSignal(a *app.App, args []string, asJSON bool) int {
	if len(args) == 1 && (args[0] == "ls" || args[0] == "list") {
		all, err := a.Signals.Load()
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(all)
		}
		for _, one := range all {
			fmt.Printf("%-14s %-12s %-10s %3d  %s\n", one.Worktree, one.Kind, one.From, one.Rank, one.Text)
		}
		return exitOK
	}
	style := takeValue(&args, "--style")
	rankText := takeValue(&args, "--rank")
	from := takeValue(&args, "--from")
	lasts := takeValue(&args, "--for")
	until := takeValue(&args, "--until")
	if len(args) < 2 {
		return usageErr(signalUsage)
	}
	worktree := args[0]
	if worktree == "." {
		worktree = "" // the worktree this runs in: what a hook started there means
	}
	record, err := a.RecordOrHere(worktree)
	if err != nil {
		return fail(err)
	}
	if args[1] == "clear" && len(args) == 2 {
		return exitFor(a.Signals.Clear(record.Name, from))
	}
	one := signal.Signal{Worktree: record.Name, Kind: args[1], Text: strings.Join(args[2:], " "),
		Style: style, Rank: signal.DefaultRank, From: from, At: time.Now()}
	if one.From == "" {
		one.From = "you"
	}
	if rankText != "" {
		if one.Rank, err = strconv.Atoi(rankText); err != nil {
			return usageErr("--rank takes a whole number; lower comes first")
		}
	}
	switch {
	case lasts != "" && until != "":
		return usageErr("a signal lasts --for a while or --until seen, not both")
	case lasts != "":
		d, err := time.ParseDuration(lasts)
		if err != nil || d <= 0 {
			return usageErr("--for takes a duration: 90s, 10m, 2h")
		}
		one.Until = one.At.Add(d)
	case until == "seen":
		one.UntilSeen = true
	case until != "":
		return usageErr("--until takes seen")
	}
	return exitFor(a.Signals.Set(one))
}
