package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"fieldserve/internal/audit"
	"fieldserve/internal/auth"
	"fieldserve/internal/platform/httpx"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Valid severity values.
var validSeverities = map[string]bool{
	"low":      true,
	"medium":   true,
	"high":     true,
	"critical": true,
}

// Supported metrics that the worker can evaluate.
var supportedMetrics = map[string]bool{
	"unresolved_interests":      true,
	"low_provider_utilization":  true,
	"overdue_work_orders":       true,
}

// Service manages alert rules, alerts, and assignments.
type Service struct {
	db    *sql.DB
	audit *audit.Service
}

// NewService creates a new alerts service.
func NewService(db *sql.DB, audit *audit.Service) *Service {
	return &Service{db: db, audit: audit}
}

// AlertRule represents an alert rule configuration.
type AlertRule struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Condition       json.RawMessage `json:"condition"`
	Severity        string          `json:"severity"`
	QuietHoursStart *string         `json:"quiet_hours_start"`
	QuietHoursEnd   *string         `json:"quiet_hours_end"`
	Enabled         bool            `json:"enabled"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

// Alert represents a triggered alert.
type Alert struct {
	ID         string          `json:"id"`
	RuleID     string          `json:"rule_id"`
	Severity   string          `json:"severity"`
	Status     string          `json:"status"`
	Data       json.RawMessage `json:"data"`
	CreatedAt  string          `json:"created_at"`
	ResolvedAt *string         `json:"resolved_at"`
}

// AlertWithRule includes the rule name for list views.
type AlertWithRule struct {
	Alert
	RuleName string `json:"rule_name"`
}

// Assignment represents an alert-to-user assignment.
type Assignment struct {
	ID             string  `json:"id"`
	AlertID        string  `json:"alert_id"`
	AssigneeID     string  `json:"assignee_id"`
	AssignedAt     string  `json:"assigned_at"`
	AcknowledgedAt *string `json:"acknowledged_at"`
}

// HandleListRules handles GET /admin/alert-rules.
func (s *Service) HandleListRules(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, name, condition, severity, quiet_hours_start::text, quiet_hours_end::text, enabled, created_at::text, updated_at::text
		 FROM alert_rules ORDER BY created_at DESC`)
	if err != nil {
		return fmt.Errorf("alerts: list rules: %w", err)
	}
	defer rows.Close()

	rules := []AlertRule{}
	for rows.Next() {
		var r AlertRule
		if err := rows.Scan(&r.ID, &r.Name, &r.Condition, &r.Severity, &r.QuietHoursStart, &r.QuietHoursEnd, &r.Enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return fmt.Errorf("alerts: scan rule: %w", err)
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("alerts: rules rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"alert_rules": rules})
}

// HandleCreateRule handles POST /admin/alert-rules.
func (s *Service) HandleCreateRule(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)

	var req struct {
		Name            string          `json:"name"`
		Condition       json.RawMessage `json:"condition"`
		Severity        string          `json:"severity"`
		QuietHoursStart *string         `json:"quiet_hours_start"`
		QuietHoursEnd   *string         `json:"quiet_hours_end"`
		Enabled         *bool           `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	// Validate
	fieldErrors := map[string][]string{}
	if req.Name == "" {
		fieldErrors["name"] = []string{"Name is required."}
	}
	if !validSeverities[req.Severity] {
		fieldErrors["severity"] = []string{"Severity must be one of: low, medium, high, critical."}
	}
	if len(req.Condition) == 0 || string(req.Condition) == "null" {
		fieldErrors["condition"] = []string{"Condition is required."}
	} else {
		var cond struct{ Metric string `json:"metric"` }
		if json.Unmarshal(req.Condition, &cond) == nil && cond.Metric != "" && !supportedMetrics[cond.Metric] {
			fieldErrors["condition"] = []string{"Unsupported metric. Supported: unresolved_interests, low_provider_utilization, overdue_work_orders."}
		}
	}
	if len(fieldErrors) > 0 {
		return httpx.NewValidationError(fieldErrors)
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	var r AlertRule
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO alert_rules (id, name, condition, severity, quiet_hours_start, quiet_hours_end, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		 RETURNING id, name, condition, severity, quiet_hours_start::text, quiet_hours_end::text, enabled, created_at::text, updated_at::text`,
		uuid.New().String(), req.Name, req.Condition, req.Severity, req.QuietHoursStart, req.QuietHoursEnd, enabled,
	).Scan(&r.ID, &r.Name, &r.Condition, &r.Severity, &r.QuietHoursStart, &r.QuietHoursEnd, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("alerts: create rule: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "alert_rule_created",
		ActorID:      user.ID,
		ResourceType: "alert_rule",
		ResourceID:   r.ID,
		Metadata:     map[string]interface{}{"name": r.Name, "severity": r.Severity},
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{"alert_rule": r})
}

// HandleUpdateRule handles PATCH /admin/alert-rules/:ruleId.
func (s *Service) HandleUpdateRule(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	ruleID := c.Param("ruleId")

	// Check rule exists
	var existing AlertRule
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, condition, severity, quiet_hours_start::text, quiet_hours_end::text, enabled, created_at::text, updated_at::text
		 FROM alert_rules WHERE id = $1`, ruleID,
	).Scan(&existing.ID, &existing.Name, &existing.Condition, &existing.Severity, &existing.QuietHoursStart, &existing.QuietHoursEnd, &existing.Enabled, &existing.CreatedAt, &existing.UpdatedAt)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Alert rule not found.")
	}
	if err != nil {
		return fmt.Errorf("alerts: get rule: %w", err)
	}

	var req struct {
		Name            *string          `json:"name"`
		Condition       *json.RawMessage `json:"condition"`
		Severity        *string          `json:"severity"`
		QuietHoursStart *string          `json:"quiet_hours_start"`
		QuietHoursEnd   *string          `json:"quiet_hours_end"`
		Enabled         *bool            `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	// Apply partial updates
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	condition := existing.Condition
	if req.Condition != nil {
		condition = *req.Condition
		// Validate metric
		var cond struct{ Metric string `json:"metric"` }
		if json.Unmarshal(condition, &cond) == nil && cond.Metric != "" && !supportedMetrics[cond.Metric] {
			return httpx.NewValidationError(map[string][]string{
				"condition": {"Unsupported metric. Supported: unresolved_interests, low_provider_utilization, overdue_work_orders."},
			})
		}
	}
	severity := existing.Severity
	if req.Severity != nil {
		if !validSeverities[*req.Severity] {
			return httpx.NewValidationError(map[string][]string{
				"severity": {"Severity must be one of: low, medium, high, critical."},
			})
		}
		severity = *req.Severity
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	quietStart := existing.QuietHoursStart
	if req.QuietHoursStart != nil {
		quietStart = req.QuietHoursStart
	}
	quietEnd := existing.QuietHoursEnd
	if req.QuietHoursEnd != nil {
		quietEnd = req.QuietHoursEnd
	}

	var r AlertRule
	err = s.db.QueryRowContext(ctx,
		`UPDATE alert_rules SET name = $1, condition = $2, severity = $3, quiet_hours_start = $4, quiet_hours_end = $5, enabled = $6, updated_at = NOW()
		 WHERE id = $7
		 RETURNING id, name, condition, severity, quiet_hours_start::text, quiet_hours_end::text, enabled, created_at::text, updated_at::text`,
		name, condition, severity, quietStart, quietEnd, enabled, ruleID,
	).Scan(&r.ID, &r.Name, &r.Condition, &r.Severity, &r.QuietHoursStart, &r.QuietHoursEnd, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("alerts: update rule: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "alert_rule_updated",
		ActorID:      user.ID,
		ResourceType: "alert_rule",
		ResourceID:   r.ID,
		Metadata:     map[string]interface{}{"name": r.Name},
	})

	return c.JSON(http.StatusOK, map[string]interface{}{"alert_rule": r})
}

// HandleListAlerts handles GET /admin/alerts.
func (s *Service) HandleListAlerts(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT a.id, a.rule_id, a.severity, a.status, a.data, a.created_at::text, a.resolved_at::text, ar.name
		 FROM alerts a
		 JOIN alert_rules ar ON ar.id = a.rule_id
		 ORDER BY a.created_at DESC`)
	if err != nil {
		return fmt.Errorf("alerts: list alerts: %w", err)
	}
	defer rows.Close()

	alerts := []AlertWithRule{}
	for rows.Next() {
		var a AlertWithRule
		if err := rows.Scan(&a.ID, &a.RuleID, &a.Severity, &a.Status, &a.Data, &a.CreatedAt, &a.ResolvedAt, &a.RuleName); err != nil {
			return fmt.Errorf("alerts: scan alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("alerts: alerts rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"alerts": alerts})
}

// HandleGetAlert handles GET /admin/alerts/:alertId.
func (s *Service) HandleGetAlert(c echo.Context) error {
	ctx := c.Request().Context()
	alertID := c.Param("alertId")

	var a AlertWithRule
	err := s.db.QueryRowContext(ctx,
		`SELECT a.id, a.rule_id, a.severity, a.status, a.data, a.created_at::text, a.resolved_at::text, ar.name
		 FROM alerts a
		 JOIN alert_rules ar ON ar.id = a.rule_id
		 WHERE a.id = $1`, alertID,
	).Scan(&a.ID, &a.RuleID, &a.Severity, &a.Status, &a.Data, &a.CreatedAt, &a.ResolvedAt, &a.RuleName)
	if err == sql.ErrNoRows {
		return httpx.NewNotFoundError("Alert not found.")
	}
	if err != nil {
		return fmt.Errorf("alerts: get alert: %w", err)
	}

	// Fetch assignments
	assignRows, err := s.db.QueryContext(ctx,
		`SELECT id, alert_id, assignee_id, assigned_at::text, acknowledged_at::text
		 FROM alert_assignments WHERE alert_id = $1`, alertID)
	if err != nil {
		return fmt.Errorf("alerts: list assignments: %w", err)
	}
	defer assignRows.Close()

	assignments := []Assignment{}
	for assignRows.Next() {
		var asgn Assignment
		if err := assignRows.Scan(&asgn.ID, &asgn.AlertID, &asgn.AssigneeID, &asgn.AssignedAt, &asgn.AcknowledgedAt); err != nil {
			return fmt.Errorf("alerts: scan assignment: %w", err)
		}
		assignments = append(assignments, asgn)
	}
	if err := assignRows.Err(); err != nil {
		return fmt.Errorf("alerts: assignment rows: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"alert":       a,
		"assignments": assignments,
	})
}

// OnCallSchedule represents an on-call schedule entry.
type OnCallSchedule struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Tier      int    `json:"tier"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	CreatedAt string `json:"created_at"`
}

// HandleListOnCall handles GET /admin/on-call — returns only currently active
// on-call schedules (start_time <= NOW < end_time), ordered by tier ascending.
func (s *Service) HandleListOnCall(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, user_id, tier, start_time::text, end_time::text, created_at::text
		 FROM on_call_schedules
		 WHERE start_time <= NOW() AND end_time > NOW()
		 ORDER BY tier, start_time`)
	if err != nil {
		return fmt.Errorf("alerts: list on-call: %w", err)
	}
	defer rows.Close()

	schedules := []OnCallSchedule{}
	for rows.Next() {
		var oc OnCallSchedule
		if err := rows.Scan(&oc.ID, &oc.UserID, &oc.Tier, &oc.StartTime, &oc.EndTime, &oc.CreatedAt); err != nil {
			return fmt.Errorf("alerts: scan on-call: %w", err)
		}
		schedules = append(schedules, oc)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"on_call_schedules": schedules})
}

// HandleCreateOnCall handles POST /admin/on-call — creates an on-call schedule.
func (s *Service) HandleCreateOnCall(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)

	var req struct {
		UserID    string `json:"user_id"`
		Tier      int    `json:"tier"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	fieldErrors := map[string][]string{}
	if req.UserID == "" {
		fieldErrors["user_id"] = []string{"User ID is required."}
	}
	if req.Tier < 1 || req.Tier > 3 {
		fieldErrors["tier"] = []string{"Tier must be 1, 2, or 3."}
	}
	if req.StartTime == "" {
		fieldErrors["start_time"] = []string{"Start time is required."}
	}
	if req.EndTime == "" {
		fieldErrors["end_time"] = []string{"End time is required."}
	}
	if len(fieldErrors) > 0 {
		return httpx.NewValidationError(fieldErrors)
	}

	// Verify user exists and has administrator role
	var userExists bool
	s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1 AND r.name = 'administrator')`,
		req.UserID,
	).Scan(&userExists)
	if !userExists {
		return httpx.NewValidationError(map[string][]string{
			"user_id": {"User must have administrator role to be on-call."},
		})
	}

	var oc OnCallSchedule
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO on_call_schedules (id, user_id, tier, start_time, end_time, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 RETURNING id, user_id, tier, start_time::text, end_time::text, created_at::text`,
		uuid.New().String(), req.UserID, req.Tier, req.StartTime, req.EndTime,
	).Scan(&oc.ID, &oc.UserID, &oc.Tier, &oc.StartTime, &oc.EndTime, &oc.CreatedAt)
	if err != nil {
		return fmt.Errorf("alerts: create on-call: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "on_call_created",
		ActorID:      user.ID,
		ResourceType: "on_call_schedule",
		ResourceID:   oc.ID,
		Metadata:     map[string]interface{}{"user_id": req.UserID, "tier": req.Tier},
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{"on_call_schedule": oc})
}

// isUserOnCall checks if a user has an active on-call schedule (covering NOW).
func (s *Service) isUserOnCall(ctx context.Context, userID string) (bool, error) {
	var onCall bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM on_call_schedules
			WHERE user_id = $1 AND start_time <= NOW() AND end_time > NOW()
		)`, userID,
	).Scan(&onCall)
	return onCall, err
}

// HandleAssignAlert handles POST /admin/alerts/:alertId/assign.
// Assignment is restricted to users with an active on-call schedule.
func (s *Service) HandleAssignAlert(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	alertID := c.Param("alertId")

	// Check alert exists
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM alerts WHERE id = $1)`, alertID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("alerts: check alert: %w", err)
	}
	if !exists {
		return httpx.NewNotFoundError("Alert not found.")
	}

	var req struct {
		AssigneeID string `json:"assignee_id"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}
	if req.AssigneeID == "" {
		return httpx.NewValidationError(map[string][]string{
			"assignee_id": {"Assignee ID is required."},
		})
	}

	// Enforce on-call eligibility
	onCall, err := s.isUserOnCall(ctx, req.AssigneeID)
	if err != nil {
		return fmt.Errorf("alerts: check on-call: %w", err)
	}
	if !onCall {
		return httpx.NewValidationError(map[string][]string{
			"assignee_id": {"Assignee must have an active on-call schedule."},
		})
	}

	var asgn Assignment
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO alert_assignments (id, alert_id, assignee_id, assigned_at)
		 VALUES ($1, $2, $3, NOW())
		 RETURNING id, alert_id, assignee_id, assigned_at::text, acknowledged_at::text`,
		uuid.New().String(), alertID, req.AssigneeID,
	).Scan(&asgn.ID, &asgn.AlertID, &asgn.AssigneeID, &asgn.AssignedAt, &asgn.AcknowledgedAt)
	if err != nil {
		return fmt.Errorf("alerts: assign alert: %w", err)
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "alert_assigned",
		ActorID:      user.ID,
		ResourceType: "alert",
		ResourceID:   alertID,
		Metadata:     map[string]interface{}{"assignee_id": req.AssigneeID},
	})

	return c.JSON(http.StatusOK, map[string]interface{}{"assignment": asgn})
}

