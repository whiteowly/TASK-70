package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fieldserve/internal/platform/httpx"

	"github.com/labstack/echo/v4"
)

func cleanupEngagementData(t *testing.T, db *sql.DB) {
	t.Helper()
	_, _ = db.Exec("DELETE FROM message_receipts")
	_, _ = db.Exec("DELETE FROM messages")
	_, _ = db.Exec("DELETE FROM interest_status_events")
	_, _ = db.Exec("DELETE FROM interests")
	_, _ = db.Exec("DELETE FROM blocks")
	_, _ = db.Exec("DELETE FROM idempotency_keys")
	cleanupCatalogData(t, db)
}


func TestCustomerSubmitInterest(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	interest := resp["interest"].(map[string]interface{})
	if interest["status"] != "submitted" {
		t.Fatalf("expected status submitted, got %v", interest["status"])
	}
	if interest["service_id"] != svcID {
		t.Fatalf("expected service_id %s, got %v", svcID, interest["service_id"])
	}
}

func TestInterestRejectsMismatchedServiceProvider(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Mismatch", "mismatch")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Mismatch Service")
	svcID := svc["id"].(string)

	// Create a second provider
	_, _ = db.Exec(`INSERT INTO users (id, username, password_hash, email) VALUES ('cccccccc-0000-0000-0000-000000000001', 'provider_other', crypt('providerother', gen_salt('bf')), 'pother@test.com') ON CONFLICT DO NOTHING`)
	var providerRoleID string
	db.QueryRow(`SELECT id FROM roles WHERE name = 'provider'`).Scan(&providerRoleID)
	_, _ = db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES ('cccccccc-0000-0000-0000-000000000001', $1) ON CONFLICT DO NOTHING`, providerRoleID)
	_, _ = db.Exec(`INSERT INTO provider_profiles (id, user_id, business_name) VALUES ('cccccccc-0000-0000-0000-000000000002', 'cccccccc-0000-0000-0000-000000000001', 'Other Provider') ON CONFLICT DO NOTHING`)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Try to submit interest with the service belonging to provider1 but provider_id of provider2
	wrongProviderID := "cccccccc-0000-0000-0000-000000000002"
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, wrongProviderID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for mismatched service/provider, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "validation_error" {
		t.Fatalf("expected code validation_error, got %v", errObj["code"])
	}
	fieldErrors := errObj["field_errors"].(map[string]interface{})
	if _, ok := fieldErrors["service_id"]; !ok {
		t.Fatal("expected field_errors to contain service_id")
	}
}

func TestDuplicateInterestRule(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)

	// First submit
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first submit: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Second submit (duplicate)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "duplicate_interest" {
		t.Fatalf("expected code duplicate_interest, got %v", errObj["code"])
	}
	fieldErrors := errObj["field_errors"].(map[string]interface{})
	if _, ok := fieldErrors["provider_id"]; !ok {
		t.Fatal("expected field_errors to contain provider_id")
	}
}

func TestCustomerWithdraw(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit: expected 201, got %d", rec.Code)
	}
	var submitResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &submitResp)
	interestID := submitResp["interest"].(map[string]interface{})["id"].(string)

	// Withdraw
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests/"+interestID+"/withdraw", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify status
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/interests/"+interestID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var getResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &getResp)
	interest := getResp["interest"].(map[string]interface{})
	if interest["status"] != "withdrawn" {
		t.Fatalf("expected status withdrawn, got %v", interest["status"])
	}
}

func TestProviderAccept(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit interest
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var submitResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &submitResp)
	interestID := submitResp["interest"].(map[string]interface{})["id"].(string)

	// Accept as provider
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/interests/"+interestID+"/accept", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify status via customer get
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/interests/"+interestID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var getResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &getResp)
	interest := getResp["interest"].(map[string]interface{})
	if interest["status"] != "accepted" {
		t.Fatalf("expected status accepted, got %v", interest["status"])
	}
}

func TestProviderDecline(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit failed: %d", rec.Code)
	}
	var submitResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &submitResp)
	interestID := submitResp["interest"].(map[string]interface{})["id"].(string)

	// Decline
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/interests/"+interestID+"/decline", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/interests/"+interestID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var getResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &getResp)
	interest := getResp["interest"].(map[string]interface{})
	if interest["status"] != "declined" {
		t.Fatalf("expected status declined, got %v", interest["status"])
	}
}

func TestInterestObjectAuthorization(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)

	// Create a second customer
	_, _ = db.Exec(`INSERT INTO users (id, username, password_hash, email) VALUES ('bbbbbbbb-0000-0000-0000-000000000001', 'customer2', crypt('customer2pw', gen_salt('bf')), 'customer2@test.com') ON CONFLICT DO NOTHING`)
	var customerRoleID string
	err := db.QueryRow(`SELECT id FROM roles WHERE name = 'customer'`).Scan(&customerRoleID)
	if err != nil {
		t.Fatalf("customer role not found: %v", err)
	}
	_, _ = db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES ('bbbbbbbb-0000-0000-0000-000000000001', $1) ON CONFLICT DO NOTHING`, customerRoleID)
	_, _ = db.Exec(`INSERT INTO customer_profiles (id, user_id) VALUES ('bbbbbbbb-0000-0000-0000-000000000002', 'bbbbbbbb-0000-0000-0000-000000000001') ON CONFLICT DO NOTHING`)

	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	// Customer 1 submits interest
	customer1Cookie := loginAs(t, e, "customer", "customer123")
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customer1Cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit failed: %d %s", rec.Code, rec.Body.String())
	}
	var submitResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &submitResp)
	interestID := submitResp["interest"].(map[string]interface{})["id"].(string)

	// Customer 2 tries to view Customer 1's interest
	customer2Cookie := loginAs(t, e, "customer2", "customer2pw")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/interests/"+interestID, nil)
	req.AddCookie(customer2Cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for other customer's interest, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestMessageSend(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit interest
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit failed: %d %s", rec.Code, rec.Body.String())
	}
	var submitResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &submitResp)
	interestID := submitResp["interest"].(map[string]interface{})["id"].(string)

	// Send message (thread = interest ID)
	msgBody := `{"body":"Hello provider!"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+interestID, strings.NewReader(msgBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var msgResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &msgResp)
	msg := msgResp["message"].(map[string]interface{})
	if msg["body"] != "Hello provider!" {
		t.Fatalf("expected message body 'Hello provider!', got %v", msg["body"])
	}
	if msg["thread_id"] != interestID {
		t.Fatalf("expected thread_id %s, got %v", interestID, msg["thread_id"])
	}
}

func TestMessageRead(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit interest
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit failed: %d", rec.Code)
	}
	var submitResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &submitResp)
	interestID := submitResp["interest"].(map[string]interface{})["id"].(string)

	// Customer sends message
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+interestID, strings.NewReader(`{"body":"Hi!"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("send message failed: %d %s", rec.Code, rec.Body.String())
	}

	// Provider marks read
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/messages/"+interestID+"/read", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("mark read: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Provider gets thread — verify read status
	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/messages/"+interestID, nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get thread: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var threadResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &threadResp)
	msgs := threadResp["messages"].([]interface{})
	if len(msgs) < 1 {
		t.Fatal("expected at least 1 message")
	}
	firstMsg := msgs[0].(map[string]interface{})
	if firstMsg["read_status"] != "read" {
		t.Fatalf("expected read_status 'read', got %v", firstMsg["read_status"])
	}
}

