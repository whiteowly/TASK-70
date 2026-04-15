package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

func getTestDB(t *testing.T) *sql.DB {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func cleanupSessions(t *testing.T, db *sql.DB) {
	t.Helper()
	_, _ = db.Exec("DELETE FROM auth_sessions")
	_, _ = db.Exec("DELETE FROM audit_event_index")
}

// extractCookie parses the Set-Cookie header from the response and returns an
// *http.Cookie that can be added to subsequent requests.
func extractCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func loginAdmin(t *testing.T, e http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"username":"admin","password":"admin123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestLoginSuccess(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	e := newServer(db)
	rec := loginAdmin(t, e)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		User struct {
			ID       string   `json:"id"`
			Username string   `json:"username"`
			Email    string   `json:"email"`
			Roles    []string `json:"roles"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.User.Username != "admin" {
		t.Fatalf("expected username admin, got %q", resp.User.Username)
	}

	foundAdmin := false
	for _, r := range resp.User.Roles {
		if r == "administrator" {
			foundAdmin = true
		}
	}
	if !foundAdmin {
		t.Fatalf("expected administrator role, got %v", resp.User.Roles)
	}

	cookie := extractCookie(rec, "fieldserve_session")
	if cookie == nil {
		t.Fatal("expected fieldserve_session cookie in response")
	}
}

func TestLoginFailure(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	e := newServer(db)
	body := `{"username":"admin","password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error.Code != "invalid_credentials" {
		t.Fatalf("expected error code invalid_credentials, got %q", resp.Error.Code)
	}
}

func TestMeWithSession(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	e := newServer(db)

	// Login first
	loginRec := loginAdmin(t, e)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d; body: %s", loginRec.Code, loginRec.Body.String())
	}
	cookie := extractCookie(loginRec, "fieldserve_session")
	if cookie == nil {
		t.Fatal("no session cookie")
	}

	// GET /auth/me with cookie
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.User.Username != "admin" {
		t.Fatalf("expected admin, got %q", resp.User.Username)
	}
}

func TestMeWithoutSession(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	e := newServer(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestLogout(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	e := newServer(db)

	// Login
	loginRec := loginAdmin(t, e)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d", loginRec.Code)
	}
	cookie := extractCookie(loginRec, "fieldserve_session")
	if cookie == nil {
		t.Fatal("no session cookie")
	}

	// Logout
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Try /me with the same cookie — should be 401
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteUnauthenticated(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	e := newServer(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/customer", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestBootstrapAdminConflict verifies POST /api/v1/auth/bootstrap-admin returns
// 409 admin_exists when an administrator role assignment already exists in the
// database (the seeded admin user). True no-mock: hits the real handler chain
// that runs an actual transaction against Postgres.
func TestBootstrapAdminConflict(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	e := newServer(db)
	body := `{"username":"would_be_admin","password":"newpass123","email":"would_be@admin.local"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap-admin", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "admin_exists" {
		t.Fatalf("expected error code admin_exists, got %q", resp.Error.Code)
	}

	// Verify no user got created with the test username
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE username = 'would_be_admin'`).Scan(&count); err != nil {
		t.Fatalf("query users: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected user to NOT be created on conflict, found %d", count)
	}
}

// TestBootstrapAdminValidation verifies POST /api/v1/auth/bootstrap-admin
// returns 422 with field errors when required fields are missing.
func TestBootstrapAdminValidation(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	e := newServer(db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap-admin", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "validation_error" {
		t.Fatalf("expected code validation_error, got %v", errObj["code"])
	}
	fieldErrors := errObj["field_errors"].(map[string]interface{})
	if _, ok := fieldErrors["username"]; !ok {
		t.Fatal("expected field_errors[username]")
	}
	if _, ok := fieldErrors["password"]; !ok {
		t.Fatal("expected field_errors[password]")
	}
	if _, ok := fieldErrors["email"]; !ok {
		t.Fatal("expected field_errors[email]")
	}
}

// TestBootstrapAdminSuccess verifies POST /api/v1/auth/bootstrap-admin
// successfully creates a new administrator user when no admin role
// assignments exist. The test temporarily removes the seeded admin's role
// assignment, performs the bootstrap, then restores the original state.
func TestBootstrapAdminSuccess(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	// Snapshot existing administrator role assignments
	rows, err := db.Query(
		`SELECT user_id, role_id FROM user_roles
		 WHERE role_id = (SELECT id FROM roles WHERE name = 'administrator')`)
	if err != nil {
		t.Fatalf("snapshot admin roles: %v", err)
	}
	type userRole struct{ userID, roleID string }
	var snapshot []userRole
	for rows.Next() {
		var ur userRole
		if err := rows.Scan(&ur.userID, &ur.roleID); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		snapshot = append(snapshot, ur)
	}
	rows.Close()

	const bootstrapUsername = "bootstrap_test_admin"

	// Restore admin role assignment after the test (and remove the newly-created
	// admin) so subsequent tests that rely on the seeded admin still work.
	t.Cleanup(func() {
		_, _ = db.Exec(
			`DELETE FROM admin_profiles
			 WHERE user_id IN (SELECT id FROM users WHERE username = $1)`,
			bootstrapUsername)
		_, _ = db.Exec(`DELETE FROM user_roles
		                 WHERE user_id IN (SELECT id FROM users WHERE username = $1)`,
			bootstrapUsername)
		_, _ = db.Exec(`DELETE FROM users WHERE username = $1`, bootstrapUsername)
		for _, ur := range snapshot {
			_, _ = db.Exec(
				`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				ur.userID, ur.roleID)
		}
	})

	// Remove existing admin role assignments so bootstrap is permitted
	if _, err := db.Exec(
		`DELETE FROM user_roles
		 WHERE role_id = (SELECT id FROM roles WHERE name = 'administrator')`); err != nil {
		t.Fatalf("delete admin roles: %v", err)
	}

	e := newServer(db)
	body := `{"username":"bootstrap_test_admin","password":"strongpass1","email":"bootstrap@test.local"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap-admin", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		User struct {
			ID       string   `json:"id"`
			Username string   `json:"username"`
			Email    string   `json:"email"`
			Roles    []string `json:"roles"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.User.Username != bootstrapUsername {
		t.Fatalf("expected username %q, got %q", bootstrapUsername, resp.User.Username)
	}
	if resp.User.Email != "bootstrap@test.local" {
		t.Fatalf("expected email bootstrap@test.local, got %q", resp.User.Email)
	}
	foundAdmin := false
	for _, r := range resp.User.Roles {
		if r == "administrator" {
			foundAdmin = true
		}
	}
	if !foundAdmin {
		t.Fatalf("expected administrator role, got %v", resp.User.Roles)
	}

	// Verify a real admin profile was created in DB
	var profileCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM admin_profiles WHERE user_id = $1`, resp.User.ID).Scan(&profileCount); err != nil {
		t.Fatalf("query admin_profiles: %v", err)
	}
	if profileCount != 1 {
		t.Fatalf("expected 1 admin profile, got %d", profileCount)
	}

	// A second bootstrap call must now return 409
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap-admin", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second bootstrap: expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminRouteAsCustomer(t *testing.T) {
	db := getTestDB(t)
	cleanupSessions(t, db)

	// Seed a customer user if not already present. We need a user with the
	// "customer" role. For simplicity we insert directly.
	var customerUserID string
	err := db.QueryRow(
		`SELECT id FROM users WHERE username = 'customer'`,
	).Scan(&customerUserID)
	if err != nil {
		t.Fatalf("seeded customer user not found — run ./init_db.sh first: %v", err)
	}

	e := newServer(db)

	// Login as customer (seeded account)
	body := `{"username":"customer","password":"customer123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("customer login failed: %d; body: %s", rec.Code, rec.Body.String())
	}

	cookie := extractCookie(rec, "fieldserve_session")
	if cookie == nil {
		t.Fatal("no session cookie for customer")
	}

	// Clear audit events before the test request
	_, _ = db.Exec("DELETE FROM audit_event_index WHERE event_type = 'privilege_escalation'")

	// Try to access /admin route
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify audit event was created
	var auditCount int
	err = db.QueryRow(
		`SELECT COUNT(*) FROM audit_event_index WHERE event_type = 'privilege_escalation' AND actor_id = $1`,
		customerUserID,
	).Scan(&auditCount)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected privilege_escalation audit event, found none")
	}
}
