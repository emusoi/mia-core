package cli

import (
	"strings"
	"testing"
)

func TestShellInitCoversEachShellWithACdWrapperAndCompletion(t *testing.T) {
	for shell, text := range map[string]string{"zsh": zshInit, "bash": bashInit, "fish": fishInit} {
		for _, must := range []string{"cd)", "mia path", "__complete verbs", "__complete names"} {
			if !strings.Contains(strings.Replace(text, "case cd", "cd)", 1), must) {
				t.Errorf("%s init lacks %q", shell, must)
			}
		}
		if strings.Contains(text, "switch|") || strings.Contains(text, "switch sw") || strings.Contains(text, "|sw|") {
			t.Errorf("%s init still spells mia switch: %s", shell, text)
		}
	}
}
