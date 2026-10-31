package cli

import "testing"

func TestASettersTextCannotBecomeTmuxFormatting(t *testing.T) {
	if got := tmuxStyled("#[fg=red]x #(rm -rf ~)", "red bold"); got != "#[fg=red,bold]##[fg=red]x ##(rm -rf ~)#[default]" {
		t.Errorf("styled = %q", got)
	}
	if got := tmuxStyled("plain", "sparkly"); got != "plain" {
		t.Errorf("a style tmux does not know is left out: %q", got)
	}
	if got := fit("retry   the\n503s with backoff", 10); got != "retry the…" {
		t.Errorf("fit = %q", got)
	}
}
