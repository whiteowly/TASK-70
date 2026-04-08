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
)

func cleanupSearchData(t *testing.T, db *sql.DB) {
	t.Helper()
	_, _ = db.Exec("DELETE FROM search_history")
	_, _ = db.Exec("DELETE FROM search_events")
	_, _ = db.Exec("DELETE FROM favorites")
	_, _ = db.Exec("DELETE FROM search_keyword_config")
	_, _ = db.Exec("DELETE FROM autocomplete_terms")
	cleanupCatalogData(t, db)
}

// providerCreateServiceWithPrice creates a service with a specific price.
func providerCreateServiceWithPrice(t *testing.T, e http.Handler, cookie *http.Cookie, categoryID string, tagIDs []string, title string, priceCents int) map[string]interface{} {
	t.Helper()
	tagJSON, _ := json.Marshal(tagIDs)
	body := fmt.Sprintf(`{"category_id":%q,"title":%q,"description":"Test description","price_cents":%d,"tag_ids":%s,"status":"active"}`,
		categoryID, title, priceCents, string(tagJSON))
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

func TestFuzzySearch(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Expert Plumbing Repair")

	// Search with intentional typo
	customerCookie := loginAs(t, e, "customer", "customer123")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?q=plumbng", nil)
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
		t.Fatal("expected at least 1 service from fuzzy search")
	}

	svc := services[0].(map[string]interface{})
	if svc["title"] != "Expert Plumbing Repair" {
		t.Fatalf("expected title 'Expert Plumbing Repair', got %v", svc["title"])
	}
}

func TestSearchFilters(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Cleaning", "cleaning")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Cheap Cleaning", 1000)
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Expensive Cleaning", 5000)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Filter for min_price=3000 — should only return the expensive one
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?min_price=3000", nil)
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
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	svc := services[0].(map[string]interface{})
	if svc["title"] != "Expensive Cleaning" {
		t.Fatalf("expected 'Expensive Cleaning', got %v", svc["title"])
	}
}

func TestSearchSorting(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Sorting", "sorting")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Medium Sort", 3000)
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Cheap Sort", 1000)
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Expensive Sort", 5000)

	customerCookie := loginAs(t, e, "customer", "customer123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?sort=price_asc", nil)
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
	if len(services) != 3 {
		t.Fatalf("expected 3 services, got %d", len(services))
	}

	prices := make([]int, len(services))
	for i, s := range services {
		prices[i] = int(s.(map[string]interface{})["price_cents"].(float64))
	}

	if prices[0] != 1000 || prices[1] != 3000 || prices[2] != 5000 {
		t.Fatalf("expected prices [1000, 3000, 5000], got %v", prices)
	}
}

func TestSearchPagination(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Paging", "paging")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Page Service A", 1000)
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Page Service B", 2000)
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Page Service C", 3000)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Page 1, size 2
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?page_size=2&page=1", nil)
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
	total := int(resp["total"].(float64))

	if len(services) != 2 {
		t.Fatalf("expected 2 services on page 1, got %d", len(services))
	}
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}

	// Page 2
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?page_size=2&page=2", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	services = resp["services"].([]interface{})
	if len(services) != 1 {
		t.Fatalf("expected 1 service on page 2, got %d", len(services))
	}
}

func TestSearchHistory(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Plumbing", "plumbing-hist")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Plumbing Service")

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Search with a query
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?q=plumbing", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("search failed: %d %s", rec.Code, rec.Body.String())
	}

	// Give the background goroutine a moment to record
	time.Sleep(200 * time.Millisecond)

	// Get search history
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/search-history", nil)
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

	history := resp["history"].([]interface{})
	if len(history) < 1 {
		t.Fatal("expected at least 1 search history entry")
	}

	entry := history[0].(map[string]interface{})
	if entry["query_text"] != "plumbing" {
		t.Fatalf("expected query_text 'plumbing', got %v", entry["query_text"])
	}
}

