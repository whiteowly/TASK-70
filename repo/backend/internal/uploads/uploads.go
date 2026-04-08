package uploads

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"fieldserve/internal/audit"
	"fieldserve/internal/auth"
	"fieldserve/internal/platform/httpx"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const (
	MaxUploadSize = 10 << 20 // 10 MB
	UploadsRoot   = "/app/data/uploads"
)

// AllowedExtensions is the explicit set of accepted file extensions for
// provider document uploads. Both the extension AND the detected MIME type
// must match for an upload to be accepted.
var AllowedExtensions = map[string]bool{
	".pdf":  true,
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".txt":  true,
	".csv":  true,
	".doc":  true,
	".docx": true,
}

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

var blockedExtensions = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".sh": true,
	".ps1": true, ".com": true, ".scr": true, ".msi": true,
	".dll": true, ".so": true, ".dylib": true, ".bin": true,
	".js": true, ".vbs": true, ".wsf": true, ".jar": true,
}

// Service manages provider document uploads.
type Service struct {
	db    *sql.DB
	audit *audit.Service
}

// NewService creates a new uploads service.
func NewService(db *sql.DB, audit *audit.Service) *Service {
	return &Service{db: db, audit: audit}
}

// Document represents a provider document record.
type Document struct {
	ID          string `json:"id"`
	ProviderID  string `json:"provider_id"`
	Filename    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	SizeBytes   int    `json:"size_bytes"`
	Checksum    string `json:"checksum_sha256"`
	StoragePath string `json:"storage_path"`
	CreatedAt   string `json:"created_at"`
}

func getProviderProfileID(db *sql.DB, ctx context.Context, userID string) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM provider_profiles WHERE user_id = $1`, userID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", httpx.NewNotFoundError("Provider profile not found.")
	}
	return id, err
}

// HandleUpload handles POST /provider/documents.
func (s *Service) HandleUpload(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	providerID, err := getProviderProfileID(s.db, ctx, user.ID)
	if err != nil {
		return err
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

	// Check extension: must not be in denylist AND must be in allowlist
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if blockedExtensions[ext] {
		return httpx.NewAPIError(http.StatusUnsupportedMediaType, "invalid_file_type", "File type not allowed.")
	}
	if !AllowedExtensions[ext] {
		return httpx.NewAPIError(http.StatusUnsupportedMediaType, "invalid_file_type", "File type not allowed. Allowed: .pdf, .jpg, .jpeg, .png, .gif, .txt, .csv, .doc, .docx")
	}

	// Open file for reading
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("uploads: open file: %w", err)
	}
	defer src.Close()

	// MIME sniff first 512 bytes
	sniffBuf := make([]byte, 512)
	n, _ := src.Read(sniffBuf)
	detectedMIME := http.DetectContentType(sniffBuf[:n])
	// Normalize: DetectContentType may add ";charset=..." - strip it
	if idx := strings.Index(detectedMIME, ";"); idx > 0 {
		detectedMIME = strings.TrimSpace(detectedMIME[:idx])
	}
	if !allowedMIMETypes[detectedMIME] {
		s.audit.Log(ctx, audit.Event{
			EventType:    "document_rejected",
			ActorID:      user.ID,
			ResourceType: "document",
			Metadata:     map[string]interface{}{"reason": "invalid_mime", "detected": detectedMIME},
		})
		return httpx.NewAPIError(http.StatusUnsupportedMediaType, "invalid_file_type", "File type not allowed.")
	}

	// Seek back to start
	src.Seek(0, io.SeekStart)

	// Compute SHA-256 and read full file
	hasher := sha256.New()
	fileBytes, err := io.ReadAll(io.TeeReader(src, hasher))
	if err != nil {
		return fmt.Errorf("uploads: read file: %w", err)
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Generate safe storage path
	storageFilename := newUUID() + ext
	storagePath := filepath.Join(UploadsRoot, storageFilename)
	// Verify path is under uploads root
	absPath, _ := filepath.Abs(storagePath)
	absRoot, _ := filepath.Abs(UploadsRoot)
	if !strings.HasPrefix(absPath, absRoot) {
		return httpx.NewAPIError(http.StatusBadRequest, "invalid_path", "Invalid storage path.")
	}

	// Ensure directory exists
	os.MkdirAll(UploadsRoot, 0755)

	// Write file
	if err := os.WriteFile(storagePath, fileBytes, 0644); err != nil {
		return fmt.Errorf("uploads: write file: %w", err)
	}

	// Insert metadata
	var doc Document
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO provider_documents (provider_id, filename, mime_type, size_bytes, checksum_sha256, storage_path)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, provider_id, filename, mime_type, size_bytes, checksum_sha256, storage_path, created_at::text`,
		providerID, file.Filename, detectedMIME, len(fileBytes), checksum, storagePath,
	).Scan(&doc.ID, &doc.ProviderID, &doc.Filename, &doc.MimeType, &doc.SizeBytes, &doc.Checksum, &doc.StoragePath, &doc.CreatedAt)
	if err != nil {
		return fmt.Errorf("uploads: insert: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "document_uploaded",
		ActorID:      user.ID,
		ResourceType: "document",
		ResourceID:   doc.ID,
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{"document": doc})
}

