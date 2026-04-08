package search

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"fieldserve/internal/platform/cache"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// Service provides search and discovery operations.
type Service struct {
	db    *sql.DB
	cache *cache.LRU
}

// NewService creates a new search service.
func NewService(db *sql.DB) *Service {
	return &Service{
		db:    db,
		cache: cache.NewLRU(500, 10*time.Minute),
	}
}

// InvalidateCache clears the entire search cache. Called when catalog,
// taxonomy, or favorites change.
func (s *Service) InvalidateCache() {
	s.cache.Clear()
}

// CacheLen returns the number of entries in the search cache (for testing).
func (s *Service) CacheLen() int {
	return s.cache.Len()
}

// SearchParams holds all supported query parameters for the search endpoint.
type SearchParams struct {
	Q           string
	CategoryID  string
	TagIDs      []string
	MinPrice    *int
	MaxPrice    *int
	MinRating   *float64
	RadiusMiles *int
	AvailDate string // available_date: YYYY-MM-DD — date the customer needs the service; resolved to weekday for schedule lookup
	AvailTime string // available_time: HH:MM — optional; service must have a window covering this time on the resolved day
	Sort      string
	Page        int
	PageSize    int
	ExcludeProviderUserIDs []string // user IDs to exclude (e.g. blocked users)
}

// SearchResult is the envelope returned by the Search method.
type SearchResult struct {
	Services []ServiceRow `json:"services"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

// ServiceRow represents a service in search results.
type ServiceRow struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     *string  `json:"description"`
	PriceCents      int      `json:"price_cents"`
	RatingAvg       string   `json:"rating_avg"`
	PopularityScore int      `json:"popularity_score"`
	Status          string   `json:"status"`
	Category        *CatRef  `json:"category"`
	Provider        ProvRef  `json:"provider"`
	Tags            []TagRef `json:"tags"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

// CatRef is a minimal category reference.
type CatRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProvRef is a minimal provider reference.
type ProvRef struct {
	ID               string `json:"id"`
	BusinessName     string `json:"business_name"`
	ServiceAreaMiles *int   `json:"service_area_miles"`
}

// TagRef is a minimal tag reference.
type TagRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SearchHistoryEntry represents a row from search_history.
type SearchHistoryEntry struct {
	ID        string `json:"id"`
	QueryText string `json:"query_text"`
	CreatedAt string `json:"created_at"`
}

// ---------- parameter parsing ----------

func queryParam(c echo.Context, names ...string) string {
	for _, n := range names {
		if v := c.QueryParam(n); v != "" {
			return v
		}
	}
	return ""
}

// ParseSearchParams extracts search parameters from the query string,
// accepting both camelCase and snake_case variants.
func ParseSearchParams(c echo.Context) SearchParams {
	p := SearchParams{}

	p.Q = c.QueryParam("q")

	p.CategoryID = queryParam(c, "categoryId", "category_id")

	// tag IDs: accept tagIds[] (repeated) or tag_ids (comma-separated)
	tagVals := c.QueryParams()["tagIds[]"]
	if len(tagVals) == 0 {
		if csv := queryParam(c, "tag_ids"); csv != "" {
			tagVals = strings.Split(csv, ",")
		}
	}
	for _, v := range tagVals {
		v = strings.TrimSpace(v)
		if v != "" {
			p.TagIDs = append(p.TagIDs, v)
		}
	}

	if v := queryParam(c, "minPrice", "min_price"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.MinPrice = &n
		}
	}
	if v := queryParam(c, "maxPrice", "max_price"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.MaxPrice = &n
		}
	}
	if v := queryParam(c, "minRating", "min_rating"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			p.MinRating = &f
		}
	}
	if v := queryParam(c, "radiusMiles", "radius_miles"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.RadiusMiles = &n
		}
	}
	p.AvailDate = queryParam(c, "availableDate", "available_date")
	p.AvailTime = queryParam(c, "availableTime", "available_time")

	p.Sort = c.QueryParam("sort")
	if p.Sort == "" {
		if p.Q != "" {
			p.Sort = "relevance"
		} else {
			p.Sort = "newest"
		}
	}

	p.Page, _ = strconv.Atoi(c.QueryParam("page"))
	if p.Page < 1 {
		p.Page = 1
	}

	ps := queryParam(c, "pageSize", "page_size")
	p.PageSize, _ = strconv.Atoi(ps)
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}

	return p
}

