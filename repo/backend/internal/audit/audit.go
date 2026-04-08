package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"
)

type Service struct {
	db       *sql.DB
	FileSink *FileSink
}

func NewService(db *sql.DB) *Service {
	return &Service{
		db:       db,
		FileSink: NewFileSink(AuditLogRoot),
	}
}

type Event struct {
	EventType    string
	ActorID      string // empty if no actor
	ResourceType string
	ResourceID   string // empty if no resource
	Metadata     map[string]interface{}
}

func (s *Service) Log(ctx context.Context, e Event) {
	metaJSON, _ := json.Marshal(e.Metadata)
	var actorID, resourceID *string
	if e.ActorID != "" {
		actorID = &e.ActorID
	}
	if e.ResourceID != "" {
		resourceID = &e.ResourceID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_event_index (event_type, actor_id, resource_type, resource_id, metadata) VALUES ($1, $2, $3, $4, $5)`,
		e.EventType, actorID, e.ResourceType, resourceID, metaJSON,
	)
	if err != nil {
		log.Printf("audit: failed to log event: %v", err)
	}

	// Also write to append-only file sink (no secrets, tokens, or passwords)
	s.FileSink.Write(map[string]interface{}{
		"event_type":    e.EventType,
		"actor_id":      e.ActorID,
		"resource_type": e.ResourceType,
		"resource_id":   e.ResourceID,
		"metadata":      e.Metadata,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})
}
