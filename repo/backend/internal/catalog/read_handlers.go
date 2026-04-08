package catalog

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"fieldserve/internal/auth"
	"fieldserve/internal/platform/httpx"

	"github.com/labstack/echo/v4"
)

// HandleListCategories returns all categories (public read).
func (s *Service) HandleListCategories(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, parent_id, name, slug, sort_order, created_at::text FROM categories ORDER BY sort_order, name`)
	if err != nil {
		return fmt.Errorf("catalog: list categories: %w", err)
	}
	defer rows.Close()

	categories := make([]Category, 0)
	for rows.Next() {
		var cat Category
		if err := rows.Scan(&cat.ID, &cat.ParentID, &cat.Name, &cat.Slug, &cat.SortOrder, &cat.CreatedAt); err != nil {
			return fmt.Errorf("catalog: scan category: %w", err)
		}
		categories = append(categories, cat)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("catalog: iterate categories: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"categories": categories})
}

// HandleListTags returns all tags (public read).
func (s *Service) HandleListTags(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, name, created_at::text FROM tags ORDER BY name`)
	if err != nil {
		return fmt.Errorf("catalog: list tags: %w", err)
	}
	defer rows.Close()

	tags := make([]Tag, 0)
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.CreatedAt); err != nil {
			return fmt.Errorf("catalog: scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("catalog: iterate tags: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"tags": tags})
}