// HandleList handles GET /provider/documents.
func (s *Service) HandleList(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	providerID, err := getProviderProfileID(s.db, ctx, user.ID)
	if err != nil {
		return err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, provider_id, filename, mime_type, size_bytes, checksum_sha256, storage_path, created_at::text
		 FROM provider_documents WHERE provider_id = $1 ORDER BY created_at DESC`, providerID)
	if err != nil {
		return fmt.Errorf("uploads: list: %w", err)
	}
	defer rows.Close()

	docs := []Document{}
	for rows.Next() {
		var doc Document
		if err := rows.Scan(&doc.ID, &doc.ProviderID, &doc.Filename, &doc.MimeType, &doc.SizeBytes, &doc.Checksum, &doc.StoragePath, &doc.CreatedAt); err != nil {
			return fmt.Errorf("uploads: scan: %w", err)
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("uploads: rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"documents": docs})
}

// HandleDelete handles DELETE /provider/documents/:documentId.
func (s *Service) HandleDelete(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	providerID, err := getProviderProfileID(s.db, ctx, user.ID)
	if err != nil {
		return err
	}

	documentID := c.Param("documentId")

	// Verify ownership
	var storagePath string
	err = s.db.QueryRowContext(ctx,
		`SELECT storage_path FROM provider_documents WHERE id = $1 AND provider_id = $2`,
		documentID, providerID).Scan(&storagePath)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Document not found.")
	}
	if err != nil {
		return fmt.Errorf("uploads: query: %w", err)
	}

	// Delete from DB
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM provider_documents WHERE id = $1 AND provider_id = $2`,
		documentID, providerID)
	if err != nil {
		return fmt.Errorf("uploads: delete: %w", err)
	}

	// Delete file from disk (best effort)
	os.Remove(storagePath)

	s.audit.Log(ctx, audit.Event{
		EventType:    "document_deleted",
		ActorID:      user.ID,
		ResourceType: "document",
		ResourceID:   documentID,
	})

	return c.JSON(http.StatusOK, map[string]interface{}{"message": "Document deleted."})
}

// VerifyChecksum recomputes the SHA-256 checksum of a stored file and compares
// it against the recorded checksum. Returns nil on match, an error describing
// the mismatch otherwise.
func (s *Service) VerifyChecksum(ctx context.Context, documentID string) error {
	var storedChecksum, storagePath string
	err := s.db.QueryRowContext(ctx,
		`SELECT checksum_sha256, storage_path FROM provider_documents WHERE id = $1`, documentID,
	).Scan(&storedChecksum, &storagePath)
	if err == sql.ErrNoRows {
		return fmt.Errorf("verify checksum: document not found")
	}
	if err != nil {
		return fmt.Errorf("verify checksum: query: %w", err)
	}

	f, err := os.Open(storagePath)
	if err != nil {
		return fmt.Errorf("verify checksum: open file: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("verify checksum: read file: %w", err)
	}
	computed := hex.EncodeToString(hasher.Sum(nil))

	if computed != storedChecksum {
		s.audit.Log(ctx, audit.Event{
			EventType:    "checksum_mismatch",
			ResourceType: "document",
			ResourceID:   documentID,
			Metadata:     map[string]interface{}{"expected": storedChecksum, "computed": computed},
		})
		return fmt.Errorf("verify checksum: mismatch for document %s: expected %s, got %s", documentID, storedChecksum, computed)
	}

	return nil
}

// VerifyEvidenceChecksum recomputes the SHA-256 checksum of a stored evidence
// file and compares it against the recorded checksum.
func VerifyEvidenceChecksum(ctx context.Context, db *sql.DB, auditSvc *audit.Service, evidenceID string) error {
	var storedChecksum sql.NullString
	var filePath string
	err := db.QueryRowContext(ctx,
		`SELECT checksum_sha256, file_path FROM work_order_evidence WHERE id = $1`, evidenceID,
	).Scan(&storedChecksum, &filePath)
	if err == sql.ErrNoRows {
		return fmt.Errorf("verify evidence checksum: evidence not found")
	}
	if err != nil {
		return fmt.Errorf("verify evidence checksum: query: %w", err)
	}
	if !storedChecksum.Valid || storedChecksum.String == "" {
		return fmt.Errorf("verify evidence checksum: no checksum recorded for evidence %s", evidenceID)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("verify evidence checksum: open file: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("verify evidence checksum: read file: %w", err)
	}
	computed := hex.EncodeToString(hasher.Sum(nil))

	if computed != storedChecksum.String {
		if auditSvc != nil {
			auditSvc.Log(ctx, audit.Event{
				EventType:    "evidence_checksum_mismatch",
				ResourceType: "work_order_evidence",
				ResourceID:   evidenceID,
				Metadata:     map[string]interface{}{"expected": storedChecksum.String, "computed": computed},
			})
		}
		return fmt.Errorf("verify evidence checksum: mismatch for evidence %s: expected %s, got %s", evidenceID, storedChecksum.String, computed)
	}

	return nil
}

// newUUID generates a new UUID string.
func newUUID() string {
	return uuid.New().String()
}
