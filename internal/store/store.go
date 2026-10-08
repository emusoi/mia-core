package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/emusoi/mia-core/internal/model"
)

type Store struct {
	Dir string
}

func (s Store) path() string { return filepath.Join(s.Dir, "worktrees.json") }

func (s Store) Load() ([]model.Record, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []model.Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("read %s: %w", s.path(), err)
	}
	for i, record := range records {
		if err := record.Validate(); err != nil {
			return nil, fmt.Errorf("%s: record %d: %w", s.path(), i, err)
		}
	}
	return records, nil
}

func (s Store) Update(change func([]model.Record) ([]model.Record, error)) error {
	unlock, err := lock(s.Dir)
	if err != nil {
		return err
	}
	defer unlock()

	current, err := s.Load()
	if err != nil {
		return err
	}
	next, err := change(current)
	if err != nil {
		return err
	}
	if err := validateSet(next); err != nil {
		return err
	}
	return s.write(next)
}

func (s Store) Put(record model.Record) error {
	return s.Update(func(records []model.Record) ([]model.Record, error) {
		if err := record.Validate(); err != nil {
			return nil, err
		}
		for i := range records {
			if records[i].Path == record.Path {
				records[i] = record
				return records, nil
			}
		}
		return append(records, record), nil
	})
}

func (s Store) Remove(path string) error {
	return s.Update(func(records []model.Record) ([]model.Record, error) {
		kept := records[:0]
		for _, record := range records {
			if record.Path != path {
				kept = append(kept, record)
			}
		}
		return kept, nil
	})
}

func validateSet(records []model.Record) error {
	byPath := make(map[string]bool, len(records))
	byName := make(map[string]string, len(records))
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
		if byPath[record.Path] {
			return fmt.Errorf("two records for %s: a worktree is one identity, so a second record for it is a bug in whatever wrote it", record.Path)
		}
		byPath[record.Path] = true

		if held, taken := byName[record.Name]; taken {
			return fmt.Errorf("%q names both %s and %s, and a name becomes a hostname", record.Name, held, record.Path)
		}
		byName[record.Name] = record.Path
	}
	return nil
}

func (s Store) write(records []model.Record) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.Dir, ".worktrees-*")
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
	return os.Rename(temp.Name(), s.path())
}