func TestFavoritesAddRemove(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Favs", "favs")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Fav Service")
	svcID := svc["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Add favorite
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/favorites/"+svcID, nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// List favorites
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/favorites", nil)
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

	favs := resp["favorites"].([]interface{})
	if len(favs) != 1 {
		t.Fatalf("expected 1 favorite, got %d", len(favs))
	}

	fav := favs[0].(map[string]interface{})
	if fav["service_id"] != svcID {
		t.Fatalf("expected service_id %s, got %v", svcID, fav["service_id"])
	}

	// Remove favorite
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/customer/favorites/"+svcID, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify empty
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/favorites", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	favs = resp["favorites"].([]interface{})
	if len(favs) != 0 {
		t.Fatalf("expected 0 favorites after removal, got %d", len(favs))
	}
}

func TestFavoritesNoDuplicate(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "DupFav", "dupfav")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Dup Fav Service")
	svcID := svc["id"].(string)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Add favorite twice
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/favorites/"+svcID, nil)
		req.AddCookie(customerCookie)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("add favorite attempt %d failed: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}

	// List — should have exactly 1
	req := httptest.NewRequest(http.MethodGet, "/api/v1/customer/favorites", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	favs := resp["favorites"].([]interface{})
	if len(favs) != 1 {
		t.Fatalf("expected exactly 1 favorite, got %d", len(favs))
	}
}

func TestTrending(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Trending", "trending")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Trending Service")

	customerCookie := loginAs(t, e, "customer", "customer123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/trending", nil)
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
		t.Fatal("expected at least 1 trending service")
	}
}

