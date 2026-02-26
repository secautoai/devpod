package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/skevetter/devpod/cmd/web/db"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	return d
}

// initTestJWT sets up a fresh JWT secret for the test.
func initTestJWT(t *testing.T) {
	t.Helper()
	if err := initJWTSecret(t.TempDir()); err != nil {
		t.Fatalf("initJWTSecret: %v", err)
	}
}

func tokenFor(t *testing.T, users *db.UserStore, username, password string) string {
	t.Helper()
	user, err := users.Authenticate(username, password)
	if err != nil {
		t.Fatalf("authenticate %s: %v", username, err)
	}
	tok, err := generateToken(user)
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	return tok
}

func bearer(tok string) string { return "Bearer " + tok }

func doRequest(t *testing.T, mux http.Handler, method, path string, body interface{}, tok string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", bearer(tok))
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// buildTestMux wires all handlers without starting a TCP listener.
func buildTestMux(t *testing.T, database *db.DB) http.Handler {
	t.Helper()
	initTestJWT(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/version", handleVersion)
	mux.HandleFunc("/api/platform", handlePlatform)
	mux.HandleFunc("/releases", handleReleases)
	mux.HandleFunc("/api/branding", makeBrandingGetHandler(database.Branding))
	mux.HandleFunc("/api/login", makeLoginHandler(database.Users))
	mux.HandleFunc("/api/me", makeMeHandler(database.Users))
	mux.HandleFunc("/api/command", withAuth(makeCommandRunHandler(database)))
	mux.HandleFunc("/api/signal", withAuth(handleSignal))
	mux.HandleFunc("/api/providers", withAuth(makeProvidersHandler(database)))
	mux.HandleFunc("/api/workspaces", withAuth(makeWorkspacesHandler(database)))
	mux.HandleFunc("/api/logs", withAuth(makeUserLogsHandler(database.OpLog)))
	mux.HandleFunc("/api/user/password", withAuth(makeChangePasswordHandler(database.Users)))
	mux.HandleFunc("/api/admin/users", withAdmin(makeAdminUsersHandler(database)))
	mux.HandleFunc("/api/admin/users/", withAdmin(makeAdminUserDetailHandler(database)))
	mux.HandleFunc("/api/admin/workspaces", makeAdminWorkspacesHandler(database.Users))
	mux.HandleFunc("/api/admin/logs", withAdmin(makeAdminLogsHandler(database.OpLog)))
	mux.HandleFunc("/api/admin/branding", withAdmin(makeBrandingUpdateHandler(database.Branding)))
	return withCORS(mux)
}

// fakeDevpodBin writes a shell stub and returns its path.
func fakeDevpodBin(t *testing.T, exitCode int, stdout string) string {
	t.Helper()
	dir := t.TempDir()
	p := dir + "/devpod"
	script := "#!/bin/sh\n"
	if stdout != "" {
		script += "printf '%s\\n' '" + stdout + "'\n"
	}
	if exitCode != 0 {
		script += "exit 1\n"
	}
	if err := os.WriteFile(p, []byte(script), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return p
}

// ─── Public endpoint tests ────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	w := doRequest(t, mux, http.MethodGet, "/api/health", nil, "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestVersion(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	t.Setenv("DEVPOD_VERSION", "v1.2.3-test")
	w := doRequest(t, mux, http.MethodGet, "/api/version", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["version"] != "v1.2.3-test" {
		t.Errorf("unexpected version: %q", resp["version"])
	}
}

func TestBrandingPublicNoAuth(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	w := doRequest(t, mux, http.MethodGet, "/api/branding", nil, "")
	if w.Code != http.StatusOK {
		t.Errorf("branding should be public (no auth): got %d", w.Code)
	}
	var b db.BrandingSettings
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if b.AppName != "DevPod" {
		t.Errorf("expected default AppName=DevPod, got %q", b.AppName)
	}
}

// ─── Auth tests ───────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	w := doRequest(t, mux, http.MethodPost, "/api/login",
		map[string]string{"username": "admin", "password": "admin"}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["token"] == "" {
		t.Error("expected non-empty token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	w := doRequest(t, mux, http.MethodPost, "/api/login",
		map[string]string{"username": "admin", "password": "badpass"}, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMe_RequiresAuth(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	w := doRequest(t, mux, http.MethodGet, "/api/me", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}
}

func TestMe_WithToken(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "admin", "admin")
	w := doRequest(t, mux, http.MethodGet, "/api/me", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var u db.UserPublic
	_ = json.Unmarshal(w.Body.Bytes(), &u)
	if u.Username != "admin" {
		t.Errorf("unexpected username: %q", u.Username)
	}
}

func TestCommandRequiresAuth(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	w := doRequest(t, mux, http.MethodPost, "/api/command",
		map[string]interface{}{"args": []string{"version"}}, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ─── RBAC tests ───────────────────────────────────────────────────────────────

func TestRBAC_AllowedProvider(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("alice", "pass", db.RoleUser)
	_ = d.Permissions.SetAllowed("alice", []string{"docker"})

	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "alice", "pass")

	devpodSelfOverride = fakeDevpodBin(t, 0, `{"stdout":"ok"}`)
	defer func() { devpodSelfOverride = "" }()

	w := doRequest(t, mux, http.MethodPost, "/api/command",
		map[string]interface{}{"args": []string{"up", "--provider=docker", "ws1"}}, tok)
	if w.Code == http.StatusForbidden {
		t.Errorf("docker should be allowed for alice, got 403: %s", w.Body.String())
	}
}

func TestRBAC_DeniedProvider(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("alice", "pass", db.RoleUser)
	_ = d.Permissions.SetAllowed("alice", []string{"docker"}) // only docker allowed

	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "alice", "pass")

	w := doRequest(t, mux, http.MethodPost, "/api/command",
		map[string]interface{}{"args": []string{"up", "--provider=kubernetes", "ws1"}}, tok)
	if w.Code != http.StatusForbidden {
		t.Errorf("kubernetes should be denied for alice, got %d", w.Code)
	}
}

func TestRBAC_AdminUnrestricted(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "admin", "admin")

	devpodSelfOverride = fakeDevpodBin(t, 0, "")
	defer func() { devpodSelfOverride = "" }()

	w := doRequest(t, mux, http.MethodPost, "/api/command",
		map[string]interface{}{"args": []string{"up", "--provider=anyprovider", "ws1"}}, tok)
	if w.Code == http.StatusForbidden {
		t.Errorf("admin should bypass RBAC, got 403: %s", w.Body.String())
	}
}

// ─── Admin user management ────────────────────────────────────────────────────

func TestAdminCreateUser(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "admin", "admin")

	w := doRequest(t, mux, http.MethodPost, "/api/admin/users",
		map[string]string{"username": "bob", "password": "secret", "role": "user"}, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var u db.UserPublic
	_ = json.Unmarshal(w.Body.Bytes(), &u)
	if u.Username != "bob" {
		t.Errorf("unexpected username: %q", u.Username)
	}
}

func TestAdminCreateUser_ForbiddenForUser(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("bob", "pass", db.RoleUser)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "bob", "pass")

	w := doRequest(t, mux, http.MethodPost, "/api/admin/users",
		map[string]string{"username": "charlie", "password": "secret"}, tok)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminDeleteUser(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("alice", "pass", db.RoleUser)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "admin", "admin")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/alice", nil)
	req.Header.Set("Authorization", bearer(tok))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminUpdateUser_Role(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("alice", "pass", db.RoleUser)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "admin", "admin")

	newRole := db.RoleAdmin
	w := doRequest(t, mux, http.MethodPut, "/api/admin/users/alice",
		map[string]interface{}{"role": newRole}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var u db.UserPublic
	_ = json.Unmarshal(w.Body.Bytes(), &u)
	if u.Role != db.RoleAdmin {
		t.Errorf("expected admin role, got %s", u.Role)
	}
}

// ─── Provider permissions API ─────────────────────────────────────────────────

func TestAdminProviderPermissions_SetAndGet(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("alice", "pass", db.RoleUser)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "admin", "admin")

	// Set allowed providers
	providers := []string{"docker", "kubernetes"}
	w := doRequest(t, mux, http.MethodPut, "/api/admin/users/alice/providers",
		map[string]interface{}{"providers": &providers}, tok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT providers: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Get allowed providers
	w2 := doRequest(t, mux, http.MethodGet, "/api/admin/users/alice/providers", nil, tok)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET providers: expected 200, got %d", w2.Code)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["unrestricted"] == true {
		t.Error("should not be unrestricted after setting providers")
	}
}

// ─── Branding admin tests ─────────────────────────────────────────────────────

func TestAdminBranding_UpdateAndRead(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "admin", "admin")

	w := doRequest(t, mux, http.MethodPut, "/api/admin/branding",
		db.BrandingSettings{AppName: "MyDevPod", LogoURL: "https://example.com/logo.png"}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Public endpoint reflects update
	w2 := doRequest(t, mux, http.MethodGet, "/api/branding", nil, "")
	var b db.BrandingSettings
	_ = json.Unmarshal(w2.Body.Bytes(), &b)
	if b.AppName != "MyDevPod" {
		t.Errorf("public branding not updated: %q", b.AppName)
	}
}

func TestAdminBranding_ForbiddenForUser(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("alice", "pass", db.RoleUser)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "alice", "pass")

	w := doRequest(t, mux, http.MethodPut, "/api/admin/branding",
		db.BrandingSettings{AppName: "Hacked"}, tok)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// ─── Operation log tests ──────────────────────────────────────────────────────

func TestUserLogs_OnlyOwnEntries(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("alice", "pass", db.RoleUser)
	_, _ = d.Users.CreateUser("bob", "pass", db.RoleUser)
	_, _ = d.OpLog.Append("alice", "create", "ws1", nil)
	_, _ = d.OpLog.Append("bob", "create", "ws2", nil)

	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "alice", "pass")

	w := doRequest(t, mux, http.MethodGet, "/api/logs", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var entries []*db.OperationLog
	_ = json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 1 {
		t.Errorf("alice should see only 1 log entry, got %d", len(entries))
	}
	if len(entries) > 0 && entries[0].Username != "alice" {
		t.Errorf("unexpected username: %q", entries[0].Username)
	}
}

func TestAdminLogs_AllEntries(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("alice", "pass", db.RoleUser)
	_, _ = d.OpLog.Append("alice", "create", "ws1", nil)
	_, _ = d.OpLog.Append("admin", "delete", "ws2", nil)

	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "admin", "admin")

	w := doRequest(t, mux, http.MethodGet, "/api/admin/logs", nil, tok)
	var entries []*db.OperationLog
	_ = json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) < 2 {
		t.Errorf("admin should see all entries, got %d", len(entries))
	}
}

// ─── CORS tests ───────────────────────────────────────────────────────────────

func TestCORS_Options(t *testing.T) {
	d := newTestDB(t)
	mux := buildTestMux(t, d)
	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	req.Header.Set("Origin", "http://localhost:1420")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS expected 204, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Origin"), "*") {
		t.Error("missing CORS header")
	}
}

// ─── Password change ──────────────────────────────────────────────────────────

func TestChangePassword(t *testing.T) {
	d := newTestDB(t)
	_, _ = d.Users.CreateUser("alice", "oldpass", db.RoleUser)
	mux := buildTestMux(t, d)
	tok := tokenFor(t, d.Users, "alice", "oldpass")

	w := doRequest(t, mux, http.MethodPost, "/api/user/password",
		map[string]string{"currentPassword": "oldpass", "newPassword": "newpass123"}, tok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := d.Users.Authenticate("alice", "oldpass"); err == nil {
		t.Error("old password should be rejected")
	}
	if _, err := d.Users.Authenticate("alice", "newpass123"); err != nil {
		t.Errorf("new password should work: %v", err)
	}
}

// ─── extractProvider helper ───────────────────────────────────────────────────

func TestExtractProvider(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"up", "--provider=docker", "ws"}, "docker"},
		{[]string{"up", "--provider", "kubernetes", "ws"}, "kubernetes"},
		{[]string{"up", "-p", "ssh", "ws"}, "ssh"},
		{[]string{"list"}, ""},
	}
	for _, tc := range cases {
		got := extractProvider(tc.args)
		if got != tc.want {
			t.Errorf("extractProvider(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}
