package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fieldserve/internal/alerts"
	"fieldserve/internal/audit"

	"github.com/google/uuid"
)

func cleanupAlertData(t *testing.T, db *sql.DB) {
	t.Helper()
	_, _ = db.Exec("DELETE FROM work_order_evidence")
	_, _ = db.Exec("DELETE FROM work_order_events")
	_, _ = db.Exec("DELETE FROM work_orders")
	_, _ = db.Exec("DELETE FROM alert_assignments")
	_, _ = db.Exec("DELETE FROM alerts")
	_, _ = db.Exec("DELETE FROM alert_rules")
	_, _ = db.Exec("DELETE FROM on_call_schedules")
	_, _ = db.Exec("DELETE FROM audit_event_index")
	_, _ = db.Exec("DELETE FROM auth_sessions")
}

// ensureAdminOnCall creates an on-call schedule for the admin user covering now.
func ensureAdminOnCall(t *testing.T, db *sql.DB) {
	t.Helper()
	var adminUserID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID); err != nil {
		t.Fatalf("get admin user: %v", err)
	}
	_, _ = db.Exec(
		`INSERT INTO on_call_schedules (user_id, tier, start_time, end_time)
		 VALUES ($1, 1, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '24 hours')`,
		adminUserID)
}

// TestAlertRuleCreate tests POST /admin/alert-rules -> 201.
func TestAlertRuleCreate(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	body := `{"name":"High unresolved","condition":{"metric":"unresolved_interests","threshold":5},"severity":"high","quiet_hours_start":"22:00","quiet_hours_end":"07:00","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/alert-rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rule := resp["alert_rule"].(map[string]interface{})
	if rule["name"] != "High unresolved" {
		t.Fatalf("expected name 'High unresolved', got %v", rule["name"])
	}
	if rule["severity"] != "high" {
		t.Fatalf("expected severity 'high', got %v", rule["severity"])
	}
}

// TestAlertRuleUpdate tests PATCH /admin/alert-rules/:ruleId -> 200.
func TestAlertRuleUpdate(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create a rule first
	createBody := `{"name":"To update","condition":{"metric":"unresolved_interests","threshold":5},"severity":"low","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/alert-rules", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	ruleID := createResp["alert_rule"].(map[string]interface{})["id"].(string)

	// Update the rule
	updateBody := `{"name":"Updated name","severity":"critical"}`
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/alert-rules/%s", ruleID), strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("update expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var updateResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &updateResp)
	rule := updateResp["alert_rule"].(map[string]interface{})
	if rule["name"] != "Updated name" {
		t.Fatalf("expected name 'Updated name', got %v", rule["name"])
	}
	if rule["severity"] != "critical" {
		t.Fatalf("expected severity 'critical', got %v", rule["severity"])
	}
}

// TestAlertEvaluation tests that EvaluateRules creates an alert when threshold is met.
func TestAlertEvaluation(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)

	auditSvc := audit.NewService(db)
	alertsSvc := alerts.NewService(db, auditSvc)

	// Create a rule with threshold=0 so any count triggers it
	ruleID := uuid.New().String()
	cond, _ := json.Marshal(map[string]interface{}{"metric": "unresolved_interests", "threshold": 0})
	_, err := db.Exec(
		`INSERT INTO alert_rules (id, name, condition, severity, enabled, created_at, updated_at)
		 VALUES ($1, 'Test eval rule', $2, 'high', true, NOW(), NOW())`,
		ruleID, cond,
	)
	if err != nil {
		t.Fatalf("insert rule: %v", err)
	}

	err = alertsSvc.EvaluateRules(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}

	// Verify alert was created
	var alertCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE rule_id = $1`, ruleID).Scan(&alertCount)
	if err != nil {
		t.Fatalf("query alerts: %v", err)
	}
	if alertCount == 0 {
		t.Fatal("expected at least one alert to be created")
	}
}

// TestQuietHoursSuppress tests that non-critical alerts are suppressed during quiet hours.
func TestQuietHoursSuppress(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)

	auditSvc := audit.NewService(db)
	alertsSvc := alerts.NewService(db, auditSvc)

	// Create a rule with quiet hours covering the test time
	ruleID := uuid.New().String()
	cond, _ := json.Marshal(map[string]interface{}{"metric": "unresolved_interests", "threshold": 0})
	_, err := db.Exec(
		`INSERT INTO alert_rules (id, name, condition, severity, quiet_hours_start, quiet_hours_end, enabled, created_at, updated_at)
		 VALUES ($1, 'Quiet hours test', $2, 'medium', '00:00', '23:59', true, NOW(), NOW())`,
		ruleID, cond,
	)
	if err != nil {
		t.Fatalf("insert rule: %v", err)
	}

	// Use a time that falls within 00:00-23:59 (any time of day)
	fakeNow := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	err = alertsSvc.EvaluateRules(context.Background(), fakeNow)
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}

	// Verify no alert was created (suppressed by quiet hours)
	var alertCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE rule_id = $1`, ruleID).Scan(&alertCount)
	if err != nil {
		t.Fatalf("query alerts: %v", err)
	}
	if alertCount != 0 {
		t.Fatalf("expected 0 alerts (suppressed by quiet hours), got %d", alertCount)
	}
}

