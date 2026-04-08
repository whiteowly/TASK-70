package workorders

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

const (
	MaxUploadSize = 10 << 20 // 10 MB
	EvidenceRoot  = "/app/data/evidence"
	RetentionDays = 180
)

var allowedMIMETypes = map[string]bool{
	"application/pdf":    true,
	"image/jpeg":         true,
	"image/png":          true,
	"image/gif":          true,
	"text/plain":         true,
	"text/csv":           true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
}

var allowedExtensions = map[string]bool{
	".pdf": true, ".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".txt": true, ".csv": true, ".doc": true, ".docx": true,
}

var blockedExtensions = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".sh": true,
	".ps1": true, ".com": true, ".scr": true, ".msi": true,
	".dll": true, ".so": true, ".dylib": true, ".bin": true,
	".js": true, ".vbs": true, ".wsf": true, ".jar": true,
}

// validTransitions defines the allowed status transitions.
var validTransitions = map[string]string{
	"new":                  "dispatched",
	"dispatched":           "acknowledged",
	"acknowledged":         "in_progress",
	"in_progress":          "resolved",
	"resolved":             "post_incident_review",
	"post_incident_review": "closed",
}

// Service manages work orders.
type Service struct {
	db    *sql.DB
	audit *audit.Service
}

// NewService creates a new work orders service.
func NewService(db *sql.DB, audit *audit.Service) *Service {
	return &Service{db: db, audit: audit}
}

// WorkOrder represents a work order record.
type WorkOrder struct {
	ID         string  `json:"id"`
	AlertID    *string `json:"alert_id"`
	Status     string  `json:"status"`
	AssignedTo *string `json:"assigned_to"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// WorkOrderEvent represents a status change event.
type WorkOrderEvent struct {
	ID          string  `json:"id"`
	WorkOrderID string  `json:"work_order_id"`
	OldStatus   *string `json:"old_status"`
	NewStatus   string  `json:"new_status"`
	ActorID     *string `json:"actor_id"`
	CreatedAt   string  `json:"created_at"`
}

// Evidence represents an evidence file attached to a work order.
type Evidence struct {
	ID                 string  `json:"id"`
	WorkOrderID        string  `json:"work_order_id"`
	FilePath           string  `json:"file_path"`
	Checksum           *string `json:"checksum_sha256"`
	UploadedBy         *string `json:"uploaded_by"`
	CreatedAt          string  `json:"created_at"`
	RetentionExpiresAt string  `json:"retention_expires_at"`
}

// HandleCreate handles POST /admin/work-orders.
func (s *Service) HandleCreate(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)

	var req struct {
		AlertID    *string `json:"alert_id"`
		AssignedTo *string `json:"assigned_to"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	woID := uuid.New().String()
	var wo WorkOrder
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO work_orders (id, alert_id, status, assigned_to, created_at, updated_at)
		 VALUES ($1, $2, 'new', $3, NOW(), NOW())
		 RETURNING id, alert_id, status, assigned_to, created_at::text, updated_at::text`,
		woID, req.AlertID, req.AssignedTo,
	).Scan(&wo.ID, &wo.AlertID, &wo.Status, &wo.AssignedTo, &wo.CreatedAt, &wo.UpdatedAt)
	if err != nil {
		return fmt.Errorf("workorders: create: %w", err)
	}

	// Record initial event (nil -> new)
	eventID := uuid.New().String()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO work_order_events (id, work_order_id, old_status, new_status, actor_id, created_at)
		 VALUES ($1, $2, NULL, 'new', $3, NOW())`,
		eventID, woID, user.ID,
	)
	if err != nil {
		return fmt.Errorf("workorders: create event: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "work_order_created",
		ActorID:      user.ID,
		ResourceType: "work_order",
		ResourceID:   wo.ID,
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{"work_order": wo})
}

