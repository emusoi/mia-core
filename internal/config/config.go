package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/emusoi/mia-core/internal/env"
)

type Config struct {
	Base string `toml:"base"`

	Prefix string `toml:"prefix"`

	Setup []string `toml:"setup"`

	// Clone names paths (globs allowed) copied from the main checkout into a new worktree
	// before setup runs — node_modules, build outputs, caches — copy-on-write where the disk
	// allows, so a fresh worktree starts built without costing the space twice.
	Clone []string `toml:"clone"`

	// CommitTimes gives a new worktree's tracked files the time of their last commit, not of
	// the checkout, so tools that compare modification times see what the main checkout sees.
	CommitTimes bool `toml:"commit_times"`

	Editor string `toml:"editor"`

	Env env.Settings `toml:"env"`

	Tools env.Tools `toml:"tools"`

	// Dotfiles belong in the user-wide config: a person's own setup, carried
	// to every machine and environment.
	Dotfiles env.Dotfiles `toml:"dotfiles"`

	Services []env.Service `toml:"service"`

	Popup Popup `toml:"popup"`

	Launch map[string]string `toml:"launch"`

	Keys map[string]Keys `toml:"keys"`

	Plugins []string `toml:"plugins"`

	Names []string `toml:"names"`

	Plugin map[string]map[string]any `toml:"plugin"`
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
	Width  string `toml:"width"`
	Height string `toml:"height"`
}

func (p Popup) Size() (width, height string) {
	width, height = "80%", "75%"
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
	if miaDir != "" {
		if err := mergeFile(filepath.Join(miaDir, "config.toml"), &config); err != nil {
			return Config{}, err
		}
	}
	for _, name := range config.Names {
		if !hostname.MatchString(name) {
			return Config{}, fmt.Errorf("names: %q is not a usable name — lowercase letters, digits and hyphens only, since a name becomes a hostname and a tmux session", name)
		}
	}
	return config, nil
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
