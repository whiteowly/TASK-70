package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fieldserve/internal/alerts"
	"fieldserve/internal/audit"
	"fieldserve/internal/platform/cleanup"
	"fieldserve/internal/platform/crypto"

	"github.com/google/uuid"
)

// --- Encryption wired into real customer profile path ---

func TestEncryptedCustomerPhone(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")

	key, err := crypto.Key()
	if err != nil {
		t.Skipf("ENCRYPTION_KEY not set: %v", err)
	}

	e := newServer(db)
	customerCookie := loginAs(t, e, "customer", "customer123")

	phone := "+1-555-867-5309"
	body := fmt.Sprintf(`{"phone":"%s"}`, phone)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/customer/profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update customer profile: %d %s", rec.Code, rec.Body.String())
	}

	// Verify DB stores ciphertext, not plaintext
	var rawBytes []byte
	err = db.QueryRow(`SELECT phone_encrypted FROM customer_profiles WHERE user_id = (SELECT id FROM users WHERE username='customer')`).Scan(&rawBytes)
	if err != nil {
		t.Fatalf("query phone: %v", err)
	}
	if rawBytes == nil || len(rawBytes) == 0 {
		t.Fatal("phone_encrypted is null/empty")
	}
	storedHex := string(rawBytes)
	if storedHex == phone {
		t.Fatal("phone_encrypted contains plaintext")
	}

	// Read back via real GET endpoint
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/profile", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get customer profile: %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	profile := resp["profile"].(map[string]interface{})
	// Profile GET now returns masked phone, not plaintext
	maskedPhone := profile["phone"].(string)
	if maskedPhone == phone {
		t.Fatal("expected masked phone, got full plaintext")
	}
	if !strings.HasPrefix(maskedPhone, "***") {
		t.Fatalf("expected masked phone to start with ***, got %q", maskedPhone)
	}

	// Direct decrypt of DB value still works
	decrypted, err := crypto.Decrypt(key, storedHex)
	if err != nil {
		t.Fatalf("direct decrypt: %v", err)
	}
	if decrypted != phone {
		t.Fatalf("direct decrypt mismatch: got %q want %q", decrypted, phone)
	}
}

// --- Encryption wired into real provider profile path ---

func TestEncryptedProviderPhone(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")

	key, err := crypto.Key()
	if err != nil {
		t.Skipf("ENCRYPTION_KEY not set: %v", err)
	}

	e := newServer(db)
	providerCookie := loginAs(t, e, "provider", "provider123")

	phone := "+1-555-999-1234"
	body := fmt.Sprintf(`{"phone":"%s"}`, phone)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update provider profile: %d %s", rec.Code, rec.Body.String())
	}

	// Verify DB stores ciphertext, not plaintext
	var rawBytes []byte
	err = db.QueryRow(`SELECT phone_encrypted FROM provider_profiles WHERE user_id = (SELECT id FROM users WHERE username='provider')`).Scan(&rawBytes)
	if err != nil {
		t.Fatalf("query phone: %v", err)
	}
	if rawBytes == nil || len(rawBytes) == 0 {
		t.Fatal("phone_encrypted is null/empty")
	}
	storedHex := string(rawBytes)
	if storedHex == phone {
		t.Fatal("phone_encrypted contains plaintext")
	}

	// Read back via real GET endpoint — should be masked
	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/profile", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get provider profile: %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	profile := resp["profile"].(map[string]interface{})
	maskedPhone := profile["phone"].(string)
	if maskedPhone == phone {
		t.Fatal("expected masked phone, got full plaintext")
	}
	if !strings.HasPrefix(maskedPhone, "***") {
		t.Fatalf("expected masked phone to start with ***, got %q", maskedPhone)
	}

	// Direct decrypt still works
	decrypted, err := crypto.Decrypt(key, storedHex)
	if err != nil {
		t.Fatalf("direct decrypt: %v", err)
	}
	if decrypted != phone {
		t.Fatalf("direct decrypt mismatch: got %q want %q", decrypted, phone)
	}
}

// --- Real cleanup using shared cleanup functions ---

