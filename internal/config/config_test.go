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

func TestCommittedProjectConfigLayersBetweenGlobalAndLocal(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	root := t.TempDir()
	miaDir := filepath.Join(root, ".git", "mia")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(xdg, "mia", "config.toml"), "base = \"global\"\nprefix = \"global/\"\neditor = \"global-editor\"\n")
	write(filepath.Join(root, ".mia.toml"), "base = \"project\"\nprefix = \"project/\"\n")
	write(filepath.Join(miaDir, "config.toml"), "prefix = \"local/\"\n")

	got, err := Load(miaDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "project" || got.Prefix != "local/" || got.Editor != "global-editor" {
		t.Fatalf("precedence was not global < project < local: %+v", got)
	}
	if path := ProjectPath(miaDir); path != filepath.Join(root, ".mia.toml") {
		t.Fatalf("project path = %q", path)
	}
}

func TestServiceConfigurationIsValidatedOnLoad(t *testing.T) {
	for _, check := range []struct {
		name, body, want string
	}{
		{"duplicate", "[[service]]\nid = \"web\"\nrun = [\"true\"]\n[[service]]\nid = \"web\"\nrun = [\"true\"]\n", "more than once"},
		{"missing run", "[[service]]\nid = \"web\"\n", "run is required"},
		{"bad restart", "[[service]]\nid = \"web\"\nrun = [\"true\"]\nrestart = \"sometimes\"\n", "on-failure"},
	} {
		t.Run(check.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			miaDir := filepath.Join(t.TempDir(), ".git", "mia")
			if err := os.MkdirAll(miaDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(miaDir, "config.toml"), []byte(check.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(miaDir); err == nil || !strings.Contains(err.Error(), check.want) {
				t.Fatalf("error = %v, want %q", err, check.want)
			}
		})
	}
}
