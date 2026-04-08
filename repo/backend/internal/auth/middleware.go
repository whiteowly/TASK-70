package auth

import (
	"net/http"

	"fieldserve/internal/audit"
	"fieldserve/internal/platform/httpx"

	"github.com/labstack/echo/v4"
)

// RequireAuth returns middleware that validates the session cookie and sets
// the authenticated user in the echo context.
func (s *Service) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := s.GetSessionToken(c)
			if token == "" {
				return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
			}

			user, err := s.ResolveSession(c.Request().Context(), token)
			if err != nil {
				return err
			}
			if user == nil {
				s.ClearSessionCookie(c)
				return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
			}

			c.Set("auth_user", user)
			return next(c)
		}
	}
}

// RequireRole returns middleware that checks if the authenticated user has at
// least one of the required roles. RequireAuth must run before this middleware.
func RequireRole(auditSvc *audit.Service, roles ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user := UserFromContext(c)
			if user == nil {
				return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
			}

			for _, required := range roles {
				for _, userRole := range user.Roles {
					if userRole == required {
						return next(c)
					}
				}
			}

			// User does not have any of the required roles
			auditSvc.Log(c.Request().Context(), audit.Event{
				EventType:    "privilege_escalation",
				ActorID:      user.ID,
				ResourceType: "auth",
				Metadata: map[string]interface{}{
					"route":          c.Request().Method + " " + c.Request().URL.Path,
					"required_roles": roles,
				},
			})

			return httpx.NewAPIError(http.StatusForbidden, "forbidden", "Insufficient permissions.")
		}
	}
}
