package analytics

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fieldserve/internal/audit"
	"fieldserve/internal/auth"
	"fieldserve/internal/platform/httpx"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const ExportsRoot = "/app/data/exports"

// Service provides analytics computation and export capabilities.
type Service struct {
	db    *sql.DB
	audit *audit.Service
}

// NewService creates a new analytics service.
func NewService(db *sql.DB, audit *audit.Service) *Service {
	return &Service{db: db, audit: audit}
}

// DashboardMetric represents a single data point for dashboard charts.
type DashboardMetric struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
	Label string  `json:"label,omitempty"`
}

// ConversionMetric represents search-to-interest conversion data for a day.
type ConversionMetric struct {
	Date      string  `json:"date"`
	Searches  int     `json:"searches"`
	Interests int     `json:"interests"`
	Rate      float64 `json:"rate"`
}

// ProviderUtilization represents per-provider activity data.
type ProviderUtilization struct {
	ID             string `json:"id"`
	BusinessName   string `json:"business_name"`
	ActiveServices int    `json:"active_services"`
	TotalInterests int    `json:"total_interests"`
	MessagesSent   int    `json:"messages_sent"`
}

// ExportJob represents an export job record.
type ExportJob struct {
	ID          string  `json:"id"`
	AdminID     string  `json:"admin_id"`
	ExportType  string  `json:"export_type"`
	Status      string  `json:"status"`
	FilePath    *string `json:"file_path"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt *string `json:"completed_at"`
}

func getAdminProfileID(db *sql.DB, ctx context.Context, userID string) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM admin_profiles WHERE user_id = $1`, userID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", httpx.NewNotFoundError("Admin profile not found.")
	}
	return id, err
}

