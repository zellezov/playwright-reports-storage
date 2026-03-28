package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"prs/internal/model"
)

// Store manages metadata JSON files and in-memory status counters.
type Store struct {
	mu       sync.RWMutex
	dataDir  string
	counters map[model.Status]int
}

// New creates a Store rooted at dataDir/metadata.
func New(dataDir string) *Store {
	return &Store{
		dataDir:  dataDir,
		counters: make(map[model.Status]int),
	}
}

// metadataDir returns the sharded directory for a report ID.
func (s *Store) metadataDir(id string) string {
	shard := id[:2]
	return filepath.Join(s.dataDir, "metadata", shard)
}

// metadataPath returns the path of the JSON file for id.
func (s *Store) metadataPath(id string) string {
	return filepath.Join(s.metadataDir(id), id+".json")
}

// Write persists r atomically and updates in-memory counters.
// It must be called for every status change.
func (s *Store) Write(r *model.Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.metadataDir(r.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil { // rwxr-xr-x: owner full, group+others read+execute
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	tmpPath := s.metadataPath(r.ID) + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	if err := json.NewEncoder(f).Encode(r); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("encode: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close tmp: %w", err)
	}

	// Read the current on-disk record (if any) so we can decrement its status
	// counter before incrementing the new one — keeping counters consistent.
	existing, _ := s.readLocked(r.ID)
	if existing != nil {
		s.counters[existing.Status]--
	}
	s.counters[r.Status]++

	if err := os.Rename(tmpPath, s.metadataPath(r.ID)); err != nil {
		// Rename failed: roll back the counter changes we just applied.
		s.counters[r.Status]--
		if existing != nil {
			s.counters[existing.Status]++
		}
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// Read returns the metadata for id, or an error if not found.
func (s *Store) Read(id string) (*model.Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readLocked(id)
}

// readLocked reads from disk without acquiring the mutex (caller must hold it).
func (s *Store) readLocked(id string) (*model.Report, error) {
	f, err := os.Open(s.metadataPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var r model.Report
	if err := json.NewDecoder(f).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode %s: %w", id, err)
	}
	return &r, nil
}

// Delete removes the metadata file for id and decrements its counter.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, err := s.readLocked(id)
	if err != nil {
		return err
	}
	if r == nil {
		return nil // already gone
	}

	if err := os.Remove(s.metadataPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.counters[r.Status]--
	return nil
}

// List returns all metadata records found on disk.
// It rebuilds nothing in memory; use RebuildCounters after a full scan.
func (s *Store) List() ([]*model.Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metadataRoot := filepath.Join(s.dataDir, "metadata")
	var reports []*model.Report

	shards, err := os.ReadDir(metadataRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	for _, shard := range shards {
		if !shard.IsDir() {
			continue
		}
		shardDir := filepath.Join(metadataRoot, shard.Name())
		entries, err := os.ReadDir(shardDir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if filepath.Ext(e.Name()) != ".json" {
				continue
			}
			id := e.Name()[:len(e.Name())-5] // strip .json
			r, err := s.readLocked(id)
			if err != nil || r == nil {
				continue
			}
			reports = append(reports, r)
		}
	}
	return reports, nil
}

// RebuildCounters replaces in-memory counters from a fresh disk scan.
// Must be called during startup before any concurrent access.
func (s *Store) RebuildCounters(reports []*model.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters = make(map[model.Status]int)
	for _, r := range reports {
		s.counters[r.Status]++
	}
}

// Counters returns a snapshot of the current status counters.
func (s *Store) Counters() map[model.Status]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[model.Status]int, len(s.counters))
	for k, v := range s.counters {
		out[k] = v
	}
	return out
}
