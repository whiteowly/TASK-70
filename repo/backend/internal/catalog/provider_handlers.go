package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"fieldserve/internal/audit"
	"fieldserve/internal/auth"
	"fieldserve/internal/platform/httpx"

	"github.com/labstack/echo/v4"
)

// HandleProviderListServices lists all services owned by the authenticated provider.
func (s *Service) HandleProviderListServices(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)

	providerID, err := s.getProviderProfileID(ctx, user.ID)
	if err != nil {
		return err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id, s.title, s.description, s.price_cents, s.rating_avg, s.popularity_score, s.status,
		        s.created_at::text, s.updated_at::text,
		        c.id, c.name,
		        pp.id, pp.business_name, pp.service_area_miles
		 FROM services s
		 LEFT JOIN categories c ON s.category_id = c.id
		 JOIN provider_profiles pp ON s.provider_id = pp.id
		 WHERE s.provider_id = $1
		 ORDER BY s.created_at DESC`, providerID)
	if err != nil {
		return fmt.Errorf("catalog: list provider services: %w", err)
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

	return c.JSON(http.StatusOK, map[string]interface{}{"services": services})
}

// HandleProviderCreateService creates a new service for the authenticated provider.
func (s *Service) HandleProviderCreateService(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)

	providerID, err := s.getProviderProfileID(ctx, user.ID)
	if err != nil {
		return err
	}

	var req struct {
		CategoryID  string   `json:"category_id"`
		Title       string   `json:"title"`
		Description *string  `json:"description"`
		PriceCents  int      `json:"price_cents"`
		TagIDs      []string `json:"tag_ids"`
		Status      string   `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	fieldErrors := map[string][]string{}
	if strings.TrimSpace(req.Title) == "" {
		fieldErrors["title"] = []string{"Title is required."}
	}
	if strings.TrimSpace(req.CategoryID) == "" {
		fieldErrors["category_id"] = []string{"Category ID is required."}
	}
	if req.PriceCents < 0 {
		fieldErrors["price_cents"] = []string{"Price must be non-negative."}
	}
	if len(fieldErrors) > 0 {
		return httpx.NewValidationError(fieldErrors)
	}

	if req.Status == "" {
		req.Status = "active"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalog: begin tx: %w", err)
	}
	defer tx.Rollback()

	var svc ServiceSummary
	var catID, catName *string
	var provID, provBizName string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO services (provider_id, category_id, title, description, price_cents, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, title, description, price_cents, rating_avg, popularity_score, status, created_at::text, updated_at::text`,
		providerID, req.CategoryID, req.Title, req.Description, req.PriceCents, req.Status,
	).Scan(&svc.ID, &svc.Title, &svc.Description, &svc.PriceCents, &svc.RatingAvg, &svc.PopularityScore, &svc.Status, &svc.CreatedAt, &svc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("catalog: insert service: %w", err)
	}

	// Load category ref
	err = tx.QueryRowContext(ctx, `SELECT id, name FROM categories WHERE id = $1`, req.CategoryID).Scan(&catID, &catName)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("catalog: load category: %w", err)
	}
	if catID != nil {
		svc.Category = &CategoryRef{ID: *catID, Name: *catName}
	}

	// Load provider ref
	err = tx.QueryRowContext(ctx, `SELECT id, business_name FROM provider_profiles WHERE id = $1`, providerID).Scan(&provID, &provBizName)
	if err != nil {
		return fmt.Errorf("catalog: load provider: %w", err)
	}
	svc.Provider = ProviderRef{ID: provID, BusinessName: provBizName}

	// Insert tags
	svc.Tags = make([]TagRef, 0)
	for _, tagID := range req.TagIDs {
		_, err := tx.ExecContext(ctx, `INSERT INTO service_tags (service_id, tag_id) VALUES ($1, $2)`, svc.ID, tagID)
		if err != nil {
			return fmt.Errorf("catalog: insert service tag: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("catalog: commit: %w", err)
	}

	// Load tags after commit
	if len(req.TagIDs) > 0 {
		tagMap, err := s.getServiceTags(ctx, []string{svc.ID})
		if err == nil {
			if tags, ok := tagMap[svc.ID]; ok {
				svc.Tags = tags
			}
		}
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "service_created",
		ActorID:      user.ID,
		ResourceType: "service",
		ResourceID:   svc.ID,
	})

	s.notifyCatalogChange()
	return c.JSON(http.StatusCreated, map[string]interface{}{"service": svc})
}

// HandleProviderUpdateService partially updates a service owned by the authenticated provider.
func (s *Service) HandleProviderUpdateService(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	serviceID := c.Param("serviceId")

	providerID, err := s.getProviderProfileID(ctx, user.ID)
	if err != nil {
		return err
	}

	// Verify ownership
	var ownerProviderID string
	err = s.db.QueryRowContext(ctx, `SELECT provider_id FROM services WHERE id = $1`, serviceID).Scan(&ownerProviderID)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Service not found.")
	}
	if err != nil {
		return fmt.Errorf("catalog: check service ownership: %w", err)
	}
	if ownerProviderID != providerID {
		return httpx.NewNotFoundError("Service not found.")
	}

	var req struct {
		CategoryID  *string  `json:"category_id"`
		Title       *string  `json:"title"`
		Description *string  `json:"description"`
		PriceCents  *int     `json:"price_cents"`
		TagIDs      []string `json:"tag_ids"`
		Status      *string  `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.CategoryID != nil {
		setClauses = append(setClauses, fmt.Sprintf("category_id = $%d", argIdx))
		args = append(args, *req.CategoryID)
		argIdx++
	}
	if req.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *req.Title)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.PriceCents != nil {
		setClauses = append(setClauses, fmt.Sprintf("price_cents = $%d", argIdx))
		args = append(args, *req.PriceCents)
		argIdx++
	}
	if req.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++
	}

	// Always update updated_at
	setClauses = append(setClauses, "updated_at = NOW()")

	tagsProvided := req.TagIDs != nil

	if len(setClauses) <= 1 && !tagsProvided {
		// Only updated_at and no tag update — nothing meaningful to change
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "No fields to update.")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalog: begin tx: %w", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(
		`UPDATE services SET %s WHERE id = $%d AND provider_id = $%d
		 RETURNING id, title, description, price_cents, rating_avg, popularity_score, status, created_at::text, updated_at::text`,
		strings.Join(setClauses, ", "), argIdx, argIdx+1,
	)
	args = append(args, serviceID, providerID)

	var svc ServiceSummary
	err = tx.QueryRowContext(ctx, query, args...).
		Scan(&svc.ID, &svc.Title, &svc.Description, &svc.PriceCents, &svc.RatingAvg, &svc.PopularityScore, &svc.Status, &svc.CreatedAt, &svc.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return httpx.NewNotFoundError("Service not found.")
		}
		return fmt.Errorf("catalog: update service: %w", err)
	}

	// Replace tags if provided
	if tagsProvided {
		_, err = tx.ExecContext(ctx, `DELETE FROM service_tags WHERE service_id = $1`, serviceID)
		if err != nil {
			return fmt.Errorf("catalog: delete service tags: %w", err)
		}
		for _, tagID := range req.TagIDs {
			_, err = tx.ExecContext(ctx, `INSERT INTO service_tags (service_id, tag_id) VALUES ($1, $2)`, serviceID, tagID)
			if err != nil {
				return fmt.Errorf("catalog: insert service tag: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("catalog: commit: %w", err)
	}

	// Load category and provider refs
	s.loadServiceRefs(ctx, &svc, serviceID)

	// Load tags
	svc.Tags = make([]TagRef, 0)
	tagMap, err := s.getServiceTags(ctx, []string{serviceID})
	if err == nil {
		if tags, ok := tagMap[serviceID]; ok {
			svc.Tags = tags
		}
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "service_updated",
		ActorID:      user.ID,
		ResourceType: "service",
		ResourceID:   svc.ID,
	})

	s.notifyCatalogChange()
	return c.JSON(http.StatusOK, map[string]interface{}{"service": svc})
}

// HandleProviderDeleteService deletes a service owned by the authenticated provider.
func (s *Service) HandleProviderDeleteService(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	serviceID := c.Param("serviceId")

	providerID, err := s.getProviderProfileID(ctx, user.ID)
	if err != nil {
		return err
	}

	// Verify ownership
	var ownerProviderID string
	err = s.db.QueryRowContext(ctx, `SELECT provider_id FROM services WHERE id = $1`, serviceID).Scan(&ownerProviderID)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Service not found.")
	}
	if err != nil {
		return fmt.Errorf("catalog: check service ownership: %w", err)
	}
	if ownerProviderID != providerID {
		return httpx.NewNotFoundError("Service not found.")
	}

	// Delete related records first
	_, _ = s.db.ExecContext(ctx, `DELETE FROM service_availability_windows WHERE service_id = $1`, serviceID)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM service_tags WHERE service_id = $1`, serviceID)

	res, err := s.db.ExecContext(ctx, `DELETE FROM services WHERE id = $1 AND provider_id = $2`, serviceID, providerID)
	if err != nil {
		return fmt.Errorf("catalog: delete service: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return httpx.NewNotFoundError("Service not found.")
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "service_deleted",
		ActorID:      user.ID,
		ResourceType: "service",
		ResourceID:   serviceID,
	})

	s.notifyCatalogChange()
	return c.JSON(http.StatusOK, map[string]interface{}{"message": "Service deleted."})
}

// HandleProviderSetAvailability replaces all availability windows for a service.
func (s *Service) HandleProviderSetAvailability(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	serviceID := c.Param("serviceId")

	providerID, err := s.getProviderProfileID(ctx, user.ID)
	if err != nil {
		return err
	}

	// Verify ownership
	var ownerProviderID string
	err = s.db.QueryRowContext(ctx, `SELECT provider_id FROM services WHERE id = $1`, serviceID).Scan(&ownerProviderID)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Service not found.")
	}
	if err != nil {
		return fmt.Errorf("catalog: check service ownership: %w", err)
	}
	if ownerProviderID != providerID {
		return httpx.NewNotFoundError("Service not found.")
	}

	var req struct {
		Windows []struct {
			DayOfWeek int    `json:"day_of_week"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
		} `json:"windows"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	// Validate windows
	for i, w := range req.Windows {
		fieldErrors := map[string][]string{}
		if w.DayOfWeek < 0 || w.DayOfWeek > 6 {
			fieldErrors[fmt.Sprintf("windows[%d].day_of_week", i)] = []string{"Day of week must be 0-6."}
		}
		if strings.TrimSpace(w.StartTime) == "" {
			fieldErrors[fmt.Sprintf("windows[%d].start_time", i)] = []string{"Start time is required."}
		}
		if strings.TrimSpace(w.EndTime) == "" {
			fieldErrors[fmt.Sprintf("windows[%d].end_time", i)] = []string{"End time is required."}
		}
		if len(fieldErrors) > 0 {
			return httpx.NewValidationError(fieldErrors)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalog: begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `DELETE FROM service_availability_windows WHERE service_id = $1`, serviceID)
	if err != nil {
		return fmt.Errorf("catalog: delete availability: %w", err)
	}

	windows := make([]AvailabilityWindow, 0, len(req.Windows))
	for _, w := range req.Windows {
		var aw AvailabilityWindow
		err := tx.QueryRowContext(ctx,
			`INSERT INTO service_availability_windows (service_id, day_of_week, start_time, end_time)
			 VALUES ($1, $2, $3, $4) RETURNING id, day_of_week, start_time::text, end_time::text`,
			serviceID, w.DayOfWeek, w.StartTime, w.EndTime,
		).Scan(&aw.ID, &aw.DayOfWeek, &aw.StartTime, &aw.EndTime)
		if err != nil {
			return fmt.Errorf("catalog: insert availability window: %w", err)
		}
		windows = append(windows, aw)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("catalog: commit: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "availability_updated",
		ActorID:      user.ID,
		ResourceType: "service",
		ResourceID:   serviceID,
	})

	s.notifyCatalogChange()
	return c.JSON(http.StatusOK, map[string]interface{}{"availability": windows})
}

// HandleProviderGetService returns a single service owned by the authenticated
// provider, regardless of status. This allows providers to load their own
// inactive services for editing and availability management.
func (s *Service) HandleProviderGetService(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	serviceID := c.Param("serviceId")

	providerID, err := s.getProviderProfileID(ctx, user.ID)
	if err != nil {
		return err
	}

	var svc ServiceSummary
	var catID, catName *string
	var provID, provBizName string
	var provAreaMiles *int

	err = s.db.QueryRowContext(ctx,
		`SELECT s.id, s.title, s.description, s.price_cents, s.rating_avg, s.popularity_score, s.status,
		        s.created_at::text, s.updated_at::text,
		        c.id, c.name,
		        pp.id, pp.business_name, pp.service_area_miles
		 FROM services s
		 LEFT JOIN categories c ON s.category_id = c.id
		 JOIN provider_profiles pp ON s.provider_id = pp.id
		 WHERE s.id = $1 AND s.provider_id = $2`, serviceID, providerID,
	).Scan(
		&svc.ID, &svc.Title, &svc.Description, &svc.PriceCents,
		&svc.RatingAvg, &svc.PopularityScore, &svc.Status,
		&svc.CreatedAt, &svc.UpdatedAt,
		&catID, &catName,
		&provID, &provBizName, &provAreaMiles,
	)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Service not found.")
	}
	if err != nil {
		return fmt.Errorf("catalog: get provider service: %w", err)
	}

	if catID != nil {
		svc.Category = &CategoryRef{ID: *catID, Name: *catName}
	}
	svc.Provider = ProviderRef{ID: provID, BusinessName: provBizName, ServiceAreaMiles: provAreaMiles}

	svc.Tags = make([]TagRef, 0)
	tagMap, err := s.getServiceTags(ctx, []string{serviceID})
	if err == nil {
		if tags, ok := tagMap[serviceID]; ok {
			svc.Tags = tags
		}
	}

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

// loadServiceRefs populates the Category and Provider refs on a ServiceSummary.
func (s *Service) loadServiceRefs(ctx context.Context, svc *ServiceSummary, serviceID string) {
	var catID, catName *string
	var provID, provBizName string

	_ = s.db.QueryRowContext(ctx,
		`SELECT c.id, c.name FROM categories c JOIN services s ON s.category_id = c.id WHERE s.id = $1`, serviceID,
	).Scan(&catID, &catName)
	if catID != nil {
		svc.Category = &CategoryRef{ID: *catID, Name: *catName}
	}

	var loadAreaMiles *int
	_ = s.db.QueryRowContext(ctx,
		`SELECT pp.id, pp.business_name, pp.service_area_miles FROM provider_profiles pp JOIN services s ON s.provider_id = pp.id WHERE s.id = $1`, serviceID,
	).Scan(&provID, &provBizName, &loadAreaMiles)
	svc.Provider = ProviderRef{ID: provID, BusinessName: provBizName, ServiceAreaMiles: loadAreaMiles}
}

// scanServiceRows scans service list query rows into ServiceSummary slices.
func (s *Service) scanServiceRows(rows *sql.Rows) ([]ServiceSummary, []string, error) {
	services := make([]ServiceSummary, 0)
	var serviceIDs []string

	for rows.Next() {
		var svc ServiceSummary
		var catID, catName *string
		var provID, provBizName string
		var provAreaMiles *int

		if err := rows.Scan(
			&svc.ID, &svc.Title, &svc.Description, &svc.PriceCents,
			&svc.RatingAvg, &svc.PopularityScore, &svc.Status,
			&svc.CreatedAt, &svc.UpdatedAt,
			&catID, &catName,
			&provID, &provBizName, &provAreaMiles,
		); err != nil {
			return nil, nil, fmt.Errorf("catalog: scan service: %w", err)
		}

		if catID != nil {
			svc.Category = &CategoryRef{ID: *catID, Name: *catName}
		}
		svc.Provider = ProviderRef{ID: provID, BusinessName: provBizName, ServiceAreaMiles: provAreaMiles}
		svc.Tags = make([]TagRef, 0)

		services = append(services, svc)
		serviceIDs = append(serviceIDs, svc.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("catalog: iterate services: %w", err)
	}

	return services, serviceIDs, nil
}