// ---------- cache key ----------

// NormalizeCacheKey produces a deterministic SHA-256 hash for a SearchParams
// value so that logically equivalent queries share a cache entry.
func NormalizeCacheKey(p SearchParams) string {
	tags := make([]string, len(p.TagIDs))
	copy(tags, p.TagIDs)
	sort.Strings(tags)

	excludeIDs := make([]string, len(p.ExcludeProviderUserIDs))
	copy(excludeIDs, p.ExcludeProviderUserIDs)
	sort.Strings(excludeIDs)

	m := map[string]interface{}{
		"q":            strings.ToLower(strings.TrimSpace(p.Q)),
		"category_id":  p.CategoryID,
		"tag_ids":      tags,
		"min_price":    p.MinPrice,
		"max_price":    p.MaxPrice,
		"min_rating":   p.MinRating,
		"radius_miles":  p.RadiusMiles,
		"avail_date":    p.AvailDate,
		"avail_time":    p.AvailTime,
		"sort":          p.Sort,
		"page":         p.Page,
		"page_size":    p.PageSize,
		"exclude_provider_user_ids": excludeIDs,
	}

	b, _ := json.Marshal(m)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ---------- search ----------

// Search executes a filtered, sorted, paginated search against the services
// table using pg_trgm similarity for fuzzy keyword matching.
func (s *Service) Search(ctx context.Context, params SearchParams) (*SearchResult, error) {
	key := NormalizeCacheKey(params)
	if cached, ok := s.cache.Get(key); ok {
		return cached.(*SearchResult), nil
	}

	args := []interface{}{}
	argIdx := 1
	var whereClauses []string

	whereClauses = append(whereClauses, "s.status = 'active'")

	// We need the q parameter index for ORDER BY relevance later.
	qArgIdx := 0

	if p := strings.TrimSpace(params.Q); p != "" {
		whereClauses = append(whereClauses, fmt.Sprintf(
			"(similarity(s.title, $%d) > 0.1 OR similarity(pp.business_name, $%d) > 0.1)", argIdx, argIdx))
		args = append(args, p)
		qArgIdx = argIdx
		argIdx++
	}
	if params.CategoryID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("s.category_id = $%d", argIdx))
		args = append(args, params.CategoryID)
		argIdx++
	}
	if len(params.TagIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM service_tags st WHERE st.service_id = s.id AND st.tag_id = ANY($%d::uuid[]))", argIdx))
		args = append(args, pq.Array(params.TagIDs))
		argIdx++
	}
	if params.MinPrice != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("s.price_cents >= $%d", argIdx))
		args = append(args, *params.MinPrice)
		argIdx++
	}
	if params.MaxPrice != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("s.price_cents <= $%d", argIdx))
		args = append(args, *params.MaxPrice)
		argIdx++
	}
	if params.MinRating != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("s.rating_avg >= $%d", argIdx))
		args = append(args, *params.MinRating)
		argIdx++
	}
	if params.RadiusMiles != nil {
		whereClauses = append(whereClauses, fmt.Sprintf(
			"pp.service_area_miles IS NOT NULL AND pp.service_area_miles >= $%d", argIdx))
		args = append(args, *params.RadiusMiles)
		argIdx++
	}
	if params.AvailDate != "" {
		dayOfWeek, ok := resolveDateToWeekday(params.AvailDate)
		if ok {
			availClause := fmt.Sprintf("saw.day_of_week = $%d", argIdx)
			args = append(args, dayOfWeek)
			argIdx++

			if params.AvailTime != "" {
				availClause += fmt.Sprintf(" AND saw.end_time >= $%d::time", argIdx)
				args = append(args, params.AvailTime)
				argIdx++
			}

			whereClauses = append(whereClauses, fmt.Sprintf(
				"EXISTS (SELECT 1 FROM service_availability_windows saw WHERE saw.service_id = s.id AND %s)", availClause))
		}
	}

	if len(params.ExcludeProviderUserIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("pp.user_id != ALL($%d::uuid[])", argIdx))
		args = append(args, pq.Array(params.ExcludeProviderUserIDs))
		argIdx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// ORDER BY
	var orderSQL string
	switch params.Sort {
	case "relevance":
		if qArgIdx > 0 {
			orderSQL = fmt.Sprintf(
				"ORDER BY GREATEST(similarity(s.title, $%d), similarity(pp.business_name, $%d)) DESC, s.created_at DESC",
				qArgIdx, qArgIdx)
		} else {
			orderSQL = "ORDER BY s.created_at DESC"
		}
	case "price_asc":
		orderSQL = "ORDER BY s.price_cents ASC, s.created_at DESC"
	case "price_desc":
		orderSQL = "ORDER BY s.price_cents DESC, s.created_at DESC"
	case "popularity":
		orderSQL = "ORDER BY s.popularity_score DESC, s.created_at DESC"
	case "rating":
		orderSQL = "ORDER BY s.rating_avg DESC, s.created_at DESC"
	case "distance":
		// Local proxy: smaller service_area_miles = more local. NULLs sort last.
		orderSQL = "ORDER BY pp.service_area_miles ASC NULLS LAST, s.created_at DESC"
	default: // newest
		orderSQL = "ORDER BY s.created_at DESC"
	}

	// Count query
	countSQL := fmt.Sprintf(
		`SELECT COUNT(*) FROM services s
		 LEFT JOIN categories c ON s.category_id = c.id
		 JOIN provider_profiles pp ON s.provider_id = pp.id
		 WHERE %s`, whereSQL)

	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("search: count: %w", err)
	}

	// Data query
	offset := (params.Page - 1) * params.PageSize

	limitArg := argIdx
	args = append(args, params.PageSize)
	argIdx++
	offsetArg := argIdx
	args = append(args, offset)

	dataSQL := fmt.Sprintf(
		`SELECT s.id, s.title, s.description, s.price_cents, s.rating_avg::text,
		        s.popularity_score, s.status, s.created_at::text, s.updated_at::text,
		        c.id, c.name, pp.id, pp.business_name, pp.service_area_miles
		 FROM services s
		 LEFT JOIN categories c ON s.category_id = c.id
		 JOIN provider_profiles pp ON s.provider_id = pp.id
		 WHERE %s
		 %s
		 LIMIT $%d OFFSET $%d`, whereSQL, orderSQL, limitArg, offsetArg)

	rows, err := s.db.QueryContext(ctx, dataSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("search: query: %w", err)
	}
	defer rows.Close()

	services := make([]ServiceRow, 0)
	var serviceIDs []string

	for rows.Next() {
		var row ServiceRow
		var catID, catName *string
		var provID, provBizName string
		var provAreaMiles *int

		if err := rows.Scan(
			&row.ID, &row.Title, &row.Description, &row.PriceCents,
			&row.RatingAvg, &row.PopularityScore, &row.Status,
			&row.CreatedAt, &row.UpdatedAt,
			&catID, &catName,
			&provID, &provBizName, &provAreaMiles,
		); err != nil {
			return nil, fmt.Errorf("search: scan: %w", err)
		}

		if catID != nil {
			row.Category = &CatRef{ID: *catID, Name: *catName}
		}
		row.Provider = ProvRef{ID: provID, BusinessName: provBizName, ServiceAreaMiles: provAreaMiles}
		row.Tags = make([]TagRef, 0)

		services = append(services, row)
		serviceIDs = append(serviceIDs, row.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: iterate: %w", err)
	}

	// Load tags
	tagMap, err := s.loadServiceTags(ctx, serviceIDs)
	if err != nil {
		return nil, err
	}
	for i := range services {
		if tags, ok := tagMap[services[i].ID]; ok {
			services[i].Tags = tags
		}
	}

	result := &SearchResult{
		Services: services,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}

	s.cache.Set(key, result)
	return result, nil
}