// HandleAcknowledgeAlert handles POST /admin/alerts/:alertId/acknowledge.
func (s *Service) HandleAcknowledgeAlert(c echo.Context) error {
	ctx := c.Request().Context()
	user := auth.UserFromContext(c)
	alertID := c.Param("alertId")

	result, err := s.db.ExecContext(ctx,
		`UPDATE alert_assignments SET acknowledged_at = NOW()
		 WHERE alert_id = $1 AND assignee_id = $2 AND acknowledged_at IS NULL`,
		alertID, user.ID,
	)
	if err != nil {
		return fmt.Errorf("alerts: acknowledge: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return httpx.NewNotFoundError("No pending assignment found for this alert.")
	}

	s.audit.Log(ctx, audit.Event{
		EventType:    "alert_acknowledged",
		ActorID:      user.ID,
		ResourceType: "alert",
		ResourceID:   alertID,
	})

	return c.JSON(http.StatusOK, map[string]interface{}{"message": "Alert acknowledged."})
}

// autoAssignToLowestTier assigns a newly created alert to the lowest-tier
// active on-call user. If no on-call user is active, the alert remains
// unassigned until manual action or escalation.
func (s *Service) autoAssignToLowestTier(ctx context.Context, alertID string) {
	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM on_call_schedules
		 WHERE start_time <= NOW() AND end_time > NOW()
		 ORDER BY tier ASC, start_time ASC
		 LIMIT 1`,
	).Scan(&userID)
	if err != nil {
		// No active on-call user — alert stays unassigned
		return
	}

	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO alert_assignments (id, alert_id, assignee_id, assigned_at)
		 VALUES ($1, $2, $3, NOW())`,
		uuid.New().String(), alertID, userID,
	)

	s.audit.Log(ctx, audit.Event{
		EventType:    "alert_auto_assigned",
		ResourceType: "alert",
		ResourceID:   alertID,
		Metadata:     map[string]interface{}{"assignee_id": userID, "reason": "lowest_tier_on_call"},
	})
}

