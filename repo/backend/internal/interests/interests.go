package interests

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

// Interest represents a customer-to-provider interest expression.
type Interest struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	ProviderID string `json:"provider_id"`
	ServiceID  string `json:"service_id"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// StatusEvent represents a status change in the interest lifecycle.
type StatusEvent struct {
	ID        string  `json:"id"`
	OldStatus *string `json:"old_status"`
	NewStatus string  `json:"new_status"`
	CreatedAt string  `json:"created_at"`
}

// Service provides interest management operations.
type Service struct {
	db     *sql.DB
	audit  *audit.Service
	blocks *blocks.Service
}

// NewService creates a new interests service.
func NewService(db *sql.DB, audit *audit.Service, blocks *blocks.Service) *Service {
	return &Service{db: db, audit: audit, blocks: blocks}
}

// GetCustomerProfileID resolves a user ID to its customer_profiles.id.
func (s *Service) GetCustomerProfileID(ctx context.Context, userID string) (string, error) {
	var profileID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM customer_profiles WHERE user_id=$1`, userID,
	).Scan(&profileID)
	if err == sql.ErrNoRows {
		return "", httpx.NewNotFoundError("Customer profile not found.")
	}
	if err != nil {
		return "", fmt.Errorf("interests: get customer profile: %w", err)
	}
	return profileID, nil
}

// GetProviderProfileID resolves a user ID to its provider_profiles.id.
func (s *Service) GetProviderProfileID(ctx context.Context, userID string) (string, error) {
	var profileID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM provider_profiles WHERE user_id=$1`, userID,
	).Scan(&profileID)
	if err == sql.ErrNoRows {
		return "", httpx.NewNotFoundError("Provider profile not found.")
	}
	if err != nil {
		return "", fmt.Errorf("interests: get provider profile: %w", err)
	}
	return profileID, nil
}

// GetProviderUserID resolves a provider_profiles.id to its user_id.
func (s *Service) GetProviderUserID(ctx context.Context, providerProfileID string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM provider_profiles WHERE id=$1`, providerProfileID,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", httpx.NewNotFoundError("Provider not found.")
	}
	if err != nil {
		return "", fmt.Errorf("interests: get provider user: %w", err)
	}
	return userID, nil
}

// GetCustomerUserID resolves a customer_profiles.id to its user_id.
func (s *Service) GetCustomerUserID(ctx context.Context, customerProfileID string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM customer_profiles WHERE id=$1`, customerProfileID,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", httpx.NewNotFoundError("Customer not found.")
	}
	if err != nil {
		return "", fmt.Errorf("interests: get customer user: %w", err)
	}
	return userID, nil
}

// Submit creates a new interest from a customer to a provider for a service.
func (s *Service) Submit(ctx context.Context, customerProfileID, providerProfileID, serviceID, actorID string) (*Interest, error) {
	// Resolve user IDs for block check
	customerUserID, err := s.GetCustomerUserID(ctx, customerProfileID)
	if err != nil {
		return nil, err
	}
	providerUserID, err := s.GetProviderUserID(ctx, providerProfileID)
	if err != nil {
		return nil, err
	}

	// Validate that the service belongs to the specified provider
	var actualProviderID string
	err = s.db.QueryRowContext(ctx,
		`SELECT provider_id FROM services WHERE id=$1`, serviceID,
	).Scan(&actualProviderID)
	if err == sql.ErrNoRows {
		return nil, httpx.NewValidationError(map[string][]string{
			"service_id": {"Service not found."},
		})
	}
	if err != nil {
		return nil, fmt.Errorf("interests: validate service provider: %w", err)
	}
	if actualProviderID != providerProfileID {
		return nil, httpx.NewValidationError(map[string][]string{
			"service_id": {"Service does not belong to the specified provider."},
		})
	}

	// Check blocks
	blocked, err := s.blocks.IsBlocked(ctx, customerUserID, providerUserID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, httpx.NewForbiddenError("blocked")
	}

	// Check duplicate within 7 days
	var count int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM interests WHERE customer_id=$1 AND provider_id=$2 AND status IN ('submitted','accepted') AND created_at > NOW() - INTERVAL '7 days'`,
		customerProfileID, providerProfileID,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("interests: check duplicate: %w", err)
	}
	if count > 0 {
		return nil, &echo.HTTPError{
			Code: http.StatusConflict,
			Message: httpx.APIError{
				Error: httpx.ErrorBody{
					Code:    "duplicate_interest",
					Message: "An active interest already exists for this provider.",
					FieldErrors: map[string][]string{
						"provider_id": {"Only one active interest is allowed within 7 days."},
					},
				},
			},
		}
	}

	// Insert interest
	var interest Interest
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO interests (customer_id, provider_id, service_id) VALUES ($1, $2, $3) RETURNING id, customer_id, provider_id, service_id, status, created_at::text, updated_at::text`,
		customerProfileID, providerProfileID, serviceID,
	).Scan(&interest.ID, &interest.CustomerID, &interest.ProviderID, &interest.ServiceID, &interest.Status, &interest.CreatedAt, &interest.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("interests: insert: %w", err)
	}

	// Insert initial status event
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO interest_status_events (interest_id, old_status, new_status) VALUES ($1, NULL, 'submitted')`,
		interest.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("interests: insert status event: %w", err)
	}

	// Audit
	s.audit.Log(ctx, audit.Event{
		EventType:    "interest_submitted",
		ActorID:      actorID,
		ResourceType: "interest",
		ResourceID:   interest.ID,
		Metadata: map[string]interface{}{
			"customer_id": customerProfileID,
			"provider_id": providerProfileID,
			"service_id":  serviceID,
		},
	})

	return &interest, nil
}

