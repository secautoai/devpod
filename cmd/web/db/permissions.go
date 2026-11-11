package db

import (
	"encoding/json"
	"os"
	"sync"
)

// PermissionStore tracks which providers each user is allowed to use.
//
// Semantics:
//   - No entry for a user  → unrestricted (may use any provider)
//   - Entry with []string{}  → deny all providers
//   - Entry with ["docker"]  → may only use the docker provider
type PermissionStore struct {
	mu       sync.RWMutex
	perms    map[string][]string // username → allowed provider IDs (nil = unrestricted)
	filePath string
}

type permissionsFile struct {
	// null value means unrestricted; absent key means unrestricted too
	Entries map[string]*[]string `json:"entries"`
}

func newPermissionStore(filePath string) (*PermissionStore, error) {
	s := &PermissionStore{
		perms:    make(map[string][]string),
		filePath: filePath,
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *PermissionStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var f permissionsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	for username, providers := range f.Entries {
		if providers == nil {
			// explicit unrestricted marker – skip (same as absent)
			continue
		}
		s.perms[username] = *providers
	}
	return nil
}

func (s *PermissionStore) save() error {
	entries := make(map[string]*[]string, len(s.perms))
	for username, providers := range s.perms {
		p := make([]string, len(providers))
		copy(p, providers)
		entries[username] = &p
	}
	data, err := json.MarshalIndent(permissionsFile{Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0600)
}

// SetAllowed replaces the allowlist for a user.
//   - providers == nil  → unrestricted (removes entry)
//   - providers == []string{} → deny all
//   - providers == ["docker", "k8s"] → allowed set
func (s *PermissionStore) SetAllowed(username string, providers []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if providers == nil {
		delete(s.perms, username)
	} else {
		s.perms[username] = providers
	}
	return s.save()
}

// GetAllowed returns nil if the user is unrestricted, or the explicit list.
func (s *PermissionStore) GetAllowed(username string) ([]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, restricted := s.perms[username]
	return p, restricted
}

// IsAllowed returns true if the user may use providerID.
func (s *PermissionStore) IsAllowed(username, providerID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	providers, restricted := s.perms[username]
	if !restricted {
		return true // no entry → unrestricted
	}
	for _, p := range providers {
		if p == providerID {
			return true
		}
	}
	return false
}

// DeleteUser removes all permission entries for a user (call on user deletion).
func (s *PermissionStore) DeleteUser(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.perms, username)
	return s.save()
}
