package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/emusoi/mia-core/internal/env"
)

type Config struct {
	Base string `toml:"base" json:"base,omitempty"`

	Prefix string `toml:"prefix" json:"prefix,omitempty"`

	Setup []string `toml:"setup" json:"setup,omitempty"`

	// Clone names paths (globs allowed) copied from the main checkout into a new worktree
	// before setup runs — node_modules, build outputs, caches — copy-on-write where the disk
	// allows, so a fresh worktree starts built without costing the space twice.
	Clone []string `toml:"clone" json:"clone,omitempty"`

	// CommitTimes gives a new worktree's tracked files the time of their last commit, not of
	// the checkout, so tools that compare modification times see what the main checkout sees.
	CommitTimes bool `toml:"commit_times" json:"commit_times,omitempty"`

	Editor string `toml:"editor" json:"editor,omitempty"`

	Env env.Settings `toml:"env" json:"env"`

	Tools env.Tools `toml:"tools" json:"tools"`

	// Dotfiles belong in the user-wide config: a person's own setup, carried
	// to every machine and environment.
	Dotfiles env.Dotfiles `toml:"dotfiles" json:"dotfiles"`

	Services []env.Service `toml:"service" json:"service,omitempty"`

	Popup Popup `toml:"popup" json:"popup"`

	Launch map[string]string `toml:"launch" json:"launch,omitempty"`

	Keys map[string]Keys `toml:"keys" json:"keys,omitempty"`

	Plugins []string `toml:"plugins" json:"plugins,omitempty"`

	Names []string `toml:"names" json:"names,omitempty"`

	Plugin map[string]map[string]any `toml:"plugin" json:"plugin,omitempty"`
}

type Keys []string

func (k *Keys) UnmarshalTOML(value any) error {
	switch v := value.(type) {
	case string:
		*k = Keys{v}
	case []any:
		for _, one := range v {
			if text, ok := one.(string); ok {
				*k = append(*k, text)
			}
		}
	default:
		return fmt.Errorf("a key binding is a string or a list of strings, not %T", value)
	}
	return nil
}

func (c Config) KeyMap() map[string][]string {
	out := map[string][]string{}
	for name, keys := range c.Keys {
		out[name] = keys
	}
	return out
}

func (c Config) Launches() []string {
	names := make([]string, 0, len(c.Launch))
	for name := range c.Launch {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type Popup struct {
	Width  string `toml:"width" json:"width,omitempty"`
	Height string `toml:"height" json:"height,omitempty"`
}

func (p Popup) Size() (width, height string) {
	width, height = "100%", "100%"
	if p.Width != "" {
		width = p.Width
	}
	if p.Height != "" {
		height = p.Height
	}
	return width, height
}

func Load(miaDir string) (Config, error) {
	var config Config
	if global := GlobalPath(); global != "" {
		if err := mergeFile(global, &config); err != nil {
			return Config{}, err
		}
	}
	if project := ProjectPath(miaDir); project != "" {
		if err := mergeFile(project, &config); err != nil {
			return Config{}, err
		}
	}
	if miaDir != "" {
		if err := mergeFile(filepath.Join(miaDir, "config.toml"), &config); err != nil {
			return Config{}, err
		}
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// ProjectPath names the committed configuration in the main checkout. A
// repository has one effective configuration, whichever worktree reads it.
func ProjectPath(miaDir string) string {
	if miaDir == "" || filepath.Base(filepath.Dir(miaDir)) != ".git" {
		return ""
	}
	return filepath.Join(filepath.Dir(filepath.Dir(miaDir)), ".mia.toml")
}

func (c Config) Validate() error {
	for _, name := range c.Names {
		if !hostname.MatchString(name) {
			return fmt.Errorf("names: %q is not a usable name — lowercase letters, digits and hyphens only, since a name becomes a hostname and a tmux session", name)
		}
	}
	seen := map[string]bool{}
	if c.Env.Session != "" && c.Env.Session != "local" && c.Env.Session != "runtime" {
		return fmt.Errorf("env.session is \"local\" or \"runtime\", not %q", c.Env.Session)
	}
	for _, pattern := range c.Env.SyncIgnore {
		if pattern == "" || filepath.IsAbs(pattern) || pattern == ".." || strings.HasPrefix(pattern, "../") {
			return fmt.Errorf("env.sync_ignore: %q must be a relative path or glob inside the worktree", pattern)
		}
		if _, err := path.Match(pattern, "probe"); err != nil {
			return fmt.Errorf("env.sync_ignore: %q is not a valid glob: %w", pattern, err)
		}
	}
	for _, service := range c.Services {
		if err := service.Validate(); err != nil {
			return err
		}
		if seen[service.ID] {
			return fmt.Errorf("service %s: declared more than once", service.ID)
		}
		seen[service.ID] = true
	}
	return nil
}

var hostname = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func mergeFile(path string, into *Config) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	meta, err := toml.Decode(string(data), into)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, key := range meta.Undecoded() {
		if len(key) > 2 && key[0] == "plugin" {
			continue
		}
		return fmt.Errorf("%s: unknown setting %q — mia would have ignored it silently", path, key.String())
	}
	return nil
}

func (c Config) EditorCommand() string {
	for _, candidate := range []string{c.Editor, os.Getenv("EDITOR"), os.Getenv("VISUAL")} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}
