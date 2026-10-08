package paths

import (
	"os"
	"path/filepath"
)

func Dir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mia")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "mia")
}

func In(parts ...string) string {
	dir := Dir()
	if dir == "" {
		return ""
	}
	return filepath.Join(append([]string{dir}, parts...)...)
}