// EscalateUnacknowledged finds alerts assigned but not acknowledged within
// 30 minutes and escalates them to the next on-call tier. Tier 1 → Tier 2,
// Tier 2 → Tier 3. Already at Tier 3 or no next-tier user available: no-op.
// This is called by the worker on each tick.
func (s *Service) EscalateUnacknowledged(ctx context.Context, now time.Time) error {
	// Find assignments that are unacknowledged and older than 30 minutes
	rows, err := s.db.QueryContext(ctx,
		`SELECT aa.id, aa.alert_id, aa.assignee_id, ocs.tier
		 FROM alert_assignments aa
		 JOIN on_call_schedules ocs ON ocs.user_id = aa.assignee_id
		   AND ocs.start_time <= NOW() AND ocs.end_time > NOW()
		 WHERE aa.acknowledged_at IS NULL
		   AND aa.assigned_at < NOW() - INTERVAL '30 minutes'
		 ORDER BY aa.assigned_at`)
	if err != nil {
		return fmt.Errorf("alerts: escalation query: %w", err)
	}
	defer rows.Close()

	type pending struct {
		assignmentID string
		alertID      string
		assigneeID   string
		currentTier  int
	}
	var items []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.assignmentID, &p.alertID, &p.assigneeID, &p.currentTier); err != nil {
			continue
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range items {
		nextTier := p.currentTier + 1
		if nextTier > 3 {
			continue
		}

		// Check if already escalated to this alert at a higher tier
		var alreadyEscalated bool
		s.db.QueryRowContext(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM alert_assignments aa2
				JOIN on_call_schedules ocs2 ON ocs2.user_id = aa2.assignee_id
				  AND ocs2.start_time <= NOW() AND ocs2.end_time > NOW()
				WHERE aa2.alert_id = $1 AND ocs2.tier >= $2
			)`, p.alertID, nextTier,
		).Scan(&alreadyEscalated)
		if alreadyEscalated {
			continue
		}

		// Find a next-tier on-call user
		var nextUserID string
		err := s.db.QueryRowContext(ctx,
			`SELECT user_id FROM on_call_schedules
			 WHERE tier = $1 AND start_time <= NOW() AND end_time > NOW()
			 LIMIT 1`, nextTier,
		).Scan(&nextUserID)
		if err != nil {
			continue
		}

		// Create escalation assignment
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO alert_assignments (id, alert_id, assignee_id, assigned_at)
			 VALUES ($1, $2, $3, NOW())`,
			uuid.New().String(), p.alertID, nextUserID,
		)

		s.audit.Log(ctx, audit.Event{
			EventType:    "alert_escalated",
			ResourceType: "alert",
			ResourceID:   p.alertID,
			Metadata: map[string]interface{}{
				"from_tier":    p.currentTier,
				"to_tier":      nextTier,
				"from_user":    p.assigneeID,
				"to_user":      nextUserID,
			},
		})
	}

	return nil
}