func parseDateRange(c echo.Context) (string, string) {
	from := c.QueryParam("from")
	to := c.QueryParam("to")
	if from == "" {
		from = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	return from, to
}

// HandleUserGrowth handles GET /admin/analytics/user-growth.
func (s *Service) HandleUserGrowth(c echo.Context) error {
	from, to := parseDateRange(c)

	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT u.created_at::date AS day, r.name AS role, COUNT(*) AS count
		 FROM users u
		 JOIN user_roles ur ON ur.user_id = u.id
		 JOIN roles r ON r.id = ur.role_id
		 WHERE u.created_at::date >= $1 AND u.created_at::date <= $2
		 GROUP BY day, r.name
		 ORDER BY day`, from, to)
	if err != nil {
		return fmt.Errorf("analytics: user growth query: %w", err)
	}
	defer rows.Close()

	metrics := []DashboardMetric{}
	for rows.Next() {
		var m DashboardMetric
		if err := rows.Scan(&m.Date, &m.Label, &m.Value); err != nil {
			return fmt.Errorf("analytics: scan: %w", err)
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("analytics: rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"metrics": metrics})
}

// HandleConversion handles GET /admin/analytics/conversion.
func (s *Service) HandleConversion(c echo.Context) error {
	ctx := c.Request().Context()
	from, to := parseDateRange(c)

	// Get search counts per day
	searchRows, err := s.db.QueryContext(ctx,
		`SELECT created_at::date AS day, COUNT(*) AS searches
		 FROM search_events
		 WHERE created_at::date >= $1 AND created_at::date <= $2
		 GROUP BY day ORDER BY day`, from, to)
	if err != nil {
		return fmt.Errorf("analytics: search query: %w", err)
	}
	defer searchRows.Close()

	searchMap := map[string]int{}
	for searchRows.Next() {
		var day string
		var count int
		if err := searchRows.Scan(&day, &count); err != nil {
			return fmt.Errorf("analytics: scan search: %w", err)
		}
		searchMap[day] = count
	}
	if err := searchRows.Err(); err != nil {
		return fmt.Errorf("analytics: search rows: %w", err)
	}

	// Get interest counts per day
	interestRows, err := s.db.QueryContext(ctx,
		`SELECT created_at::date AS day, COUNT(*) AS interests
		 FROM interests
		 WHERE created_at::date >= $1 AND created_at::date <= $2
		 GROUP BY day ORDER BY day`, from, to)
	if err != nil {
		return fmt.Errorf("analytics: interest query: %w", err)
	}
	defer interestRows.Close()

	interestMap := map[string]int{}
	allDays := map[string]bool{}
	for interestRows.Next() {
		var day string
		var count int
		if err := interestRows.Scan(&day, &count); err != nil {
			return fmt.Errorf("analytics: scan interest: %w", err)
		}
		interestMap[day] = count
		allDays[day] = true
	}
	if err := interestRows.Err(); err != nil {
		return fmt.Errorf("analytics: interest rows: %w", err)
	}

	// Merge search days
	for day := range searchMap {
		allDays[day] = true
	}

	// Build sorted metrics
	metrics := []ConversionMetric{}
	for day := range allDays {
		searches := searchMap[day]
		interests := interestMap[day]
		var rate float64
		if searches > 0 {
			rate = float64(interests) / float64(searches)
		}
		metrics = append(metrics, ConversionMetric{
			Date:      day,
			Searches:  searches,
			Interests: interests,
			Rate:      rate,
		})
	}

	// Sort by date
	sortConversionMetrics(metrics)

	return c.JSON(http.StatusOK, map[string]interface{}{"metrics": metrics})
}

func sortConversionMetrics(metrics []ConversionMetric) {
	for i := 0; i < len(metrics); i++ {
		for j := i + 1; j < len(metrics); j++ {
			if metrics[i].Date > metrics[j].Date {
				metrics[i], metrics[j] = metrics[j], metrics[i]
			}
		}
	}
}

// HandleProviderUtilization handles GET /admin/analytics/provider-utilization.
func (s *Service) HandleProviderUtilization(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT pp.id, pp.business_name,
			(SELECT COUNT(*) FROM services s WHERE s.provider_id = pp.id AND s.status = 'active') AS active_services,
			(SELECT COUNT(*) FROM interests i WHERE i.provider_id = pp.id) AS total_interests,
			(SELECT COUNT(*) FROM messages m WHERE m.sender_id = pp.user_id) AS messages_sent
		 FROM provider_profiles pp
		 ORDER BY total_interests DESC`)
	if err != nil {
		return fmt.Errorf("analytics: provider utilization query: %w", err)
	}
	defer rows.Close()

	providers := []ProviderUtilization{}
	for rows.Next() {
		var p ProviderUtilization
		if err := rows.Scan(&p.ID, &p.BusinessName, &p.ActiveServices, &p.TotalInterests, &p.MessagesSent); err != nil {
			return fmt.Errorf("analytics: scan provider: %w", err)
		}
		providers = append(providers, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("analytics: provider rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"providers": providers})
}

// HandleGenerateRollups handles POST /admin/analytics/rollup.
func (s *Service) HandleGenerateRollups(c echo.Context) error {
	ctx := c.Request().Context()

	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}
	if req.From == "" {
		req.From = time.Now().Format("2006-01-02")
	}
	if req.To == "" {
		req.To = time.Now().Format("2006-01-02")
	}

	// User growth rollups
	growthRows, err := s.db.QueryContext(ctx,
		`SELECT u.created_at::date AS day, r.name AS role, COUNT(*) AS count
		 FROM users u
		 JOIN user_roles ur ON ur.user_id = u.id
		 JOIN roles r ON r.id = ur.role_id
		 WHERE u.created_at::date >= $1 AND u.created_at::date <= $2
		 GROUP BY day, r.name`, req.From, req.To)
	if err != nil {
		return fmt.Errorf("analytics: rollup growth query: %w", err)
	}
	defer growthRows.Close()

	rollupCount := 0
	for growthRows.Next() {
		var day, role string
		var count float64
		if err := growthRows.Scan(&day, &role, &count); err != nil {
			return fmt.Errorf("analytics: scan rollup: %w", err)
		}
		meta, _ := json.Marshal(map[string]string{"role": role})
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO analytics_daily_rollups (rollup_date, metric_name, metric_value, metadata)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT DO NOTHING`,
			day, "user_growth", count, meta)
		if err != nil {
			return fmt.Errorf("analytics: insert rollup: %w", err)
		}
		rollupCount++
	}

	// Search conversion rollups
	searchRows, err := s.db.QueryContext(ctx,
		`SELECT created_at::date AS day, COUNT(*) FROM search_events
		 WHERE created_at::date >= $1 AND created_at::date <= $2
		 GROUP BY day`, req.From, req.To)
	if err != nil {
		return fmt.Errorf("analytics: rollup search query: %w", err)
	}
	defer searchRows.Close()

	for searchRows.Next() {
		var day string
		var count float64
		if err := searchRows.Scan(&day, &count); err != nil {
			return fmt.Errorf("analytics: scan search rollup: %w", err)
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO analytics_daily_rollups (rollup_date, metric_name, metric_value)
			 VALUES ($1, $2, $3)
			 ON CONFLICT DO NOTHING`,
			day, "search_events", count)
		if err != nil {
			return fmt.Errorf("analytics: insert search rollup: %w", err)
		}
		rollupCount++
	}

	// Interest rollups
	interestRows, err := s.db.QueryContext(ctx,
		`SELECT created_at::date AS day, COUNT(*) FROM interests
		 WHERE created_at::date >= $1 AND created_at::date <= $2
		 GROUP BY day`, req.From, req.To)
	if err != nil {
		return fmt.Errorf("analytics: rollup interest query: %w", err)
	}
	defer interestRows.Close()

	for interestRows.Next() {
		var day string
		var count float64
		if err := interestRows.Scan(&day, &count); err != nil {
			return fmt.Errorf("analytics: scan interest rollup: %w", err)
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO analytics_daily_rollups (rollup_date, metric_name, metric_value)
			 VALUES ($1, $2, $3)
			 ON CONFLICT DO NOTHING`,
			day, "interests", count)
		if err != nil {
			return fmt.Errorf("analytics: insert interest rollup: %w", err)
		}
		rollupCount++
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":      "Rollups generated.",
		"rollup_count": rollupCount,
	})
}

// HandleCreateExport handles POST /admin/exports.
func (s *Service) HandleCreateExport(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)

	adminID, err := getAdminProfileID(s.db, ctx, user.ID)
	if err != nil {
		return err
	}

	var req struct {
		ExportType string `json:"export_type"`
		From       string `json:"from"`
		To         string `json:"to"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	validTypes := map[string]bool{
		"user_growth":          true,
		"conversion":           true,
		"provider_utilization": true,
	}
	if !validTypes[req.ExportType] {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid export type.")
	}

	if req.From == "" {
		req.From = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	if req.To == "" {
		req.To = time.Now().Format("2006-01-02")
	}

	// Create pending export job
	var export ExportJob
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO export_jobs (admin_id, export_type, status)
		 VALUES ($1, $2, 'pending')
		 RETURNING id, admin_id, export_type, status, file_path, created_at::text, completed_at::text`,
		adminID, req.ExportType,
	).Scan(&export.ID, &export.AdminID, &export.ExportType, &export.Status, &export.FilePath, &export.CreatedAt, &export.CompletedAt)
	if err != nil {
		return fmt.Errorf("analytics: insert export: %w", err)
	}

	// Generate CSV content
	csvContent, csvErr := s.generateCSV(ctx, req.ExportType, req.From, req.To)
	if csvErr != nil {
		// Mark as failed
		s.db.ExecContext(ctx,
			`UPDATE export_jobs SET status = 'failed' WHERE id = $1`, export.ID)
		s.audit.Log(ctx, audit.Event{
			EventType:    "export_failed",
			ActorID:      user.ID,
			ResourceType: "export",
			ResourceID:   export.ID,
			Metadata:     map[string]interface{}{"error": csvErr.Error()},
		})
		return fmt.Errorf("analytics: generate csv: %w", csvErr)
	}

	// Write CSV to disk
	os.MkdirAll(ExportsRoot, 0755)
	filePath := filepath.Join(ExportsRoot, uuid.New().String()+".csv")
	if err := os.WriteFile(filePath, csvContent, 0644); err != nil {
		s.db.ExecContext(ctx,
			`UPDATE export_jobs SET status = 'failed' WHERE id = $1`, export.ID)
		return fmt.Errorf("analytics: write csv: %w", err)
	}

	// Update export job as completed
	err = s.db.QueryRowContext(ctx,
		`UPDATE export_jobs SET status = 'completed', file_path = $1, completed_at = NOW()
		 WHERE id = $2
		 RETURNING id, admin_id, export_type, status, file_path, created_at::text, completed_at::text`,
		filePath, export.ID,
	).Scan(&export.ID, &export.AdminID, &export.ExportType, &export.Status, &export.FilePath, &export.CreatedAt, &export.CompletedAt)
	if err != nil {
		return fmt.Errorf("analytics: update export: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "export_created",
		ActorID:      user.ID,
		ResourceType: "export",
		ResourceID:   export.ID,
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{"export": export})
}

func (s *Service) generateCSV(ctx context.Context, exportType, from, to string) ([]byte, error) {
	var buf []byte
	switch exportType {
	case "user_growth":
		return s.generateUserGrowthCSV(from, to)
	case "conversion":
		return s.generateConversionCSV(from, to)
	case "provider_utilization":
		return s.generateProviderUtilizationCSV()
	}
	return buf, fmt.Errorf("unknown export type: %s", exportType)
}

func (s *Service) generateUserGrowthCSV(from, to string) ([]byte, error) {
	rows, err := s.db.Query(
		`SELECT u.created_at::date AS day, r.name AS role, COUNT(*) AS count
		 FROM users u
		 JOIN user_roles ur ON ur.user_id = u.id
		 JOIN roles r ON r.id = ur.role_id
		 WHERE u.created_at::date >= $1 AND u.created_at::date <= $2
		 GROUP BY day, r.name
		 ORDER BY day`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records [][]string
	records = append(records, []string{"date", "role", "count"})
	for rows.Next() {
		var day, role string
		var count int
		if err := rows.Scan(&day, &role, &count); err != nil {
			return nil, err
		}
		records = append(records, []string{day, role, fmt.Sprintf("%d", count)})
	}
	return writeCSV(records)
}

func (s *Service) generateConversionCSV(from, to string) ([]byte, error) {
	searchRows, err := s.db.Query(
		`SELECT created_at::date AS day, COUNT(*) FROM search_events
		 WHERE created_at::date >= $1 AND created_at::date <= $2
		 GROUP BY day ORDER BY day`, from, to)
	if err != nil {
		return nil, err
	}
	defer searchRows.Close()

	searchMap := map[string]int{}
	for searchRows.Next() {
		var day string
		var count int
		if err := searchRows.Scan(&day, &count); err != nil {
			return nil, err
		}
		searchMap[day] = count
	}

	interestRows, err := s.db.Query(
		`SELECT created_at::date AS day, COUNT(*) FROM interests
		 WHERE created_at::date >= $1 AND created_at::date <= $2
		 GROUP BY day ORDER BY day`, from, to)
	if err != nil {
		return nil, err
	}
	defer interestRows.Close()

	interestMap := map[string]int{}
	allDays := map[string]bool{}
	for interestRows.Next() {
		var day string
		var count int
		if err := interestRows.Scan(&day, &count); err != nil {
			return nil, err
		}
		interestMap[day] = count
		allDays[day] = true
	}
	for d := range searchMap {
		allDays[d] = true
	}

	var records [][]string
	records = append(records, []string{"date", "searches", "interests", "rate"})
	for day := range allDays {
		searches := searchMap[day]
		interests := interestMap[day]
		var rate float64
		if searches > 0 {
			rate = float64(interests) / float64(searches)
		}
		records = append(records, []string{day, fmt.Sprintf("%d", searches), fmt.Sprintf("%d", interests), fmt.Sprintf("%.4f", rate)})
	}
	return writeCSV(records)
}

func (s *Service) generateProviderUtilizationCSV() ([]byte, error) {
	rows, err := s.db.Query(
		`SELECT pp.id, pp.business_name,
			(SELECT COUNT(*) FROM services s WHERE s.provider_id = pp.id AND s.status = 'active') AS active_services,
			(SELECT COUNT(*) FROM interests i WHERE i.provider_id = pp.id) AS total_interests,
			(SELECT COUNT(*) FROM messages m WHERE m.sender_id = pp.user_id) AS messages_sent
		 FROM provider_profiles pp
		 ORDER BY total_interests DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records [][]string
	records = append(records, []string{"id", "business_name", "active_services", "total_interests", "messages_sent"})
	for rows.Next() {
		var id, name string
		var active, interests, messages int
		if err := rows.Scan(&id, &name, &active, &interests, &messages); err != nil {
			return nil, err
		}
		records = append(records, []string{id, name, fmt.Sprintf("%d", active), fmt.Sprintf("%d", interests), fmt.Sprintf("%d", messages)})
	}
	return writeCSV(records)
}

func writeCSV(records [][]string) ([]byte, error) {
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	for _, record := range records {
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// HandleListExports handles GET /admin/exports.
func (s *Service) HandleListExports(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, admin_id, export_type, status, file_path, created_at::text, completed_at::text
		 FROM export_jobs ORDER BY created_at DESC`)
	if err != nil {
		return fmt.Errorf("analytics: list exports: %w", err)
	}
	defer rows.Close()

	exports := []ExportJob{}
	for rows.Next() {
		var e ExportJob
		if err := rows.Scan(&e.ID, &e.AdminID, &e.ExportType, &e.Status, &e.FilePath, &e.CreatedAt, &e.CompletedAt); err != nil {
			return fmt.Errorf("analytics: scan export: %w", err)
		}
		exports = append(exports, e)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("analytics: export rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"exports": exports})
}

// HandleGetExport handles GET /admin/exports/:exportId.
func (s *Service) HandleGetExport(c echo.Context) error {
	exportID := c.Param("exportId")
	var e ExportJob
	err := s.db.QueryRowContext(c.Request().Context(),
		`SELECT id, admin_id, export_type, status, file_path, created_at::text, completed_at::text
		 FROM export_jobs WHERE id = $1`, exportID,
	).Scan(&e.ID, &e.AdminID, &e.ExportType, &e.Status, &e.FilePath, &e.CreatedAt, &e.CompletedAt)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Export not found.")
	}
	if err != nil {
		return fmt.Errorf("analytics: get export: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"export": e})
}

// HandleDownloadExport handles GET /admin/exports/:exportId/download.
func (s *Service) HandleDownloadExport(c echo.Context) error {
	exportID := c.Param("exportId")
	var filePath *string
	var status string
	err := s.db.QueryRowContext(c.Request().Context(),
		`SELECT status, file_path FROM export_jobs WHERE id = $1`, exportID,
	).Scan(&status, &filePath)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Export not found.")
	}
	if err != nil {
		return fmt.Errorf("analytics: download export: %w", err)
	}
	if status != "completed" || filePath == nil || *filePath == "" {
		return httpx.NewNotFoundError("Export not available for download.")
	}

	return c.File(*filePath)
}

// RunDailyRollups computes and stores rollups for the given date. Called by the
// worker process.
func RunDailyRollups(db *sql.DB, date string) error {
	// Check if rollups already exist for this date
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM analytics_daily_rollups WHERE rollup_date = $1`, date).Scan(&count); err != nil {
		return fmt.Errorf("analytics: check rollups: %w", err)
	}
	if count > 0 {
		return nil // already generated
	}

	// User growth rollup
	growthRows, err := db.Query(
		`SELECT r.name, COUNT(*) FROM users u
		 JOIN user_roles ur ON ur.user_id = u.id
		 JOIN roles r ON r.id = ur.role_id
		 WHERE u.created_at::date = $1
		 GROUP BY r.name`, date)
	if err != nil {
		return fmt.Errorf("analytics: rollup growth: %w", err)
	}
	defer growthRows.Close()

	for growthRows.Next() {
		var role string
		var count float64
		if err := growthRows.Scan(&role, &count); err != nil {
			return fmt.Errorf("analytics: scan growth: %w", err)
		}
		meta, _ := json.Marshal(map[string]string{"role": role})
		db.Exec(
			`INSERT INTO analytics_daily_rollups (rollup_date, metric_name, metric_value, metadata)
			 VALUES ($1, $2, $3, $4)`,
			date, "user_growth", count, meta)
	}

	// Search event count
	var searchCount float64
	if err := db.QueryRow(`SELECT COUNT(*) FROM search_events WHERE created_at::date = $1`, date).Scan(&searchCount); err == nil && searchCount > 0 {
		db.Exec(
			`INSERT INTO analytics_daily_rollups (rollup_date, metric_name, metric_value)
			 VALUES ($1, $2, $3)`,
			date, "search_events", searchCount)
	}

	// Interest count
	var interestCount float64
	if err := db.QueryRow(`SELECT COUNT(*) FROM interests WHERE created_at::date = $1`, date).Scan(&interestCount); err == nil && interestCount > 0 {
		db.Exec(
			`INSERT INTO analytics_daily_rollups (rollup_date, metric_name, metric_value)
			 VALUES ($1, $2, $3)`,
			date, "interests", interestCount)
	}

	return nil
}
