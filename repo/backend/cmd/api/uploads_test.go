package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func cleanupUploadsData(t *testing.T, db *sql.DB) {
	t.Helper()
	_, _ = db.Exec("DELETE FROM provider_documents")
	_, _ = db.Exec("DELETE FROM export_jobs")
	_, _ = db.Exec("DELETE FROM analytics_daily_rollups")
	_, _ = db.Exec("DELETE FROM audit_event_index")
	_, _ = db.Exec("DELETE FROM auth_sessions")
}

func createMultipartFile(t *testing.T, fieldName, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write(content)
	writer.Close()
	return body, writer.FormDataContentType()
}

func TestValidUpload(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")

	// Create a small test file with PDF magic bytes
	pdfContent := []byte("%PDF-1.0 test content here for validation purposes")
	body, contentType := createMultipartFile(t, "file", "test.pdf", pdfContent)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/documents", body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	doc := resp["document"].(map[string]interface{})
	if doc["filename"] != "test.pdf" {
		t.Fatalf("expected filename test.pdf, got %v", doc["filename"])
	}
	if doc["checksum_sha256"] == nil || doc["checksum_sha256"] == "" {
		t.Fatal("expected non-empty checksum")
	}
	sizeBytes := doc["size_bytes"].(float64)
	if int(sizeBytes) != len(pdfContent) {
		t.Fatalf("expected size %d, got %v", len(pdfContent), sizeBytes)
	}
}

func TestExecutableRejected(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")

	body, contentType := createMultipartFile(t, "file", "malware.exe", []byte("MZ fake exe content"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/documents", body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestOversizedRejected(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")

	// Create a file larger than 10 MB
	bigContent := make([]byte, 11*1024*1024)
	copy(bigContent, []byte("%PDF-1.0 "))
	body, contentType := createMultipartFile(t, "file", "big.pdf", bigContent)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/documents", body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestDocumentListDelete(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")

	// Upload a document
	pdfContent := []byte("%PDF-1.0 test list delete")
	body, contentType := createMultipartFile(t, "file", "listdelete.pdf", pdfContent)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/documents", body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("upload expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var uploadResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &uploadResp)
	docID := uploadResp["document"].(map[string]interface{})["id"].(string)

	// List documents
	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/documents", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var listResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &listResp)
	docs := listResp["documents"].([]interface{})
	if len(docs) == 0 {
		t.Fatal("expected at least one document in list")
	}

	// Delete document
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/provider/documents/%s", docID), nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// List again — should be empty
	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/documents", nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	json.Unmarshal(rec.Body.Bytes(), &listResp)
	docs = listResp["documents"].([]interface{})
	if len(docs) != 0 {
		t.Fatalf("expected 0 documents after delete, got %d", len(docs))
	}
}

func TestAnalyticsUserGrowth(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/user-growth?from=2020-01-01&to=2030-12-31", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["metrics"]; !ok {
		t.Fatal("expected 'metrics' key in response")
	}
}

func TestAnalyticsConversion(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/conversion?from=2020-01-01&to=2030-12-31", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["metrics"]; !ok {
		t.Fatal("expected 'metrics' key in response")
	}
}

func TestAnalyticsProviderUtilization(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/provider-utilization", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["providers"]; !ok {
		t.Fatal("expected 'providers' key in response")
	}
}

func TestNonAdminExportRejected(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")

	body := `{"export_type":"user_growth","from":"2020-01-01","to":"2030-12-31"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/exports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestExportCreation(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	body := `{"export_type":"user_growth","from":"2020-01-01","to":"2030-12-31"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/exports", strings.NewReader(body))
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
	export := resp["export"].(map[string]interface{})
	if export["status"] != "completed" {
		t.Fatalf("expected status completed, got %v", export["status"])
	}
	if export["file_path"] == nil || export["file_path"] == "" {
		t.Fatal("expected non-empty file_path")
	}
}

func TestExportAuditEvent(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	body := `{"export_type":"conversion","from":"2020-01-01","to":"2030-12-31"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/exports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Check audit event
	var auditCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM audit_event_index WHERE event_type = 'export_created'`).Scan(&auditCount)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected export_created audit event, found none")
	}
}

