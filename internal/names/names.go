package names

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var pool = []string{
	"kijenge", "lemara", "olmatejo", "ngarenaro", "kisongo", "njiro", "sakina",
	"themi", "sekei", "kaloleni", "elerai", "sombetini", "levolosi", "olasiti",
	"moshono", "muriet", "oloirien", "kimandolu", "baraa", "sinoni", "burka",
	"osunyai", "terrat", "engutoto", "sanawari", "ilboru", "mianzini", "majengo",
	"nduruma", "tengeru", "duluti", "kiranyi", "ngaramtoni", "monduli",

	"kariakoo", "msasani", "mikocheni", "masaki", "upanga", "kinondoni", "ilala",
	"temeke", "ubungo", "sinza", "tabata", "segerea", "kigamboni", "mbezi",
	"tegeta", "kunduchi", "bunju", "goba", "kimara", "manzese", "magomeni",
	"buguruni", "vingunguti", "kurasini", "mbagala", "changombe", "keko", "yombo",
	"mburahati", "mwenge", "kijitonyama", "mlimani", "makumbusho", "tandale",
	"mabibo", "kigogo", "hananasif", "mzimuni", "kipawa", "ukonga", "chanika",
	"kitunda", "pugu", "oysterbay",

	"kilimani", "kileleshwa", "lavington", "westlands", "karen", "langata",
	"parklands", "eastleigh", "kibera", "mathare", "githurai", "kasarani",
	"ruaraka", "embakasi", "donholm", "buruburu", "umoja", "kayole", "dandora",
	"huruma", "pangani", "ngara", "muthaiga", "runda", "gigiri", "loresho",
	"kangemi", "dagoretti", "riruta", "ngong", "hurlingham", "madaraka",
	"highridge", "roysambu", "zimmerman", "kahawa", "komarock", "ruai",
	"kitisuru", "kabete", "uthiru", "kawangware", "jamhuri", "woodley", "otiende",
}

type Store struct {
	Path string
	Own  []string
}

func (s *Store) pool() []string {
	seen := map[string]bool{}
	var names []string
	for _, name := range append(append([]string{}, s.Own...), pool...) {
		if name = strings.ToLower(name); !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

type registry struct {
	Owners map[string]string `json:"owners"`
}

func (s *Store) AllocateFor(derive func(name string) (owner string, usable bool)) (string, error) {
	unlock, err := lock(s.Path)
	if err != nil {
		return "", err
	}
	defer unlock()

	reg, err := s.read()
	if err != nil {
		return "", err
	}
	taken := make(map[string]bool, len(reg.Owners))
	for name := range reg.Owners {
		taken[strings.ToLower(name)] = true
	}
	names := s.pool()
	for attempt := 0; attempt < len(names)*4; attempt++ {
		name := firstFree(taken, names)
		owner, usable := derive(name)
		if !usable {
			taken[name] = true
			continue
		}
		if !filepath.IsAbs(owner) {
			return "", fmt.Errorf("a name is owned by a worktree path, and %q is not absolute", owner)
		}
		reg.Owners[name] = owner
		if err := s.write(reg); err != nil {
			return "", err
		}
		return name, nil
	}
	return "", errors.New("no usable name available")
}

func (s *Store) Allocate(owner string) (string, error) {
	if !filepath.IsAbs(owner) {
		return "", fmt.Errorf("a name is owned by a worktree path, and %q is not absolute", owner)
	}
	unlock, err := lock(s.Path)
	if err != nil {
		return "", err
	}
	defer unlock()

	reg, err := s.read()
	if err != nil {
		return "", err
	}
	for name, held := range reg.Owners {
		if held == owner {
			return name, nil
		}
	}
	name := firstFree(takenIn(reg), s.pool())
	reg.Owners[name] = owner
	if err := s.write(reg); err != nil {
		return "", err
	}
	return name, nil
}

func (s *Store) Release(owner string) error {
	unlock, err := lock(s.Path)
	if err != nil {
		return err
	}
	defer unlock()

	reg, err := s.read()
	if err != nil {
		return err
	}
	for name, held := range reg.Owners {
		if held == owner {
			delete(reg.Owners, name)
			return s.write(reg)
		}
	}
	return nil
}

func (s *Store) Owner(name string) (string, bool) {
	reg, err := s.read()
	if err != nil {
		return "", false
	}
	owner, ok := reg.Owners[name]
	return owner, ok
}

func (s *Store) All() (map[string]string, error) {
	reg, err := s.read()
	if err != nil {
		return nil, err
	}
	return reg.Owners, nil
}

func firstFree(taken map[string]bool, pool []string) string {
	for _, name := range pool {
		if !taken[name] {
			return name
		}
	}
	for round := 2; ; round++ {
		for _, name := range pool {
			candidate := name + "-" + strconv.Itoa(round)
			if !taken[candidate] {
				return candidate
			}
		}
	}
}

func (s *Store) read() (registry, error) {
	reg := registry{Owners: map[string]string{}}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return reg, nil
	}
	if err != nil {
		return registry{}, err
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return registry{}, fmt.Errorf("read %s: %w", s.Path, err)
	}
	if reg.Owners == nil {
		reg.Owners = map[string]string{}
	}
	return reg, nil
}

func (s *Store) write(reg registry) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	keys := make([]string, 0, len(reg.Owners))
	for name := range reg.Owners {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(keys))
	for _, name := range keys {
		ordered[name] = reg.Owners[name]
	}

	data, err := json.MarshalIndent(registry{Owners: ordered}, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.Path), ".names-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.Path)
}

func takenIn(reg registry) map[string]bool {
	taken := make(map[string]bool, len(reg.Owners))
	for name := range reg.Owners {
		taken[strings.ToLower(name)] = true
	}
	return taken
}

func (s *Store) Claim(name, owner string) error {
	if !filepath.IsAbs(owner) {
		return fmt.Errorf("a name is owned by a worktree path, and %q is not absolute", owner)
	}
	unlock, err := lock(s.Path)
	if err != nil {
		return err
	}
	defer unlock()

	reg, err := s.read()
	if err != nil {
		return err
	}
	for existing, held := range reg.Owners {
		if !strings.EqualFold(existing, name) || held == owner {
			continue
		}
		if _, err := os.Stat(held); errors.Is(err, fs.ErrNotExist) {
			delete(reg.Owners, existing)
			continue
		}
		return fmt.Errorf("%s is already the name of %s", existing, held)
	}
	for existing, held := range reg.Owners {
		if held == owner && existing != name {
			return fmt.Errorf("%s is already called %s", owner, existing)
		}
	}
	reg.Owners[name] = owner
	return s.write(reg)
}
