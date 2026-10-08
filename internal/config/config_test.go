package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheExampleConfigLoads(t *testing.T) {
	var c Config
	if err := mergeFile("../../examples/config.toml", &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Launch) == 0 || len(c.Services) != 1 {
		t.Errorf("the example lost its launches or service: %+v", c)
	}
}

func TestAPluginsOwnSettingsAreItsToCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[plugin.plan.checks]\nok = [\"true\"]\n\n[plugin.gh]\ntoken_env = \"GH\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := mergeFile(path, &c); err != nil {
		t.Fatal(err)
	}
	checks, _ := c.Plugin["plan"]["checks"].(map[string]any)
	if len(checks) != 1 || c.Plugin["gh"]["token_env"] != "GH" {
		t.Errorf("plugin settings = %#v", c.Plugin)
	}
	if err := os.WriteFile(path, []byte("[plugins]\nnope = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mergeFile(path, &Config{}); err == nil {
		t.Error("an unknown core setting must still be refused")
	}
}

func TestYourOwnNamesMustBeUsableAsHostnames(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	write := func(body string) error {
		if err := os.MkdirAll(filepath.Join(dir, "mia"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "mia", "config.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load("")
		return err
	}
	if err := write(`names = ["brooklyn", "camden-town", "k2"]`); err != nil {
		t.Errorf("good names were refused: %v", err)
	}
	for _, bad := range []string{`"Brooklyn"`, `"st. pauli"`, `"a:b"`, `"-x"`} {
		if err := write(`names = [` + bad + `]`); err == nil || !strings.Contains(err.Error(), "hostname") {
			t.Errorf("names = [%s] was accepted: %v", bad, err)
		}
	}
}
