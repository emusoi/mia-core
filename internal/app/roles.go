package app

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/paths"
)

// A role is a setup with opening instructions: a file of the person's,
// roles/<name>.md in mia's config folder, whose front matter may say the
// agent, model, effort and account, and whose body is the first message
// the agent is sent once it is ready. What the front matter leaves out is
// the default setup's.
//
//	---
//	agent: claude
//	model: opus
//	effort: high
//	---
//	You review. Read the diff against main and say what is wrong.

// Role is one role file, as it is written.
type Role struct {
	Name    string `json:"name"`
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Account string `json:"account,omitempty"`
	Body    string `json:"body"`
	Path    string `json:"path"`
}

// RolePath is where the role of a name is kept, there or not.
func RolePath(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) || strings.TrimSpace(name) != name {
		return "", fmt.Errorf("%q is no role's name — a name is a file's, roles/<name>.md, with no slash", name)
	}
	dir := paths.In("roles")
	if dir == "" {
		return "", errors.New("mia has no config folder: neither XDG_CONFIG_HOME nor HOME is set")
	}
	return filepath.Join(dir, name+".md"), nil
}

// ReadRole is the role of a name.
func ReadRole(name string) (Role, error) {
	path, err := RolePath(name)
	if err != nil {
		return Role{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Role{}, fmt.Errorf("there is no role %s — `mia role path %s` says where to write it", name, name)
	}
	if err != nil {
		return Role{}, err
	}
	return parseRole(name, path, string(data))
}

// Roles is every role, by name, and what is wrong with each that cannot
// be read: one bad file hides no other.
func Roles() ([]Role, []error) {
	dir := paths.In("roles")
	if dir == "" {
		return nil, nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, []error{err}
	}
	var roles []Role
	var problems []error
	for _, file := range files { // Glob sorts them
		role, err := ReadRole(strings.TrimSuffix(filepath.Base(file), ".md"))
		if err != nil {
			problems = append(problems, err)
			continue
		}
		roles = append(roles, role)
	}
	return roles, problems
}

// parseRole reads a role file: front matter between two lines of ---, of
// key: value lines, when it starts with one; the rest is the body.
func parseRole(name, path, text string) (Role, error) {
	role := Role{Name: name, Path: path}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	rest, fronted := strings.CutPrefix(text, "---\n")
	if !fronted {
		role.Body = strings.TrimSpace(text)
		return role, nil
	}
	rest = "\n" + rest // so that front matter of no lines closes too
	front, body, closed := strings.Cut(rest, "\n---\n")
	if !closed {
		if front, closed = strings.CutSuffix(rest, "\n---"); !closed {
			return Role{}, fmt.Errorf("role %s: its front matter has no closing --- line (%s)", name, path)
		}
	}
	front = strings.TrimPrefix(front, "\n")
	fields := map[string]*string{"agent": &role.Agent, "model": &role.Model, "effort": &role.Effort, "account": &role.Account}
	for i, line := range strings.Split(front, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		key, value = strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), `"'`)
		field, known := fields[key]
		switch {
		case !ok:
			return Role{}, fmt.Errorf("role %s: line %d of its front matter is no key: value (%s)", name, i+2, path)
		case !known:
			return Role{}, fmt.Errorf("role %s: its front matter says %s, and a role says only agent, model, effort and account (%s)", name, key, path)
		case strings.ContainsAny(value, " \t"):
			return Role{}, fmt.Errorf("role %s: its %s is %q, and is one word (%s)", name, key, value, path)
		}
		*field = value
	}
	role.Body = strings.TrimSpace(body)
	return role, nil
}

// RoleSetup is the setup a role starts: its own agent, model and effort,
// and what it leaves out the default setup's — the agent named, when one
// is, else the one used most here, else the main agent's. An agent mia
// does not know, or a model or an effort it does not have, is refused.
func (a *App) RoleSetup(role Role, named string) (Setup, error) {
	setup := Setup{Agent: role.Agent, Model: role.Model, Effort: role.Effort, Role: role.Name, Body: role.Body, Account: role.Account}
	if setup.Agent == "" {
		if _, known := agentNamed(a.agents(), named); known {
			setup.Agent = named
		} else if usual := a.usualSetup(); usual.Agent != "" {
			setup.Agent, setup.Model, setup.Effort = usual.Agent, cmp.Or(setup.Model, usual.Model), cmp.Or(setup.Effort, usual.Effort)
		}
	}
	agent, known := agentNamed(a.agents(), setup.Agent)
	if !known {
		return Setup{}, fmt.Errorf("role %s: mia knows no agent %s — `mia api agents` lists those it does", role.Name, setup.Agent)
	}
	if err := allowed(agent, setup.Model, setup.Effort, a.Config.Agents[agent.Name]); err != nil {
		return Setup{}, fmt.Errorf("role %s: %w", role.Name, err)
	}
	return setup, nil
}

// usualSetup is the setup a picker offers first: the one used most here,
// else the main agent's.
func (a *App) usualSetup() Setup {
	repository := ""
	if a.Root != "" {
		repository = filepath.Base(a.Root)
	}
	if used := a.usedSetups(repository); len(used) > 0 {
		return used[0]
	}
	if agent, err := a.mainAgent(); err == nil {
		return Setup{Agent: agent.Name}
	}
	return Setup{}
}

// roleSetups is each role that can be read, as the setup it starts.
func (a *App) roleSetups() []Setup {
	roles, _ := Roles() // `mia role ls` says what is wrong with one that cannot be read
	setups := []Setup{}
	for _, role := range roles {
		if setup, err := a.RoleSetup(role, ""); err == nil {
			setups = append(setups, setup)
		} else {
			setups = append(setups, Setup{Agent: role.Agent, Model: role.Model, Effort: role.Effort, Role: role.Name, Body: role.Body, Account: role.Account})
		}
	}
	return setups
}

// SayOnceReady sends text into a window of an agent just started, once it
// is ready: when it has drawn the same for a second, asks something or
// ends, as mia main start waits. It is pasted, so a line break is not an
// Enter, with Enter after; never into a question, which Enter would
// answer.
func (a *App) SayOnceReady(record model.Record, window, text string) error {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return err
	}
	if said, ended := settle(host, record.Name, window, settleReady); ended {
		return fmt.Errorf("window %s ended as soon as it started, saying:\n%s", window, said)
	}
	return say(host, record.Name, window, a.agentOfWindow(host, record.Name, window), "mia window press "+record.Name+" "+window+" --", text)
}