// ListCustomer returns all interests for a customer, newest first.
func (s *Service) ListCustomer(ctx context.Context, customerProfileID string) ([]Interest, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, customer_id, provider_id, service_id, status, created_at::text, updated_at::text FROM interests WHERE customer_id=$1 ORDER BY created_at DESC`,
		customerProfileID,
	)
	if err != nil {
		return nil, fmt.Errorf("interests: list customer: %w", err)
	}
	defer rows.Close()

	interests := make([]Interest, 0)
	for rows.Next() {
		var i Interest
		if err := rows.Scan(&i.ID, &i.CustomerID, &i.ProviderID, &i.ServiceID, &i.Status, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, fmt.Errorf("interests: scan: %w", err)
		}
		interests = append(interests, i)
	}
	return interests, rows.Err()
}

// GetCustomer returns a single interest with its status events, scoped to a customer.
func (s *Service) GetCustomer(ctx context.Context, interestID, customerProfileID string) (*Interest, []StatusEvent, error) {
	var interest Interest
	err := s.db.QueryRowContext(ctx,
		`SELECT id, customer_id, provider_id, service_id, status, created_at::text, updated_at::text FROM interests WHERE id=$1 AND customer_id=$2`,
		interestID, customerProfileID,
	).Scan(&interest.ID, &interest.CustomerID, &interest.ProviderID, &interest.ServiceID, &interest.Status, &interest.CreatedAt, &interest.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil, httpx.NewNotFoundError("Interest not found.")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("interests: get customer: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, old_status, new_status, created_at::text FROM interest_status_events WHERE interest_id=$1 ORDER BY created_at`,
		interestID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("interests: list events: %w", err)
	}
	defer rows.Close()

	events := make([]StatusEvent, 0)
	for rows.Next() {
		var e StatusEvent
		if err := rows.Scan(&e.ID, &e.OldStatus, &e.NewStatus, &e.CreatedAt); err != nil {
			return nil, nil, fmt.Errorf("interests: scan event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("interests: iterate events: %w", err)
	}

	return &interest, events, nil
}

// Withdraw withdraws an interest (customer action).
func (s *Service) Withdraw(ctx context.Context, interestID, customerProfileID, actorID string) error {
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM interests WHERE id=$1 AND customer_id=$2`,
		interestID, customerProfileID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Interest not found.")
	}
	if err != nil {
		return fmt.Errorf("interests: withdraw lookup: %w", err)
	}

	if status != "submitted" && status != "accepted" {
		return httpx.NewAPIError(http.StatusUnprocessableEntity, "invalid_transition", "Cannot withdraw an interest with status '"+status+"'.")
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE interests SET status='withdrawn', updated_at=NOW() WHERE id=$1`,
		interestID,
	)
	if err != nil {
		return fmt.Errorf("interests: withdraw update: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO interest_status_events (interest_id, old_status, new_status) VALUES ($1, $2, 'withdrawn')`,
		interestID, status,
	)
	if err != nil {
		return fmt.Errorf("interests: withdraw event: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "interest_withdrawn",
		ActorID:      actorID,
		ResourceType: "interest",
		ResourceID:   interestID,
	})

	return nil
}

// ListProvider returns all interests for a provider, newest first.
func (s *Service) ListProvider(ctx context.Context, providerProfileID string) ([]Interest, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, customer_id, provider_id, service_id, status, created_at::text, updated_at::text FROM interests WHERE provider_id=$1 ORDER BY created_at DESC`,
		providerProfileID,
	)
	if err != nil {
		return nil, fmt.Errorf("interests: list provider: %w", err)
	}
	defer rows.Close()

	interests := make([]Interest, 0)
	for rows.Next() {
		var i Interest
		if err := rows.Scan(&i.ID, &i.CustomerID, &i.ProviderID, &i.ServiceID, &i.Status, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, fmt.Errorf("interests: scan: %w", err)
		}
		interests = append(interests, i)
	}
	return interests, rows.Err()
}

// Accept accepts an interest (provider action).
func (s *Service) Accept(ctx context.Context, interestID, providerProfileID, actorID string) error {
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM interests WHERE id=$1 AND provider_id=$2`,
		interestID, providerProfileID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Interest not found.")
	}
	if err != nil {
		return fmt.Errorf("interests: accept lookup: %w", err)
	}

	if status != "submitted" {
		return httpx.NewAPIError(http.StatusUnprocessableEntity, "invalid_transition", "Cannot accept an interest with status '"+status+"'.")
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE interests SET status='accepted', updated_at=NOW() WHERE id=$1`,
		interestID,
	)
	if err != nil {
		return fmt.Errorf("interests: accept update: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO interest_status_events (interest_id, old_status, new_status) VALUES ($1, $2, 'accepted')`,
		interestID, status,
	)
	if err != nil {
		return fmt.Errorf("interests: accept event: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "interest_accepted",
		ActorID:      actorID,
		ResourceType: "interest",
		ResourceID:   interestID,
	})

	return nil
}

