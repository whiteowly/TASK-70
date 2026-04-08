package messages

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"fieldserve/internal/audit"
	"fieldserve/internal/auth"
	"fieldserve/internal/blocks"
	"fieldserve/internal/platform/httpx"

	"github.com/labstack/echo/v4"
)

// Thread represents a message thread summary.
type Thread struct {
	ThreadID    string `json:"thread_id"`
	OtherUserID string `json:"other_user_id"`
	OtherName   string `json:"other_name"`
	LastMessage string `json:"last_message"`
	LastAt      string `json:"last_at"`
	UnreadCount int    `json:"unread_count"`
}

// Message represents a single message in a thread.
type Message struct {
	ID          string `json:"id"`
	ThreadID    string `json:"thread_id"`
	SenderID    string `json:"sender_id"`
	RecipientID string `json:"recipient_id"`
	Body        string `json:"body"`
	CreatedAt   string `json:"created_at"`
	ReadStatus  string `json:"read_status"`
}

// Service provides messaging operations.
type Service struct {
	db     *sql.DB
	audit  *audit.Service
	blocks *blocks.Service
}

// NewService creates a new messages service.
func NewService(db *sql.DB, audit *audit.Service, blocks *blocks.Service) *Service {
	return &Service{db: db, audit: audit, blocks: blocks}
}

// ListThreads returns all message threads for a user.
func (s *Service) ListThreads(ctx context.Context, userID string) ([]Thread, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT sub.thread_id, sub.other_user_id, u.username AS other_name,
		        sub.last_body, sub.last_at, sub.unread_count
		 FROM (
		     SELECT m.thread_id,
		            CASE WHEN m.sender_id = $1 THEN m.recipient_id ELSE m.sender_id END AS other_user_id,
		            (SELECT body FROM messages WHERE thread_id = m.thread_id ORDER BY created_at DESC LIMIT 1) AS last_body,
		            (SELECT created_at::text FROM messages WHERE thread_id = m.thread_id ORDER BY created_at DESC LIMIT 1) AS last_at,
		            (SELECT COUNT(*) FROM messages msg
		             JOIN message_receipts mr ON mr.message_id = msg.id AND mr.user_id = $1
		             WHERE msg.thread_id = m.thread_id AND msg.sender_id != $1 AND mr.status != 'read') AS unread_count
		     FROM messages m
		     WHERE m.sender_id = $1 OR m.recipient_id = $1
		     GROUP BY m.thread_id, other_user_id
		 ) sub
		 JOIN users u ON u.id = sub.other_user_id
		 ORDER BY sub.last_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("messages: list threads: %w", err)
	}
	defer rows.Close()

	threads := make([]Thread, 0)
	for rows.Next() {
		var t Thread
		if err := rows.Scan(&t.ThreadID, &t.OtherUserID, &t.OtherName, &t.LastMessage, &t.LastAt, &t.UnreadCount); err != nil {
			return nil, fmt.Errorf("messages: scan thread: %w", err)
		}
		threads = append(threads, t)
	}
	return threads, rows.Err()
}

