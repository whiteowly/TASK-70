package httpx

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/labstack/echo/v4"
)

// APIAccessAudit returns middleware that logs every authenticated API request
// into the audit_event_index table. It runs after the handler completes and
// only fires when auth_user is set on the context.
func APIAccessAudit(db *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			err := next(c)

			// Only log for authenticated requests
			userVal := c.Get("auth_user")
			if userVal == nil {
				return err
			}
			userID := extractUserID(userVal)
			if userID == "" {
				return err
			}

			// Use only the path, never query params (may contain sensitive values)
			path := c.Request().URL.Path

			// Don't log health checks
			if strings.Contains(path, "/system/health") {
				return err
			}

			reqID := c.Response().Header().Get(echo.HeaderXRequestID)
			status := c.Response().Status

			metadata := map[string]interface{}{
				"request_id": reqID,
				"method":     c.Request().Method,
				"path":       path,
				"status":     status,
			}
			metaJSON, _ := json.Marshal(metadata)

			go func() {
				db.Exec(
					`INSERT INTO audit_event_index (event_type, actor_id, resource_type, metadata) VALUES ($1, $2, $3, $4)`,
					"api_access", userID, "api", metaJSON)
			}()

			return err
		}
	}
}

// extractUserID is defined in idempotency.go — shared across this package.
