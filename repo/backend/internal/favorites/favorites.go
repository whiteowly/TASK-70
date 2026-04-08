package favorites

import (
	"context"
	"database/sql"
	"fmt"

	"fieldserve/internal/platform/httpx"

	"github.com/lib/pq"
)

// Service provides favorites operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new favorites service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// Favorite represents a saved favorite entry.
type Favorite struct {
	ID        string `json:"id"`
	ServiceID string `json:"service_id"`
	CreatedAt string `json:"created_at"`
}

// GetCustomerProfileID resolves a user ID to its customer_profiles.id.
func (s *Service) GetCustomerProfileID(ctx context.Context, userID string) (string, error) {
	var profileID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM customer_profiles WHERE user_id = $1`, userID).Scan(&profileID)
	if err == sql.ErrNoRows {
		return "", httpx.NewNotFoundError("Customer profile not found.")
	}
	if err != nil {
		return "", fmt.Errorf("favorites: get customer profile: %w", err)
	}
	return profileID, nil
}

// List returns all favorites for a customer profile, newest first.
func (s *Service) List(ctx context.Context, customerProfileID string) ([]Favorite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, service_id, created_at::text FROM favorites WHERE customer_id = $1 ORDER BY created_at DESC`,
		customerProfileID)
	if err != nil {
		return nil, fmt.Errorf("favorites: list: %w", err)
	}
	defer rows.Close()

	favs := make([]Favorite, 0)
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.ServiceID, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("favorites: scan: %w", err)
		}
		favs = append(favs, f)
	}
	return favs, rows.Err()
}

// Add creates a favorite. If the favorite already exists (duplicate), it
// returns the existing row.
func (s *Service) Add(ctx context.Context, customerProfileID, serviceID string) (*Favorite, error) {
	var f Favorite
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO favorites (customer_id, service_id)
		 VALUES ($1, $2)
		 ON CONFLICT (customer_id, service_id) DO NOTHING
		 RETURNING id, service_id, created_at::text`,
		customerProfileID, serviceID,
	).Scan(&f.ID, &f.ServiceID, &f.CreatedAt)

	if err == sql.ErrNoRows {
		// Duplicate — fetch the existing row
		err = s.db.QueryRowContext(ctx,
			`SELECT id, service_id, created_at::text FROM favorites WHERE customer_id = $1 AND service_id = $2`,
			customerProfileID, serviceID,
		).Scan(&f.ID, &f.ServiceID, &f.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("favorites: fetch existing: %w", err)
		}
		return &f, nil
	}
	if err != nil {
		return nil, fmt.Errorf("favorites: add: %w", err)
	}
	return &f, nil
}

// Remove deletes a favorite. Returns a 404 error if the favorite does not
// exist.
func (s *Service) Remove(ctx context.Context, customerProfileID, serviceID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM favorites WHERE customer_id = $1 AND service_id = $2`,
		customerProfileID, serviceID)
	if err != nil {
		return fmt.Errorf("favorites: remove: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return httpx.NewNotFoundError("Favorite not found.")
	}
	return nil
}

// IsFavorited checks which of the given service IDs are favorited by a
// customer. Returns a map of serviceID -> true for favorited items.
func (s *Service) IsFavorited(ctx context.Context, customerProfileID string, serviceIDs []string) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(serviceIDs) == 0 {
		return result, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT service_id FROM favorites WHERE customer_id = $1 AND service_id = ANY($2::uuid[])`,
		customerProfileID, pq.Array(serviceIDs))
	if err != nil {
		return nil, fmt.Errorf("favorites: is favorited: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			return nil, fmt.Errorf("favorites: scan is favorited: %w", err)
		}
		result[sid] = true
	}
	return result, rows.Err()
}
