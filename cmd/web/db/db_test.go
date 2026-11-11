package db_test

import (
	"os"
	"testing"
	"time"

	"github.com/skevetter/devpod/cmd/web/db"
)

func tempDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	return d
}

// ─── UserStore ────────────────────────────────────────────────────────────────

func TestUserStore_DefaultAdmin(t *testing.T) {
	d := tempDB(t)
	users := d.Users.ListUsers()
	if len(users) != 1 {
		t.Fatalf("expected 1 default user, got %d", len(users))
	}
	if users[0].Username != "admin" || users[0].Role != db.RoleAdmin {
		t.Errorf("unexpected default user: %+v", users[0])
	}
}

func TestUserStore_Authenticate(t *testing.T) {
	d := tempDB(t)

	_, err := d.Users.Authenticate("admin", "admin")
	if err != nil {
		t.Fatalf("authenticate default admin: %v", err)
	}

	_, err = d.Users.Authenticate("admin", "wrongpassword")
	if err == nil {
		t.Error("expected error for wrong password")
	}

	_, err = d.Users.Authenticate("noexist", "pass")
	if err == nil {
		t.Error("expected error for non-existent user")
	}
}

func TestUserStore_CRUD(t *testing.T) {
	d := tempDB(t)

	// Create
	u, err := d.Users.CreateUser("alice", "secret", db.RoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Username != "alice" || u.Role != db.RoleUser {
		t.Errorf("unexpected user: %+v", u)
	}

	// Duplicate create
	_, err = d.Users.CreateUser("alice", "secret2", db.RoleUser)
	if err == nil {
		t.Error("expected error for duplicate username")
	}

	// Get
	got, ok := d.Users.GetUser("alice")
	if !ok || got.Username != "alice" {
		t.Errorf("GetUser failed: ok=%v user=%+v", ok, got)
	}

	// Update role
	newRole := db.RoleAdmin
	updated, err := d.Users.UpdateUser("alice", &newRole, nil)
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if updated.Role != db.RoleAdmin {
		t.Errorf("expected admin role, got %s", updated.Role)
	}

	// Change password
	if err := d.Users.ChangePassword("alice", "newpass"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	_, err = d.Users.Authenticate("alice", "newpass")
	if err != nil {
		t.Fatalf("authenticate after password change: %v", err)
	}

	// Delete non-last admin (alice is admin, and so is the default admin)
	if err := d.Users.DeleteUser("alice"); err != nil {
		t.Fatalf("DeleteUser alice: %v", err)
	}
	_, ok = d.Users.GetUser("alice")
	if ok {
		t.Error("alice should be deleted")
	}
}

func TestUserStore_PreventLastAdminDeletion(t *testing.T) {
	d := tempDB(t)
	err := d.Users.DeleteUser("admin")
	if err == nil {
		t.Error("expected error when deleting last admin")
	}
}

// ─── PermissionStore ─────────────────────────────────────────────────────────

func TestPermissionStore_Unrestricted(t *testing.T) {
	d := tempDB(t)

	// No entries → unrestricted
	if !d.Permissions.IsAllowed("alice", "docker") {
		t.Error("new user should be unrestricted")
	}

	_, restricted := d.Permissions.GetAllowed("alice")
	if restricted {
		t.Error("new user should not have restriction entry")
	}
}

func TestPermissionStore_AllowList(t *testing.T) {
	d := tempDB(t)

	if err := d.Permissions.SetAllowed("alice", []string{"docker", "kubernetes"}); err != nil {
		t.Fatalf("SetAllowed: %v", err)
	}

	if !d.Permissions.IsAllowed("alice", "docker") {
		t.Error("docker should be allowed")
	}
	if !d.Permissions.IsAllowed("alice", "kubernetes") {
		t.Error("kubernetes should be allowed")
	}
	if d.Permissions.IsAllowed("alice", "aws") {
		t.Error("aws should not be allowed")
	}
}

func TestPermissionStore_DenyAll(t *testing.T) {
	d := tempDB(t)

	if err := d.Permissions.SetAllowed("alice", []string{}); err != nil {
		t.Fatalf("SetAllowed empty: %v", err)
	}
	if d.Permissions.IsAllowed("alice", "docker") {
		t.Error("no providers should be allowed when list is empty")
	}
}

func TestPermissionStore_SetReplaces(t *testing.T) {
	d := tempDB(t)

	_ = d.Permissions.SetAllowed("alice", []string{"docker"})
	_ = d.Permissions.SetAllowed("alice", []string{"kubernetes"})

	if d.Permissions.IsAllowed("alice", "docker") {
		t.Error("docker should have been replaced")
	}
	if !d.Permissions.IsAllowed("alice", "kubernetes") {
		t.Error("kubernetes should now be allowed")
	}
}

func TestPermissionStore_Unrestrict(t *testing.T) {
	d := tempDB(t)

	_ = d.Permissions.SetAllowed("alice", []string{"docker"})
	_ = d.Permissions.SetAllowed("alice", nil) // nil = unrestricted

	if !d.Permissions.IsAllowed("alice", "aws") {
		t.Error("user should be unrestricted after nil set")
	}
}

func TestPermissionStore_DeleteUser(t *testing.T) {
	d := tempDB(t)
	_ = d.Permissions.SetAllowed("alice", []string{"docker"})
	_ = d.Permissions.DeleteUser("alice")

	if !d.Permissions.IsAllowed("alice", "anything") {
		t.Error("deleted user should be unrestricted")
	}
}

func TestPermissionStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	d1, _ := db.Open(dir)
	_ = d1.Permissions.SetAllowed("alice", []string{"docker"})

	// Re-open
	d2, err := db.Open(dir)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	if !d2.Permissions.IsAllowed("alice", "docker") {
		t.Error("permissions should persist after re-open")
	}
}

// ─── OpLogStore ───────────────────────────────────────────────────────────────

func TestOpLogStore_AppendAndQuery(t *testing.T) {
	d := tempDB(t)

	id, err := d.OpLog.Append("alice", "create", "ws1", []string{"create", "ws1"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}

	entries := d.OpLog.Query(db.OpLogFilter{Username: "alice"})
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "create" || entries[0].Target != "ws1" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
}

func TestOpLogStore_Finish(t *testing.T) {
	d := tempDB(t)
	id, _ := d.OpLog.Append("alice", "create", "ws1", nil)

	if err := d.OpLog.Finish(id, 0, ""); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	entries := d.OpLog.Query(db.OpLogFilter{})
	if entries[0].ExitCode == nil || *entries[0].ExitCode != 0 {
		t.Error("expected exit code 0")
	}
	if entries[0].FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
}

func TestOpLogStore_FilterByUser(t *testing.T) {
	d := tempDB(t)
	_, _ = d.OpLog.Append("alice", "create", "ws1", nil)
	_, _ = d.OpLog.Append("bob", "delete", "ws2", nil)
	_, _ = d.OpLog.Append("alice", "start", "ws1", nil)

	entries := d.OpLog.Query(db.OpLogFilter{Username: "alice"})
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for alice, got %d", len(entries))
	}
}

func TestOpLogStore_FilterByAction(t *testing.T) {
	d := tempDB(t)
	_, _ = d.OpLog.Append("alice", "create", "ws1", nil)
	_, _ = d.OpLog.Append("alice", "delete", "ws1", nil)

	entries := d.OpLog.Query(db.OpLogFilter{Action: "create"})
	if len(entries) != 1 || entries[0].Action != "create" {
		t.Fatalf("filter by action failed: %+v", entries)
	}
}

func TestOpLogStore_Pagination(t *testing.T) {
	d := tempDB(t)
	for i := 0; i < 10; i++ {
		_, _ = d.OpLog.Append("alice", "command", "ws", nil)
		time.Sleep(time.Millisecond) // ensure distinct timestamps
	}

	page1 := d.OpLog.Query(db.OpLogFilter{Limit: 3, Offset: 0})
	page2 := d.OpLog.Query(db.OpLogFilter{Limit: 3, Offset: 3})

	if len(page1) != 3 {
		t.Fatalf("page1 expected 3, got %d", len(page1))
	}
	if len(page2) != 3 {
		t.Fatalf("page2 expected 3, got %d", len(page2))
	}
	if page1[0].ID == page2[0].ID {
		t.Error("pages should not overlap")
	}
}

func TestOpLogStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	d1, _ := db.Open(dir)
	_, _ = d1.OpLog.Append("alice", "create", "ws1", nil)

	d2, err := db.Open(dir)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	entries := d2.OpLog.Query(db.OpLogFilter{})
	if len(entries) != 1 {
		t.Errorf("expected 1 persisted entry, got %d", len(entries))
	}
}

