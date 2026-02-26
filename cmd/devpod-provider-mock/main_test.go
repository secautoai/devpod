package main

import (
	"encoding/json"
	"os"
	"testing"
)

func withTempDir(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MOCK_PROVIDER_DIR", dir)
	return func() {}
}

func setWorkspaceID(t *testing.T, id string) {
	t.Helper()
	t.Setenv("WORKSPACE_ID", id)
}

func TestInit(t *testing.T) {
	withTempDir(t)
	if err := os.MkdirAll(stateDir(), 0700); err != nil {
		t.Fatalf("init mkdir: %v", err)
	}
	if _, err := os.Stat(stateDir()); err != nil {
		t.Errorf("state dir should exist after init: %v", err)
	}
}

func TestCreate(t *testing.T) {
	withTempDir(t)
	setWorkspaceID(t, "test-ws")

	r := &WorkspaceRecord{ID: "test-ws", State: StateStopped}
	if err := persist(r); err != nil {
		t.Fatalf("persist: %v", err)
	}

	loaded, err := load("test-ws")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.State != StateStopped {
		t.Errorf("expected Stopped, got %s", loaded.State)
	}
}

func TestStartStop(t *testing.T) {
	withTempDir(t)

	id := "lifecycle-ws"
	// Create
	if err := persist(&WorkspaceRecord{ID: id, State: StateStopped}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Start
	r, _ := load(id)
	r.State = StateRunning
	if err := persist(r); err != nil {
		t.Fatalf("start persist: %v", err)
	}
	r2, _ := load(id)
	if r2.State != StateRunning {
		t.Errorf("expected Running after start, got %s", r2.State)
	}

	// Stop
	r2.State = StateStopped
	_ = persist(r2)
	r3, _ := load(id)
	if r3.State != StateStopped {
		t.Errorf("expected Stopped after stop, got %s", r3.State)
	}
}

func TestDelete(t *testing.T) {
	withTempDir(t)

	id := "delete-ws"
	_ = persist(&WorkspaceRecord{ID: id, State: StateStopped})
	_ = remove(id)

	r, err := load(id)
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if r.State != StateNotFound {
		t.Errorf("expected NotFound after delete, got %s", r.State)
	}
}

func TestStatus_JSON(t *testing.T) {
	withTempDir(t)

	id := "status-ws"
	_ = persist(&WorkspaceRecord{ID: id, State: StateRunning})

	r, _ := load(id)
	out, err := json.Marshal(map[string]string{
		"id":    r.ID,
		"state": string(r.State),
	})
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if parsed["id"] != id {
		t.Errorf("unexpected id: %q", parsed["id"])
	}
	if parsed["state"] != "Running" {
		t.Errorf("unexpected state: %q", parsed["state"])
	}
}

func TestNotFound(t *testing.T) {
	withTempDir(t)

	r, err := load("nonexistent")
	if err != nil {
		t.Fatalf("load nonexistent: %v", err)
	}
	if r.State != StateNotFound {
		t.Errorf("expected NotFound, got %s", r.State)
	}
}
