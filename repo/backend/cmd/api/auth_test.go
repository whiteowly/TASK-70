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