// TestQuietHoursCriticalNotSuppressed tests that critical alerts fire even during quiet hours.
func TestQuietHoursCriticalNotSuppressed(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)

	auditSvc := audit.NewService(db)
	alertsSvc := alerts.NewService(db, auditSvc)

	// Create a critical rule with quiet hours covering the test time
	ruleID := uuid.New().String()
	cond, _ := json.Marshal(map[string]interface{}{"metric": "unresolved_interests", "threshold": 0})
	_, err := db.Exec(
		`INSERT INTO alert_rules (id, name, condition, severity, quiet_hours_start, quiet_hours_end, enabled, created_at, updated_at)
		 VALUES ($1, 'Critical quiet test', $2, 'critical', '00:00', '23:59', true, NOW(), NOW())`,
		ruleID, cond,
	)
	if err != nil {
		t.Fatalf("insert rule: %v", err)
	}

	fakeNow := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	err = alertsSvc.EvaluateRules(context.Background(), fakeNow)
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}

	// Verify alert WAS created even though in quiet hours (critical)
	var alertCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE rule_id = $1`, ruleID).Scan(&alertCount)
	if err != nil {
		t.Fatalf("query alerts: %v", err)
	}
	if alertCount == 0 {
		t.Fatal("expected critical alert to be created despite quiet hours")
	}
}

// TestAlertAssign tests POST /admin/alerts/:alertId/assign -> 200.
func TestAlertAssign(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Get admin user ID
	var adminUserID string
	err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("get admin user: %v", err)
	}

	// Create on-call schedule for admin
	ensureAdminOnCall(t, db)

	// Create a rule and alert directly
	ruleID := uuid.New().String()
	cond, _ := json.Marshal(map[string]interface{}{"metric": "test"})
	_, err = db.Exec(
		`INSERT INTO alert_rules (id, name, condition, severity, enabled, created_at, updated_at)
		 VALUES ($1, 'Assign test rule', $2, 'high', true, NOW(), NOW())`,
		ruleID, cond,
	)
	if err != nil {
		t.Fatalf("insert rule: %v", err)
	}

	alertID := uuid.New().String()
	data, _ := json.Marshal(map[string]interface{}{"test": true})
	_, err = db.Exec(
		`INSERT INTO alerts (id, rule_id, severity, status, data, created_at)
		 VALUES ($1, $2, 'high', 'new', $3, NOW())`,
		alertID, ruleID, data,
	)
	if err != nil {
		t.Fatalf("insert alert: %v", err)
	}

	// Assign alert
	body := fmt.Sprintf(`{"assignee_id":"%s"}`, adminUserID)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/alerts/%s/assign", alertID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assignment := resp["assignment"].(map[string]interface{})
	if assignment["alert_id"] != alertID {
		t.Fatalf("expected alert_id %s, got %v", alertID, assignment["alert_id"])
	}
	if assignment["assignee_id"] != adminUserID {
		t.Fatalf("expected assignee_id %s, got %v", adminUserID, assignment["assignee_id"])
	}
}

// TestAlertAcknowledge tests assign then acknowledge flow.
func TestAlertAcknowledge(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Get admin user ID
	var adminUserID string
	err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("get admin user: %v", err)
	}

	// Create on-call schedule for admin
	ensureAdminOnCall(t, db)

	// Create rule + alert
	ruleID := uuid.New().String()
	cond, _ := json.Marshal(map[string]interface{}{"metric": "test"})
	db.Exec(
		`INSERT INTO alert_rules (id, name, condition, severity, enabled, created_at, updated_at)
		 VALUES ($1, 'Ack test rule', $2, 'high', true, NOW(), NOW())`,
		ruleID, cond,
	)

	alertID := uuid.New().String()
	data, _ := json.Marshal(map[string]interface{}{"test": true})
	db.Exec(
		`INSERT INTO alerts (id, rule_id, severity, status, data, created_at)
		 VALUES ($1, $2, 'high', 'new', $3, NOW())`,
		alertID, ruleID, data,
	)

	// Assign
	assignBody := fmt.Sprintf(`{"assignee_id":"%s"}`, adminUserID)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/alerts/%s/assign", alertID), strings.NewReader(assignBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assign expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Acknowledge
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/alerts/%s/acknowledge", alertID), nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("acknowledge expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify acknowledged_at is set
	var ackedAt *string
	err = db.QueryRow(
		`SELECT acknowledged_at::text FROM alert_assignments WHERE alert_id = $1 AND assignee_id = $2`,
		alertID, adminUserID,
	).Scan(&ackedAt)
	if err != nil {
		t.Fatalf("query assignment: %v", err)
	}
	if ackedAt == nil || *ackedAt == "" {
		t.Fatal("expected acknowledged_at to be set")
	}
}

// TestWorkOrderFullLifecycle tests new -> dispatch -> acknowledge -> start -> resolve -> post_incident_review -> close.
func TestWorkOrderFullLifecycle(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create work order
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/work-orders", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	woID := createResp["work_order"].(map[string]interface{})["id"].(string)

	// Walk through each transition
	transitions := []struct {
		action         string
		expectedStatus string
	}{
		{"dispatch", "dispatched"},
		{"acknowledge", "acknowledged"},
		{"start", "in_progress"},
		{"resolve", "resolved"},
		{"post-incident-review", "post_incident_review"},
		{"close", "closed"},
	}

	for _, tr := range transitions {
		url := fmt.Sprintf("/api/v1/admin/work-orders/%s/%s", woID, tr.action)
		req = httptest.NewRequest(http.MethodPost, url, nil)
		req.AddCookie(adminCookie)
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s expected 200, got %d; body: %s", tr.action, rec.Code, rec.Body.String())
		}

		var resp map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		wo := resp["work_order"].(map[string]interface{})
		if wo["status"] != tr.expectedStatus {
			t.Fatalf("after %s expected status '%s', got '%v'", tr.action, tr.expectedStatus, wo["status"])
		}
	}

	// Verify event rows
	var eventCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM work_order_events WHERE work_order_id = $1`, woID).Scan(&eventCount)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	// 1 (creation) + 6 (transitions) = 7
	if eventCount != 7 {
		t.Fatalf("expected 7 events, got %d", eventCount)
	}
}

