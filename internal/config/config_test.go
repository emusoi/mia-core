package config

import (
	"os"
	"path/filepath"
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
