package blocks

import (
	"context"
	"database/sql"
	"fmt"

	"fieldserve/internal/audit"
	"fieldserve/internal/platform/httpx"
)

// BlockEntry represents a block relationship.
type BlockEntry struct {
	ID        string `json:"id"`
	BlockedID string `json:"blocked_id"`
	CreatedAt string `json:"created_at"`
}

// Service provides user-to-user blocking operations.
type Service struct {
	db    *sql.DB
	audit *audit.Service
}

// NewService creates a new blocks service.
func NewService(db *sql.DB, audit *audit.Service) *Service {
	return &Service{db: db, audit: audit}
}

// IsBlocked returns true if either user has blocked the other.
func (s *Service) IsBlocked(ctx context.Context, userA, userB string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1))`,
		userA, userB,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("blocks: is blocked: %w", err)
	}
	return exists, nil
}

// Block creates a block relationship. Idempotent — does nothing if already blocked.
func (s *Service) Block(ctx context.Context, blockerID, blockedID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		blockerID, blockedID,
	)
	if err != nil {
		return fmt.Errorf("blocks: create: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "block_created",
		ActorID:      blockerID,
		ResourceType: "block",
		Metadata: map[string]interface{}{
			"blocked_id": blockedID,
		},
	})

	return nil
}

// Unblock removes a block relationship. Returns 404 if no such block exists.
func (s *Service) Unblock(ctx context.Context, blockerID, blockedID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM blocks WHERE blocker_id=$1 AND blocked_id=$2`,
		blockerID, blockedID,
	)
	if err != nil {
		return fmt.Errorf("blocks: remove: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return httpx.NewNotFoundError("Block not found.")
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "block_removed",
		ActorID:      blockerID,
		ResourceType: "block",
		Metadata: map[string]interface{}{
			"blocked_id": blockedID,
		},
	})

	return nil
}

// ListBlocked returns all users blocked by the given user.
func (s *Service) ListBlocked(ctx context.Context, blockerID string) ([]BlockEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, blocked_id, created_at::text FROM blocks WHERE blocker_id=$1`,
		blockerID,
	)
	if err != nil {
		return nil, fmt.Errorf("blocks: list: %w", err)
	}
	defer rows.Close()

	entries := make([]BlockEntry, 0)
	for rows.Next() {
		var e BlockEntry
		if err := rows.Scan(&e.ID, &e.BlockedID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("blocks: scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