func TestHotKeywordsAdminCRUD(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create hot keyword
	body := `{"keyword":"plumbing","is_hot":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/search-config/hot-keywords", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	kw := createResp["keyword"].(map[string]interface{})
	kwID := kw["id"].(string)

	if kw["keyword"] != "plumbing" {
		t.Fatalf("expected keyword 'plumbing', got %v", kw["keyword"])
	}
	if kw["is_hot"] != true {
		t.Fatalf("expected is_hot true, got %v", kw["is_hot"])
	}

	// Public hot keywords endpoint
	customerCookie := loginAs(t, e, "customer", "customer123")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/hot-keywords", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var listResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	keywords := listResp["keywords"].([]interface{})
	if len(keywords) < 1 {
		t.Fatal("expected at least 1 hot keyword")
	}

	// Update
	body = `{"keyword":"plumbing updated"}`
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/search-config/hot-keywords/"+kwID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var updateResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	updated := updateResp["keyword"].(map[string]interface{})
	if updated["keyword"] != "plumbing updated" {
		t.Fatalf("expected updated keyword, got %v", updated["keyword"])
	}
}

func TestAutocompleteCRUD(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")

	// Create autocomplete term
	body := `{"term":"plumbing repair","weight":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/search-config/autocomplete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	term := createResp["term"].(map[string]interface{})
	if term["term"] != "plumbing repair" {
		t.Fatalf("expected term 'plumbing repair', got %v", term["term"])
	}

	// Query autocomplete with prefix
	customerCookie := loginAs(t, e, "customer", "customer123")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/autocomplete?q=plumb", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var acResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &acResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	terms := acResp["terms"].([]interface{})
	if len(terms) < 1 {
		t.Fatal("expected at least 1 autocomplete match")
	}

	matchedTerm := terms[0].(map[string]interface{})
	if matchedTerm["term"] != "plumbing repair" {
		t.Fatalf("expected 'plumbing repair', got %v", matchedTerm["term"])
	}

	// Query with non-matching prefix
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/autocomplete?q=zzzzz", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &acResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	terms = acResp["terms"].([]interface{})
	if len(terms) != 0 {
		t.Fatalf("expected 0 results for non-matching prefix, got %d", len(terms))
	}
}

func TestSearchAvailabilityDateFilter(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "AvailFilter", "availfilter")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc1 := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Monday Service")
	svc2 := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Wednesday Service")

	// Set availability: svc1 available Monday 09:00-17:00
	body := `{"windows":[{"day_of_week":1,"start_time":"09:00","end_time":"17:00"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/services/"+svc1["id"].(string)+"/availability", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set availability: %d %s", rec.Code, rec.Body.String())
	}

	// Set availability: svc2 available Wednesday 10:00-14:00
	body = `{"windows":[{"day_of_week":3,"start_time":"10:00","end_time":"14:00"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/provider/services/"+svc2["id"].(string)+"/availability", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// Find a real Monday date and a real Wednesday date to use.
	// 2026-04-13 is a Monday, 2026-04-15 is a Wednesday.
	mondayDate := "2026-04-13"
	wednesdayDate := "2026-04-15"

	// Verify our date assumptions
	mon, _ := time.Parse("2006-01-02", mondayDate)
	wed, _ := time.Parse("2006-01-02", wednesdayDate)
	if mon.Weekday() != time.Monday {
		t.Fatalf("expected %s to be Monday, got %s", mondayDate, mon.Weekday())
	}
	if wed.Weekday() != time.Wednesday {
		t.Fatalf("expected %s to be Wednesday, got %s", wednesdayDate, wed.Weekday())
	}

	// Filter for a Monday date — should only return Monday Service
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?available_date="+mondayDate, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	services := resp["services"].([]interface{})

	if len(services) != 1 {
		t.Fatalf("expected 1 service for Monday date filter, got %d", len(services))
	}
	if services[0].(map[string]interface{})["title"] != "Monday Service" {
		t.Fatalf("expected Monday Service, got %v", services[0].(map[string]interface{})["title"])
	}

	// Filter for a Wednesday date — should only return Wednesday Service
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?available_date="+wednesdayDate, nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	json.Unmarshal(rec.Body.Bytes(), &resp)
	services = resp["services"].([]interface{})
	if len(services) != 1 {
		t.Fatalf("expected 1 service for Wednesday date, got %d", len(services))
	}
	if services[0].(map[string]interface{})["title"] != "Wednesday Service" {
		t.Fatalf("expected Wednesday Service, got %v", services[0].(map[string]interface{})["title"])
	}
}

func TestSearchAvailabilityDateWithTime(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "AvailDateTime", "availdt")

	providerCookie := loginAs(t, e, "provider", "provider123")
	svc1 := providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "DateTimeService")

	// Available Monday 09:00-17:00
	body := `{"windows":[{"day_of_week":1,"start_time":"09:00","end_time":"17:00"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/services/"+svc1["id"].(string)+"/availability", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(providerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	customerCookie := loginAs(t, e, "customer", "customer123")

	// 2026-04-13 is Monday. Filter with available_time=16:00 → match (17:00 >= 16:00)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?available_date=2026-04-13&available_time=16:00", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	services := resp["services"].([]interface{})
	if len(services) != 1 {
		t.Fatalf("expected 1 service for Monday 16:00, got %d", len(services))
	}

	// Filter with available_time=18:00 → no match (17:00 < 18:00)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?available_date=2026-04-13&available_time=18:00", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	json.Unmarshal(rec.Body.Bytes(), &resp)
	services = resp["services"].([]interface{})
	if len(services) != 0 {
		t.Fatalf("expected 0 services for Monday 18:00, got %d", len(services))
	}

	// Filter for a Tuesday date (2026-04-14) → no match (no Tuesday window)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?available_date=2026-04-14", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	json.Unmarshal(rec.Body.Bytes(), &resp)
	services = resp["services"].([]interface{})
	if len(services) != 0 {
		t.Fatalf("expected 0 services for Tuesday date, got %d", len(services))
	}
}

func TestSearchSortingDistance(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "DistSort", "distsort")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Near Service", 1000)
	providerCreateServiceWithPrice(t, e, providerCookie, cat["id"].(string), nil, "Far Service", 2000)

	// Set service_area_miles for the provider so distance sort has data.
	// Both services belong to the same provider, so we set the provider's
	// service_area_miles directly. To test ordering, we need a second provider.

	// Create a second provider with a larger service area
	_, _ = db.Exec(`INSERT INTO users (id, username, password_hash, email) VALUES ('bbbbbbbb-0000-0000-0000-000000000010', 'provider_far', crypt('provider_far', gen_salt('bf')), 'pfar@test.com') ON CONFLICT DO NOTHING`)
	var providerRoleID string
	db.QueryRow(`SELECT id FROM roles WHERE name = 'provider'`).Scan(&providerRoleID)
	_, _ = db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES ('bbbbbbbb-0000-0000-0000-000000000010', $1) ON CONFLICT DO NOTHING`, providerRoleID)
	_, _ = db.Exec(`INSERT INTO provider_profiles (id, user_id, business_name, service_area_miles) VALUES ('bbbbbbbb-0000-0000-0000-000000000011', 'bbbbbbbb-0000-0000-0000-000000000010', 'Far Provider', 100) ON CONFLICT DO NOTHING`)

	// Set the seeded provider to a small service area
	_, _ = db.Exec(`UPDATE provider_profiles SET service_area_miles = 10 WHERE user_id = (SELECT id FROM users WHERE username='provider')`)

	// Create a service for the far provider
	_, _ = db.Exec(`INSERT INTO services (provider_id, category_id, title, description, price_cents, status) VALUES ('bbbbbbbb-0000-0000-0000-000000000011', $1, 'Far Provider Service', 'desc', 3000, 'active')`, cat["id"].(string))

	customerCookie := loginAs(t, e, "customer", "customer123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?sort=distance", nil)
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
	if len(services) < 2 {
		t.Fatalf("expected at least 2 services, got %d", len(services))
	}

	// First services should be from provider with service_area_miles=10 (smaller = closer)
	first := services[0].(map[string]interface{})
	last := services[len(services)-1].(map[string]interface{})

	firstProvider := first["provider"].(map[string]interface{})
	lastProvider := last["provider"].(map[string]interface{})

	firstArea := 0
	if firstProvider["service_area_miles"] != nil {
		firstArea = int(firstProvider["service_area_miles"].(float64))
	}
	lastArea := 0
	if lastProvider["service_area_miles"] != nil {
		lastArea = int(lastProvider["service_area_miles"].(float64))
	}

	if firstArea > lastArea {
		t.Fatalf("distance sort should order smaller service_area_miles first: got first=%d, last=%d", firstArea, lastArea)
	}
}