// HandleList handles GET /admin/work-orders.
func (s *Service) HandleList(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, alert_id, status, assigned_to, created_at::text, updated_at::text
		 FROM work_orders ORDER BY created_at DESC`)
	if err != nil {
		return fmt.Errorf("workorders: list: %w", err)
	}
	defer rows.Close()

	orders := []WorkOrder{}
	for rows.Next() {
		var wo WorkOrder
		if err := rows.Scan(&wo.ID, &wo.AlertID, &wo.Status, &wo.AssignedTo, &wo.CreatedAt, &wo.UpdatedAt); err != nil {
			return fmt.Errorf("workorders: scan: %w", err)
		}
		orders = append(orders, wo)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("workorders: rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"work_orders": orders})
}

// HandleGet handles GET /admin/work-orders/:workOrderId.
func (s *Service) HandleGet(c echo.Context) error {
	ctx := c.Request().Context()
	woID := c.Param("workOrderId")

	var wo WorkOrder
	err := s.db.QueryRowContext(ctx,
		`SELECT id, alert_id, status, assigned_to, created_at::text, updated_at::text
		 FROM work_orders WHERE id = $1`, woID,
	).Scan(&wo.ID, &wo.AlertID, &wo.Status, &wo.AssignedTo, &wo.CreatedAt, &wo.UpdatedAt)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Work order not found.")
	}
	if err != nil {
		return fmt.Errorf("workorders: get: %w", err)
	}

	// Fetch events
	eventRows, err := s.db.QueryContext(ctx,
		`SELECT id, work_order_id, old_status, new_status, actor_id, created_at::text
		 FROM work_order_events WHERE work_order_id = $1 ORDER BY created_at`, woID)
	if err != nil {
		return fmt.Errorf("workorders: list events: %w", err)
	}
	defer eventRows.Close()

	events := []WorkOrderEvent{}
	for eventRows.Next() {
		var ev WorkOrderEvent
		if err := eventRows.Scan(&ev.ID, &ev.WorkOrderID, &ev.OldStatus, &ev.NewStatus, &ev.ActorID, &ev.CreatedAt); err != nil {
			return fmt.Errorf("workorders: scan event: %w", err)
		}
		events = append(events, ev)
	}
	if err := eventRows.Err(); err != nil {
		return fmt.Errorf("workorders: event rows: %w", err)
	}

	// Fetch evidence
	evidenceRows, err := s.db.QueryContext(ctx,
		`SELECT id, work_order_id, file_path, checksum_sha256, uploaded_by, created_at::text, retention_expires_at::text
		 FROM work_order_evidence WHERE work_order_id = $1 ORDER BY created_at`, woID)
	if err != nil {
		return fmt.Errorf("workorders: list evidence: %w", err)
	}
	defer evidenceRows.Close()

	evidenceList := []Evidence{}
	for evidenceRows.Next() {
		var ev Evidence
		if err := evidenceRows.Scan(&ev.ID, &ev.WorkOrderID, &ev.FilePath, &ev.Checksum, &ev.UploadedBy, &ev.CreatedAt, &ev.RetentionExpiresAt); err != nil {
			return fmt.Errorf("workorders: scan evidence: %w", err)
		}
		evidenceList = append(evidenceList, ev)
	}
	if err := evidenceRows.Err(); err != nil {
		return fmt.Errorf("workorders: evidence rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"work_order": wo,
		"events":     events,
		"evidence":   evidenceList,
	})
}

// transition performs a status transition on a work order.
func (s *Service) transition(c echo.Context, expectedOld, newStatus string) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	woID := c.Param("workOrderId")

	// Get current status
	var currentStatus string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM work_orders WHERE id = $1`, woID,
	).Scan(&currentStatus)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Work order not found.")
	}
	if err != nil {
		return fmt.Errorf("workorders: get status: %w", err)
	}

	// Validate transition
	allowed, ok := validTransitions[currentStatus]
	if !ok || allowed != newStatus {
		return httpx.NewAPIError(http.StatusUnprocessableEntity, "invalid_transition",
			fmt.Sprintf("Cannot transition from '%s' to '%s'.", currentStatus, newStatus))
	}

	// For dispatch, optionally set assigned_to
	var assignedTo *string
	if newStatus == "dispatched" {
		var req struct {
			AssignedTo *string `json:"assigned_to"`
		}
		// Bind body but ignore errors (body may be empty)
		_ = json.NewDecoder(c.Request().Body).Decode(&req)
		if req.AssignedTo != nil {
			assignedTo = req.AssignedTo
		}
	}

	// Update work order
	var wo WorkOrder
	if assignedTo != nil {
		err = s.db.QueryRowContext(ctx,
			`UPDATE work_orders SET status = $1, assigned_to = $2, updated_at = NOW()
			 WHERE id = $3
			 RETURNING id, alert_id, status, assigned_to, created_at::text, updated_at::text`,
			newStatus, assignedTo, woID,
		).Scan(&wo.ID, &wo.AlertID, &wo.Status, &wo.AssignedTo, &wo.CreatedAt, &wo.UpdatedAt)
	} else {
		err = s.db.QueryRowContext(ctx,
			`UPDATE work_orders SET status = $1, updated_at = NOW()
			 WHERE id = $2
			 RETURNING id, alert_id, status, assigned_to, created_at::text, updated_at::text`,
			newStatus, woID,
		).Scan(&wo.ID, &wo.AlertID, &wo.Status, &wo.AssignedTo, &wo.CreatedAt, &wo.UpdatedAt)
	}
	if err != nil {
		return fmt.Errorf("workorders: update status: %w", err)
	}

	// Record event
	eventID := uuid.New().String()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO work_order_events (id, work_order_id, old_status, new_status, actor_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())`,
		eventID, woID, currentStatus, newStatus, user.ID,
	)
	if err != nil {
		return fmt.Errorf("workorders: create event: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "work_order_" + newStatus,
		ActorID:      user.ID,
		ResourceType: "work_order",
		ResourceID:   wo.ID,
		Metadata:     map[string]interface{}{"old_status": currentStatus, "new_status": newStatus},
	})

	return c.JSON(http.StatusOK, map[string]interface{}{"work_order": wo})
}

// HandleDispatch handles POST /admin/work-orders/:workOrderId/dispatch.
func (s *Service) HandleDispatch(c echo.Context) error {
	return s.transition(c, "new", "dispatched")
}

// HandleAcknowledge handles POST /admin/work-orders/:workOrderId/acknowledge.
func (s *Service) HandleAcknowledge(c echo.Context) error {
	return s.transition(c, "dispatched", "acknowledged")
}

// HandleStart handles POST /admin/work-orders/:workOrderId/start.
func (s *Service) HandleStart(c echo.Context) error {
	return s.transition(c, "acknowledged", "in_progress")
}

// HandleResolve handles POST /admin/work-orders/:workOrderId/resolve.
func (s *Service) HandleResolve(c echo.Context) error {
	return s.transition(c, "in_progress", "resolved")
}

// HandlePostIncidentReview handles POST /admin/work-orders/:workOrderId/post-incident-review.
func (s *Service) HandlePostIncidentReview(c echo.Context) error {
	return s.transition(c, "resolved", "post_incident_review")
}

// HandleClose handles POST /admin/work-orders/:workOrderId/close.
func (s *Service) HandleClose(c echo.Context) error {
	return s.transition(c, "post_incident_review", "closed")
}

// HandleUploadEvidence handles POST /admin/work-orders/:workOrderId/evidence.
func (s *Service) HandleUploadEvidence(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	woID := c.Param("workOrderId")

	// Check work order exists
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_orders WHERE id = $1)`, woID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("workorders: check wo: %w", err)
	}
	if !exists {
		return httpx.NewNotFoundError("Work order not found.")
	}

	// Limit request body
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, MaxUploadSize+1)

	file, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			return httpx.NewAPIError(http.StatusRequestEntityTooLarge, "file_too_large", "File too large. Maximum size is 10 MB.")
		}
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "No file provided.")
	}

	if file.Size > MaxUploadSize {
		return httpx.NewAPIError(http.StatusRequestEntityTooLarge, "file_too_large", "File too large. Maximum size is 10 MB.")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if blockedExtensions[ext] {
		return httpx.NewAPIError(http.StatusUnsupportedMediaType, "invalid_file_type", "File type not allowed.")
	}
	if !allowedExtensions[ext] {
		return httpx.NewAPIError(http.StatusUnsupportedMediaType, "invalid_file_type", "File type not allowed. Allowed: .pdf, .jpg, .jpeg, .png, .gif, .txt, .csv, .doc, .docx")
	}

	// Open file for reading
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("workorders: open file: %w", err)
	}
	defer src.Close()

	// MIME sniff first 512 bytes
	sniffBuf := make([]byte, 512)
	n, _ := src.Read(sniffBuf)
	detectedMIME := http.DetectContentType(sniffBuf[:n])
	if idx := strings.Index(detectedMIME, ";"); idx > 0 {
		detectedMIME = strings.TrimSpace(detectedMIME[:idx])
	}
	if !allowedMIMETypes[detectedMIME] {
		return httpx.NewAPIError(http.StatusUnsupportedMediaType, "invalid_file_type", "File type not allowed.")
	}

	// Seek back to start
	src.Seek(0, io.SeekStart)

	// Read full file and compute checksum
	hasher := sha256.New()
	fileBytes, err := io.ReadAll(io.TeeReader(src, hasher))
	if err != nil {
		return fmt.Errorf("workorders: read file: %w", err)
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Generate safe storage path
	storageFilename := uuid.New().String() + ext
	storagePath := filepath.Join(EvidenceRoot, storageFilename)
	absPath, _ := filepath.Abs(storagePath)
	absRoot, _ := filepath.Abs(EvidenceRoot)
	if !strings.HasPrefix(absPath, absRoot) {
		return httpx.NewAPIError(http.StatusBadRequest, "invalid_path", "Invalid storage path.")
	}

	// Ensure directory exists
	os.MkdirAll(EvidenceRoot, 0755)

	// Write file
	if err := os.WriteFile(storagePath, fileBytes, 0644); err != nil {
		return fmt.Errorf("workorders: write file: %w", err)
	}

	retentionExpires := time.Now().AddDate(0, 0, RetentionDays)

	var ev Evidence
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO work_order_evidence (id, work_order_id, file_path, checksum_sha256, uploaded_by, created_at, retention_expires_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), $6)
		 RETURNING id, work_order_id, file_path, checksum_sha256, uploaded_by, created_at::text, retention_expires_at::text`,
		uuid.New().String(), woID, storagePath, checksum, user.ID, retentionExpires,
	).Scan(&ev.ID, &ev.WorkOrderID, &ev.FilePath, &ev.Checksum, &ev.UploadedBy, &ev.CreatedAt, &ev.RetentionExpiresAt)
	if err != nil {
		return fmt.Errorf("workorders: insert evidence: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "evidence_uploaded",
		ActorID:      user.ID,
		ResourceType: "work_order_evidence",
		ResourceID:   ev.ID,
		Metadata:     map[string]interface{}{"work_order_id": woID},
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{"evidence": ev})
}

// HandleListEvidence handles GET /admin/work-orders/:workOrderId/evidence.
func (s *Service) HandleListEvidence(c echo.Context) error {
	woID := c.Param("workOrderId")

	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, work_order_id, file_path, checksum_sha256, uploaded_by, created_at::text, retention_expires_at::text
		 FROM work_order_evidence WHERE work_order_id = $1 ORDER BY created_at`, woID)
	if err != nil {
		return fmt.Errorf("workorders: list evidence: %w", err)
	}
	defer rows.Close()

	evidenceList := []Evidence{}
	for rows.Next() {
		var ev Evidence
		if err := rows.Scan(&ev.ID, &ev.WorkOrderID, &ev.FilePath, &ev.Checksum, &ev.UploadedBy, &ev.CreatedAt, &ev.RetentionExpiresAt); err != nil {
			return fmt.Errorf("workorders: scan evidence: %w", err)
		}
		evidenceList = append(evidenceList, ev)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("workorders: evidence rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"evidence": evidenceList})
}