// EvaluateRules checks all enabled alert rules and creates alerts as needed.
func (s *Service) EvaluateRules(ctx context.Context, now time.Time) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, condition, severity, quiet_hours_start::text, quiet_hours_end::text, enabled
		 FROM alert_rules WHERE enabled = true`)
	if err != nil {
		return fmt.Errorf("alerts: query rules: %w", err)
	}
	defer rows.Close()

	type ruleRow struct {
		id              string
		name            string
		condition       json.RawMessage
		severity        string
		quietHoursStart *string
		quietHoursEnd   *string
	}

	var rules []ruleRow
	for rows.Next() {
		var r ruleRow
		var enabled bool
		if err := rows.Scan(&r.id, &r.name, &r.condition, &r.severity, &r.quietHoursStart, &r.quietHoursEnd, &enabled); err != nil {
			return fmt.Errorf("alerts: scan rule: %w", err)
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("alerts: rules rows: %w", err)
	}

	for _, rule := range rules {
		// Check quiet hours
		if rule.severity != "critical" && inQuietHours(rule.quietHoursStart, rule.quietHoursEnd, now) {
			continue
		}

		// Parse condition
		var cond struct {
			Metric    string `json:"metric"`
			Threshold int    `json:"threshold"`
		}
		if err := json.Unmarshal(rule.condition, &cond); err != nil {
			continue
		}

		// Evaluate metric
		var metricValue int
		var evalErr error
		switch cond.Metric {
		case "unresolved_interests":
			evalErr = s.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM interests WHERE status = 'submitted' AND created_at < NOW() - INTERVAL '3 days'`,
			).Scan(&metricValue)
		case "low_provider_utilization":
			evalErr = s.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM provider_profiles pp
				 WHERE NOT EXISTS (SELECT 1 FROM services s WHERE s.provider_id = pp.id AND s.status = 'active')`,
			).Scan(&metricValue)
		case "overdue_work_orders":
			evalErr = s.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM work_orders
				 WHERE status IN ('dispatched', 'acknowledged', 'in_progress')
				 AND updated_at < NOW() - INTERVAL '24 hours'`,
			).Scan(&metricValue)
		default:
			continue
		}
		if evalErr != nil {
			continue
		}

		if metricValue < cond.Threshold {
			continue
		}

		// Check for recent unresolved alert for this rule (last hour)
		var recentCount int
		s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM alerts
			 WHERE rule_id = $1 AND status != 'resolved' AND created_at > NOW() - INTERVAL '1 hour'`,
			rule.id,
		).Scan(&recentCount)
		if recentCount > 0 {
			continue
		}

		// Create alert
		alertID := uuid.New().String()
		data, _ := json.Marshal(map[string]interface{}{
			"metric": cond.Metric,
			"value":  metricValue,
		})
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO alerts (id, rule_id, severity, status, data, created_at)
			 VALUES ($1, $2, $3, 'new', $4, NOW())`,
			alertID, rule.id, rule.severity, data,
		)
		if err != nil {
			continue
		}

		// Auto-assign to the lowest-tier active on-call user
		s.autoAssignToLowestTier(ctx, alertID)
	}

	return nil
}