func TestHotKeywordsRequiresAuth(t *testing.T) {
	db := getTestDB(t)
	e := newServer(db)

	// Unauthenticated request to hot-keywords should return 401
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/hot-keywords", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated hot-keywords, got %d", rec.Code)
	}
}

func TestAutocompleteRequiresAuth(t *testing.T) {
	db := getTestDB(t)
	e := newServer(db)

	// Unauthenticated request to autocomplete should return 401
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/autocomplete?q=test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated autocomplete, got %d", rec.Code)
	}
}

func TestCacheBehavior(t *testing.T) {
	db := getTestDB(t)
	cleanupSearchData(t, db)
	e := newServer(db)

	adminCookie := loginAs(t, e, "admin", "admin123")
	cat := adminCreateCategory(t, e, adminCookie, "Cache", "cache-test")

	providerCookie := loginAs(t, e, "provider", "provider123")
	providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Cache Service Original")

	customerCookie := loginAs(t, e, "customer", "customer123")

	// First search — populates cache
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?sort=newest", nil)
	req.AddCookie(customerCookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp1 map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("decode: %v", err)
	}
	total1 := int(resp1["total"].(float64))

	// Create another service
	providerCreateService(t, e, providerCookie, cat["id"].(string), nil, "Cache Service New")

	// Second search — same params, should hit cache (stale total)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/catalog/services?sort=newest", nil)
	req.AddCookie(customerCookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp2 map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Note: The provider create handler calls notifyCatalogChange which
	// invalidates the cache, so the second search will show fresh data.
	// This verifies the invalidation callback works.
	total2 := int(resp2["total"].(float64))
	if total2 != total1+1 {
		t.Fatalf("expected total to increase by 1 after cache invalidation, got %d -> %d", total1, total2)
	}
}
