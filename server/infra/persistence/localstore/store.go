// Package localstore provides a file-based JSON store that mirrors S3 key layout.
// Used for local development only — NEVER deployed.
//
// S3 key → local file path:
//   "users/{id}.json"  →  ~/.samams/store/users/{id}.json
//   "tasks/{id}.json"  →  ~/.samams/store/tasks/{id}.json
//
// All operations are JSON marshal/unmarshal to plain files.
package localstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store is a file-based JSON store that mirrors S3 key layout.
type Store struct {
	mu      sync.RWMutex
	baseDir string
}

// New creates a new localstore rooted at baseDir.
// If baseDir is empty, defaults to ~/.samams/store.
func New(baseDir string) (*Store, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("user home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".samams", "store")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	return &Store{baseDir: baseDir}, nil
}

// Put writes a JSON object to the given key path.
// Uses atomic write (temp file + rename) to prevent corruption on crash.
func (s *Store) Put(key string, data any) error {
	// Marshal outside the lock for better performance.
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.resolve(key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	// Atomic: write to temp, then rename.
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, b, 0644); err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename %s: %w", key, err)
	}
	return nil
}

// Get reads a JSON object from the given key path into dst.
func (s *Store) Get(key string, dst any) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.resolve(key)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("read %s: %w", key, err)
	}
	return json.Unmarshal(b, dst)
}

// Delete removes the file at the given key path.
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.resolve(key)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// List returns all keys under the given prefix.
func (s *Store) List(prefix string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.resolve(prefix)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	var keys []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".json") {
			return nil
		}
		// Convert absolute path back to key.
		rel, _ := filepath.Rel(s.baseDir, path)
		keys = append(keys, filepath.ToSlash(rel))
		return nil
	})
	return keys, err
}

// Exists returns true if the key exists.
func (s *Store) Exists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, err := os.Stat(s.resolve(key))
	return err == nil
}

func (s *Store) resolve(key string) string {
	return filepath.Join(s.baseDir, filepath.FromSlash(key))
}

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = fmt.Errorf("not found")