func TestBlockedMessageSend(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit interest first (before blocking)
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit failed: %d %s", rec.Code, rec.Body.String())
	}
	var submitResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &submitResp)
	interestID := submitResp["interest"].(map[string]interface{})["id"].(string)

	// Customer blocks provider
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/blocks/"+providerID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("block: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Try to send message — should be blocked
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+interestID, strings.NewReader(`{"body":"Hi!"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestBlockedInterestSubmit(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Block provider first
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/blocks/"+providerID, nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("block: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Try to submit interest — should be blocked
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestBlockedProviderHiddenFromSearch(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Blocked Provider Service")
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Verify service appears in search before blocking
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	total := int(resp["total"].(float64))
	if total < 1 {
		t.Fatal("expected at least 1 service before blocking")
	}

	// Block the provider
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/blocks/"+providerID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("block: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Search again — blocked provider's service should be excluded
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	json.Unmarshal(rec.Body.Bytes(), &resp)
	totalAfter := int(resp["total"].(float64))
	if totalAfter >= total {
		t.Fatalf("expected fewer services after blocking, got %d (was %d)", totalAfter, total)
	}
}

func TestIdempotencyInterest(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)

	idempotencyKey := "test-interest-key-123"

	// First request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	firstBody := rec.Body.String()

	// Second request with same key — should replay
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("replay: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	secondBody := rec.Body.String()
	if firstBody != secondBody {
		t.Fatalf("idempotency replay mismatch:\nfirst:  %s\nsecond: %s", firstBody, secondBody)
	}

	// Verify only 1 interest was created
	var count int
	db.QueryRow("SELECT COUNT(*) FROM interests").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 interest, got %d", count)
	}
}

func TestIdempotencyMessage(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Pipe Fix")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit interest
	interestBody := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(interestBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit failed: %d", rec.Code)
	}
	var submitResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &submitResp)
	interestID := submitResp["interest"].(map[string]interface{})["id"].(string)

	idempotencyKey := "test-msg-key-456"
	msgBody := `{"body":"Hello!"}`

	// First message
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+interestID, strings.NewReader(msgBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("first msg: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Second message with same key
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+interestID, strings.NewReader(msgBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("replay msg: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify only 1 message was created
	var count int
	db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 message, got %d", count)
	}
}

func TestRateLimiting(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)

	// Create a server with a custom rate limiter (low limit for testing)
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = httpx.ErrorHandler

	rateLimiter := httpx.NewRateLimiter(3, time.Minute)
	e.Use(rateLimiter.Middleware(func(c echo.Context) string {
		return "test-user"
	}))
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	// 4th request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestBlockedServiceDetailAccess(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "BlockDetailCat", "block-detail-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Blocked Detail Service")
	serviceID := svc["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Service is accessible before block
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services/"+serviceID, nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 before block, got %d", rec.Code)
	}

	// Customer blocks the provider
	var providerProfileID string
	db.QueryRow(`SELECT id FROM provider_profiles WHERE user_id = (SELECT id FROM users WHERE username='provider')`).Scan(&providerProfileID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/blocks/"+providerProfileID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("block failed: %d %s", rec.Code, rec.Body.String())
	}

	// Service detail should now return 404
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services/"+serviceID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for blocked provider's service, got %d", rec.Code)
	}

	// Provider can still see their own service via provider endpoint
	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/services/"+serviceID, nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("provider should still see own service, got %d", rec.Code)
	}
}

func TestIdempotencyScopedByUserAndPath(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "IdempScopeCat", "idemp-scope-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Idemp Scope Service")
	serviceID := svc["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit interest with a specific idempotency key
	sharedKey := "shared-key-12345"
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, serviceID, svc["provider"].(map[string]interface{})["id"])

	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", sharedKey)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first interest submit failed: %d %s", rec.Code, rec.Body.String())
	}

	var resp1 map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp1)
	interest1 := resp1["interest"].(map[string]interface{})

	// Same key, same user, same path → should replay (same interest ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", sharedKey)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("replay should return 201, got %d", rec.Code)
	}

	var resp2 map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp2)
	interest2 := resp2["interest"].(map[string]interface{})

	if interest1["id"] != interest2["id"] {
		t.Fatalf("replay should return same interest ID: %v vs %v", interest1["id"], interest2["id"])
	}

	// Verify only 1 interest was created in DB
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM interests`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 interest, got %d", count)
	}

	// Same key used on a DIFFERENT endpoint (message send) should NOT replay the interest response
	// First we need an accepted interest to send a message
	var providerProfileID string
	db.QueryRow(`SELECT id FROM provider_profiles WHERE user_id = (SELECT id FROM users WHERE username='provider')`).Scan(&providerProfileID)

	// Accept the interest as provider
	interestID := interest1["id"].(string)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/interests/"+interestID+"/accept", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Now try to send a message with the SAME idempotency key
	msgBody := `{"body":"Hello!"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+interestID, strings.NewReader(msgBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", sharedKey)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// This should NOT replay the interest creation response — it should create a new message
	if rec.Code != http.StatusCreated {
		t.Fatalf("message send with same key on different path should succeed with 201, got %d %s", rec.Code, rec.Body.String())
	}

	// Verify the response is a message, not an interest
	var msgResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &msgResp)
	if _, ok := msgResp["message"]; !ok {
		t.Fatalf("expected message response, got: %v", msgResp)
	}
	if _, hasInterest := msgResp["interest"]; hasInterest {
		t.Fatal("message endpoint with same key replayed interest response — scoping is broken")
	}
}

func TestIdempotencyConcurrentSameKey(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "ConcurrCat", "concurr-cat")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Concurr Service")
	serviceID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	// Accept so we can message
	customerCookie := loginAs(t, e, "customer", "customer123")
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, serviceID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}
	var intResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &intResp)
	interestID := intResp["interest"].(map[string]interface{})["id"].(string)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/interests/"+interestID+"/accept", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Fire N concurrent message sends with the same idempotency key.
	concurrency := 5
	idemKey := "concurrent-test-key-msg"
	type result struct {
		status int
		body   string
	}
	results := make(chan result, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			msgBody := `{"body":"Hello concurrent!"}`
			r := httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+interestID, strings.NewReader(msgBody))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", idemKey)
			r.AddCookie(customerCookie)
			w := httptest.NewRecorder()
			e.ServeHTTP(w, r)
			results <- result{status: w.Code, body: w.Body.String()}
		}()
	}

	statuses := map[int]int{}
	for i := 0; i < concurrency; i++ {
		r := <-results
		statuses[r.status]++
	}

	// All should get 201 (one original, rest replayed).
	if statuses[201] != concurrency {
		t.Fatalf("expected all %d requests to get 201, got status distribution: %v", concurrency, statuses)
	}

	// Only 1 message should exist in the DB for this thread.
	var msgCount int
	db.QueryRow(`SELECT COUNT(*) FROM messages WHERE thread_id = $1 AND body = 'Hello concurrent!'`, interestID).Scan(&msgCount)
	if msgCount != 1 {
		t.Fatalf("expected exactly 1 message from concurrent sends, got %d", msgCount)
	}
}
