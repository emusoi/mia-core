package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/radar"
	"github.com/emusoi/mia-core/internal/run"
)

// cmdRadar is mia radar [--new]: the popup that shows and finds everything,
// or straight to making a new worktree. What it picks runs once it closes.
func cmdRadar(a *app.App, args []string) int {
	newOnly := take(&args, "--new")
	data, err := radarData(a)
	if err != nil {
		return fail(err)
	}
	if a.PollDue(pollEvery) {
		pollInBackground(a.Root)
	}
	choice, err := radar.Run(data, newOnly)
	if err != nil || choice == nil {
		return exitFor(err)
	}
	return exitFor(carryOut(a, *choice))
}

func radarData(a *app.App) (radar.Data, error) {
	local, err := a.List()
	if err != nil {
		return radar.Data{}, err
	}
	data := radar.Data{Repo: filepath.Base(a.Root), Root: a.Root, MiaDir: a.MiaDir, Base: a.BaseBranch(a.Root),
		Prefix: a.Config.Prefix, Local: local, Machines: a.KnownMachines()}
	if record, err := a.RecordOrHere(""); err == nil {
		data.Here, data.HereDir = record.Name, record.Path
		data.Branch = git.CurrentBranch(record.Path)
	}
	data.Away = readNote(a)
	if _, err := os.Stat(filepath.Join(a.MiaDir, "config.toml")); os.IsNotExist(err) {
		data.First = &radar.Guess{Base: data.Base, Setup: radar.GuessSetup(a.Root)}
	}
	return data, nil
}

// carryOut does what the radar picked: a mia command, then the switch.
func carryOut(a *app.App, choice radar.Choice) error {
	if len(choice.Argv) > 0 {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		cmd := exec.Command(self, choice.Argv...)
		cmd.Dir = choice.Dir
		if cmd.Dir == "" {
			cmd.Dir = a.Root
		}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	switch {
	case choice.Return != "":
		return goBack(choice.Return)
	case choice.Remote != "":
		if os.Getenv("TMUX") == "" {
			return cmdErr(cmdAttachOn(a, []string{choice.Remote}))
		}
		return handOver(a, choice.Remote)
	case choice.Name != "":
		os.Unsetenv("TMUX_PANE") // from a popup, switch the client rather than nest
		return a.Shell(choice.Name, false)
	}
	return nil
}

// handOver gives this terminal to the machine's tmux: the client here
// detaches and ssh attaches the worktree's session there, so every tmux key
// — a new window, a split — acts on that machine, as if it were this one.
// Detaching there brings back the session this terminal was in.
func handOver(a *app.App, target string) error {
	_, on, _ := strings.Cut(target, "@")
	if _, err := a.Runtimes.Get(on); err != nil {
		return err
	}
	back, err := run.Local("tmux").Capture("display-message", "-p", "#{socket_path}\t#{session_name}")
	if err != nil {
		return err
	}
	socket, session, _ := strings.Cut(strings.TrimSpace(back), "\t")
	self, err := os.Executable()
	if err != nil {
		return err
	}
	away := "cd " + run.ShellJoin([]string{a.Root}) + " && " + run.ShellJoin([]string{self, "away", target, session, "--back", socket})
	out, err := run.Local("tmux").Combined("detach-client", "-E", away)
	if err != nil {
		return fmt.Errorf("hand over to %s: %s", on, strings.TrimSpace(out))
	}
	return nil
}

func cmdErr(code int) error {
	if code != exitOK {
		return fmt.Errorf("exit %d", code)
	}
	return nil
}

const tmuxMarker = "# mia: status line and popups"

// tmuxSnippet is what mia adds to a tmux config: its part of the status
// line, refreshed every 5s, and the two popups.
func tmuxSnippet(self string) string {
	mia := run.ShellJoin([]string{self})
	return tmuxMarker + ` — mia tmux uninstall removes this block
set -g status-interval 5
set -g status-right-length 160
set -ga status-right ' #(` + mia + ` status-line "#{pane_current_path}") '
bind Space display-popup -E -d "#{pane_current_path}" -w 110 -h 30 -T " mia " "` + mia + ` radar"
bind n display-popup -E -d "#{pane_current_path}" -w 110 -h 14 -T " new worktree " "` + mia + ` radar --new"
# mia: end
`
}

// cmdTmux is mia tmux [install|uninstall]: print, add or remove mia's part
// of ~/.tmux.conf. install shows what it adds before it adds it.
func cmdTmux(args []string) int {
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	snippet := tmuxSnippet(self)
	if len(args) == 0 {
		fmt.Print(snippet)
		return exitOK
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fail(err)
	}
	conf := filepath.Join(home, ".tmux.conf")
	current, err := os.ReadFile(conf)
	if err != nil && !os.IsNotExist(err) {
		return fail(err)
	}
	kept := withoutBlock(string(current))
	switch args[0] {
	case "install":
		fmt.Printf("adding to %s:\n\n%s\n", conf, snippet)
		if kept != "" && !strings.HasSuffix(kept, "\n") {
			kept += "\n"
		}
		kept += "\n" + snippet
	case "uninstall":
		fmt.Printf("removing mia's block from %s\n", conf)
	default:
		return usageErr("mia tmux [install|uninstall]")
	}
	if err := os.WriteFile(conf, []byte(kept), 0o644); err != nil {
		return fail(err)
	}
	if os.Getenv("TMUX") != "" {
		if out, err := run.Local("tmux").Combined("source-file", conf); err != nil {
			return fail(fmt.Errorf("tmux source-file %s: %s", conf, strings.TrimSpace(out)))
		}
		fmt.Println("tmux has read it: prefix space opens the radar, prefix n a new worktree")
	}
	return exitOK
}

// withoutBlock is the config with mia's block taken out, whatever else is
// in it kept as it was.
func withoutBlock(conf string) string {
	start := strings.Index(conf, tmuxMarker)
	if start < 0 {
		return conf
	}
	end := strings.Index(conf[start:], "# mia: end")
	if end < 0 {
		return conf
	}
	end += start + len("# mia: end")
	if end < len(conf) && conf[end] == '\n' {
		end++
	}
	return strings.TrimRight(conf[:start], "\n") + "\n" + conf[end:]
}
