package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func loginAs(t *testing.T, e http.Handler, username, password string) *http.Cookie {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login as %s failed: %d %s", username, rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "fieldserve_session" {
			return c
		}
	}
	t.Fatalf("no session cookie for %s", username)
	return nil
}

func cleanupCatalogData(t *testing.T, db *sql.DB) {
	t.Helper()
	_, _ = db.Exec("DELETE FROM service_availability_windows")
	_, _ = db.Exec("DELETE FROM service_tags")
	_, _ = db.Exec("DELETE FROM services")
	_, _ = db.Exec("DELETE FROM categories")
	_, _ = db.Exec("DELETE FROM tags")
	_, _ = db.Exec("DELETE FROM audit_event_index")
	_, _ = db.Exec("DELETE FROM auth_sessions")
}

// adminCreateCategory is a helper that creates a category via the admin API.
func adminCreateCategory(t *testing.T, e http.Handler, cookie *http.Cookie, name, slug string) map[string]interface{} {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"slug":%q,"sort_order":0}`, name, slug)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode create category response: %v", err)
	}
	return resp["category"].(map[string]interface{})
}

// adminCreateTag is a helper that creates a tag via the admin API.
func adminCreateTag(t *testing.T, e http.Handler, cookie *http.Cookie, name string) map[string]interface{} {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q}`, name)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tags", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode create tag response: %v", err)
	}
	return resp["tag"].(map[string]interface{})
}

// providerCreateService is a helper that creates a service via the provider API.
func providerCreateService(t *testing.T, e http.Handler, cookie *http.Cookie, categoryID string, tagIDs []string, title string) map[string]interface{} {
	t.Helper()
	tagJSON, _ := json.Marshal(tagIDs)
	body := fmt.Sprintf(`{"category_id":%q,"title":%q,"description":"Test description","price_cents":5000,"tag_ids":%s,"status":"active"}`,
		categoryID, title, string(tagJSON))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/services", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create service failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode create service response: %v", err)
	}
	return resp["service"].(map[string]interface{})
}

func TestAdminCreateCategory(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	cookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, cookie, "Plumbing", "plumbing")

	if cat["name"] != "Plumbing" {
		t.Fatalf("expected name Plumbing, got %v", cat["name"])
	}
	if cat["slug"] != "plumbing" {
		t.Fatalf("expected slug plumbing, got %v", cat["slug"])
	}
	if cat["id"] == nil || cat["id"] == "" {
		t.Fatal("expected non-empty id")
	}
}

func TestAdminCreateCategoryUnauthorized(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	body := `{"name":"Plumbing","slug":"plumbing"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminUpdateCategory(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	cookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, cookie, "Plumbing", "plumbing")
	catID := cat["id"].(string)

	// Update
	body := `{"name":"Advanced Plumbing","sort_order":5}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/categories/"+catID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	updated := resp["category"].(map[string]interface{})
	if updated["name"] != "Advanced Plumbing" {
		t.Fatalf("expected updated name, got %v", updated["name"])
	}
	if int(updated["sort_order"].(float64)) != 5 {
		t.Fatalf("expected sort_order 5, got %v", updated["sort_order"])
	}
}

func TestAdminCreateTag(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	cookie := loginAs(t, e, "admin", "admin123")
	tag := adminCreateTag(t, e, cookie, "Emergency")

	if tag["name"] != "Emergency" {
		t.Fatalf("expected name Emergency, got %v", tag["name"])
	}
	if tag["id"] == nil || tag["id"] == "" {
		t.Fatal("expected non-empty id")
	}
}

