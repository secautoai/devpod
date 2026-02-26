package db

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// OperationLog records a single devpod command execution.
type OperationLog struct {
	ID         string     `json:"id"`
	Username   string     `json:"username"`
	Action     string     `json:"action"`  // e.g. "create", "delete", "start", "command"
	Target     string     `json:"target"`  // workspace id or provider id
	Args       []string   `json:"args"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	ErrorMsg   string     `json:"errorMsg,omitempty"`
}

// OpLogFilter controls which entries are returned by Query.
type OpLogFilter struct {
	Username string // empty = all users
	Action   string // empty = all actions
	Limit    int    // 0 = default (50)
	Offset   int
}

// OpLogStore is a file-backed append-only operation log.
type OpLogStore struct {
	mu       sync.Mutex
	entries  []*OperationLog
	filePath string
}

func newOpLogStore(filePath string) (*OpLogStore, error) {
	s := &OpLogStore{filePath: filePath}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *OpLogStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.entries)
}

func (s *OpLogStore) save() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0600)
}

// Append writes a new log entry and returns the assigned ID.
func (s *OpLogStore) Append(username, action, target string, args []string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.New().String()
	entry := &OperationLog{
		ID:        id,
		Username:  username,
		Action:    action,
		Target:    target,
		Args:      args,
		StartedAt: time.Now().UTC(),
	}
	s.entries = append(s.entries, entry)
	return id, s.save()
}

// Finish marks an existing log entry as completed.
func (s *OpLogStore) Finish(id string, exitCode int, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.entries {
		if e.ID == id {
			now := time.Now().UTC()
			e.FinishedAt = &now
			e.ExitCode = &exitCode
			e.ErrorMsg = errMsg
			return s.save()
		}
	}
	return nil // entry not found – ignore
}

// Query returns entries matching the filter, sorted newest-first.
func (s *OpLogStore) Query(f OpLogFilter) []*OperationLog {
	s.mu.Lock()
	defer s.mu.Unlock()

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	// Work on a copy sorted newest-first
	sorted := make([]*OperationLog, len(s.entries))
	copy(sorted, s.entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartedAt.After(sorted[j].StartedAt)
	})

	var out []*OperationLog
	for _, e := range sorted {
		if f.Username != "" && e.Username != f.Username {
			continue
		}
		if f.Action != "" && e.Action != f.Action {
			continue
		}
		out = append(out, e)
	}

	// Apply offset and limit
	if f.Offset >= len(out) {
		return []*OperationLog{}
	}
	out = out[f.Offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