// CheckSLADeadlines finds overdue work orders and creates critical alerts.
func (s *Service) CheckSLADeadlines(ctx context.Context, now time.Time) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM work_orders
		 WHERE status IN ('dispatched', 'acknowledged', 'in_progress')
		 AND updated_at < NOW() - INTERVAL '24 hours'`)
	if err != nil {
		return fmt.Errorf("alerts: sla check query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var woID string
		if err := rows.Scan(&woID); err != nil {
			continue
		}

		// Check for existing SLA alert for this work order in last hour
		var recentCount int
		s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM alerts
			 WHERE status != 'resolved' AND data->>'work_order_id' = $1 AND created_at > NOW() - INTERVAL '1 hour'`,
			woID,
		).Scan(&recentCount)
		if recentCount > 0 {
			continue
		}

		// We need a rule_id for SLA alerts. Use a sentinel approach: find or create a system SLA rule.
		var slaRuleID string
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM alert_rules WHERE name = '__sla_breach_system'`,
		).Scan(&slaRuleID)
		if err == sql.ErrNoRows {
			slaRuleID = uuid.New().String()
			cond, _ := json.Marshal(map[string]string{"metric": "sla_breach"})
			_, err = s.db.ExecContext(ctx,
				`INSERT INTO alert_rules (id, name, condition, severity, enabled, created_at, updated_at)
				 VALUES ($1, '__sla_breach_system', $2, 'critical', true, NOW(), NOW())`,
				slaRuleID, cond,
			)
			if err != nil {
				return fmt.Errorf("alerts: create sla rule: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("alerts: query sla rule: %w", err)
		}

		data, _ := json.Marshal(map[string]interface{}{
			"work_order_id": woID,
			"reason":        "sla_breach",
		})
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO alerts (id, rule_id, severity, status, data, created_at)
			 VALUES ($1, $2, 'critical', 'new', $3, NOW())`,
			uuid.New().String(), slaRuleID, data,
		)
	}

	return rows.Err()
}

// inQuietHours checks if the given time falls within quiet hours.
func inQuietHours(startStr, endStr *string, now time.Time) bool {
	if startStr == nil || endStr == nil || *startStr == "" || *endStr == "" {
		return false
	}

	start, err := time.Parse("15:04:05", *startStr)
	if err != nil {
		// Try without seconds
		start, err = time.Parse("15:04", *startStr)
		if err != nil {
			return false
		}
	}
	end, err := time.Parse("15:04:05", *endStr)
	if err != nil {
		end, err = time.Parse("15:04", *endStr)
		if err != nil {
			return false
		}
	}

	nowMinutes := now.Hour()*60 + now.Minute()
	startMinutes := start.Hour()*60 + start.Minute()
	endMinutes := end.Hour()*60 + end.Minute()

	if startMinutes <= endMinutes {
		// Same day range: e.g., 08:00 - 17:00
		return nowMinutes >= startMinutes && nowMinutes < endMinutes
	}
	// Overnight range: e.g., 22:00 - 07:00
	return nowMinutes >= startMinutes || nowMinutes < endMinutes
}
