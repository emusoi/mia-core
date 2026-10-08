package cli

import (
	"strings"
	"testing"
)

func TestShellInitCoversEachShellWithACdWrapperAndCompletion(t *testing.T) {
	for shell, text := range map[string]string{"zsh": zshInit, "bash": bashInit, "fish": fishInit} {
		for _, must := range []string{"switch", "mia path", "__complete verbs", "__complete names"} {
			if !strings.Contains(text, must) {
				t.Errorf("%s init lacks %q", shell, must)
			}
		}
	}
}
