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

	// Keep names programs a pane keeps after they end, as it keeps an agent:
	// "nvim", "npm run dev". mia resurrect starts them again. Editors and
	// dev servers are kept already; a program that finishes — a build, a
	// push — must not be named here, or it runs again.
	Keep []string `toml:"keep" json:"keep,omitempty"`

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

	// Menu names the plugins' verbs prefix m offers, in order: type, quiz.
	// Every verb stays one word away in the radar; the menu holds a few.
	Menu []string `toml:"menu" json:"menu,omitempty"`

	Names []string `toml:"names" json:"names,omitempty"`

	Plugin map[string]map[string]any `toml:"plugin" json:"plugin,omitempty"`

	// Main is the main agent's: a person has one, so it belongs in the
	// user-wide config.
	Main Main `toml:"main" json:"main"`

	// Agents is each coding agent's own, by its name: [agents.claude].
	Agents map[string]Agent `toml:"agents" json:"agents,omitempty"`
}

// Agent is what an agent is started with when mia starts it and nothing
// else is said: a model and an effort, each from those it has.
type Agent struct {
	Model  string `toml:"model" json:"model,omitempty"`
	Effort string `toml:"effort" json:"effort,omitempty"`
}

type Main struct {
	// Agent is which agent the main one is; claude when empty.
	Agent string `toml:"agent" json:"agent,omitempty"`
	// Machine is the machine it lives on, by the name `mia api machines`
	// gives it; this one when empty.
	Machine string `toml:"machine" json:"machine,omitempty"`
	// Model and Effort it is started with; its agent's own in [agents]
	// when empty.
	Model  string `toml:"model" json:"model,omitempty"`
	Effort string `toml:"effort" json:"effort,omitempty"`
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
	if err := c.Env.Compose.Validate(); err != nil {
		return err
	}
	if c.Env.Compose.Enabled() {
		switch {
		case c.Env.Image != "" || len(c.Env.ImageSetup) > 0:
			return fmt.Errorf("env.compose owns images; remove env.image and env.image_setup")
		case len(c.Env.ContainerOnly) > 0 || len(c.Env.Cache) > 0:
			return fmt.Errorf("env.compose owns volumes; remove env.container_only and env.cache")
		case len(c.Env.Pass) > 0:
			return fmt.Errorf("env.compose owns service environment; remove env.pass and use the compose file")
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
	if err := decode(string(data), into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Check says whether text is a config mia would load, as Load reads one.
func Check(text string) error {
	var c Config
	return decode(text, &c)
}

func decode(text string, into *Config) error {
	meta, err := toml.Decode(text, into)
	if err != nil {
		return err
	}
	for _, key := range meta.Undecoded() {
		if len(key) > 2 && key[0] == "plugin" {
			continue
		}
		return fmt.Errorf("unknown setting %q — mia would have ignored it silently", key.String())
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
