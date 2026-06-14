// Package state persists the mapping of worktree path -> database name under the
// repo's shared git dir (.git/wtdb/state.json), so prune knows which databases
// wtdb created and can safely drop orphans.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Entry records one worktree's provisioned database.
type Entry struct {
	WorktreePath string `json:"worktreePath"`
	DBName       string `json:"dbName"`
	SourceDB     string `json:"sourceDb"`
}

// State is the on-disk registry.
type State struct {
	path    string
	Entries []Entry `json:"entries"`
}

// pathFor returns .git/wtdb/state.json given the shared git common dir.
func pathFor(commonDir string) string {
	return filepath.Join(commonDir, "wtdb", "state.json")
}

// Load reads the registry from the repo's common git dir, returning an empty
// state if none exists yet.
func Load(commonDir string) (*State, error) {
	p := pathFor(commonDir)
	s := &State{path: p}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Get returns the entry for a worktree path, if present.
func (s *State) Get(worktreePath string) (Entry, bool) {
	for _, e := range s.Entries {
		if e.WorktreePath == worktreePath {
			return e, true
		}
	}
	return Entry{}, false
}

// Upsert inserts or replaces the entry for its worktree path.
func (s *State) Upsert(e Entry) {
	for i := range s.Entries {
		if s.Entries[i].WorktreePath == e.WorktreePath {
			s.Entries[i] = e
			return
		}
	}
	s.Entries = append(s.Entries, e)
}

// Remove deletes the entry for a worktree path.
func (s *State) Remove(worktreePath string) {
	out := s.Entries[:0]
	for _, e := range s.Entries {
		if e.WorktreePath != worktreePath {
			out = append(out, e)
		}
	}
	s.Entries = out
}

// Save writes the registry back to disk.
func (s *State) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