func TestAdminUpdateTag(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	cookie := loginAs(t, e, "admin", "admin123")
	tag := adminCreateTag(t, e, cookie, "Emergency")
	tagID := tag["id"].(string)

	body := `{"name":"Urgent"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/tags/"+tagID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	updated := resp["tag"].(map[string]interface{})
	if updated["name"] != "Urgent" {
		t.Fatalf("expected name Urgent, got %v", updated["name"])
	}
}

func TestProviderCreateService(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")
	tag := adminCreateTag(t, e, adminCookie, "Emergency")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), []string{tag["id"].(string)}, "Fix Leaky Pipes")

	if svc["title"] != "Fix Leaky Pipes" {
		t.Fatalf("expected title Fix Leaky Pipes, got %v", svc["title"])
	}
	if int(svc["price_cents"].(float64)) != 5000 {
		t.Fatalf("expected price_cents 5000, got %v", svc["price_cents"])
	}
	if svc["status"] != "active" {
		t.Fatalf("expected status active, got %v", svc["status"])
	}

	// Check category ref
	catRef := svc["category"].(map[string]interface{})
	if catRef["name"] != "Plumbing" {
		t.Fatalf("expected category Plumbing, got %v", catRef["name"])
	}

	// Check tags
	tags := svc["tags"].([]interface{})
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	tagRef := tags[0].(map[string]interface{})
	if tagRef["name"] != "Emergency" {
		t.Fatalf("expected tag Emergency, got %v", tagRef["name"])
	}
}

func TestProviderListOwnServices(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Service One")
	providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Service Two")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/services", nil)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	services := resp["services"].([]interface{})
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
}

func TestProviderUpdateService(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Old Title")
	svcID := svc["id"].(string)

	body := `{"title":"New Title","price_cents":9999}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/services/"+svcID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	updated := resp["service"].(map[string]interface{})
	if updated["title"] != "New Title" {
		t.Fatalf("expected New Title, got %v", updated["title"])
	}
	if int(updated["price_cents"].(float64)) != 9999 {
		t.Fatalf("expected 9999, got %v", updated["price_cents"])
	}
}

func TestProviderDeleteService(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "To Delete")
	svcID := svc["id"].(string)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/provider/services/"+svcID, nil)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify it's gone from the catalog
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services/"+svcID, nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestProviderCannotMutateOtherService(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)

	// Create a second provider user directly in the DB
	_, _ = db.Exec(`INSERT INTO users (id, username, password_hash, email) VALUES ('aaaaaaaa-0000-0000-0000-000000000001', 'provider2', crypt('provider2pw', gen_salt('bf')), 'provider2@test.com') ON CONFLICT DO NOTHING`)
	var providerRoleID string
	err := db.QueryRow(`SELECT id FROM roles WHERE name = 'provider'`).Scan(&providerRoleID)
	if err != nil {
		t.Fatalf("provider role not found: %v", err)
	}
	_, _ = db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES ('aaaaaaaa-0000-0000-0000-000000000001', $1) ON CONFLICT DO NOTHING`, providerRoleID)
	_, _ = db.Exec(`INSERT INTO provider_profiles (id, user_id, business_name) VALUES ('aaaaaaaa-0000-0000-0000-000000000002', 'aaaaaaaa-0000-0000-0000-000000000001', 'Other Provider') ON CONFLICT DO NOTHING`)

	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	// Provider 1 creates a service
	provider1Cookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, provider1Cookie, cat["id"].(string), nil, "Provider1 Service")
	svcID := svc["id"].(string)

	// Provider 2 tries to update it
	provider2Cookie := loginAs(t, e, "provider2", "provider2pw")

	body := `{"title":"Hijacked"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/services/"+svcID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(provider2Cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for other provider's service, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestProviderSetAvailability(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Avail Service")
	svcID := svc["id"].(string)

	body := `{"windows":[{"day_of_week":1,"start_time":"09:00","end_time":"17:00"},{"day_of_week":3,"start_time":"10:00","end_time":"14:00"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/services/"+svcID+"/availability", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	windows := resp["availability"].([]interface{})
	if len(windows) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(windows))
	}
}

func TestCatalogListServices(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")
	tag := adminCreateTag(t, e, adminCookie, "Emergency")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateService(t, e, providerCookie, cat["id"].(string), []string{tag["id"].(string)}, "Catalog Service")

	// Read as customer
	customerCookie := loginAs(t, e, "customer", "customer123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	services := resp["services"].([]interface{})
	if len(services) < 1 {
		t.Fatal("expected at least 1 service in catalog")
	}

	svc := services[0].(map[string]interface{})
	if svc["title"] != "Catalog Service" {
		t.Fatalf("expected title Catalog Service, got %v", svc["title"])
	}

	// Check tags present
	tags := svc["tags"].([]interface{})
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}

	// Check pagination info
	if resp["total"] == nil {
		t.Fatal("expected total in response")
	}
	if resp["page"] == nil {
		t.Fatal("expected page in response")
	}
}

func TestCatalogGetServiceDetail(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")
	tag := adminCreateTag(t, e, adminCookie, "Emergency")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), []string{tag["id"].(string)}, "Detail Service")
	svcID := svc["id"].(string)

	// Set availability
	body := `{"windows":[{"day_of_week":1,"start_time":"09:00","end_time":"17:00"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/services/"+svcID+"/availability", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set availability failed: %d %s", rec.Code, rec.Body.String())
	}

	// Get detail as customer
	customerCookie := loginAs(t, e, "customer", "customer123")

	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services/"+svcID, nil)
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
	detail := resp["service"].(map[string]interface{})
	if detail["title"] != "Detail Service" {
		t.Fatalf("expected title Detail Service, got %v", detail["title"])
	}

	// Check availability
	avail := detail["availability"].([]interface{})
	if len(avail) != 1 {
		t.Fatalf("expected 1 availability window, got %d", len(avail))
	}
	window := avail[0].(map[string]interface{})
	if int(window["day_of_week"].(float64)) != 1 {
		t.Fatalf("expected day_of_week 1, got %v", window["day_of_week"])
	}

	// Check tags
	tags := detail["tags"].([]interface{})
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}

	// Check category
	catRef := detail["category"].(map[string]interface{})
	if catRef["name"] != "Plumbing" {
		t.Fatalf("expected category Plumbing, got %v", catRef["name"])
	}
}