// ─── BrandingStore ────────────────────────────────────────────────────────────

func TestBrandingStore_Defaults(t *testing.T) {
	d := tempDB(t)
	b := d.Branding.Get()
	if b.AppName != "DevPod" {
		t.Errorf("expected default AppName=DevPod, got %q", b.AppName)
	}
	if b.DocsURL != "https://devpod.sh/docs" {
		t.Errorf("unexpected default DocsURL: %q", b.DocsURL)
	}
}

func TestBrandingStore_Update(t *testing.T) {
	d := tempDB(t)

	err := d.Branding.Update(db.BrandingSettings{
		AppName:  "MyDevPod",
		LogoURL:  "https://example.com/logo.png",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	b := d.Branding.Get()
	if b.AppName != "MyDevPod" {
		t.Errorf("expected AppName=MyDevPod, got %q", b.AppName)
	}
	if b.LogoURL != "https://example.com/logo.png" {
		t.Errorf("unexpected LogoURL: %q", b.LogoURL)
	}
	// Unset fields keep defaults
	if b.DocsURL != "https://devpod.sh/docs" {
		t.Errorf("DocsURL should be unchanged: %q", b.DocsURL)
	}
}

func TestBrandingStore_Reset(t *testing.T) {
	d := tempDB(t)
	_ = d.Branding.Update(db.BrandingSettings{AppName: "Custom"})
	_ = d.Branding.Reset()
	if d.Branding.Get().AppName != "DevPod" {
		t.Error("Reset should restore default AppName")
	}
}

func TestBrandingStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	d1, _ := db.Open(dir)
	_ = d1.Branding.Update(db.BrandingSettings{AppName: "Saved"})

	d2, err := db.Open(dir)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	if d2.Branding.Get().AppName != "Saved" {
		t.Error("branding should persist across restarts")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
