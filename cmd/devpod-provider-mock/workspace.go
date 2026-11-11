package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WorkspaceState mirrors the states devpod expects.
type WorkspaceState string

const (
	StateNotFound WorkspaceState = "NotFound"
	StateRunning  WorkspaceState = "Running"
	StateStopped  WorkspaceState = "Stopped"
)

// WorkspaceRecord is persisted to disk as JSON.
type WorkspaceRecord struct {
	ID    string         `json:"id"`
	State WorkspaceState `json:"state"`
}

// stateDir returns the directory where workspace JSON files are stored.
func stateDir() string {
	if d := os.Getenv("MOCK_PROVIDER_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/devpod-mock"
	}
	return filepath.Join(home, ".devpod-mock", "workspaces")
}

func workspacePath(id string) string {
	return filepath.Join(stateDir(), id+".json")
}

// load reads a workspace record from disk. Returns NotFound if absent.
func load(id string) (*WorkspaceRecord, error) {
	data, err := os.ReadFile(workspacePath(id))
	if os.IsNotExist(err) {
		return &WorkspaceRecord{ID: id, State: StateNotFound}, nil
	}
	if err != nil {
		return nil, err
	}
	var r WorkspaceRecord
	return &r, json.Unmarshal(data, &r)
}

// persist saves a workspace record to disk.
func persist(r *WorkspaceRecord) error {
	if err := os.MkdirAll(stateDir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(workspacePath(r.ID), data, 0600)
}

// remove deletes a workspace record from disk.
func remove(id string) error {
	err := os.Remove(workspacePath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// logLine emits a devpod-format JSON log line to stdout.
func logLine(level, msg string) {
	type logEntry struct {
		Time    string `json:"time"`
		Level   string `json:"level"`
		Message string `json:"message"`
	}
	data, _ := json.Marshal(logEntry{Level: level, Message: msg})
	fmt.Println(string(data))
}

// workspaceID returns the workspace id from WORKSPACE_ID env (set by devpod).
func workspaceID() string {
	if id := os.Getenv("WORKSPACE_ID"); id != "" {
		return id
	}
	// Fallback: first positional argument
	if len(os.Args) > 2 {
		return os.Args[2]
	}
	return "default"
}