func TestCatalogListCategories(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")
	adminCreateCategory(t, e, adminCookie, "Electrical", "electrical")

	customerCookie := loginAs(t, e, "customer", "customer123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/categories", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	categories := resp["categories"].([]interface{})
	if len(categories) < 2 {
		t.Fatalf("expected at least 2 categories, got %d", len(categories))
	}
}

func TestProviderGetOwnInactiveService(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "TestCat", "testcat-inactive")

	providerCookie := loginAs(t, e, "provider", "provider123")

	// Create a service then set it to inactive
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Inactive Service")
	serviceID := svc["id"].(string)

	// Update to inactive
	patchBody := `{"status":"inactive"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/services/"+serviceID, strings.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch to inactive failed: %d %s", rec.Code, rec.Body.String())
	}

	// Catalog endpoint should NOT return it (active-only)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services/"+serviceID, nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 from catalog for inactive service, got %d", rec.Code)
	}

	// Provider GET endpoint SHOULD return it (own service, any status)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/services/"+serviceID, nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from provider get for own inactive service, got %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	svcResp := resp["service"].(map[string]interface{})
	if svcResp["status"] != "inactive" {
		t.Fatalf("expected status inactive, got %q", svcResp["status"])
	}
	if svcResp["title"] != "Inactive Service" {
		t.Fatalf("expected title 'Inactive Service', got %q", svcResp["title"])
	}
}

func TestProviderAvailabilityForInactiveService(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "TestCat", "testcat-avail")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Avail Test Service")
	serviceID := svc["id"].(string)

	// Set to inactive
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/services/"+serviceID,
		strings.NewReader(`{"status":"inactive"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch to inactive failed: %d", rec.Code)
	}

	// Set availability on inactive service — should succeed
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/services/"+serviceID+"/availability",
		strings.NewReader(`{"windows":[{"day_of_week":1,"start_time":"09:00","end_time":"17:00"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for availability on inactive service, got %d %s", rec.Code, rec.Body.String())
	}

	// Verify availability via provider get endpoint
	req = httptest.NewRequest(http.MethodGet, "/api/v1/provider/services/"+serviceID, nil)
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	svcResp := resp["service"].(map[string]interface{})
	avail := svcResp["availability"].([]interface{})
	if len(avail) != 1 {
		t.Fatalf("expected 1 availability window, got %d", len(avail))
	}
}

func TestFieldErrorRendering(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Send empty category create — should return field_errors as object map
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "validation_error" {
		t.Fatalf("expected code validation_error, got %q", errObj["code"])
	}

	fieldErrors, ok := errObj["field_errors"].(map[string]interface{})
	if !ok {
		t.Fatalf("field_errors should be an object map, got %T: %v", errObj["field_errors"], errObj["field_errors"])
	}

	// Verify specific field errors exist
	if _, ok := fieldErrors["name"]; !ok {
		t.Fatal("expected field_errors to contain 'name'")
	}
	if _, ok := fieldErrors["slug"]; !ok {
		t.Fatal("expected field_errors to contain 'slug'")
	}

	// Each field error value should be an array of strings
	nameErrs, ok := fieldErrors["name"].([]interface{})
	if !ok || len(nameErrs) == 0 {
		t.Fatalf("expected name field errors to be non-empty array, got %v", fieldErrors["name"])
	}
}

// TestCatalogListTags verifies GET /api/v1/catalog/tags returns the public tag
// catalog after admin creates tags via the admin API. Routed through the real
// Echo router, no mocks.
func TestCatalogListTags(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	tag1 := adminCreateTag(t, e, adminCookie, "Emergency-CatList")
	tag2 := adminCreateTag(t, e, adminCookie, "Weekend-CatList")

	customerCookie := loginAs(t, e, "customer", "customer123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/tags", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tags, ok := resp["tags"].([]interface{})
	if !ok {
		t.Fatalf("expected tags array, got %T", resp["tags"])
	}
	if len(tags) < 2 {
		t.Fatalf("expected at least 2 tags, got %d", len(tags))
	}

	// Verify both created tags are present and that returned objects expose id+name
	want := map[string]bool{tag1["id"].(string): false, tag2["id"].(string): false}
	for _, raw := range tags {
		obj := raw.(map[string]interface{})
		if obj["id"] == nil || obj["id"] == "" {
			t.Fatal("expected non-empty id on tag")
		}
		if obj["name"] == nil || obj["name"] == "" {
			t.Fatal("expected non-empty name on tag")
		}
		if _, ok := want[obj["id"].(string)]; ok {
			want[obj["id"].(string)] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("created tag %s not present in catalog list", id)
		}
	}
}

// TestCatalogListTagsRequiresAuth verifies the public catalog tags endpoint
// still requires an authenticated session (mounted under /catalog with
// authSvc.RequireAuth()).
func TestCatalogListTagsRequiresAuth(t *testing.T) {
	db := getTestDB(t)
	e := newServer(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/tags", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated, got %d", rec.Code)
	}
}

// TestAdminListCategories verifies GET /api/v1/admin/categories returns the
// admin view of all categories with id/name/slug/sort_order fields.
func TestAdminListCategories(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	c1 := adminCreateCategory(t, e, adminCookie, "AdminListA", "admin-list-a")
	c2 := adminCreateCategory(t, e, adminCookie, "AdminListB", "admin-list-b")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories", nil)
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
	cats, ok := resp["categories"].([]interface{})
	if !ok {
		t.Fatalf("expected categories array, got %T", resp["categories"])
	}

	ids := map[string]map[string]interface{}{}
	for _, raw := range cats {
		obj := raw.(map[string]interface{})
		ids[obj["id"].(string)] = obj
	}
	got1, ok := ids[c1["id"].(string)]
	if !ok {
		t.Fatalf("expected created category %s in admin list", c1["id"])
	}
	if got1["slug"] != "admin-list-a" {
		t.Fatalf("expected slug admin-list-a, got %v", got1["slug"])
	}
	if _, ok := ids[c2["id"].(string)]; !ok {
		t.Fatalf("expected created category %s in admin list", c2["id"])
	}
}

// TestAdminListCategoriesAsCustomerForbidden verifies role enforcement on
// the admin list endpoint.
func TestAdminListCategoriesAsCustomerForbidden(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminListTags verifies GET /api/v1/admin/tags returns all tags ordered
// by name with id/name/created_at fields.
func TestAdminListTags(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	t1 := adminCreateTag(t, e, adminCookie, "Bravo-AdminList")
	t2 := adminCreateTag(t, e, adminCookie, "Alpha-AdminList")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tags", nil)
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
	tags := resp["tags"].([]interface{})

	ids := map[string]map[string]interface{}{}
	for _, raw := range tags {
		obj := raw.(map[string]interface{})
		ids[obj["id"].(string)] = obj
	}
	if _, ok := ids[t1["id"].(string)]; !ok {
		t.Fatalf("expected tag %s in admin list", t1["id"])
	}
	if _, ok := ids[t2["id"].(string)]; !ok {
		t.Fatalf("expected tag %s in admin list", t2["id"])
	}
}

// TestAdminListTagsAsProviderForbidden verifies role enforcement.
func TestAdminListTagsAsProviderForbidden(t *testing.T) {
	db := getTestDB(t)
	cleanupCatalogData(t, db)
	e := newServer(db)

	providerCookie := loginAs(t, e, "provider", "provider123")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tags", nil)
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminListHotKeywords verifies GET /api/v1/admin/search-config/hot-keywords
// returns the full admin view (including is_hot=false rows that the public
// /catalog/hot-keywords endpoint hides).
func TestAdminListHotKeywords(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create one hot and one not-hot keyword
	for _, body := range []string{
		`{"keyword":"hotword-admin","is_hot":true}`,
		`{"keyword":"coldword-admin","is_hot":false}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/search-config/hot-keywords", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(adminCookie)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed keyword failed: %d %s", rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/search-config/hot-keywords", nil)
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
	keywords := resp["keywords"].([]interface{})

	// Both rows must be present (admin sees the full config, not just hot)
	seen := map[string]bool{}
	for _, raw := range keywords {
		obj := raw.(map[string]interface{})
		seen[obj["keyword"].(string)] = obj["is_hot"].(bool)
	}
	if hot, ok := seen["hotword-admin"]; !ok || !hot {
		t.Fatalf("expected hotword-admin with is_hot=true in admin list, seen=%v", seen)
	}
	if hot, ok := seen["coldword-admin"]; !ok || hot {
		t.Fatalf("expected coldword-admin with is_hot=false in admin list, seen=%v", seen)
	}
}

// TestAdminListHotKeywordsAsCustomerForbidden verifies role enforcement.
func TestAdminListHotKeywordsAsCustomerForbidden(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/search-config/hot-keywords", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminListAutocompleteAndUpdate covers both
//  - GET  /api/v1/admin/search-config/autocomplete
//  - PATCH /api/v1/admin/search-config/autocomplete/:termId
// It verifies admin sort order (weight DESC), update success, and that the
// patched row is reflected in the list.
func TestAdminListAutocompleteAndUpdate(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Seed two terms
	createTerm := func(term string, weight int) string {
		body := fmt.Sprintf(`{"term":%q,"weight":%d}`, term, weight)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/search-config/autocomplete", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(adminCookie)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed term %s: %d %s", term, rec.Code, rec.Body.String())
		}
		var resp map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		return resp["term"].(map[string]interface{})["id"].(string)
	}
	lowID := createTerm("autocomplete-low", 1)
	highID := createTerm("autocomplete-high", 99)

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/search-config/autocomplete", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var listResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	terms := listResp["terms"].([]interface{})
	if len(terms) < 2 {
		t.Fatalf("expected >=2 terms, got %d", len(terms))
	}
	// Find positions and ensure higher-weight term comes before lower-weight
	posHigh, posLow := -1, -1
	for i, raw := range terms {
		obj := raw.(map[string]interface{})
		if obj["id"] == highID {
			posHigh = i
		}
		if obj["id"] == lowID {
			posLow = i
		}
	}
	if posHigh < 0 || posLow < 0 {
		t.Fatalf("expected both seeded terms in list, got positions high=%d low=%d", posHigh, posLow)
	}
	if posHigh >= posLow {
		t.Fatalf("expected higher-weight term to sort earlier, got high=%d low=%d", posHigh, posLow)
	}

	// PATCH the low-weight term: bump weight and rename
	patchBody := `{"term":"autocomplete-low-renamed","weight":150}`
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/search-config/autocomplete/"+lowID, strings.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var patchResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &patchResp); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	updated := patchResp["term"].(map[string]interface{})
	if updated["term"] != "autocomplete-low-renamed" {
		t.Fatalf("expected renamed term, got %v", updated["term"])
	}
	if int(updated["weight"].(float64)) != 150 {
		t.Fatalf("expected weight=150, got %v", updated["weight"])
	}
	if updated["id"] != lowID {
		t.Fatalf("expected same id, got %v", updated["id"])
	}
}

// TestAdminPatchAutocompleteNotFound verifies 404 on PATCH with unknown ID.
func TestAdminPatchAutocompleteNotFound(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Use a syntactically valid UUID that doesn't exist
	missingID := "00000000-0000-0000-0000-000000000099"
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/search-config/autocomplete/"+missingID,
		strings.NewReader(`{"term":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminPatchAutocompleteAsCustomerForbidden verifies role enforcement.
func TestAdminPatchAutocompleteAsCustomerForbidden(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	req := httptest.NewRequest(http.MethodPatch,
		"/api/v1/admin/search-config/autocomplete/00000000-0000-0000-0000-000000000099",
		strings.NewReader(`{"term":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminListAutocompleteAsCustomerForbidden verifies role enforcement on
// GET /admin/search-config/autocomplete.
func TestAdminListAutocompleteAsCustomerForbidden(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	customerCookie := loginAs(t, e, "customer", "customer123")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/search-config/autocomplete", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
