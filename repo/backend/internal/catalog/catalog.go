package catalog

import (
	"context"
	"database/sql"
	"fmt"

	"fieldserve/internal/audit"
	"fieldserve/internal/platform/httpx"

	"github.com/lib/pq"
)

// Category represents a service category in the catalog.
type Category struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parent_id"`
	Name      string  `json:"name"`
	Slug      string  `json:"slug"`
	SortOrder int     `json:"sort_order"`
	CreatedAt string  `json:"created_at"`
}

// Tag represents a service tag.
type Tag struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// CategoryRef is a minimal category reference embedded in service responses.
type CategoryRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProviderRef is a minimal provider reference embedded in service responses.
type ProviderRef struct {
	ID               string `json:"id"`
	BusinessName     string `json:"business_name"`
	ServiceAreaMiles *int   `json:"service_area_miles"`
}

// TagRef is a minimal tag reference embedded in service responses.
type TagRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AvailabilityWindow represents a recurring availability slot for a service.
type AvailabilityWindow struct {
	ID        string `json:"id"`
	DayOfWeek int    `json:"day_of_week"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// ServiceSummary is the common service representation returned in list endpoints.
type ServiceSummary struct {
	ID              string       `json:"id"`
	Title           string       `json:"title"`
	Description     *string      `json:"description"`
	PriceCents      int          `json:"price_cents"`
	RatingAvg       string       `json:"rating_avg"`
	PopularityScore int          `json:"popularity_score"`
	Status          string       `json:"status"`
	Category        *CategoryRef `json:"category"`
	Provider        ProviderRef  `json:"provider"`
	Tags            []TagRef     `json:"tags"`
	CreatedAt       string       `json:"created_at"`
	UpdatedAt       string       `json:"updated_at"`
}

// ServiceDetail extends ServiceSummary with availability windows.
type ServiceDetail struct {
	ServiceSummary
	Availability []AvailabilityWindow `json:"availability"`
}

// Service provides catalog operations backed by the database.
type Service struct {
	db    *sql.DB
	audit *audit.Service
	// OnCatalogChange is called after any write mutation (category, tag, or
	// service change) so that dependent caches can be invalidated. It may be nil.
	OnCatalogChange func()
	// IsBlocked checks if two users have a block relationship. If set, the
	// service detail endpoint will return 404 for blocked provider services.
	IsBlocked func(ctx context.Context, userA, userB string) (bool, error)
}

// NewService creates a new catalog service.
func NewService(db *sql.DB, audit *audit.Service) *Service {
	return &Service{db: db, audit: audit}
}

// notifyCatalogChange calls the OnCatalogChange callback if set.
func (s *Service) notifyCatalogChange() {
	if s.OnCatalogChange != nil {
		s.OnCatalogChange()
	}
}

// getProviderProfileID resolves a user ID to its provider_profiles.id.
func (s *Service) getProviderProfileID(ctx context.Context, userID string) (string, error) {
	var profileID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM provider_profiles WHERE user_id = $1`, userID).Scan(&profileID)
	if err == sql.ErrNoRows {
		return "", httpx.NewNotFoundError("Provider profile not found.")
	}
	if err != nil {
		return "", fmt.Errorf("catalog: get provider profile: %w", err)
	}
	return profileID, nil
}

// getServiceTags loads tags for a set of service IDs and returns them grouped by service ID.
func (s *Service) getServiceTags(ctx context.Context, serviceIDs []string) (map[string][]TagRef, error) {
	result := make(map[string][]TagRef)
	if len(serviceIDs) == 0 {
		return result, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT st.service_id, t.id, t.name FROM tags t JOIN service_tags st ON st.tag_id = t.id WHERE st.service_id = ANY($1::uuid[])`,
		pq.Array(serviceIDs))
	if err != nil {
		return nil, fmt.Errorf("catalog: get service tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var serviceID, tagID, tagName string
		if err := rows.Scan(&serviceID, &tagID, &tagName); err != nil {
			return nil, fmt.Errorf("catalog: scan service tag: %w", err)
		}
		result[serviceID] = append(result[serviceID], TagRef{ID: tagID, Name: tagName})
	}
	return result, rows.Err()
}