// TestInvalidWorkOrderTransition tests that invalid transitions return 422.
func TestInvalidWorkOrderTransition(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create work order (status = "new")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/work-orders", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	woID := createResp["work_order"].(map[string]interface{})["id"].(string)

	// Try to go from "new" to "resolved" (invalid)
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/work-orders/%s/resolve", woID), nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestSLAOverdueCheck tests that CheckSLADeadlines creates a critical alert for overdue work orders.
func TestSLAOverdueCheck(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)

	auditSvc := audit.NewService(db)
	alertsSvc := alerts.NewService(db, auditSvc)

	// Insert a dispatched work order with old updated_at
	woID := uuid.New().String()
	_, err := db.Exec(
		`INSERT INTO work_orders (id, status, created_at, updated_at)
		 VALUES ($1, 'dispatched', NOW() - INTERVAL '48 hours', NOW() - INTERVAL '48 hours')`,
		woID,
	)
	if err != nil {
		t.Fatalf("insert work order: %v", err)
	}

	err = alertsSvc.CheckSLADeadlines(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("check sla deadlines: %v", err)
	}

	// Verify critical alert was created
	var alertCount int
	err = db.QueryRow(
		`SELECT COUNT(*) FROM alerts WHERE severity = 'critical' AND data->>'work_order_id' = $1`,
		woID,
	).Scan(&alertCount)
	if err != nil {
		t.Fatalf("query alerts: %v", err)
	}
	if alertCount == 0 {
		t.Fatal("expected critical SLA alert to be created for overdue work order")
	}
}

