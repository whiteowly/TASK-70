package auth

import (
	"net/http"

	"fieldserve/internal/audit"
	"fieldserve/internal/platform/httpx"

	"github.com/labstack/echo/v4"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type bootstrapRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

type userResponse struct {
	User *User `json:"user"`
}

// HandleLogin handles POST /auth/login.
func (s *Service) HandleLogin(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	if req.Username == "" || req.Password == "" {
		return httpx.NewValidationError(map[string][]string{
			"username": {"Username is required."},
			"password": {"Password is required."},
		})
	}

	user, err := s.Authenticate(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		return err
	}
	if user == nil {
		s.audit.Log(c.Request().Context(), audit.Event{
			EventType:    "failed_login",
			ResourceType: "auth",
			Metadata:     map[string]interface{}{"username": req.Username},
		})
		return httpx.NewAPIError(http.StatusUnauthorized, "invalid_credentials", "Invalid username or password.")
	}

	token, err := s.CreateSession(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	s.SetSessionCookie(c, token)

	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "login",
		ActorID:      user.ID,
		ResourceType: "auth",
	})

	return c.JSON(http.StatusOK, userResponse{User: user})
}

// HandleLogout handles POST /auth/logout.
func (s *Service) HandleLogout(c echo.Context) error {
	token := s.GetSessionToken(c)
	if token != "" {
		_ = s.InvalidateSession(c.Request().Context(), token)
	}

	s.ClearSessionCookie(c)

	user := UserFromContext(c)
	if user != nil {
		s.audit.Log(c.Request().Context(), audit.Event{
			EventType:    "logout",
			ActorID:      user.ID,
			ResourceType: "auth",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Logged out."})
}

// HandleMe handles GET /auth/me.
func (s *Service) HandleMe(c echo.Context) error {
	user := UserFromContext(c)
	return c.JSON(http.StatusOK, userResponse{User: user})
}

// HandleBootstrapAdmin handles POST /auth/bootstrap-admin.
func (s *Service) HandleBootstrapAdmin(c echo.Context) error {
	var req bootstrapRequest
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	fieldErrors := map[string][]string{}
	if req.Username == "" {
		fieldErrors["username"] = []string{"Username is required."}
	}
	if req.Password == "" {
		fieldErrors["password"] = []string{"Password is required."}
	} else if len(req.Password) < 8 {
		fieldErrors["password"] = []string{"Password must be at least 8 characters."}
	}
	if req.Email == "" {
		fieldErrors["email"] = []string{"Email is required."}
	}
	if len(fieldErrors) > 0 {
		return httpx.NewValidationError(fieldErrors)
	}

	user, err := s.BootstrapAdmin(c.Request().Context(), req.Username, req.Password, req.Email)
	if err != nil {
		return err
	}

	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "bootstrap_admin",
		ActorID:      user.ID,
		ResourceType: "auth",
	})

	return c.JSON(http.StatusCreated, userResponse{User: user})
}
