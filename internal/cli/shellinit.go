package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
)

const zshInit = `mia() {
  case "$1" in
    switch|sw|cd)
      local __mia_dir
      __mia_dir="$(command mia path "${@:2}")" && cd "$__mia_dir" ;;
    *) command mia "$@" ;;
  esac
}
_mia() {
  local -a words_
  if (( CURRENT == 2 )); then
    words_=(${(f)"$(command mia __complete verbs 2>/dev/null)"})
    _describe 'mia' words_
  else
    words_=(${(f)"$(command mia __complete names 2>/dev/null)"})
    _describe 'worktree' words_
  fi
}
if (( ${+_comps} )); then compdef _mia mia; fi
`

const bashInit = `mia() {
  case "$1" in
    switch|sw|cd)
      local dir
      dir="$(command mia path "${@:2}")" && cd "$dir" ;;
    *) command mia "$@" ;;
  esac
}
_mia() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  if [ "$COMP_CWORD" -eq 1 ]; then
    COMPREPLY=($(compgen -W "$(command mia __complete verbs 2>/dev/null)" -- "$cur"))
  else
    COMPREPLY=($(compgen -W "$(command mia __complete names 2>/dev/null)" -- "$cur"))
  fi
}
complete -F _mia mia
`

const fishInit = `function mia
  switch "$argv[1]"
    case switch sw cd
      set -l dir (command mia path $argv[2..-1]); and cd $dir
    case '*'
      command mia $argv
  end
end
complete -c mia -f -n '__fish_use_subcommand' -a '(command mia __complete verbs)'
complete -c mia -f -n 'not __fish_use_subcommand' -a '(command mia __complete names)'
`

func cmdShellInit(args []string) int {
	shell := ""
	if len(args) > 0 {
		shell = args[0]
	} else if base := os.Getenv("SHELL"); base != "" {
		shell = base[strings.LastIndex(base, "/")+1:]
	}
	switch shell {
	case "zsh":
		fmt.Print(zshInit)
	case "bash":
		fmt.Print(bashInit)
	case "fish":
		fmt.Print(fishInit)
	default:
		return usageErr("mia shell-init <zsh|bash|fish>   — then: eval \"$(mia shell-init zsh)\" in your rc file")
	}
	return exitOK
}

func cmdComplete(a *app.App, args []string) int {
	if len(args) == 0 {
		return exitUsage
	}
	switch args[0] {
	case "verbs":
		for _, verb := range Verbs() {
			fmt.Println(verb.Name)
		}
		fmt.Println("switch")
		return exitOK
	case "names":
		listings, err := a.List()
		if err != nil {
			return fail(err)
		}
		for _, l := range listings {
			fmt.Println(l.Name)
			if l.Branch != "" {
				fmt.Println(l.Branch)
			}
		}
		return exitOK
	}
	return exitUsage
}