// GetThread returns all messages in a thread, verifying the user is a participant.
func (s *Service) GetThread(ctx context.Context, threadID, userID string) ([]Message, error) {
	// Verify user is participant
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE thread_id=$1 AND (sender_id=$2 OR recipient_id=$2)`,
		threadID, userID,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("messages: verify participant: %w", err)
	}
	if count == 0 {
		return nil, httpx.NewNotFoundError("Thread not found.")
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.thread_id, m.sender_id, m.recipient_id, m.body, m.created_at::text,
		        COALESCE(mr.status, 'sent') AS read_status
		 FROM messages m
		 LEFT JOIN message_receipts mr ON mr.message_id = m.id AND mr.user_id = $2
		 WHERE m.thread_id = $1
		 ORDER BY m.created_at ASC`,
		threadID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("messages: get thread: %w", err)
	}
	defer rows.Close()

	messages := make([]Message, 0)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.SenderID, &m.RecipientID, &m.Body, &m.CreatedAt, &m.ReadStatus); err != nil {
			return nil, fmt.Errorf("messages: scan message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// SendMessage sends a message in a thread (thread_id = interest.id).
func (s *Service) SendMessage(ctx context.Context, threadID, senderID, body string) (*Message, error) {
	// Thread = interest. Look up interest to get participants.
	var customerProfileID, providerProfileID string
	err := s.db.QueryRowContext(ctx,
		`SELECT customer_id, provider_id FROM interests WHERE id=$1`,
		threadID,
	).Scan(&customerProfileID, &providerProfileID)
	if err == sql.ErrNoRows {
		return nil, httpx.NewNotFoundError("Thread not found.")
	}
	if err != nil {
		return nil, fmt.Errorf("messages: lookup interest: %w", err)
	}

	// Resolve user IDs from profile IDs
	var customerUserID, providerUserID string
	err = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM customer_profiles WHERE id=$1`, customerProfileID,
	).Scan(&customerUserID)
	if err != nil {
		return nil, fmt.Errorf("messages: resolve customer user: %w", err)
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM provider_profiles WHERE id=$1`, providerProfileID,
	).Scan(&providerUserID)
	if err != nil {
		return nil, fmt.Errorf("messages: resolve provider user: %w", err)
	}

	// Verify sender is a participant
	var recipientID string
	if senderID == customerUserID {
		recipientID = providerUserID
	} else if senderID == providerUserID {
		recipientID = customerUserID
	} else {
		return nil, httpx.NewNotFoundError("Thread not found.")
	}

	// Check blocks
	blocked, err := s.blocks.IsBlocked(ctx, senderID, recipientID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, httpx.NewForbiddenError("Cannot send messages to a blocked user.")
	}

	// Insert message
	var msg Message
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO messages (thread_id, sender_id, recipient_id, body) VALUES ($1, $2, $3, $4) RETURNING id, created_at::text`,
		threadID, senderID, recipientID, body,
	).Scan(&msg.ID, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("messages: insert: %w", err)
	}
	msg.ThreadID = threadID
	msg.SenderID = senderID
	msg.RecipientID = recipientID
	msg.Body = body
	msg.ReadStatus = "sent"

	// Insert receipt for recipient
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO message_receipts (message_id, user_id, status) VALUES ($1, $2, 'sent')`,
		msg.ID, recipientID,
	)
	if err != nil {
		return nil, fmt.Errorf("messages: insert receipt: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "message_sent",
		ActorID:      senderID,
		ResourceType: "message",
		ResourceID:   msg.ID,
		Metadata: map[string]interface{}{
			"thread_id":    threadID,
			"recipient_id": recipientID,
		},
	})

	return &msg, nil
}

// MarkRead marks all unread messages in a thread as read for the given user.
func (s *Service) MarkRead(ctx context.Context, threadID, userID string) error {
	// Verify user is participant
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE thread_id=$1 AND (sender_id=$2 OR recipient_id=$2)`,
		threadID, userID,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("messages: verify participant: %w", err)
	}
	if count == 0 {
		return httpx.NewNotFoundError("Thread not found.")
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE message_receipts SET status='read', updated_at=NOW()
		 WHERE message_id IN (SELECT id FROM messages WHERE thread_id=$1 AND sender_id != $2)
		 AND user_id=$2 AND status != 'read'`,
		threadID, userID,
	)
	if err != nil {
		return fmt.Errorf("messages: mark read: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "message_read",
		ActorID:      userID,
		ResourceType: "thread",
		ResourceID:   threadID,
	})

	return nil
}

// ---------- HTTP Handlers ----------

// HandleListThreads handles GET /messages.
func (s *Service) HandleListThreads(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	threads, err := s.ListThreads(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"threads": threads})
}

// HandleGetThread handles GET /messages/:threadId.
func (s *Service) HandleGetThread(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	messages, err := s.GetThread(c.Request().Context(), c.Param("threadId"), user.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"messages": messages})
}

// HandleSendMessage handles POST /messages/:threadId.
func (s *Service) HandleSendMessage(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	var body struct {
		Body string `json:"body"`
	}
	if err := c.Bind(&body); err != nil {
		return httpx.NewValidationError(map[string][]string{"body": {"Invalid request body."}})
	}
	if body.Body == "" {
		return httpx.NewValidationError(map[string][]string{"body": {"Message body is required."}})
	}

	msg, err := s.SendMessage(c.Request().Context(), c.Param("threadId"), user.ID, body.Body)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"message": msg})
}

// HandleMarkRead handles POST /messages/:threadId/read.
func (s *Service) HandleMarkRead(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	if err := s.MarkRead(c.Request().Context(), c.Param("threadId"), user.ID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"message": "Messages marked as read."})
}
