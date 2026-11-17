package cli

import (
	"io"
	"os"
)

// zshIntegration marks each prompt and each command's output for the
// terminal (OSC 133: A where the prompt starts, B where it ends, C where
// the output starts, D with the command's exit status) and says the folder
// (OSC 7), so tmux's next-prompt and previous-prompt jump between commands.
// A and B are in the prompt itself, on the line it is drawn on; a prompt
// a theme draws anew each time gets them again. Sourced twice it adds
// nothing; in a shell with no terminal it says nothing.
const zshIntegration = `# mia shell integration: prompt marks (OSC 133) and the folder (OSC 7)
if [[ -o interactive && -t 1 && -z ${_mia_shell_integration-} ]]; then
  typeset -g _mia_shell_integration=1 _mia_ran=
  _mia_precmd() {
    local s=$?
    [[ -n $_mia_ran ]] && printf '\e]133;D;%s\e\\' $s
    _mia_ran=
    printf '\e]7;file://%s%s\e\\' "$HOST" "${PWD// /%20}"
    [[ $PS1 == *$'\e]133;A'* ]] || PS1=$'%{\e]133;A\e\\%}'$PS1$'%{\e]133;B\e\\%}'
  }
  _mia_preexec() { _mia_ran=1; printf '\e]133;C\e\\'; }
  precmd_functions=(_mia_precmd ${precmd_functions:#_mia_precmd})
  preexec_functions+=(_mia_preexec)
fi
`

func cmdShellIntegration(args []string) int {
	if len(args) != 1 || args[0] != "zsh" {
		return usageErr(shellIntegrationUsage)
	}
	_, _ = io.WriteString(os.Stdout, zshIntegration)
	return exitOK
}

const shellIntegrationUsage = "mia shell-integration zsh   — then: eval \"$(mia shell-integration zsh)\" in your zshrc"