// HandleListServices returns paginated active services with optional category filter.
func (s *Service) HandleListServices(c echo.Context) error {
	ctx := c.Request().Context()

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	categoryID := c.QueryParam("category_id")

	// Count total
	var total int
	if categoryID != "" {
		err := s.db.QueryRowContext(ctx,
			`SELECT count(*) FROM services WHERE status = 'active' AND category_id = $1`, categoryID,
		).Scan(&total)
		if err != nil {
			return fmt.Errorf("catalog: count services: %w", err)
		}
	} else {
		err := s.db.QueryRowContext(ctx,
			`SELECT count(*) FROM services WHERE status = 'active'`,
		).Scan(&total)
		if err != nil {
			return fmt.Errorf("catalog: count services: %w", err)
		}
	}

	// List services
	var rows *sql.Rows
	var queryErr error
	baseQuery := `SELECT s.id, s.title, s.description, s.price_cents, s.rating_avg, s.popularity_score, s.status,
	              s.created_at::text, s.updated_at::text,
	              c.id, c.name,
	              pp.id, pp.business_name, pp.service_area_miles
	       FROM services s
	       LEFT JOIN categories c ON s.category_id = c.id
	       JOIN provider_profiles pp ON s.provider_id = pp.id
	       WHERE s.status = 'active'`

	if categoryID != "" {
		rows, queryErr = s.db.QueryContext(ctx,
			baseQuery+` AND s.category_id = $1 ORDER BY s.created_at DESC LIMIT $2 OFFSET $3`,
			categoryID, pageSize, offset)
	} else {
		rows, queryErr = s.db.QueryContext(ctx,
			baseQuery+` ORDER BY s.created_at DESC LIMIT $1 OFFSET $2`,
			pageSize, offset)
	}
	if queryErr != nil {
		return fmt.Errorf("catalog: list services: %w", queryErr)
	}
	defer rows.Close()

	services, serviceIDs, err := s.scanServiceRows(rows)
	if err != nil {
		return err
	}

	tagMap, err := s.getServiceTags(ctx, serviceIDs)
	if err != nil {
		return err
	}
	for i := range services {
		if tags, ok := tagMap[services[i].ID]; ok {
			services[i].Tags = tags
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"services":  services,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// HandleGetService returns a single active service with full detail including availability.
func (s *Service) HandleGetService(c echo.Context) error {
	ctx := c.Request().Context()
	serviceID := c.Param("serviceId")

	var svc ServiceSummary
	var catID, catName *string
	var provID, provBizName string
	var provAreaMiles *int
	var provUserID string

	err := s.db.QueryRowContext(ctx,
		`SELECT s.id, s.title, s.description, s.price_cents, s.rating_avg, s.popularity_score, s.status,
		        s.created_at::text, s.updated_at::text,
		        c.id, c.name,
		        pp.id, pp.business_name, pp.service_area_miles, pp.user_id
		 FROM services s
		 LEFT JOIN categories c ON s.category_id = c.id
		 JOIN provider_profiles pp ON s.provider_id = pp.id
		 WHERE s.id = $1 AND s.status = 'active'`, serviceID,
	).Scan(
		&svc.ID, &svc.Title, &svc.Description, &svc.PriceCents,
		&svc.RatingAvg, &svc.PopularityScore, &svc.Status,
		&svc.CreatedAt, &svc.UpdatedAt,
		&catID, &catName,
		&provID, &provBizName, &provAreaMiles, &provUserID,
	)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Service not found.")
	}
	if err != nil {
		return fmt.Errorf("catalog: get service: %w", err)
	}

	// Block check: if the requesting user has a block relationship with the provider, return 404
	if s.IsBlocked != nil {
		if reqUser := auth.UserFromContext(c); reqUser != nil {
			blocked, bErr := s.IsBlocked(ctx, reqUser.ID, provUserID)
			if bErr == nil && blocked {
				return httpx.NewNotFoundError("Service not found.")
			}
		}
	}

	if catID != nil {
		svc.Category = &CategoryRef{ID: *catID, Name: *catName}
	}
	svc.Provider = ProviderRef{ID: provID, BusinessName: provBizName, ServiceAreaMiles: provAreaMiles}

	// Load tags
	svc.Tags = make([]TagRef, 0)
	tagMap, err := s.getServiceTags(ctx, []string{serviceID})
	if err == nil {
		if tags, ok := tagMap[serviceID]; ok {
			svc.Tags = tags
		}
	}

	// Load availability windows
	awRows, err := s.db.QueryContext(ctx,
		`SELECT id, day_of_week, start_time::text, end_time::text
		 FROM service_availability_windows
		 WHERE service_id = $1
		 ORDER BY day_of_week, start_time`, serviceID)
	if err != nil {
		return fmt.Errorf("catalog: list availability: %w", err)
	}
	defer awRows.Close()

	availability := make([]AvailabilityWindow, 0)
	for awRows.Next() {
		var aw AvailabilityWindow
		if err := awRows.Scan(&aw.ID, &aw.DayOfWeek, &aw.StartTime, &aw.EndTime); err != nil {
			return fmt.Errorf("catalog: scan availability: %w", err)
		}
		availability = append(availability, aw)
	}
	if err := awRows.Err(); err != nil {
		return fmt.Errorf("catalog: iterate availability: %w", err)
	}

	detail := ServiceDetail{
		ServiceSummary: svc,
		Availability:   availability,
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"service": detail})
}

// HandleHotKeywords returns all keywords marked as hot (public read).
func (s *Service) HandleHotKeywords(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, keyword FROM search_keyword_config WHERE is_hot = true ORDER BY keyword`)
	if err != nil {
		return fmt.Errorf("catalog: hot keywords: %w", err)
	}
	defer rows.Close()

	type kwRef struct {
		ID      string `json:"id"`
		Keyword string `json:"keyword"`
	}
	keywords := make([]kwRef, 0)
	for rows.Next() {
		var k kwRef
		if err := rows.Scan(&k.ID, &k.Keyword); err != nil {
			return fmt.Errorf("catalog: scan hot keyword: %w", err)
		}
		keywords = append(keywords, k)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("catalog: iterate hot keywords: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"keywords": keywords})
}

// HandleAutocomplete returns autocomplete terms matching a prefix.
func (s *Service) HandleAutocomplete(c echo.Context) error {
	q := c.QueryParam("q")

	var rows *sql.Rows
	var err error
	if q != "" {
		rows, err = s.db.QueryContext(c.Request().Context(),
			`SELECT id, term, weight FROM autocomplete_terms WHERE term ILIKE $1 ORDER BY weight DESC, term LIMIT 10`,
			q+"%")
	} else {
		rows, err = s.db.QueryContext(c.Request().Context(),
			`SELECT id, term, weight FROM autocomplete_terms ORDER BY weight DESC, term LIMIT 10`)
	}
	if err != nil {
		return fmt.Errorf("catalog: autocomplete: %w", err)
	}
	defer rows.Close()

	type termRef struct {
		ID     string `json:"id"`
		Term   string `json:"term"`
		Weight int    `json:"weight"`
	}
	terms := make([]termRef, 0)
	for rows.Next() {
		var t termRef
		if err := rows.Scan(&t.ID, &t.Term, &t.Weight); err != nil {
			return fmt.Errorf("catalog: scan autocomplete: %w", err)
		}
		terms = append(terms, t)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("catalog: iterate autocomplete: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"terms": terms})
}
