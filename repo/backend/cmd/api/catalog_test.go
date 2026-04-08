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
