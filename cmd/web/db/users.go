package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// UserRole is the access level of a user.
type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

// User is the stored user record.
type User struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	PasswordHash string   `json:"passwordHash,omitempty"`
	Role         UserRole `json:"role"`
	DockerHost   string   `json:"dockerHost,omitempty"`
	CreatedAt    string   `json:"createdAt"`
}

// UserPublic is the user record without the password hash.
type UserPublic struct {
	ID         string   `json:"id"`
	Username   string   `json:"username"`
	Role       UserRole `json:"role"`
	DockerHost string   `json:"dockerHost,omitempty"`
	CreatedAt  string   `json:"createdAt"`
}

// Public returns the public view of a user.
func (u *User) Public() UserPublic {
	return UserPublic{
		ID:         u.ID,
		Username:   u.Username,
		Role:       u.Role,
		DockerHost: u.DockerHost,
		CreatedAt:  u.CreatedAt,
	}
}

// UserStore is a file-backed user database.
type UserStore struct {
	mu       sync.RWMutex
	users    map[string]*User
	filePath string
}

func newUserStore(filePath string) (*UserStore, error) {
	s := &UserStore{
		users:    make(map[string]*User),
		filePath: filePath,
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load user store: %w", err)
	}

	// Migrate legacy users.json from the old location if present
	if len(s.users) == 0 {
		if err := s.createDefaultAdmin(); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *UserStore) createDefaultAdmin() error {
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash default password: %w", err)
	}
	s.users["admin"] = &User{
		ID:           uuid.New().String(),
		Username:     "admin",
		PasswordHash: string(hash),
		Role:         RoleAdmin,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	return s.save()
}

func (s *UserStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var users []*User
	if err := json.Unmarshal(data, &users); err != nil {
		return err
	}
	s.users = make(map[string]*User, len(users))
	for _, u := range users {
		s.users[u.Username] = u
	}
	return nil
}

func (s *UserStore) save() error {
	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0600)
}

// Authenticate verifies credentials and returns the user.
func (s *UserStore) Authenticate(username, password string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[username]
	if !ok {
		return nil, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	return user, nil
}

// CreateUser adds a new user.
func (s *UserStore) CreateUser(username, password string, role UserRole) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		return nil, fmt.Errorf("username %q already exists", username)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	s.users[username] = user
	if err := s.save(); err != nil {
		delete(s.users, username)
		return nil, err
	}
	return user, nil
}

// GetUser returns a user by username.
func (s *UserStore) GetUser(username string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	return u, ok
}

// ListUsers returns all users (public view).
func (s *UserStore) ListUsers() []UserPublic {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]UserPublic, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u.Public())
	}
	return out
}

// UpdateUser updates mutable fields.
func (s *UserStore) UpdateUser(username string, role *UserRole, dockerHost *string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[username]
	if !ok {
		return nil, fmt.Errorf("user %q not found", username)
	}
	if role != nil {
		user.Role = *role
	}
	if dockerHost != nil {
		user.DockerHost = *dockerHost
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	return user, nil
}

// DeleteUser removes a user. Cannot delete the last admin.
func (s *UserStore) DeleteUser(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[username]
	if !ok {
		return fmt.Errorf("user %q not found", username)
	}
	if user.Role == RoleAdmin {
		adminCount := 0
		for _, u := range s.users {
			if u.Role == RoleAdmin {
				adminCount++
			}
		}
		if adminCount <= 1 {
			return errors.New("cannot delete the last admin user")
		}
	}
	delete(s.users, username)
	return s.save()
}

// ChangePassword updates a user's password.
func (s *UserStore) ChangePassword(username, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[username]
	if !ok {
		return fmt.Errorf("user %q not found", username)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	user.PasswordHash = string(hash)
	return s.save()
}