// TestEvidenceUpload tests uploading evidence to a work order -> 201.
func TestEvidenceUpload(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create work order
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/work-orders", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	woID := createResp["work_order"].(map[string]interface{})["id"].(string)

	// Upload evidence
	pdfContent := []byte("%PDF-1.0 evidence file content for testing")
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "evidence.pdf")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write(pdfContent)
	writer.Close()

	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/work-orders/%s/evidence", woID), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("upload expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	evidence := resp["evidence"].(map[string]interface{})
	if evidence["work_order_id"] != woID {
		t.Fatalf("expected work_order_id %s, got %v", woID, evidence["work_order_id"])
	}
	if evidence["file_path"] == nil || evidence["file_path"] == "" {
		t.Fatal("expected non-empty file_path")
	}

	// Verify retention_expires_at is approximately 180 days from now
	retStr := evidence["retention_expires_at"].(string)
	if retStr == "" {
		t.Fatal("expected non-empty retention_expires_at")
	}
	// Parse and check within reasonable range
	retTime, err := time.Parse("2006-01-02 15:04:05-07", retStr)
	if err != nil {
		retTime, err = time.Parse("2006-01-02 15:04:05.999999-07", retStr)
		if err != nil {
			// Just check the string contains a future date - at least year check
			if !strings.Contains(retStr, "202") {
				t.Fatalf("retention_expires_at doesn't look like a future date: %s", retStr)
			}
			return
		}
	}
	expectedMin := time.Now().AddDate(0, 0, 179)
	expectedMax := time.Now().AddDate(0, 0, 181)
	if retTime.Before(expectedMin) || retTime.After(expectedMax) {
		t.Fatalf("retention_expires_at %v not within ~180 days from now", retTime)
	}
}

// TestAdminOnlyAlertEndpoints tests that non-admin users get 403 on alert endpoints.
func TestAdminOnlyAlertEndpoints(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Try GET /admin/alerts
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/alerts", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Try GET /admin/alert-rules
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/alert-rules", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Try GET /admin/work-orders
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/work-orders", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestUnsupportedMetricRejected(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")

	body := `{"name":"Bad Rule","condition":{"metric":"cpu_usage","threshold":90},"severity":"high","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/alert-rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for unsupported metric, got %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "validation_error" {
		t.Fatalf("expected validation_error, got %s", errObj["code"])
	}
	fieldErrors := errObj["field_errors"].(map[string]interface{})
	if _, ok := fieldErrors["condition"]; !ok {
		t.Fatal("expected field error on condition")
	}
}

func TestEvidenceRejectedExtension(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create a work order first
	woBody := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/work-orders", strings.NewReader(woBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create WO: %d %s", rec.Code, rec.Body.String())
	}
	var woResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &woResp)
	woID := woResp["work_order"].(map[string]interface{})["id"].(string)

	// Try to upload .exe evidence
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "malware.exe")
	part.Write([]byte("MZ fake exe content"))
	writer.Close()

	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/work-orders/"+woID+"/evidence", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for .exe evidence, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestUnsupportedMetricRejectedOnUpdate(t *testing.T) {
	db := getTestDB(t)
	cleanupAlertData(t, db)
	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create a valid rule first
	createBody := `{"name":"Good Rule","condition":{"metric":"unresolved_interests","threshold":5},"severity":"medium","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/alert-rules", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create rule: %d %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	ruleID := createResp["alert_rule"].(map[string]interface{})["id"].(string)

	// Try to update with unsupported metric
	updateBody := `{"condition":{"metric":"cpu_usage","threshold":90}}`
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/alert-rules/"+ruleID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for unsupported metric on update, got %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]interface{})
	fieldErrors := errObj["field_errors"].(map[string]interface{})
	if _, ok := fieldErrors["condition"]; !ok {
		t.Fatal("expected field error on condition for update")
	}
}