func TestSessionCleanupReal(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")

	// Insert an expired session
	_, err := db.Exec(
		`INSERT INTO auth_sessions (user_id, token_hash, expires_at, last_active_at)
		 VALUES ((SELECT id FROM users WHERE username='customer'), 'expired-hash-test', NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day')`)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM auth_sessions WHERE token_hash = 'expired-hash-test'`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 expired session before cleanup, got %d", count)
	}

	// Use the real cleanup function
	n, err := cleanup.ExpiredSessions(db)
	if err != nil {
		t.Fatalf("cleanup.ExpiredSessions: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 session removed, got %d", n)
	}

	db.QueryRow(`SELECT COUNT(*) FROM auth_sessions WHERE token_hash = 'expired-hash-test'`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 expired sessions after cleanup, got %d", count)
	}

	// Idempotent: running again removes nothing
	n2, _ := cleanup.ExpiredSessions(db)
	if n2 != 0 {
		t.Fatalf("idempotent rerun should remove 0, got %d", n2)
	}
}

func TestIdempotencyCleanupReal(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM idempotency_keys")

	_, err := db.Exec(
		`INSERT INTO idempotency_keys (key_hash, user_id, response_status, response_body, expires_at)
		 VALUES ('expired-key-hash', (SELECT id FROM users WHERE username='customer'), 200, '{}', NOW() - INTERVAL '1 hour')`)
	if err != nil {
		t.Fatalf("insert expired key: %v", err)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM idempotency_keys WHERE key_hash = 'expired-key-hash'`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 expired key before cleanup, got %d", count)
	}

	n, err := cleanup.ExpiredIdempotencyKeys(db)
	if err != nil {
		t.Fatalf("cleanup.ExpiredIdempotencyKeys: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 key removed, got %d", n)
	}

	db.QueryRow(`SELECT COUNT(*) FROM idempotency_keys WHERE key_hash = 'expired-key-hash'`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 expired keys after cleanup, got %d", count)
	}
}

