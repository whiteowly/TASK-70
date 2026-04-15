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

// ---------- Customer engagement coverage ----------

// TestCustomerListInterests covers GET /api/v1/customer/interests. It seeds
// two interests against different providers and asserts both are returned in
// the customer's list.
func TestCustomerListInterests(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)

	// Second provider so we can submit two distinct active interests
	_, _ = db.Exec(`INSERT INTO users (id, username, password_hash, email) VALUES ('dddddddd-0000-0000-0000-000000000001', 'provider_list', crypt('providerlist', gen_salt('bf')), 'plist@test.com') ON CONFLICT DO NOTHING`)
	var providerRoleID string
	if err := db.QueryRow(`SELECT id FROM roles WHERE name = 'provider'`).Scan(&providerRoleID); err != nil {
		t.Fatalf("provider role: %v", err)
	}
	_, _ = db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES ('dddddddd-0000-0000-0000-000000000001', $1) ON CONFLICT DO NOTHING`, providerRoleID)
	_, _ = db.Exec(`INSERT INTO provider_profiles (id, user_id, business_name) VALUES ('dddddddd-0000-0000-0000-000000000002', 'dddddddd-0000-0000-0000-000000000001', 'Provider List') ON CONFLICT DO NOTHING`)

	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "ListInterests", "list-interests")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc1 := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Svc 1")
	provider1ProfileID := svc1["provider"].(map[string]interface{})["id"].(string)

	// Create a service belonging to the second provider via direct SQL so we don't need its login
	var svc2ID string
	if err := db.QueryRow(
		`INSERT INTO services (provider_id, category_id, title, description, price_cents, status)
		 VALUES ('dddddddd-0000-0000-0000-000000000002', $1, 'Svc 2', 'desc', 1000, 'active') RETURNING id`,
		cat["id"].(string)).Scan(&svc2ID); err != nil {
		t.Fatalf("insert svc2: %v", err)
	}

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit interest with provider 1
	body1 := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svc1["id"].(string), provider1ProfileID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body1))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest 1: %d %s", rec.Code, rec.Body.String())
	}

	// Submit interest with provider 2
	body2 := fmt.Sprintf(`{"service_id":%q,"provider_id":"dddddddd-0000-0000-0000-000000000002"}`, svc2ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body2))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest 2: %d %s", rec.Code, rec.Body.String())
	}

	// GET /customer/interests
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/interests", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	interests, ok := resp["interests"].([]interface{})
	if !ok {
		t.Fatalf("expected interests array, got %T", resp["interests"])
	}
	if len(interests) != 2 {
		t.Fatalf("expected 2 interests, got %d", len(interests))
	}

	// Each entry must have the expected ownership/identity fields
	for _, raw := range interests {
		obj := raw.(map[string]interface{})
		if obj["id"] == nil || obj["id"] == "" {
			t.Fatal("expected non-empty id")
		}
		if obj["status"] != "submitted" {
			t.Fatalf("expected status submitted, got %v", obj["status"])
		}
	}
}

// TestCustomerListInterestsRequiresCustomerRole verifies role enforcement on
// GET /customer/interests.
func TestCustomerListInterestsRequiresCustomerRole(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/customer/interests", nil)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerListAndGetMessages covers GET /customer/messages and
// GET /customer/messages/:threadId. After exchanging messages, the customer's
// thread list must show the thread with an unread count, and the per-thread
// fetch must return the messages in order.
func TestCustomerListAndGetMessages(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "MsgListCat", "msg-list-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Msg List Svc")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit interest -> creates thread id
	intBody := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(intBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}
	var sub map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &sub)
	threadID := sub["interest"].(map[string]interface{})["id"].(string)

	// Customer sends a message (so a thread exists for both sides)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+threadID,
		strings.NewReader(`{"body":"customer-first"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("customer send: %d %s", rec.Code, rec.Body.String())
	}

	// Provider replies — this gives the customer an unread message.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/messages/"+threadID,
		strings.NewReader(`{"body":"provider-reply"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provider send: %d %s", rec.Code, rec.Body.String())
	}

	// GET /customer/messages
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/messages", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list threads: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var listResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	threads := listResp["threads"].([]interface{})
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	thread := threads[0].(map[string]interface{})
	if thread["thread_id"] != threadID {
		t.Fatalf("expected thread_id %s, got %v", threadID, thread["thread_id"])
	}
	// Provider replied last, customer hasn't read it yet → unread_count >= 1
	if int(thread["unread_count"].(float64)) < 1 {
		t.Fatalf("expected unread_count >= 1 for customer, got %v", thread["unread_count"])
	}
	if thread["last_message"] != "provider-reply" {
		t.Fatalf("expected last_message 'provider-reply', got %v", thread["last_message"])
	}

	// GET /customer/messages/:threadId
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/messages/"+threadID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get thread: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var threadResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &threadResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	msgs := threadResp["messages"].([]interface{})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in thread, got %d", len(msgs))
	}
	first := msgs[0].(map[string]interface{})
	second := msgs[1].(map[string]interface{})
	if first["body"] != "customer-first" {
		t.Fatalf("expected first body 'customer-first', got %v", first["body"])
	}
	if second["body"] != "provider-reply" {
		t.Fatalf("expected second body 'provider-reply', got %v", second["body"])
	}
}

// TestCustomerGetThreadOtherCustomerForbidden verifies a different customer
// cannot see a thread they aren't a participant in (404 from GET
// /customer/messages/:threadId).
func TestCustomerGetThreadOtherCustomerForbidden(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)

	// Second customer
	_, _ = db.Exec(`INSERT INTO users (id, username, password_hash, email) VALUES ('eeeeeeee-0000-0000-0000-000000000001', 'customer_msg2', crypt('customermsg2', gen_salt('bf')), 'cmsg2@test.com') ON CONFLICT DO NOTHING`)
	var customerRoleID string
	if err := db.QueryRow(`SELECT id FROM roles WHERE name = 'customer'`).Scan(&customerRoleID); err != nil {
		t.Fatalf("customer role: %v", err)
	}
	_, _ = db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES ('eeeeeeee-0000-0000-0000-000000000001', $1) ON CONFLICT DO NOTHING`, customerRoleID)
	_, _ = db.Exec(`INSERT INTO customer_profiles (id, user_id) VALUES ('eeeeeeee-0000-0000-0000-000000000002', 'eeeeeeee-0000-0000-0000-000000000001') ON CONFLICT DO NOTHING`)

	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "MsgGetCat", "msg-get-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Msg Get Svc")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customer1Cookie := loginAs(t, e, "customer", "customer123")
	intBody := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(intBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customer1Cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}
	var sub map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &sub)
	threadID := sub["interest"].(map[string]interface{})["id"].(string)

	// Customer 1 sends a message so the thread exists
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+threadID,
		strings.NewReader(`{"body":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customer1Cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("send message: %d %s", rec.Code, rec.Body.String())
	}

	// Customer 2 attempts to view the thread → 404
	customer2Cookie := loginAs(t, e, "customer_msg2", "customermsg2")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/messages/"+threadID, nil)
	req.AddCookie(customer2Cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-participant, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerMarkRead covers POST /api/v1/customer/messages/:threadId/read.
// After provider sends a message, customer marking the thread read flips the
// unread count to 0 in subsequent thread/list views.
func TestCustomerMarkRead(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "MarkReadCat", "mark-read-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "MarkRead Svc")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")
	intBody := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(intBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}
	var sub map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &sub)
	threadID := sub["interest"].(map[string]interface{})["id"].(string)

	// Customer sends a message so thread exists, then provider replies → customer has unread.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+threadID,
		strings.NewReader(`{"body":"open"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("customer first send: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/messages/"+threadID,
		strings.NewReader(`{"body":"reply"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provider reply: %d %s", rec.Code, rec.Body.String())
	}

	// POST /customer/messages/:threadId/read
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+threadID+"/read", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mark read: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var ackResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &ackResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ackResp["message"] == nil || ackResp["message"] == "" {
		t.Fatal("expected non-empty message field on mark-read response")
	}

	// Verify thread's read status reflected via /customer/messages/:threadId
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/messages/"+threadID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var threadResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &threadResp)
	msgs := threadResp["messages"].([]interface{})

	// The provider's reply (where customer is recipient) must now be 'read'.
	foundReplyRead := false
	for _, raw := range msgs {
		m := raw.(map[string]interface{})
		if m["body"] == "reply" {
			if m["read_status"] != "read" {
				t.Fatalf("expected provider's reply to be marked read for customer, got %v", m["read_status"])
			}
			foundReplyRead = true
		}
	}
	if !foundReplyRead {
		t.Fatal("did not find provider's reply in customer's thread fetch")
	}

	// Verify unread_count == 0 in /customer/messages
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/messages", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var listResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &listResp)
	threads := listResp["threads"].([]interface{})
	if len(threads) < 1 {
		t.Fatal("expected at least 1 thread")
	}
	if int(threads[0].(map[string]interface{})["unread_count"].(float64)) != 0 {
		t.Fatalf("expected unread_count=0 after mark-read, got %v",
			threads[0].(map[string]interface{})["unread_count"])
	}
}

// TestCustomerMarkReadOnInvalidThread verifies POST .../read on a thread the
// customer isn't a participant of returns 404.
func TestCustomerMarkReadOnInvalidThread(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	bogusID := "00000000-0000-0000-0000-000000000abc"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+bogusID+"/read", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerUnblockProvider covers DELETE /api/v1/customer/blocks/:providerId.
// First blocks a provider, verifies search excludes them, unblocks, then
// verifies the service reappears.
func TestCustomerUnblockProvider(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "UnblockCat", "unblock-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Unblock Svc")
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Block first
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/blocks/"+providerID, nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("block: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify blocked: search should exclude this provider's service
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	totalBlocked := int(resp["total"].(float64))

	// DELETE /customer/blocks/:providerId
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/customer/blocks/"+providerID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unblock: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var unblockResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &unblockResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if unblockResp["message"] == nil || unblockResp["message"] == "" {
		t.Fatal("expected non-empty message field")
	}

	// After unblock the service should be visible again
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var resp2 map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp2)
	totalAfterUnblock := int(resp2["total"].(float64))
	if totalAfterUnblock <= totalBlocked {
		t.Fatalf("expected more services after unblock (was %d, now %d)", totalBlocked, totalAfterUnblock)
	}

	// Verify block row is gone in DB
	var customerUserID string
	db.QueryRow(`SELECT id FROM users WHERE username = 'customer'`).Scan(&customerUserID)
	var providerUserID string
	db.QueryRow(`SELECT user_id FROM provider_profiles WHERE id=$1`, providerID).Scan(&providerUserID)
	var blockCount int
	db.QueryRow(`SELECT COUNT(*) FROM blocks WHERE blocker_id=$1 AND blocked_id=$2`, customerUserID, providerUserID).Scan(&blockCount)
	if blockCount != 0 {
		t.Fatalf("expected 0 block rows after unblock, got %d", blockCount)
	}
}

// TestCustomerUnblockUnknownProvider verifies DELETE /customer/blocks/:providerId
// returns 404 when the provider profile doesn't exist.
func TestCustomerUnblockUnknownProvider(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	missingID := "00000000-0000-0000-0000-000000000111"
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/customer/blocks/"+missingID, nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------- Provider engagement coverage ----------

// TestProviderListInterests covers GET /api/v1/provider/interests. After a
// customer submits an interest the provider's list must include it with the
// correct provider profile id.
func TestProviderListInterests(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "ProvListCat", "prov-list-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Prov List Svc")
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
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/interests", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	interests := resp["interests"].([]interface{})
	if len(interests) != 1 {
		t.Fatalf("expected 1 interest, got %d", len(interests))
	}
	got := interests[0].(map[string]interface{})
	if got["provider_id"] != providerID {
		t.Fatalf("expected provider_id %s, got %v", providerID, got["provider_id"])
	}
	if got["service_id"] != svcID {
		t.Fatalf("expected service_id %s, got %v", svcID, got["service_id"])
	}
	if got["status"] != "submitted" {
		t.Fatalf("expected status submitted, got %v", got["status"])
	}
}

// TestProviderListInterestsAsCustomerForbidden verifies role enforcement.
func TestProviderListInterestsAsCustomerForbidden(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/interests", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestProviderListMessages covers GET /api/v1/provider/messages — returns
// the provider's threads with last_message and unread_count.
func TestProviderListMessages(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "ProvMsgsCat", "prov-msgs-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Prov Msgs Svc")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")
	intBody := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(intBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}
	var sub map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &sub)
	threadID := sub["interest"].(map[string]interface{})["id"].(string)

	// Customer sends to provider — provider has 1 unread.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+threadID,
		strings.NewReader(`{"body":"hi-provider"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("customer send: %d %s", rec.Code, rec.Body.String())
	}

	// GET /provider/messages
	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/messages", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	threads := resp["threads"].([]interface{})
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	thread := threads[0].(map[string]interface{})
	if thread["thread_id"] != threadID {
		t.Fatalf("expected thread_id %s, got %v", threadID, thread["thread_id"])
	}
	if thread["last_message"] != "hi-provider" {
		t.Fatalf("expected last_message 'hi-provider', got %v", thread["last_message"])
	}
	if int(thread["unread_count"].(float64)) < 1 {
		t.Fatalf("expected unread_count >= 1 for provider, got %v", thread["unread_count"])
	}
}

// TestProviderSendMessage covers POST /api/v1/provider/messages/:threadId.
// Verifies provider can send into a thread their service participates in,
// with correct thread/sender/recipient.
func TestProviderSendMessage(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "ProvSendCat", "prov-send-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Prov Send Svc")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")
	intBody := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(intBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}
	var sub map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &sub)
	threadID := sub["interest"].(map[string]interface{})["id"].(string)

	// Provider sends — should succeed, recipient is the customer user id
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/messages/"+threadID,
		strings.NewReader(`{"body":"provider-says-hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	msg := resp["message"].(map[string]interface{})
	if msg["body"] != "provider-says-hi" {
		t.Fatalf("expected body 'provider-says-hi', got %v", msg["body"])
	}
	if msg["thread_id"] != threadID {
		t.Fatalf("expected thread_id %s, got %v", threadID, msg["thread_id"])
	}

	var providerUserID string
	db.QueryRow(`SELECT id FROM users WHERE username='provider'`).Scan(&providerUserID)
	if msg["sender_id"] != providerUserID {
		t.Fatalf("expected sender_id %s (provider user), got %v", providerUserID, msg["sender_id"])
	}

	var customerUserID string
	db.QueryRow(`SELECT id FROM users WHERE username='customer'`).Scan(&customerUserID)
	if msg["recipient_id"] != customerUserID {
		t.Fatalf("expected recipient_id %s (customer user), got %v", customerUserID, msg["recipient_id"])
	}
}

// TestProviderSendMessageEmptyBodyValidation verifies POST /provider/messages/:threadId
// returns 422 when the message body is empty.
func TestProviderSendMessageEmptyBodyValidation(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "ProvSendValCat", "prov-send-val-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Prov Val Svc")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")
	intBody := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(intBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}
	var sub map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &sub)
	threadID := sub["interest"].(map[string]interface{})["id"].(string)

	// Empty body → validation error
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/messages/"+threadID,
		strings.NewReader(`{"body":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty body, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestProviderBlockAndUnblockCustomer covers
//  - POST /api/v1/provider/blocks/:customerId
//  - DELETE /api/v1/provider/blocks/:customerId
// and verifies side effects: blocked customer cannot send a message in a
// thread to the provider; after unblock it works again.
func TestProviderBlockAndUnblockCustomer(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "ProvBlockCat", "prov-block-cat")
	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Prov Block Svc")
	svcID := svc["id"].(string)
	providerID := svc["provider"].(map[string]interface{})["id"].(string)

	// Resolve customer profile id (from seeded customer)
	var customerProfileID string
	if err := db.QueryRow(
		`SELECT cp.id FROM customer_profiles cp JOIN users u ON u.id = cp.user_id
		 WHERE u.username = 'customer'`).Scan(&customerProfileID); err != nil {
		t.Fatalf("get customer profile id: %v", err)
	}

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Submit interest first (so a thread exists)
	intBody := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(intBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}
	var sub map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &sub)
	threadID := sub["interest"].(map[string]interface{})["id"].(string)

	// POST /provider/blocks/:customerId
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/blocks/"+customerProfileID, nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("block: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var blockResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &blockResp)
	if blockResp["message"] == nil || blockResp["message"] == "" {
		t.Fatal("expected non-empty message field")
	}

	// Verify the block row exists in the DB
	var providerUserID string
	db.QueryRow(`SELECT user_id FROM provider_profiles WHERE id=$1`, providerID).Scan(&providerUserID)
	var customerUserID string
	db.QueryRow(`SELECT user_id FROM customer_profiles WHERE id=$1`, customerProfileID).Scan(&customerUserID)
	var blockCount int
	db.QueryRow(`SELECT COUNT(*) FROM blocks WHERE blocker_id=$1 AND blocked_id=$2`, providerUserID, customerUserID).Scan(&blockCount)
	if blockCount != 1 {
		t.Fatalf("expected 1 block row, got %d", blockCount)
	}

	// Customer attempt to send into the thread should now be 403 (blocked)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+threadID,
		strings.NewReader(`{"body":"after-block"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 after block, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// DELETE /provider/blocks/:customerId
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/provider/blocks/"+customerProfileID, nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unblock: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Customer can now send again
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/messages/"+threadID,
		strings.NewReader(`{"body":"after-unblock"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 after unblock, got %d; body: %s", rec.Code, rec.Body.String())
	}

	db.QueryRow(`SELECT COUNT(*) FROM blocks WHERE blocker_id=$1 AND blocked_id=$2`, providerUserID, customerUserID).Scan(&blockCount)
	if blockCount != 0 {
		t.Fatalf("expected 0 block rows after unblock, got %d", blockCount)
	}
}

// TestProviderBlockUnknownCustomer verifies POST /provider/blocks/:customerId
// returns 404 for an unknown customer profile id.
func TestProviderBlockUnknownCustomer(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")
	missingID := "00000000-0000-0000-0000-0000000000aa"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/blocks/"+missingID, nil)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestProviderUnblockUnknownCustomer verifies DELETE /provider/blocks/:customerId
// returns 404 for an unknown customer profile id.
func TestProviderUnblockUnknownCustomer(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")
	missingID := "00000000-0000-0000-0000-0000000000bb"
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/provider/blocks/"+missingID, nil)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestProviderBlocksRequireProviderRole verifies a customer can't call
// /provider/blocks/:customerId.
func TestProviderBlocksRequireProviderRole(t *testing.T) {
	db := getTestDB(t)
	cleanupEngagementData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/provider/blocks/00000000-0000-0000-0000-000000000001", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
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
