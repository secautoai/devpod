package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ─── Types ────────────────────────────────────────────────────────────────────

// UserRole represents the access level of a user.
type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

// User represents a registered user in the system.
type User struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	PasswordHash string   `json:"passwordHash,omitempty"`
	Role         UserRole `json:"role"`
	DockerHost   string   `json:"dockerHost,omitempty"`
	CreatedAt    string   `json:"createdAt"`
}

// UserPublic is the public view of a user (no password hash).
type UserPublic struct {
	ID         string   `json:"id"`
	Username   string   `json:"username"`
	Role       UserRole `json:"role"`
	DockerHost string   `json:"dockerHost,omitempty"`
	CreatedAt  string   `json:"createdAt"`
}

func (u *User) Public() UserPublic {
	return UserPublic{
		ID:         u.ID,
		Username:   u.Username,
		Role:       u.Role,
		DockerHost: u.DockerHost,
		CreatedAt:  u.CreatedAt,
	}
}

// ─── User Store ───────────────────────────────────────────────────────────────

// UserStore is a simple file-backed user database.
type UserStore struct {
	mu       sync.RWMutex
	users    map[string]*User // keyed by username
	filePath string
}

// NewUserStore creates or loads a user store from the given directory.
func NewUserStore(dataDir string) (*UserStore, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	store := &UserStore{
		users:    make(map[string]*User),
		filePath: filepath.Join(dataDir, "users.json"),
	}

	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load user store: %w", err)
	}

	// Create default admin if no users exist
	if len(store.users) == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash default password: %w", err)
		}
		store.users["admin"] = &User{
			ID:           uuid.New().String(),
			Username:     "admin",
			PasswordHash: string(hash),
			Role:         RoleAdmin,
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		}
		if err := store.save(); err != nil {
			return nil, fmt.Errorf("save default admin: %w", err)
		}
	}

	return store, nil
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

// Authenticate verifies username/password and returns the user.
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

// CreateUser adds a new user. Returns an error if the username is taken.
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

// UpdateUser updates mutable fields of a user (role, dockerHost).
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

	// Prevent deleting the last admin
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

// ─── JWT ──────────────────────────────────────────────────────────────────────

// jwtSecret is generated once per server start. For production, consider
// persisting this in the data directory.
var jwtSecret []byte

func initJWTSecret(dataDir string) error {
	secretFile := filepath.Join(dataDir, ".jwt-secret")
	data, err := os.ReadFile(secretFile)
	if err == nil && len(data) > 0 {
		jwtSecret = data
		return nil
	}
	jwtSecret = []byte(uuid.New().String())
	return os.WriteFile(secretFile, jwtSecret, 0600)
}

// Claims are the JWT claims for an authenticated user.
type Claims struct {
	Username string   `json:"username"`
	Role     UserRole `json:"role"`
	jwt.RegisteredClaims
}

// generateToken creates a signed JWT for the given user.
func generateToken(user *User) (string, error) {
	claims := Claims{
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// validateToken parses and validates a JWT string, returning the claims.
func validateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// ─── Auth Middleware ──────────────────────────────────────────────────────────

// contextKey is used for storing claims in request context.
type contextKey string

const claimsKey contextKey = "claims"

// withAuth wraps a handler to require a valid JWT. The claims are stored in the
// request context and can be retrieved with getClaims().
func withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			http.Error(w, "invalid authorization header", http.StatusUnauthorized)
			return
		}

		claims, err := validateToken(parts[1])
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		ctx = contextWithClaims(ctx, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// withAdmin wraps a handler to require admin role.
func withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return withAuth(func(w http.ResponseWriter, r *http.Request) {
		claims := getClaims(r)
		if claims == nil || claims.Role != RoleAdmin {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getClaims(r *http.Request) *Claims {
	v := r.Context().Value(claimsKey)
	if v == nil {
		return nil
	}
	claims, _ := v.(*Claims)
	return claims
}

// ─── Auth API Handlers ───────────────────────────────────────────────────────

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string     `json:"token"`
	User  UserPublic `json:"user"`
}

func makeLoginHandler(store *UserStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			http.Error(w, "username and password are required", http.StatusBadRequest)
			return
		}

		user, err := store.Authenticate(req.Username, req.Password)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		token, err := generateToken(user)
		if err != nil {
			http.Error(w, "failed to generate token", http.StatusInternalServerError)
			return
		}

		writeJSON(w, loginResponse{Token: token, User: user.Public()})
	}
}

func makeMeHandler(store *UserStore) http.HandlerFunc {
	return withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		claims := getClaims(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		user, ok := store.GetUser(claims.Username)
		if !ok {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		writeJSON(w, user.Public())
	})
}

// ─── Admin API Handlers ──────────────────────────────────────────────────────

func makeAdminListUsersHandler(store *UserStore) http.HandlerFunc {
	return withAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, store.ListUsers())
	})
}

type createUserRequest struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Role     UserRole `json:"role"`
}

func makeAdminCreateUserHandler(store *UserStore) http.HandlerFunc {
	return withAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req createUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			http.Error(w, "username and password are required", http.StatusBadRequest)
			return
		}

		if req.Role == "" {
			req.Role = RoleUser
		}
		if req.Role != RoleAdmin && req.Role != RoleUser {
			http.Error(w, "role must be 'admin' or 'user'", http.StatusBadRequest)
			return
		}

		user, err := store.CreateUser(req.Username, req.Password, req.Role)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}

		w.WriteHeader(http.StatusCreated)
		writeJSON(w, user.Public())
	})
}

type updateUserRequest struct {
	Role       *UserRole `json:"role,omitempty"`
	DockerHost *string   `json:"dockerHost,omitempty"`
}

func makeAdminUpdateUserHandler(store *UserStore) http.HandlerFunc {
	return withAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract username from path: /api/admin/users/{username}
		username := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
		if username == "" || strings.Contains(username, "/") {
			http.Error(w, "invalid username in path", http.StatusBadRequest)
			return
		}

		var req updateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.Role != nil && *req.Role != RoleAdmin && *req.Role != RoleUser {
			http.Error(w, "role must be 'admin' or 'user'", http.StatusBadRequest)
			return
		}

		user, err := store.UpdateUser(username, req.Role, req.DockerHost)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		writeJSON(w, user.Public())
	})
}

func makeAdminDeleteUserHandler(store *UserStore) http.HandlerFunc {
	return withAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		username := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
		if username == "" || strings.Contains(username, "/") {
			http.Error(w, "invalid username in path", http.StatusBadRequest)
			return
		}

		if err := store.DeleteUser(username); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// makeAdminWorkspacesHandler returns all workspaces across all users.
func makeAdminWorkspacesHandler(store *UserStore) http.HandlerFunc {
	return withAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		type userWorkspaces struct {
			Username   string          `json:"username"`
			Workspaces json.RawMessage `json:"workspaces"`
		}

		var results []userWorkspaces
		for _, u := range store.ListUsers() {
			homeDir := userDevpodHome(u.Username)
			self, err := devpodSelf()
			if err != nil {
				continue
			}
			cmd := buildUserCommand(self, u.Username, homeDir, "", []string{"list", "--output=json", "--log-output=json"})
			out, err := cmd.Output()
			if err != nil {
				results = append(results, userWorkspaces{Username: u.Username, Workspaces: json.RawMessage("[]")})
				continue
			}
			results = append(results, userWorkspaces{Username: u.Username, Workspaces: json.RawMessage(out)})
		}

		writeJSON(w, results)
	})
}