// Decline declines an interest (provider action).
func (s *Service) Decline(ctx context.Context, interestID, providerProfileID, actorID string) error {
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM interests WHERE id=$1 AND provider_id=$2`,
		interestID, providerProfileID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Interest not found.")
	}
	if err != nil {
		return fmt.Errorf("interests: decline lookup: %w", err)
	}

	if status != "submitted" {
		return httpx.NewAPIError(http.StatusUnprocessableEntity, "invalid_transition", "Cannot decline an interest with status '"+status+"'.")
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE interests SET status='declined', updated_at=NOW() WHERE id=$1`,
		interestID,
	)
	if err != nil {
		return fmt.Errorf("interests: decline update: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO interest_status_events (interest_id, old_status, new_status) VALUES ($1, $2, 'declined')`,
		interestID, status,
	)
	if err != nil {
		return fmt.Errorf("interests: decline event: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "interest_declined",
		ActorID:      actorID,
		ResourceType: "interest",
		ResourceID:   interestID,
	})

	return nil
}

// ---------- HTTP Handlers ----------

// HandleCustomerSubmit handles POST /customer/interests.
func (s *Service) HandleCustomerSubmit(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	var body struct {
		ServiceID  string `json:"service_id"`
		ProviderID string `json:"provider_id"`
	}
	if err := c.Bind(&body); err != nil {
		return httpx.NewValidationError(map[string][]string{"body": {"Invalid request body."}})
	}

	fieldErrors := map[string][]string{}
	if body.ServiceID == "" {
		fieldErrors["service_id"] = []string{"Service ID is required."}
	}
	if body.ProviderID == "" {
		fieldErrors["provider_id"] = []string{"Provider ID is required."}
	}
	if len(fieldErrors) > 0 {
		return httpx.NewValidationError(fieldErrors)
	}

	cpID, err := s.GetCustomerProfileID(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	interest, err := s.Submit(c.Request().Context(), cpID, body.ProviderID, body.ServiceID, user.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"interest": interest})
}

// HandleCustomerList handles GET /customer/interests.
func (s *Service) HandleCustomerList(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	cpID, err := s.GetCustomerProfileID(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	interests, err := s.ListCustomer(c.Request().Context(), cpID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"interests": interests})
}

// HandleCustomerGet handles GET /customer/interests/:interestId.
func (s *Service) HandleCustomerGet(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	cpID, err := s.GetCustomerProfileID(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	interest, events, err := s.GetCustomer(c.Request().Context(), c.Param("interestId"), cpID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"interest": interest, "events": events})
}

// HandleCustomerWithdraw handles POST /customer/interests/:interestId/withdraw.
func (s *Service) HandleCustomerWithdraw(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	cpID, err := s.GetCustomerProfileID(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	if err := s.Withdraw(c.Request().Context(), c.Param("interestId"), cpID, user.ID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"message": "Interest withdrawn."})
}

// HandleProviderList handles GET /provider/interests.
func (s *Service) HandleProviderList(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	ppID, err := s.GetProviderProfileID(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	interests, err := s.ListProvider(c.Request().Context(), ppID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"interests": interests})
}

// HandleProviderAccept handles POST /provider/interests/:interestId/accept.
func (s *Service) HandleProviderAccept(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	ppID, err := s.GetProviderProfileID(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	if err := s.Accept(c.Request().Context(), c.Param("interestId"), ppID, user.ID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"message": "Interest accepted."})
}

// HandleProviderDecline handles POST /provider/interests/:interestId/decline.
func (s *Service) HandleProviderDecline(c echo.Context) error {
	user := auth.UserFromContext(c)
	if user == nil {
		return httpx.NewAPIError(http.StatusUnauthorized, "unauthenticated", "Authentication required.")
	}

	ppID, err := s.GetProviderProfileID(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}

	if err := s.Decline(c.Request().Context(), c.Param("interestId"), ppID, user.ID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"message": "Interest declined."})
}
