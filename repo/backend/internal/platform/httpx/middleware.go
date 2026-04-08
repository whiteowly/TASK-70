package httpx

import (
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// RequestID returns middleware that sets a unique X-Request-Id header on every
// request. If the client already provides one it is preserved.
func RequestID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rid := c.Request().Header.Get(echo.HeaderXRequestID)
			if rid == "" {
				rid = uuid.New().String()
			}
			c.Request().Header.Set(echo.HeaderXRequestID, rid)
			c.Response().Header().Set(echo.HeaderXRequestID, rid)
			return next(c)
		}
	}
}

// RequestLogger returns middleware that emits a structured JSON log line for
// every completed request. This is a minimal placeholder; swap in zerolog or
// slog for production use.
func RequestLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)
			if err != nil {
				c.Error(err)
			}

			entry := map[string]interface{}{
				"method":     c.Request().Method,
				"path":       c.Request().URL.Path,
				"status":     c.Response().Status,
				"latency_ms": time.Since(start).Milliseconds(),
				"request_id": c.Response().Header().Get(echo.HeaderXRequestID),
			}

			line, _ := json.Marshal(entry)
			log.Println(string(line))

			return nil
		}
	}
}
