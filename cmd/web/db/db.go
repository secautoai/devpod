// Package db provides the persistence layer for the DevPod web server.
//
// The default implementation uses JSON files (no extra dependencies required).
// A SQLite implementation can be added later; because all callers depend only
// on the exported interfaces the swap is entirely internal to this package.
package db

import (
	"os"
	"path/filepath"
)

// DB holds references to all store implementations.
type DB struct {
	Users       *UserStore
	Permissions *PermissionStore
	OpLog       *OpLogStore
	Branding    *BrandingStore
}

// Open creates or opens the data directory and initialises all stores.
// dataDir is created with 0700 permissions if it does not exist.
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}

	users, err := newUserStore(filepath.Join(dataDir, "users.json"))
	if err != nil {
		return nil, err
	}

	perms, err := newPermissionStore(filepath.Join(dataDir, "permissions.json"))
	if err != nil {
		return nil, err
	}

	oplog, err := newOpLogStore(filepath.Join(dataDir, "oplog.json"))
	if err != nil {
		return nil, err
	}

	branding, err := newBrandingStore(filepath.Join(dataDir, "branding.json"))
	if err != nil {
		return nil, err
	}

	return &DB{
		Users:       users,
		Permissions: perms,
		OpLog:       oplog,
		Branding:    branding,
	}, nil
}