func TestAPIAccessAudit(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Clear all api_access audit events
	_, _ = db.Exec("DELETE FROM audit_event_index WHERE event_type = 'api_access'")

	// Make an authenticated request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/user-growth?from=2020-01-01&to=2030-12-31", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Give the goroutine a moment to complete
	time.Sleep(200 * time.Millisecond)

	var auditCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM audit_event_index WHERE event_type = 'api_access'`).Scan(&auditCount)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected api_access audit event, found none")
	}
}

func TestDisallowedExtension(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")

	// Upload a .html file — valid MIME (text/html is sniffed from content) but not in allowed extensions
	htmlContent := []byte("<html><body>test</body></html>")
	body, contentType := createMultipartFile(t, "file", "page.html", htmlContent)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/documents", body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for disallowed extension .html, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Also test a .xyz file — completely unknown extension
	body2, contentType2 := createMultipartFile(t, "file", "data.xyz", []byte("random content"))
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/documents", body2)
	req.Header.Set("Content-Type", contentType2)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for disallowed extension .xyz, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------- Admin analytics rollups & exports list/get/download coverage ----------

// TestAdminAnalyticsRollup covers POST /api/v1/admin/analytics/rollup.
// It seeds an interest so there is something to roll up and asserts the
// rollup row appears in analytics_daily_rollups for today.
func TestAdminAnalyticsRollup(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	// Also clean engagement-related rows so seed counts are predictable
	_, _ = db.Exec("DELETE FROM message_receipts")
	_, _ = db.Exec("DELETE FROM messages")
	_, _ = db.Exec("DELETE FROM interest_status_events")
	_, _ = db.Exec("DELETE FROM interests")
	_, _ = db.Exec("DELETE FROM service_availability_windows")
	_, _ = db.Exec("DELETE FROM service_tags")
	_, _ = db.Exec("DELETE FROM services")
	_, _ = db.Exec("DELETE FROM categories")
	_, _ = db.Exec("DELETE FROM search_events")

	e := newServer(db)
	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "RollupCat", "rollup-cat")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Rollup Svc")
	svcID := svc["id"].(string)
	providerProfileID := svc["provider"].(map[string]interface{})["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")
	body := fmt.Sprintf(`{"service_id":%q,"provider_id":%q}`, svcID, providerProfileID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/interests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit interest: %d %s", rec.Code, rec.Body.String())
	}

	today := time.Now().Format("2006-01-02")
	rollupBody := fmt.Sprintf(`{"from":%q,"to":%q}`, today, today)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/analytics/rollup", strings.NewReader(rollupBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["message"] == nil || resp["message"] == "" {
		t.Fatal("expected non-empty message")
	}
	count, ok := resp["rollup_count"].(float64)
	if !ok {
		t.Fatalf("expected numeric rollup_count, got %T %v", resp["rollup_count"], resp["rollup_count"])
	}
	if int(count) < 1 {
		t.Fatalf("expected at least 1 rollup, got %d", int(count))
	}

	// Verify a row is actually in the rollups table for today
	var rowCount int
	db.QueryRow(`SELECT COUNT(*) FROM analytics_daily_rollups WHERE rollup_date = $1`, today).Scan(&rowCount)
	if rowCount < 1 {
		t.Fatalf("expected >=1 rollup row in DB for %s, got %d", today, rowCount)
	}
}

// TestAdminAnalyticsRollupAsCustomerForbidden verifies role enforcement on
// POST /admin/analytics/rollup.
func TestAdminAnalyticsRollupAsCustomerForbidden(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/analytics/rollup", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminListExports covers GET /api/v1/admin/exports and asserts the most
// recently created export is present with completed status.
func TestAdminListExports(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create a fresh export
	body := `{"export_type":"user_growth","from":"2020-01-01","to":"2030-12-31"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/exports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed export: %d %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	createdID := createResp["export"].(map[string]interface{})["id"].(string)

	// GET /admin/exports
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/exports", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	exports := resp["exports"].([]interface{})
	if len(exports) < 1 {
		t.Fatal("expected at least 1 export")
	}

	found := false
	for _, raw := range exports {
		obj := raw.(map[string]interface{})
		if obj["id"] == createdID {
			found = true
			if obj["status"] != "completed" {
				t.Fatalf("expected status completed, got %v", obj["status"])
			}
			if obj["export_type"] != "user_growth" {
				t.Fatalf("expected export_type user_growth, got %v", obj["export_type"])
			}
		}
	}
	if !found {
		t.Fatalf("created export %s not present in list", createdID)
	}
}

// TestAdminGetExportSuccessAndNotFound covers GET /api/v1/admin/exports/:exportId.
func TestAdminGetExportSuccessAndNotFound(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create
	body := `{"export_type":"conversion","from":"2020-01-01","to":"2030-12-31"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/exports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed export: %d %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	exportID := createResp["export"].(map[string]interface{})["id"].(string)

	// GET success
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/exports/"+exportID, nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	export := resp["export"].(map[string]interface{})
	if export["id"] != exportID {
		t.Fatalf("expected id %s, got %v", exportID, export["id"])
	}
	if export["status"] != "completed" {
		t.Fatalf("expected status completed, got %v", export["status"])
	}
	if export["export_type"] != "conversion" {
		t.Fatalf("expected export_type conversion, got %v", export["export_type"])
	}
	if export["file_path"] == nil || export["file_path"] == "" {
		t.Fatal("expected non-empty file_path on completed export")
	}

	// GET 404 for unknown id
	missingID := "00000000-0000-0000-0000-0000000000ff"
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/exports/"+missingID, nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminDownloadExport covers GET /api/v1/admin/exports/:exportId/download.
// It asserts headers/content type and a non-empty body containing the CSV
// header row produced by the analytics service.
func TestAdminDownloadExport(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	body := `{"export_type":"provider_utilization","from":"2020-01-01","to":"2030-12-31"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/exports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed export: %d %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	exportID := createResp["export"].(map[string]interface{})["id"].(string)

	// Download
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/exports/"+exportID+"/download", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	if rec.Body.Len() == 0 {
		t.Fatal("expected non-empty download body")
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType == "" {
		t.Fatal("expected Content-Type header to be set on download")
	}

	// CSV content for provider_utilization starts with the column header row.
	bodyStr := rec.Body.String()
	if !strings.Contains(bodyStr, "id,business_name,active_services,total_interests,messages_sent") {
		t.Fatalf("expected CSV header row in downloaded body, got first 200 chars: %q",
			bodyStr[:min(len(bodyStr), 200)])
	}
}

// TestAdminDownloadExportNotFound covers the not-found path on download.
//

func TestAdminDownloadExportNotFound(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	missingID := "00000000-0000-0000-0000-0000000000fe"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/exports/"+missingID+"/download", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminListExportsAsCustomerForbidden verifies role enforcement.
func TestAdminListExportsAsCustomerForbidden(t *testing.T) {
	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/exports", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestStoragePathConfinement(t *testing.T) {
	// This test proves that user-controlled filenames cannot escape the uploads root.
	// The upload handler uses uuid-based filenames and verifies the resolved absolute
	// path stays under UploadsRoot.

	db := getTestDB(t)
	cleanupUploadsData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")

	// Attempt to upload with a path-traversal filename
	traversalNames := []string{
		"../../../etc/passwd.pdf",
		"..%2F..%2Fetc/shadow.pdf",
		"/absolute/path.pdf",
		"subdir/../../../escape.pdf",
	}

	for _, name := range traversalNames {
		// Use valid PDF content so MIME check passes
		pdfContent := []byte("%PDF-1.0 test content for path confinement")
		body, contentType := createMultipartFile(t, "file", name, pdfContent)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/documents", body)
		req.Header.Set("Content-Type", contentType)
		req.AddCookie(providerCookie)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code == http.StatusCreated {
			// If it succeeds, the stored path MUST be under the uploads root.
			// The handler uses uuid-based filenames, so the original traversal
			// name is stripped. Verify the stored path.
			var resp map[string]interface{}
			json.Unmarshal(rec.Body.Bytes(), &resp)
			doc := resp["document"].(map[string]interface{})
			storagePath := doc["storage_path"].(string)

			// The stored filename must be a UUID, not the user-controlled name
			if strings.Contains(storagePath, "..") {
				t.Fatalf("storage path contains traversal for filename %q: %s", name, storagePath)
			}
			if strings.Contains(storagePath, "etc") {
				t.Fatalf("storage path escaped to /etc for filename %q: %s", name, storagePath)
			}
			// Verify it's under the expected root
			if !strings.HasPrefix(storagePath, "/app/data/uploads/") {
				t.Fatalf("storage path not under uploads root for filename %q: %s", name, storagePath)
			}

			// Clean up: delete the document
			docID := doc["id"].(string)
			req = httptest.NewRequest(http.MethodDelete, "/api/v1/provider/documents/"+docID, nil)
			req.AddCookie(providerCookie)
			rec = httptest.NewRecorder()
			e.ServeHTTP(rec, req)
		}
		// If it fails with 415 (due to MIME sniff), that's also acceptable — the file is rejected
	}

	// Verify no documents ended up outside the uploads root
	rows, err := db.Query(`SELECT storage_path FROM provider_documents`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		rows.Scan(&path)
		if !strings.HasPrefix(path, "/app/data/uploads/") {
			t.Fatalf("found document outside uploads root: %s", path)
		}
	}
}