// loadServiceTags batch-loads tags for the given service IDs.
func (s *Service) loadServiceTags(ctx context.Context, serviceIDs []string) (map[string][]TagRef, error) {
	result := make(map[string][]TagRef)
	if len(serviceIDs) == 0 {
		return result, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT st.service_id, t.id, t.name
		 FROM tags t
		 JOIN service_tags st ON st.tag_id = t.id
		 WHERE st.service_id = ANY($1::uuid[])`,
		pq.Array(serviceIDs))
	if err != nil {
		return nil, fmt.Errorf("search: load tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var serviceID, tagID, tagName string
		if err := rows.Scan(&serviceID, &tagID, &tagName); err != nil {
			return nil, fmt.Errorf("search: scan tag: %w", err)
		}
		result[serviceID] = append(result[serviceID], TagRef{ID: tagID, Name: tagName})
	}
	return result, rows.Err()
}

// dateRe matches YYYY-MM-DD format.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// resolveDateToWeekday parses a YYYY-MM-DD string and returns the Go
// time.Weekday value (0=Sunday through 6=Saturday), matching the
// service_availability_windows.day_of_week convention.
func resolveDateToWeekday(s string) (int, bool) {
	if !dateRe.MatchString(s) {
		return 0, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return 0, false
	}
	return int(t.Weekday()), true
}

// ---------- search recording ----------

// RecordSearch inserts into search_events and (if queryText is non-empty)
// search_history. Safe to call from a goroutine.
func (s *Service) RecordSearch(ctx context.Context, userID, queryText string, filters map[string]interface{}, resultCount int) {
	filtersJSON, _ := json.Marshal(filters)

	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO search_events (user_id, query_text, filters, result_count) VALUES ($1, $2, $3, $4)`,
		userID, queryText, filtersJSON, resultCount)

	if strings.TrimSpace(queryText) != "" {
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO search_history (user_id, query_text) VALUES ($1, $2)`,
			userID, queryText)
	}
}

// ---------- search history ----------

// GetSearchHistory returns the most recent unique search queries for a user.
func (s *Service) GetSearchHistory(ctx context.Context, userID string, limit int) ([]SearchHistoryEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, query_text, created_at::text FROM (
			SELECT DISTINCT ON (query_text) id, query_text, created_at
			FROM search_history WHERE user_id = $1
			ORDER BY query_text, created_at DESC
		) sub ORDER BY created_at DESC LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search: history: %w", err)
	}
	defer rows.Close()

	entries := make([]SearchHistoryEntry, 0)
	for rows.Next() {
		var e SearchHistoryEntry
		if err := rows.Scan(&e.ID, &e.QueryText, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("search: scan history: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ---------- trending ----------

// GetTrending returns the top trending services based on recent favorites and
// popularity score.
func (s *Service) GetTrending(ctx context.Context, limit int) ([]ServiceRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id, s.title, s.description, s.price_cents, s.rating_avg::text,
		        s.popularity_score, s.status, s.created_at::text, s.updated_at::text,
		        c.id, c.name, pp.id, pp.business_name, pp.service_area_miles,
		        COALESCE(fav.fav_count, 0) + s.popularity_score AS trend_score
		 FROM services s
		 LEFT JOIN categories c ON s.category_id = c.id
		 JOIN provider_profiles pp ON s.provider_id = pp.id
		 LEFT JOIN (
		     SELECT service_id, COUNT(*) AS fav_count
		     FROM favorites
		     WHERE created_at > NOW() - INTERVAL '7 days'
		     GROUP BY service_id
		 ) fav ON fav.service_id = s.id
		 WHERE s.status = 'active'
		 ORDER BY trend_score DESC, s.created_at DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("search: trending: %w", err)
	}
	defer rows.Close()

	services := make([]ServiceRow, 0)
	var serviceIDs []string

	for rows.Next() {
		var row ServiceRow
		var catID, catName *string
		var provID, provBizName string
		var provAreaMiles *int
		var trendScore int

		if err := rows.Scan(
			&row.ID, &row.Title, &row.Description, &row.PriceCents,
			&row.RatingAvg, &row.PopularityScore, &row.Status,
			&row.CreatedAt, &row.UpdatedAt,
			&catID, &catName,
			&provID, &provBizName, &provAreaMiles,
			&trendScore,
		); err != nil {
			return nil, fmt.Errorf("search: scan trending: %w", err)
		}

		if catID != nil {
			row.Category = &CatRef{ID: *catID, Name: *catName}
		}
		row.Provider = ProvRef{ID: provID, BusinessName: provBizName, ServiceAreaMiles: provAreaMiles}
		row.Tags = make([]TagRef, 0)

		services = append(services, row)
		serviceIDs = append(serviceIDs, row.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: iterate trending: %w", err)
	}

	tagMap, err := s.loadServiceTags(ctx, serviceIDs)
	if err != nil {
		return nil, err
	}
	for i := range services {
		if tags, ok := tagMap[services[i].ID]; ok {
			services[i].Tags = tags
		}
	}

	return services, nil
}