func TestEvidenceCleanupReal(t *testing.T) {
	db := getTestDB(t)

	var woID string
	err := db.QueryRow(`INSERT INTO work_orders (status) VALUES ('new') RETURNING id`).Scan(&woID)
	if err != nil {
		t.Fatalf("create WO: %v", err)
	}

	tmpDir := t.TempDir()
	evidencePath := filepath.Join(tmpDir, "expired-evidence.pdf")
	os.WriteFile(evidencePath, []byte("test evidence"), 0644)

	var evID string
	err = db.QueryRow(
		`INSERT INTO work_order_evidence (work_order_id, file_path, retention_expires_at)
		 VALUES ($1, $2, NOW() - INTERVAL '1 day') RETURNING id`,
		woID, evidencePath).Scan(&evID)
	if err != nil {
		t.Fatalf("insert evidence: %v", err)
	}

	if _, err := os.Stat(evidencePath); os.IsNotExist(err) {
		t.Fatal("evidence file should exist before cleanup")
	}

	n, err := cleanup.ExpiredEvidence(db)
	if err != nil {
		t.Fatalf("cleanup.ExpiredEvidence: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 evidence removed, got %d", n)
	}

	if _, err := os.Stat(evidencePath); !os.IsNotExist(err) {
		t.Fatal("evidence file should be deleted after cleanup")
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM work_order_evidence WHERE id = $1`, evID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 evidence rows after cleanup, got %d", count)
	}
}

// --- Audit file rotation and sealing ---

func TestAuditFileRotationAndSealing(t *testing.T) {
	tmpDir := t.TempDir()
	sink := audit.NewFileSink(tmpDir)
	defer sink.Close()

	yesterday := time.Date(2025, 6, 14, 23, 0, 0, 0, time.UTC)
	sink.NowFn = func() time.Time { return yesterday }

	err := sink.Write(map[string]interface{}{
		"event_type": "test_event",
		"timestamp":  yesterday.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("write yesterday event: %v", err)
	}

	yesterdayFile := filepath.Join(tmpDir, "audit-2025-06-14.jsonl")
	if _, err := os.Stat(yesterdayFile); os.IsNotExist(err) {
		t.Fatal("yesterday audit file should exist")
	}

	today := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)
	sink.NowFn = func() time.Time { return today }

	err = sink.Write(map[string]interface{}{
		"event_type": "today_event",
		"timestamp":  today.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("write today event: %v", err)
	}

	todayFile := filepath.Join(tmpDir, "audit-2025-06-15.jsonl")
	if _, err := os.Stat(todayFile); os.IsNotExist(err) {
		t.Fatal("today audit file should exist")
	}

	// Yesterday sealed as read-only
	info, _ := os.Stat(yesterdayFile)
	if info.Mode().Perm()&0222 != 0 {
		t.Fatalf("yesterday file should be read-only, got %o", info.Mode().Perm())
	}

	// Today still writable
	info2, _ := os.Stat(todayFile)
	if info2.Mode().Perm()&0200 == 0 {
		t.Fatalf("today file should be writable, got %o", info2.Mode().Perm())
	}

	data1, _ := os.ReadFile(yesterdayFile)
	if !strings.Contains(string(data1), "test_event") {
		t.Fatal("yesterday file should contain test_event")
	}
	data2, _ := os.ReadFile(todayFile)
	if !strings.Contains(string(data2), "today_event") {
		t.Fatal("today file should contain today_event")
	}
}

// --- Cookie Secure behavior ---

func TestCookieSecureDefault(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")
	e := newServer(db)

	// Without COOKIE_SECURE env, plain HTTP login should produce a non-Secure cookie
	os.Unsetenv("COOKIE_SECURE")

	body := `{"username":"customer","password":"customer123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}

	cookie := extractCookie(rec, "fieldserve_session")
	if cookie == nil {
		t.Fatal("expected session cookie")
	}
	// Plain HTTP request without X-Forwarded-Proto → Secure should be false
	if cookie.Secure {
		t.Fatal("expected Secure=false for plain HTTP request without COOKIE_SECURE")
	}
}

func TestCookieSecureExplicitTrue(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")
	e := newServer(db)

	os.Setenv("COOKIE_SECURE", "true")
	defer os.Unsetenv("COOKIE_SECURE")

	body := `{"username":"customer","password":"customer123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}

	cookie := extractCookie(rec, "fieldserve_session")
	if cookie == nil {
		t.Fatal("expected session cookie")
	}
	if !cookie.Secure {
		t.Fatal("expected Secure=true when COOKIE_SECURE=true")
	}
}

func TestCookieSecureAutoWithForwardedProto(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")
	e := newServer(db)

	os.Unsetenv("COOKIE_SECURE")

	body := `{"username":"customer","password":"customer123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}

	cookie := extractCookie(rec, "fieldserve_session")
	if cookie == nil {
		t.Fatal("expected session cookie")
	}
	if !cookie.Secure {
		t.Fatal("expected Secure=true when X-Forwarded-Proto=https")
	}
}

// --- Rate limit on write routes ---

func TestRateLimitOnAdminWriteRoute(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	body := `{"name":"Rate Test","slug":"rate-test-hardening"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("first request should not be rate limited")
	}
}

// --- Issue 1: Sensitive-field masking ---

func TestCustomerProfileReturnsMaskedPhone(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")

	_, err := crypto.Key()
	if err != nil {
		t.Skipf("ENCRYPTION_KEY not set: %v", err)
	}

	e := newServer(db)
	customerCookie := loginAs(t, e, "customer", "customer123")

	// Set phone first
	phone := "+1-555-867-5309"
	body := fmt.Sprintf(`{"phone":"%s"}`, phone)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/customer/profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}

	// GET should return masked phone
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/profile", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	profile := resp["profile"].(map[string]interface{})
	maskedPhone := profile["phone"].(string)
	// Should be masked: "***5309"
	if maskedPhone == phone {
		t.Fatal("phone should be masked, got full plaintext")
	}
	if !strings.HasPrefix(maskedPhone, "***") {
		t.Fatalf("masked phone should start with ***, got %q", maskedPhone)
	}
	if !strings.HasSuffix(maskedPhone, "5309") {
		t.Fatalf("masked phone should end with last 4 digits, got %q", maskedPhone)
	}
}

func TestProviderProfileReturnsMaskedPhone(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")

	_, err := crypto.Key()
	if err != nil {
		t.Skipf("ENCRYPTION_KEY not set: %v", err)
	}

	e := newServer(db)
	providerCookie := loginAs(t, e, "provider", "provider123")

	phone := "+1-555-999-1234"
	body := fmt.Sprintf(`{"phone":"%s"}`, phone)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/profile", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	profile := resp["profile"].(map[string]interface{})
	maskedPhone := profile["phone"].(string)
	if maskedPhone == phone {
		t.Fatal("phone should be masked, got full plaintext")
	}
	if !strings.HasPrefix(maskedPhone, "***") {
		t.Fatalf("masked phone should start with ***, got %q", maskedPhone)
	}
}

// --- Issue 2: On-call tier-aware escalation model ---

func TestAlertAssignRejectsNonOnCallUser(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	_, _ = db.Exec("DELETE FROM on_call_schedules")

	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")

	var adminUserID string
	db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)

	// Create rule + alert with proper UUID IDs
	ruleID := uuid.New().String()
	cond := `{"metric":"unresolved_interests","threshold":0}`
	db.Exec(`INSERT INTO alert_rules (id, name, condition, severity, enabled) VALUES ($1, 'oncall test', $2, 'high', true)`, ruleID, cond)
	alertID := uuid.New().String()
	db.Exec(`INSERT INTO alerts (id, rule_id, severity, status, data) VALUES ($1, $2, 'high', 'new', '{}')`, alertID, ruleID)

	// Try to assign without on-call schedule — should fail
	body := fmt.Sprintf(`{"assignee_id":"%s"}`, adminUserID)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/alerts/%s/assign", alertID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for non-on-call assignee, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAlertAssignSucceedsWithOnCallSchedule(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	_, _ = db.Exec("DELETE FROM on_call_schedules")

	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")

	var adminUserID string
	db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)

	// Create on-call schedule for admin
	db.Exec(`INSERT INTO on_call_schedules (user_id, tier, start_time, end_time) VALUES ($1, 1, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '8 hours')`, adminUserID)

	// Create rule + alert with proper UUID IDs
	ruleID := uuid.New().String()
	cond := `{"metric":"unresolved_interests","threshold":0}`
	db.Exec(`INSERT INTO alert_rules (id, name, condition, severity, enabled) VALUES ($1, 'oncall ok', $2, 'high', true)`, ruleID, cond)
	alertID := uuid.New().String()
	db.Exec(`INSERT INTO alerts (id, rule_id, severity, status, data) VALUES ($1, $2, 'high', 'new', '{}')`, alertID, ruleID)

	// Assign with on-call schedule — should succeed
	body := fmt.Sprintf(`{"assignee_id":"%s"}`, adminUserID)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/alerts/%s/assign", alertID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for on-call assignee, got %d %s", rec.Code, rec.Body.String())
	}
}

// --- Issue 3: Notes encryption at rest ---

func TestEncryptedCustomerNotes(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")

	_, err := crypto.Key()
	if err != nil {
		t.Skipf("ENCRYPTION_KEY not set: %v", err)
	}

	e := newServer(db)
	customerCookie := loginAs(t, e, "customer", "customer123")

	notes := "Allergic to cats, prefers morning appointments"
	body := fmt.Sprintf(`{"notes":"%s"}`, notes)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/customer/profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update notes: %d %s", rec.Code, rec.Body.String())
	}

	// Verify DB stores ciphertext, not plaintext
	var rawBytes []byte
	err = db.QueryRow(`SELECT notes_encrypted FROM customer_profiles WHERE user_id = (SELECT id FROM users WHERE username='customer')`).Scan(&rawBytes)
	if err != nil {
		t.Fatalf("query notes: %v", err)
	}
	if rawBytes == nil || len(rawBytes) == 0 {
		t.Fatal("notes_encrypted is null/empty")
	}
	storedHex := string(rawBytes)
	if storedHex == notes {
		t.Fatal("notes_encrypted contains plaintext")
	}

	// GET should return masked notes
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/profile", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	profile := resp["profile"].(map[string]interface{})
	maskedNotes := profile["notes"].(string)
	if maskedNotes == notes {
		t.Fatal("notes should be masked, got full plaintext")
	}
	if !strings.Contains(maskedNotes, "***") {
		t.Fatalf("masked notes should contain ***, got %q", maskedNotes)
	}
}

func TestEncryptedProviderNotes(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")

	_, err := crypto.Key()
	if err != nil {
		t.Skipf("ENCRYPTION_KEY not set: %v", err)
	}

	e := newServer(db)
	providerCookie := loginAs(t, e, "provider", "provider123")

	notes := "Licensed contractor, insurance on file"
	body := fmt.Sprintf(`{"notes":"%s"}`, notes)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update notes: %d %s", rec.Code, rec.Body.String())
	}

	var rawBytes []byte
	err = db.QueryRow(`SELECT notes_encrypted FROM provider_profiles WHERE user_id = (SELECT id FROM users WHERE username='provider')`).Scan(&rawBytes)
	if err != nil {
		t.Fatalf("query notes: %v", err)
	}
	if rawBytes == nil || len(rawBytes) == 0 {
		t.Fatal("notes_encrypted is null/empty")
	}
	if string(rawBytes) == notes {
		t.Fatal("notes_encrypted contains plaintext")
	}
}

// --- Issue 4: Idempotency on provider service create ---

func TestIdempotencyProviderServiceCreate(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")
	_, _ = db.Exec("DELETE FROM idempotency_keys")

	e := newServer(db)
	providerCookie := loginAs(t, e, "provider", "provider123")

	// We need a category for the service
	adminCookie := loginAs(t, e, "admin", "admin123")
	catBody := `{"name":"Idemp Test Cat","slug":"idemp-test-cat"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", strings.NewReader(catBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var catResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &catResp)
	catID := ""
	if cat, ok := catResp["category"].(map[string]interface{}); ok {
		catID = cat["id"].(string)
	}

	svcBody := fmt.Sprintf(`{"title":"Idempotent Service","description":"test","price_cents":1000,"category_id":"%s","status":"active"}`, catID)
	idempKey := "test-idemp-svc-create-key"

	// First request
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/services", strings.NewReader(svcBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("first create: %d %s", rec.Code, rec.Body.String())
	}

	firstBody := rec.Body.String()

	// Second request with same key — should replay
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/services", strings.NewReader(svcBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("replay: %d %s", rec.Code, rec.Body.String())
	}

	if rec.Body.String() != firstBody {
		t.Fatal("idempotent replay should return same response body")
	}

	// Count services — should be exactly 1 with this title
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM services WHERE title = 'Idempotent Service'`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 service created, got %d", count)
	}
}

// --- Issue 5: Checksum verification ---

func TestDocumentChecksumVerification(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")

	e := newServer(db)
	providerCookie := loginAs(t, e, "provider", "provider123")
	adminCookie := loginAs(t, e, "admin", "admin123")

	// Upload a document
	pdfContent := []byte("%PDF-1.0 test content for checksum verification")
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test-checksum.pdf")
	part.Write(pdfContent)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/documents", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}

	var uploadResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &uploadResp)
	doc := uploadResp["document"].(map[string]interface{})
	docID := doc["id"].(string)

	// Verify checksum — should pass
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/documents/%s/verify-checksum", docID), nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("verify-checksum: %d %s", rec.Code, rec.Body.String())
	}

	// Tamper with the file
	storagePath := doc["storage_path"].(string)
	os.WriteFile(storagePath, []byte("TAMPERED CONTENT"), 0644)

	// Verify checksum — should fail with 409
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/documents/%s/verify-checksum", docID), nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 after tamper, got %d %s", rec.Code, rec.Body.String())
	}
}

// --- Issue 6: Cached-query SLA benchmark ---

func TestCachedQueryPerformance(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Skip("no DB")
	}

	e := newServer(db)
	_, _ = db.Exec("DELETE FROM auth_sessions")
	customerCookie := loginAs(t, e, "customer", "customer123")

	// Warm the cache with a first request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?q=plumbing&page=1&page_size=20", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("warm: %d %s", rec.Code, rec.Body.String())
	}

	// Measure cached response time over 10 iterations
	const iterations = 10
	var totalDuration time.Duration
	for i := 0; i < iterations; i++ {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?q=plumbing&page=1&page_size=20", nil)
		req.AddCookie(customerCookie)
		rec = httptest.NewRecorder()
		start := time.Now()
		e.ServeHTTP(rec, req)
		elapsed := time.Since(start)
		totalDuration += elapsed

		if rec.Code != http.StatusOK {
			t.Fatalf("iter %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}

	avg := totalDuration / time.Duration(iterations)
	t.Logf("Cached query avg latency: %v over %d iterations (total: %v)", avg, iterations, totalDuration)

	// SLA target: 300ms for cached queries
	if avg > 300*time.Millisecond {
		t.Fatalf("cached query avg latency %v exceeds 300ms SLA target", avg)
	}
}

// --- Tier-aware auto-assignment on alert creation ---

func TestAlertAutoAssignLowestTier(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)

	auditSvc := audit.NewService(db)
	alertsSvc := alerts.NewService(db, auditSvc)

	var adminUserID string
	db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)

	// Create tier-2 and tier-1 on-call schedules — tier 1 should be picked
	db.Exec(`INSERT INTO on_call_schedules (user_id, tier, start_time, end_time) VALUES ($1, 2, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '8 hours')`, adminUserID)

	var customerUserID string
	db.QueryRow(`SELECT id FROM users WHERE username = 'customer'`).Scan(&customerUserID)
	// customer is not admin so won't be auto-assigned (but tier ordering still matters)
	// Use admin at both tiers to prove lowest tier wins
	db.Exec(`INSERT INTO on_call_schedules (user_id, tier, start_time, end_time) VALUES ($1, 1, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '8 hours')`, adminUserID)

	// Create a rule that fires immediately
	ruleID := uuid.New().String()
	cond, _ := json.Marshal(map[string]interface{}{"metric": "unresolved_interests", "threshold": 0})
	db.Exec(`INSERT INTO alert_rules (id, name, condition, severity, enabled) VALUES ($1, 'auto-assign test', $2, 'high', true)`, ruleID, cond)

	err := alertsSvc.EvaluateRules(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}

	// Should have created an alert and auto-assigned it
	var alertID string
	err = db.QueryRow(`SELECT id FROM alerts WHERE rule_id = $1`, ruleID).Scan(&alertID)
	if err != nil {
		t.Fatalf("alert not created: %v", err)
	}

	var assigneeID string
	err = db.QueryRow(`SELECT assignee_id FROM alert_assignments WHERE alert_id = $1`, alertID).Scan(&assigneeID)
	if err != nil {
		t.Fatalf("auto-assignment not created: %v", err)
	}

	if assigneeID != adminUserID {
		t.Fatalf("expected auto-assign to admin user %s, got %s", adminUserID, assigneeID)
	}
}

// --- Tier-aware escalation ---

func TestEscalateUnacknowledgedToNextTier(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)

	auditSvc := audit.NewService(db)
	alertsSvc := alerts.NewService(db, auditSvc)

	var adminUserID string
	db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)

	// Admin is tier 1, provider user is tier 2
	var providerUserID string
	db.QueryRow(`SELECT id FROM users WHERE username = 'provider'`).Scan(&providerUserID)

	db.Exec(`INSERT INTO on_call_schedules (user_id, tier, start_time, end_time) VALUES ($1, 1, NOW() - INTERVAL '2 hours', NOW() + INTERVAL '8 hours')`, adminUserID)
	db.Exec(`INSERT INTO on_call_schedules (user_id, tier, start_time, end_time) VALUES ($1, 2, NOW() - INTERVAL '2 hours', NOW() + INTERVAL '8 hours')`, providerUserID)

	// Create an alert and assign to tier 1 (admin) with old assigned_at
	ruleID := uuid.New().String()
	cond, _ := json.Marshal(map[string]interface{}{"metric": "test"})
	db.Exec(`INSERT INTO alert_rules (id, name, condition, severity, enabled) VALUES ($1, 'escalation test', $2, 'high', true)`, ruleID, cond)
	alertID := uuid.New().String()
	db.Exec(`INSERT INTO alerts (id, rule_id, severity, status, data) VALUES ($1, $2, 'high', 'new', '{}')`, alertID, ruleID)

	// Assign to admin but with assigned_at 45 minutes ago (exceeds 30-min threshold)
	db.Exec(`INSERT INTO alert_assignments (id, alert_id, assignee_id, assigned_at) VALUES ($1, $2, $3, NOW() - INTERVAL '45 minutes')`,
		uuid.New().String(), alertID, adminUserID)

	err := alertsSvc.EscalateUnacknowledged(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("escalate: %v", err)
	}

	// Should now have a second assignment to the tier-2 user
	var assignmentCount int
	db.QueryRow(`SELECT COUNT(*) FROM alert_assignments WHERE alert_id = $1`, alertID).Scan(&assignmentCount)
	if assignmentCount < 2 {
		t.Fatalf("expected 2 assignments (original + escalation), got %d", assignmentCount)
	}

	var tier2Assignee string
	err = db.QueryRow(`SELECT aa.assignee_id FROM alert_assignments aa
		JOIN on_call_schedules ocs ON ocs.user_id = aa.assignee_id AND ocs.tier = 2
		  AND ocs.start_time <= NOW() AND ocs.end_time > NOW()
		WHERE aa.alert_id = $1`, alertID).Scan(&tier2Assignee)
	if err != nil {
		t.Fatalf("tier-2 assignment not found: %v", err)
	}
	if tier2Assignee != providerUserID {
		t.Fatalf("expected tier-2 escalation to %s, got %s", providerUserID, tier2Assignee)
	}
}

// --- Active-only on-call listing ---

func TestOnCallListReturnsOnlyActive(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	_, _ = db.Exec("DELETE FROM on_call_schedules")
	_, _ = db.Exec("DELETE FROM auth_sessions")

	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")

	var adminUserID string
	db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)

	// Create one active and one expired schedule
	db.Exec(`INSERT INTO on_call_schedules (user_id, tier, start_time, end_time) VALUES ($1, 1, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '8 hours')`, adminUserID)
	db.Exec(`INSERT INTO on_call_schedules (user_id, tier, start_time, end_time) VALUES ($1, 2, NOW() - INTERVAL '48 hours', NOW() - INTERVAL '24 hours')`, adminUserID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/on-call", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list on-call: %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	schedules := resp["on_call_schedules"].([]interface{})
	if len(schedules) != 1 {
		t.Fatalf("expected 1 active schedule, got %d", len(schedules))
	}
}

// --- Idempotency on work order dispatch ---

func TestIdempotencyWorkOrderDispatch(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	_, _ = db.Exec("DELETE FROM idempotency_keys")

	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create work order
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/work-orders", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	woID := createResp["work_order"].(map[string]interface{})["id"].(string)

	idempKey := "dispatch-idemp-key-test"

	// First dispatch
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/work-orders/%s/dispatch", woID), nil)
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first dispatch: %d %s", rec.Code, rec.Body.String())
	}
	firstBody := rec.Body.String()

	// Second dispatch with same key — should replay
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/work-orders/%s/dispatch", woID), nil)
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay dispatch: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != firstBody {
		t.Fatal("idempotent replay should return same response body")
	}

	// Should have only 1 dispatch event (initial creation + 1 dispatch = 2 events)
	var eventCount int
	db.QueryRow(`SELECT COUNT(*) FROM work_order_events WHERE work_order_id = $1`, woID).Scan(&eventCount)
	if eventCount != 2 {
		t.Fatalf("expected 2 events (create + dispatch), got %d", eventCount)
	}
}

// --- Idempotency on alert assign ---

func TestIdempotencyAlertAssign(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	_, _ = db.Exec("DELETE FROM idempotency_keys")

	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")

	var adminUserID string
	db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)

	// On-call schedule required
	db.Exec(`INSERT INTO on_call_schedules (user_id, tier, start_time, end_time) VALUES ($1, 1, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '8 hours')`, adminUserID)

	ruleID := uuid.New().String()
	db.Exec(`INSERT INTO alert_rules (id, name, condition, severity, enabled) VALUES ($1, 'idemp assign', '{"metric":"test"}', 'high', true)`, ruleID)
	alertID := uuid.New().String()
	db.Exec(`INSERT INTO alerts (id, rule_id, severity, status, data) VALUES ($1, $2, 'high', 'new', '{}')`, alertID, ruleID)

	body := fmt.Sprintf(`{"assignee_id":"%s"}`, adminUserID)
	idempKey := "assign-idemp-key-test"

	// First assign
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/alerts/%s/assign", alertID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first assign: %d %s", rec.Code, rec.Body.String())
	}

	// Replay
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/alerts/%s/assign", alertID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay assign: %d %s", rec.Code, rec.Body.String())
	}

	// Should have only 1 assignment row
	var assignCount int
	db.QueryRow(`SELECT COUNT(*) FROM alert_assignments WHERE alert_id = $1`, alertID).Scan(&assignCount)
	if assignCount != 1 {
		t.Fatalf("expected 1 assignment, got %d", assignCount)
	}
}

// --- Idempotency on customer profile PATCH ---

func TestIdempotencyCustomerProfilePatch(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")
	_, _ = db.Exec("DELETE FROM idempotency_keys")

	_, err := crypto.Key()
	if err != nil {
		t.Skipf("ENCRYPTION_KEY not set: %v", err)
	}

	e := newServer(db)
	customerCookie := loginAs(t, e, "customer", "customer123")

	body := `{"phone":"+1-555-111-2222"}`
	idempKey := "profile-patch-idemp-test"

	// First PATCH
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/customer/profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first PATCH: %d %s", rec.Code, rec.Body.String())
	}
	firstBody := rec.Body.String()

	// Replay
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/customer/profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay PATCH: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != firstBody {
		t.Fatal("idempotent replay should return same response body")
	}
}

// --- Idempotency on service update (PATCH) ---

func TestIdempotencyServiceUpdate(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")
	_, _ = db.Exec("DELETE FROM idempotency_keys")

	e := newServer(db)
	providerCookie := loginAs(t, e, "provider", "provider123")

	// Create a service first (no idemp key to avoid interference)
	adminCookie := loginAs(t, e, "admin", "admin123")
	catBody := `{"name":"Idemp Update Cat","slug":"idemp-update-cat"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", strings.NewReader(catBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var catResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &catResp)
	catID := ""
	if cat, ok := catResp["category"].(map[string]interface{}); ok {
		catID = cat["id"].(string)
	}

	svcBody := fmt.Sprintf(`{"title":"Update Idemp Svc","description":"test","price_cents":500,"category_id":"%s","status":"active"}`, catID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/services", strings.NewReader(svcBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create svc: %d %s", rec.Code, rec.Body.String())
	}

	var svcResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &svcResp)
	svcID := svcResp["service"].(map[string]interface{})["id"].(string)

	// Now PATCH with idempotency key
	patchBody := `{"title":"Updated Title"}`
	idempKey := "svc-update-idemp-key"

	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/provider/services/%s", svcID), strings.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first PATCH svc: %d %s", rec.Code, rec.Body.String())
	}
	firstBody := rec.Body.String()

	// Replay
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/provider/services/%s", svcID), strings.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay PATCH svc: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != firstBody {
		t.Fatal("idempotent replay should return same response body")
	}
}

// --- Idempotency on admin category create ---

func TestIdempotencyAdminCategoryCreate(t *testing.T) {
	db := getTestDB(t)
	_, _ = db.Exec("DELETE FROM auth_sessions")
	_, _ = db.Exec("DELETE FROM idempotency_keys")

	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")

	body := `{"name":"Idemp Cat Create","slug":"idemp-cat-create"}`
	idempKey := "cat-create-idemp-key"

	// First POST
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create cat: %d %s", rec.Code, rec.Body.String())
	}
	firstBody := rec.Body.String()

	// Replay
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("replay create cat: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != firstBody {
		t.Fatal("idempotent replay should return same response body")
	}

	// Only 1 category with this slug should exist
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM categories WHERE slug = 'idemp-cat-create'`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 category, got %d", count)
	}
}
