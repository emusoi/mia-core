package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Record struct {
	Path string `json:"path"`

	Name string `json:"name"`

	Starred bool `json:"starred,omitempty"`

	Seen time.Time `json:"seen,omitzero"`

	Env *Environment `json:"env,omitempty"`
}

type Environment struct {
	Placement Placement `json:"placement"`

	// Sync is how a box's copy follows this worktree: "" follows live, both
	// ways; "manual" holds changes on each side until `mia env sync <wt> now`.
	Sync string `json:"sync,omitempty"`
}

const SyncManual = "manual"

// Manual reports whether the worktree's box copy syncs only when asked.
func (r Record) Manual() bool { return r.Env != nil && r.Env.Sync == SyncManual }

type Placement struct {
	runtime string
	staging string
}

func Local() Placement { return Placement{} }

func OnRuntime(runtime, staging string) (Placement, error) {
	if runtime == "" {
		return Placement{}, errors.New("a placement on another machine needs the machine's name")
	}
	if staging == "" {
		return Placement{}, fmt.Errorf("a placement on %s needs a staging copy to sync into", runtime)
	}
	return Placement{runtime: runtime, staging: staging}, nil
}

func (p Placement) IsLocal() bool { return p.runtime == "" }

func (p Placement) Runtime() string { return p.runtime }

func (p Placement) Staging() (string, bool) {
	if p.runtime == "" {
		return "", false
	}
	return p.staging, true
}

func (p Placement) String() string {
	if p.runtime == "" {
		return "local"
	}
	return p.runtime
}

type placementJSON struct {
	Runtime string `json:"runtime,omitempty"`
	Staging string `json:"staging,omitempty"`
	Berth   string `json:"berth,omitempty"`
}

func (p Placement) MarshalJSON() ([]byte, error) {
	return marshal(placementJSON{Runtime: p.runtime, Staging: p.staging})
}

func (p *Placement) UnmarshalJSON(data []byte) error {
	var raw placementJSON
	if err := unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Staging == "" {
		raw.Staging = raw.Berth
	}
	if raw.Runtime == "" {
		if raw.Staging != "" {
			return errors.New("a local placement cannot have a staging copy")
		}
		*p = Local()
		return nil
	}
	placed, err := OnRuntime(raw.Runtime, raw.Staging)
	if err != nil {
		return err
	}
	*p = placed
	return nil
}

func (r Record) Validate() error {
	switch {
	case r.Path == "":
		return errors.New("a record with no path is not a record: the path is the identity")
	case !filepath.IsAbs(r.Path):
		return fmt.Errorf("path %q is not absolute; a relative identity means something different from every directory", r.Path)
	case r.Name == "":
		return fmt.Errorf("%s has no name, and a name is how a person and a URL refer to it", r.Path)
	case strings.ContainsAny(r.Name, "/ \t"):
		return fmt.Errorf("name %q must be one word: it becomes a hostname", r.Name)
	}
	return nil
}

func CheckName(name string) error {
	switch {
	case strings.ContainsAny(name, "/ \t"):
		return fmt.Errorf("name %q must be one word: it becomes a hostname", name)
	case strings.ContainsAny(name, ".:"):
		return fmt.Errorf("name %q cannot hold . or : — it names a tmux session, and tmux rewrites both", name)
	}
	return nil
}
