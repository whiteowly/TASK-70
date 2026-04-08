package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"fieldserve/internal/audit"
	"fieldserve/internal/platform/httpx"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName       = "fieldserve_session"
	idleTimeout      = 8 * time.Hour
	absoluteLifetime = 7 * 24 * time.Hour
)

// User represents an authenticated user with their roles.
type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
}

// Service provides authentication and session management.
type Service struct {
	db    *sql.DB
	audit *audit.Service
}

// NewService creates a new auth service.
func NewService(db *sql.DB, audit *audit.Service) *Service {
	return &Service{db: db, audit: audit}
}

// Authenticate verifies username and password. Returns nil (not error) if
// credentials are invalid.
func (s *Service) Authenticate(ctx context.Context, username, password string) (*User, error) {
	var userID, email, passwordHash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash FROM users WHERE username = $1`,
		username,
	).Scan(&userID, &email, &passwordHash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return nil, nil
	}

	roles, err := s.getUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &User{
		ID:       userID,
		Username: username,
		Email:    email,
		Roles:    roles,
	}, nil
}

// CreateSession generates a new session token, stores its SHA-256 hash in the
// database, and returns the raw token.
func (s *Service) CreateSession(ctx context.Context, userID string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("auth: generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO auth_sessions (user_id, token_hash, expires_at, last_active_at) VALUES ($1, $2, NOW() + INTERVAL '7 days', NOW())`,
		userID, tokenHash,
	)
	if err != nil {
		return "", fmt.Errorf("auth: create session: %w", err)
	}

	return token, nil
}

// ResolveSession looks up a session by SHA-256(token), checks idle timeout
// (8h) and absolute lifetime (7d), updates last_active_at, and returns the
// user. Returns nil if expired or invalid.
func (s *Service) ResolveSession(ctx context.Context, token string) (*User, error) {
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	var userID string
	var createdAt, lastActiveAt time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, created_at, last_active_at FROM auth_sessions WHERE token_hash = $1`,
		tokenHash,
	).Scan(&userID, &createdAt, &lastActiveAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: resolve session: %w", err)
	}

	now := time.Now()
	if now.Sub(lastActiveAt) > idleTimeout || now.Sub(createdAt) > absoluteLifetime {
		// Session expired — clean it up
		_, _ = s.db.ExecContext(ctx,
			`DELETE FROM auth_sessions WHERE token_hash = $1`, tokenHash,
		)
		return nil, nil
	}

	// Update last_active_at
	_, _ = s.db.ExecContext(ctx,
		`UPDATE auth_sessions SET last_active_at = NOW() WHERE token_hash = $1`, tokenHash,
	)

	var username, email string
	err = s.db.QueryRowContext(ctx,
		`SELECT username, email FROM users WHERE id = $1`, userID,
	).Scan(&username, &email)
	if err != nil {
		return nil, fmt.Errorf("auth: resolve user: %w", err)
	}

	roles, err := s.getUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &User{
		ID:       userID,
		Username: username,
		Email:    email,
		Roles:    roles,
	}, nil
}

// InvalidateSession deletes a session by its token hash.
func (s *Service) InvalidateSession(ctx context.Context, token string) error {
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM auth_sessions WHERE token_hash = $1`, tokenHash,
	)
	if err != nil {
		return fmt.Errorf("auth: invalidate session: %w", err)
	}
	return nil
}

// InvalidateUserSessions deletes all sessions for a user.
func (s *Service) InvalidateUserSessions(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM auth_sessions WHERE user_id = $1`, userID,
	)
	if err != nil {
		return fmt.Errorf("auth: invalidate user sessions: %w", err)
	}
	return nil
}

// BootstrapAdmin creates the first admin user, role, and profile in a
// transaction. Returns a conflict error if an admin already exists.
func (s *Service) BootstrapAdmin(ctx context.Context, username, password, email string) (*User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Check if an administrator role assignment already exists
	var count int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_roles ur JOIN roles r ON ur.role_id = r.id WHERE r.name = 'administrator'`,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("auth: check admin: %w", err)
	}
	if count > 0 {
		return nil, httpx.NewConflictError("admin_exists", "An administrator already exists.")
	}

	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("auth: hash password: %w", err)
	}

	// Create user
	var userID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (username, password_hash, email) VALUES ($1, $2, $3) RETURNING id`,
		username, string(hashed), email,
	).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("auth: create user: %w", err)
	}

	// Get or create administrator role
	var roleID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM roles WHERE name = 'administrator'`,
	).Scan(&roleID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx,
			`INSERT INTO roles (name, description) VALUES ('administrator', 'Full system access') RETURNING id`,
		).Scan(&roleID)
	}
	if err != nil {
		return nil, fmt.Errorf("auth: get/create role: %w", err)
	}

	// Assign role
	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`,
		userID, roleID,
	)
	if err != nil {
		return nil, fmt.Errorf("auth: assign role: %w", err)
	}

	// Create profile
	_, err = tx.ExecContext(ctx,
		`INSERT INTO admin_profiles (user_id, display_name) VALUES ($1, $2)`,
		userID, username,
	)
	if err != nil {
		return nil, fmt.Errorf("auth: create profile: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("auth: commit: %w", err)
	}

	return &User{
		ID:       userID,
		Username: username,
		Email:    email,
		Roles:    []string{"administrator"},
	}, nil
}

// getUserRoles fetches role names for a user.
func (s *Service) getUserRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.name FROM roles r JOIN user_roles ur ON r.id = ur.role_id WHERE ur.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("auth: query roles: %w", err)
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("auth: scan role: %w", err)
		}
		roles = append(roles, role)
	}
	if roles == nil {
		roles = []string{}
	}
	return roles, rows.Err()
}

// cookieSecure determines whether the Secure flag should be set on cookies.
// It reads the COOKIE_SECURE environment variable:
//   - "true"  → always Secure (for production / TLS termination)
//   - "false" → never Secure (for local HTTP development)
//   - "auto" or unset → Secure when the request arrived over TLS
func cookieSecure(c echo.Context) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv("COOKIE_SECURE")))
	switch val {
	case "true":
		return true
	case "false":
		return false
	default: // "auto" or unset
		return c.Request().TLS != nil ||
			c.Request().Header.Get("X-Forwarded-Proto") == "https"
	}
}

// SetSessionCookie sets the session cookie on the response.
func (s *Service) SetSessionCookie(c echo.Context, token string) {
	c.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   cookieSecure(c),
	})
}

// ClearSessionCookie clears the session cookie.
func (s *Service) ClearSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   cookieSecure(c),
		MaxAge:   -1,
	})
}

// GetSessionToken reads the session token from the cookie.
func (s *Service) GetSessionToken(c echo.Context) string {
	cookie, err := c.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// UserFromContext gets the authenticated user from the echo context.
func UserFromContext(c echo.Context) *User {
	u, ok := c.Get("auth_user").(*User)
	if !ok {
		return nil
	}
	return u
}
