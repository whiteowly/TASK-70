package httpx

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/labstack/echo/v4"
)

// IdempotencyCheck returns middleware that provides idempotent request handling.
// If an Idempotency-Key header is present, the middleware will replay a cached
// response for duplicate keys within a 5-minute window.
//
// The key hash is scoped by user ID + HTTP method + request path so that:
//   - the same key used by different users does not collide
//   - the same key reused on a different endpoint does not replay the wrong response
//
// Concurrency safety: the middleware uses INSERT ... RETURNING with a unique
// constraint to atomically claim ownership of a key. If the INSERT returns no
// rows (conflict), another request owns the key; this request waits for the
// owner to store the result, then replays it.
func IdempotencyCheck(db *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := c.Request().Header.Get("Idempotency-Key")
			if key == "" {
				return next(c)
			}

			userVal := c.Get("auth_user")
			if userVal == nil {
				return next(c)
			}

			// Extract user ID without importing auth (to avoid circular deps).
			userID := extractUserID(userVal)
			if userID == "" {
				return next(c)
			}

			// Scope the hash by user + method + path + key so collisions
			// across users or endpoints are impossible.
			method := c.Request().Method
			path := c.Path() // Echo route pattern e.g. "/api/v1/customer/interests"
			composite := fmt.Sprintf("%s:%s:%s:%s", userID, method, path, key)
			h := sha256.Sum256([]byte(composite))
			keyHash := hex.EncodeToString(h[:])

			// Attempt to atomically claim this key. The INSERT succeeds only
			// for the first request; concurrent duplicates hit the UNIQUE
			// constraint and get no rows back.
			var claimedID string
			err := db.QueryRowContext(c.Request().Context(),
				`INSERT INTO idempotency_keys (key_hash, user_id, response_status, response_body, expires_at)
				 VALUES ($1, $2, 0, NULL, NOW() + INTERVAL '5 minutes')
				 ON CONFLICT (key_hash) DO NOTHING
				 RETURNING id`,
				keyHash, userID,
			).Scan(&claimedID)

			if err == sql.ErrNoRows {
				// Another request owns this key. Wait for it to complete
				// and replay its response.
				return waitAndReplay(c, db, keyHash, userID)
			}
			if err != nil {
				// Unexpected DB error — let the request through rather than block.
				return next(c)
			}

			// We own the key. Execute the handler and capture the response.
			origWriter := c.Response().Writer
			capture := &bodyCapture{ResponseWriter: origWriter}
			c.Response().Writer = capture

			handlerErr := next(c)

			// Store the actual response for replay by waiting requests.
			respBody := capture.body.String()
			_, _ = db.ExecContext(c.Request().Context(),
				`UPDATE idempotency_keys SET response_status=$1, response_body=$2 WHERE key_hash=$3 AND user_id=$4`,
				c.Response().Status, respBody, keyHash, userID,
			)

			return handlerErr
		}
	}
}

// waitAndReplay polls for the owning request to finish and replays its response.
// It polls up to 50 times with 100ms intervals (5 seconds total).
func waitAndReplay(c echo.Context, db *sql.DB, keyHash, userID string) error {
	ctx := c.Request().Context()
	for i := 0; i < 50; i++ {
		var respStatus int
		var respBody sql.NullString
		err := db.QueryRowContext(ctx,
			`SELECT response_status, response_body FROM idempotency_keys WHERE key_hash=$1 AND user_id=$2 AND expires_at > NOW()`,
			keyHash, userID,
		).Scan(&respStatus, &respBody)

		if err == sql.ErrNoRows {
			// Key expired or was cleaned up; let the request fail gracefully.
			return NewAPIError(http.StatusConflict, "idempotency_conflict",
				"A concurrent request with this idempotency key is in progress.")
		}
		if err != nil {
			return NewAPIError(http.StatusInternalServerError, "internal_error",
				"Failed to check idempotency state.")
		}

		// response_status == 0 means the owner hasn't finished yet.
		if respStatus > 0 && respBody.Valid {
			c.Response().Header().Set("Content-Type", "application/json")
			return c.JSONBlob(respStatus, []byte(respBody.String))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	return NewAPIError(http.StatusConflict, "idempotency_timeout",
		"Timed out waiting for the original request to complete.")
}

// bodyCapture wraps an http.ResponseWriter to capture the response body.
type bodyCapture struct {
	http.ResponseWriter
	body bytes.Buffer
}

func (bc *bodyCapture) Write(b []byte) (int, error) {
	bc.body.Write(b)
	return bc.ResponseWriter.Write(b)
}

// extractUserID reads the ID field from an auth.User struct pointer using
// reflection, avoiding a direct import of the auth package (which would
// create a circular dependency since auth imports httpx).
func extractUserID(v interface{}) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ""
	}
	f := rv.FieldByName("ID")
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}
