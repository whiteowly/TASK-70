package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"fieldserve/internal/alerts"
	"fieldserve/internal/analytics"
	"fieldserve/internal/audit"
	"fieldserve/internal/auth"
	"fieldserve/internal/blocks"
	"fieldserve/internal/catalog"
	"fieldserve/internal/favorites"
	"fieldserve/internal/interests"
	"fieldserve/internal/messages"
	"fieldserve/internal/platform/crypto"
	"fieldserve/internal/platform/httpx"
	"fieldserve/internal/search"
	"fieldserve/internal/uploads"
	"fieldserve/internal/workorders"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	e := newServer(db)

	log.Printf("starting API server on :%s", port)
	if err := e.Start(fmt.Sprintf(":%s", port)); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// newServer creates and configures the Echo instance. Extracted so tests can
// reuse it without starting a real listener.
func newServer(db *sql.DB) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = httpx.ErrorHandler

	e.Use(httpx.RequestID())
	e.Use(httpx.RequestLogger())

	v1 := e.Group("/api/v1")

	// System routes (no auth required)
	system := v1.Group("/system")
	system.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// API access audit middleware — logs all authenticated requests
	if db != nil {
		e.Use(httpx.APIAccessAudit(db))
	}

	// Auth and protected routes require DB
	if db != nil {
		auditSvc := audit.NewService(db)
		authSvc := auth.NewService(db, auditSvc)

		// Auth routes (public)
		authGroup := v1.Group("/auth")
		authGroup.POST("/login", authSvc.HandleLogin)
		authGroup.POST("/bootstrap-admin", authSvc.HandleBootstrapAdmin)
		// Auth routes (require session)
		authGroup.POST("/logout", authSvc.HandleLogout, authSvc.RequireAuth())
		authGroup.GET("/me", authSvc.HandleMe, authSvc.RequireAuth())

		searchSvc := search.NewService(db)
		favSvc := favorites.NewService(db)

		blocksSvc := blocks.NewService(db, auditSvc)

		catalogSvc := catalog.NewService(db, auditSvc)
		catalogSvc.OnCatalogChange = func() { searchSvc.InvalidateCache() }
		catalogSvc.IsBlocked = blocksSvc.IsBlocked
		interestsSvc := interests.NewService(db, auditSvc, blocksSvc)
		messagesSvc := messages.NewService(db, auditSvc, blocksSvc)

		// Rate limiter and idempotency middleware
		rateLimiter := httpx.NewRateLimiter(60, time.Minute)
		rateLimitKeyFn := func(c echo.Context) string {
			u := auth.UserFromContext(c)
			if u != nil {
				return u.ID
			}
			return ""
		}
		rateLimitMW := rateLimiter.Middleware(rateLimitKeyFn)
		writeOnlyRateLimitMW := writeOnlyRateLimit(rateLimiter, rateLimitKeyFn)
		idempotencyMW := httpx.IdempotencyCheck(db)

		// Catalog reads (any authenticated user)
		catalogGroup := v1.Group("/catalog", authSvc.RequireAuth())
		catalogGroup.GET("/categories", catalogSvc.HandleListCategories)
		catalogGroup.GET("/tags", catalogSvc.HandleListTags)
		catalogGroup.GET("/services", func(c echo.Context) error {
			params := search.ParseSearchParams(c)
			user := auth.UserFromContext(c)

			// Exclude blocked providers from search results
			blockedIDs, err := getBlockedUserIDs(c.Request().Context(), db, user.ID)
			if err == nil && len(blockedIDs) > 0 {
				params.ExcludeProviderUserIDs = blockedIDs
			}

			result, err := searchSvc.Search(c.Request().Context(), params)
			if err != nil {
				return err
			}
			// Record search event if there was a query
			if params.Q != "" {
				filters := map[string]interface{}{"category_id": params.CategoryID, "sort": params.Sort}
				go searchSvc.RecordSearch(context.Background(), user.ID, params.Q, filters, result.Total)
			}
			return c.JSON(http.StatusOK, result)
		})
		catalogGroup.GET("/services/:serviceId", catalogSvc.HandleGetService)
		catalogGroup.GET("/trending", func(c echo.Context) error {
			result, err := searchSvc.GetTrending(c.Request().Context(), 10)
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"services": result})
		})
		catalogGroup.GET("/hot-keywords", catalogSvc.HandleHotKeywords)
		catalogGroup.GET("/autocomplete", catalogSvc.HandleAutocomplete)

		// Protected route groups — each requires auth + specific role
		// writeOnlyRateLimitMW applies the 60 RPM rate limit to all POST/PATCH/PUT/DELETE
		customerGroup := v1.Group("/customer", authSvc.RequireAuth(), auth.RequireRole(auditSvc, "customer"), writeOnlyRateLimitMW)

		// Customer profile (encrypted sensitive fields, masked by default)
		customerGroup.GET("/profile", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			var displayName sql.NullString
			var phoneEnc, notesEnc []byte
			err := db.QueryRowContext(c.Request().Context(),
				`SELECT display_name, phone_encrypted, notes_encrypted FROM customer_profiles WHERE user_id = $1`, user.ID,
			).Scan(&displayName, &phoneEnc, &notesEnc)
			if err == sql.ErrNoRows {
				return httpx.NewNotFoundError("Profile not found.")
			}
			if err != nil {
				return fmt.Errorf("profile: query: %w", err)
			}
			phone := ""
			notes := ""
			if phoneEnc != nil && len(phoneEnc) > 0 {
				if encKey, kerr := crypto.Key(); kerr == nil {
					if decrypted, derr := crypto.Decrypt(encKey, string(phoneEnc)); derr == nil {
						phone = crypto.MaskPhone(decrypted)
					}
				}
			}
			if notesEnc != nil && len(notesEnc) > 0 {
				if encKey, kerr := crypto.Key(); kerr == nil {
					if decrypted, derr := crypto.Decrypt(encKey, string(notesEnc)); derr == nil {
						notes = crypto.MaskNote(decrypted)
					}
				}
			}
			return c.JSON(http.StatusOK, map[string]interface{}{
				"profile": map[string]interface{}{
					"display_name": displayName.String,
					"phone":        phone,
					"notes":        notes,
				},
			})
		})
		customerGroup.PATCH("/profile", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			var req struct {
				Phone *string `json:"phone"`
				Notes *string `json:"notes"`
			}
			if err := c.Bind(&req); err != nil {
				return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request.")
			}
			encKey, kerr := crypto.Key()
			if kerr != nil {
				return fmt.Errorf("profile: encryption key: %w", kerr)
			}
			if req.Phone != nil {
				ct, eerr := crypto.Encrypt(encKey, *req.Phone)
				if eerr != nil {
					return fmt.Errorf("profile: encrypt phone: %w", eerr)
				}
				_, err := db.ExecContext(c.Request().Context(),
					`UPDATE customer_profiles SET phone_encrypted = $1, updated_at = NOW() WHERE user_id = $2`,
					[]byte(ct), user.ID)
				if err != nil {
					return fmt.Errorf("profile: update phone: %w", err)
				}
			}
			if req.Notes != nil {
				ct, eerr := crypto.Encrypt(encKey, *req.Notes)
				if eerr != nil {
					return fmt.Errorf("profile: encrypt notes: %w", eerr)
				}
				_, err := db.ExecContext(c.Request().Context(),
					`UPDATE customer_profiles SET notes_encrypted = $1, updated_at = NOW() WHERE user_id = $2`,
					[]byte(ct), user.ID)
				if err != nil {
					return fmt.Errorf("profile: update notes: %w", err)
				}
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"message": "Profile updated."})
		}, idempotencyMW)

		// Customer favorites
		customerGroup.GET("/favorites", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			cpID, err := favSvc.GetCustomerProfileID(c.Request().Context(), user.ID)
			if err != nil {
				return err
			}
			favs, err := favSvc.List(c.Request().Context(), cpID)
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"favorites": favs})
		})
		customerGroup.POST("/favorites/:serviceId", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			cpID, err := favSvc.GetCustomerProfileID(c.Request().Context(), user.ID)
			if err != nil {
				return err
			}
			fav, err := favSvc.Add(c.Request().Context(), cpID, c.Param("serviceId"))
			if err != nil {
				return err
			}
			searchSvc.InvalidateCache()
			return c.JSON(http.StatusCreated, map[string]interface{}{"favorite": fav})
		}, idempotencyMW)
		customerGroup.DELETE("/favorites/:serviceId", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			cpID, err := favSvc.GetCustomerProfileID(c.Request().Context(), user.ID)
			if err != nil {
				return err
			}
			if err := favSvc.Remove(c.Request().Context(), cpID, c.Param("serviceId")); err != nil {
				return err
			}
			searchSvc.InvalidateCache()
			return c.JSON(http.StatusOK, map[string]interface{}{"message": "Favorite removed."})
		}, idempotencyMW)
		customerGroup.GET("/search-history", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			history, err := searchSvc.GetSearchHistory(c.Request().Context(), user.ID, 20)
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"history": history})
		})

		// Customer interests
		customerGroup.POST("/interests", interestsSvc.HandleCustomerSubmit, idempotencyMW, rateLimitMW)
		customerGroup.GET("/interests", interestsSvc.HandleCustomerList)
		customerGroup.GET("/interests/:interestId", interestsSvc.HandleCustomerGet)
		customerGroup.POST("/interests/:interestId/withdraw", interestsSvc.HandleCustomerWithdraw, idempotencyMW)

		// Customer messages
		customerGroup.GET("/messages", messagesSvc.HandleListThreads)
		customerGroup.GET("/messages/:threadId", messagesSvc.HandleGetThread)
		customerGroup.POST("/messages/:threadId", messagesSvc.HandleSendMessage, idempotencyMW, rateLimitMW)
		customerGroup.POST("/messages/:threadId/read", messagesSvc.HandleMarkRead, idempotencyMW)

		// Customer blocks
		customerGroup.POST("/blocks/:providerId", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			providerProfileID := c.Param("providerId")
			var providerUserID string
			err := db.QueryRowContext(c.Request().Context(),
				"SELECT user_id FROM provider_profiles WHERE id=$1", providerProfileID).Scan(&providerUserID)
			if err != nil {
				return httpx.NewNotFoundError("Provider not found.")
			}
			if err := blocksSvc.Block(c.Request().Context(), user.ID, providerUserID); err != nil {
				return err
			}
			return c.JSON(http.StatusCreated, map[string]interface{}{"message": "User blocked."})
		}, idempotencyMW)
		customerGroup.DELETE("/blocks/:providerId", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			var providerUserID string
			err := db.QueryRowContext(c.Request().Context(),
				"SELECT user_id FROM provider_profiles WHERE id=$1", c.Param("providerId")).Scan(&providerUserID)
			if err != nil {
				return httpx.NewNotFoundError("Provider not found.")
			}
			if err := blocksSvc.Unblock(c.Request().Context(), user.ID, providerUserID); err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"message": "User unblocked."})
		}, idempotencyMW)

		uploadsSvc := uploads.NewService(db, auditSvc)
		analyticsSvc := analytics.NewService(db, auditSvc)

		providerGroup := v1.Group("/provider", authSvc.RequireAuth(), auth.RequireRole(auditSvc, "provider"), writeOnlyRateLimitMW)

		// Provider profile (encrypted sensitive fields, masked by default)
		providerGroup.GET("/profile", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			var businessName sql.NullString
			var phoneEnc, notesEnc []byte
			err := db.QueryRowContext(c.Request().Context(),
				`SELECT business_name, phone_encrypted, notes_encrypted FROM provider_profiles WHERE user_id = $1`, user.ID,
			).Scan(&businessName, &phoneEnc, &notesEnc)
			if err == sql.ErrNoRows {
				return httpx.NewNotFoundError("Profile not found.")
			}
			if err != nil {
				return fmt.Errorf("profile: query: %w", err)
			}
			phone := ""
			notes := ""
			if phoneEnc != nil && len(phoneEnc) > 0 {
				if encKey, kerr := crypto.Key(); kerr == nil {
					if decrypted, derr := crypto.Decrypt(encKey, string(phoneEnc)); derr == nil {
						phone = crypto.MaskPhone(decrypted)
					}
				}
			}
			if notesEnc != nil && len(notesEnc) > 0 {
				if encKey, kerr := crypto.Key(); kerr == nil {
					if decrypted, derr := crypto.Decrypt(encKey, string(notesEnc)); derr == nil {
						notes = crypto.MaskNote(decrypted)
					}
				}
			}
			return c.JSON(http.StatusOK, map[string]interface{}{
				"profile": map[string]interface{}{
					"business_name": businessName.String,
					"phone":         phone,
					"notes":         notes,
				},
			})
		})
		providerGroup.PATCH("/profile", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			var req struct {
				Phone *string `json:"phone"`
				Notes *string `json:"notes"`
			}
			if err := c.Bind(&req); err != nil {
				return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request.")
			}
			encKey, kerr := crypto.Key()
			if kerr != nil {
				return fmt.Errorf("profile: encryption key: %w", kerr)
			}
			if req.Phone != nil {
				ct, eerr := crypto.Encrypt(encKey, *req.Phone)
				if eerr != nil {
					return fmt.Errorf("profile: encrypt phone: %w", eerr)
				}
				_, err := db.ExecContext(c.Request().Context(),
					`UPDATE provider_profiles SET phone_encrypted = $1, updated_at = NOW() WHERE user_id = $2`,
					[]byte(ct), user.ID)
				if err != nil {
					return fmt.Errorf("profile: update phone: %w", err)
				}
			}
			if req.Notes != nil {
				ct, eerr := crypto.Encrypt(encKey, *req.Notes)
				if eerr != nil {
					return fmt.Errorf("profile: encrypt notes: %w", eerr)
				}
				_, err := db.ExecContext(c.Request().Context(),
					`UPDATE provider_profiles SET notes_encrypted = $1, updated_at = NOW() WHERE user_id = $2`,
					[]byte(ct), user.ID)
				if err != nil {
					return fmt.Errorf("profile: update notes: %w", err)
				}
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"message": "Profile updated."})
		}, idempotencyMW)

		// Provider document uploads
		providerGroup.GET("/documents", uploadsSvc.HandleList)
		providerGroup.POST("/documents", uploadsSvc.HandleUpload, idempotencyMW)
		providerGroup.DELETE("/documents/:documentId", uploadsSvc.HandleDelete, idempotencyMW)

		// Provider routes
		providerGroup.GET("/services", catalogSvc.HandleProviderListServices)
		providerGroup.GET("/services/:serviceId", catalogSvc.HandleProviderGetService)
		providerGroup.POST("/services", catalogSvc.HandleProviderCreateService, idempotencyMW)
		providerGroup.PATCH("/services/:serviceId", catalogSvc.HandleProviderUpdateService, idempotencyMW)
		providerGroup.DELETE("/services/:serviceId", catalogSvc.HandleProviderDeleteService, idempotencyMW)
		providerGroup.POST("/services/:serviceId/availability", catalogSvc.HandleProviderSetAvailability, idempotencyMW)

		// Provider interests
		providerGroup.GET("/interests", interestsSvc.HandleProviderList)
		providerGroup.POST("/interests/:interestId/accept", interestsSvc.HandleProviderAccept, idempotencyMW)
		providerGroup.POST("/interests/:interestId/decline", interestsSvc.HandleProviderDecline, idempotencyMW)

		// Provider messages
		providerGroup.GET("/messages", messagesSvc.HandleListThreads)
		providerGroup.GET("/messages/:threadId", messagesSvc.HandleGetThread)
		providerGroup.POST("/messages/:threadId", messagesSvc.HandleSendMessage, idempotencyMW, rateLimitMW)
		providerGroup.POST("/messages/:threadId/read", messagesSvc.HandleMarkRead, idempotencyMW)

		// Provider blocks
		providerGroup.POST("/blocks/:customerId", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			var customerUserID string
			err := db.QueryRowContext(c.Request().Context(),
				"SELECT user_id FROM customer_profiles WHERE id=$1", c.Param("customerId")).Scan(&customerUserID)
			if err != nil {
				return httpx.NewNotFoundError("Customer not found.")
			}
			if err := blocksSvc.Block(c.Request().Context(), user.ID, customerUserID); err != nil {
				return err
			}
			return c.JSON(http.StatusCreated, map[string]interface{}{"message": "User blocked."})
		}, idempotencyMW)
		providerGroup.DELETE("/blocks/:customerId", func(c echo.Context) error {
			user := auth.UserFromContext(c)
			var customerUserID string
			err := db.QueryRowContext(c.Request().Context(),
				"SELECT user_id FROM customer_profiles WHERE id=$1", c.Param("customerId")).Scan(&customerUserID)
			if err != nil {
				return httpx.NewNotFoundError("Customer not found.")
			}
			if err := blocksSvc.Unblock(c.Request().Context(), user.ID, customerUserID); err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"message": "User unblocked."})
		}, idempotencyMW)

		adminGroup := v1.Group("/admin", authSvc.RequireAuth(), auth.RequireRole(auditSvc, "administrator"), writeOnlyRateLimitMW)

		// Admin routes
		adminGroup.GET("/categories", catalogSvc.HandleAdminListCategories)
		adminGroup.POST("/categories", catalogSvc.HandleAdminCreateCategory, idempotencyMW)
		adminGroup.PATCH("/categories/:categoryId", catalogSvc.HandleAdminUpdateCategory, idempotencyMW)
		adminGroup.GET("/tags", catalogSvc.HandleAdminListTags)
		adminGroup.POST("/tags", catalogSvc.HandleAdminCreateTag, idempotencyMW)
		adminGroup.PATCH("/tags/:tagId", catalogSvc.HandleAdminUpdateTag, idempotencyMW)

		// Admin analytics
		adminGroup.GET("/analytics/user-growth", analyticsSvc.HandleUserGrowth)
		adminGroup.GET("/analytics/conversion", analyticsSvc.HandleConversion)
		adminGroup.GET("/analytics/provider-utilization", analyticsSvc.HandleProviderUtilization)
		adminGroup.POST("/analytics/rollup", analyticsSvc.HandleGenerateRollups, idempotencyMW)

		// Admin exports
		adminGroup.POST("/exports", analyticsSvc.HandleCreateExport, idempotencyMW)
		adminGroup.GET("/exports", analyticsSvc.HandleListExports)
		adminGroup.GET("/exports/:exportId", analyticsSvc.HandleGetExport)
		adminGroup.GET("/exports/:exportId/download", analyticsSvc.HandleDownloadExport)

		// Admin search config
		adminGroup.GET("/search-config/hot-keywords", catalogSvc.HandleAdminListHotKeywords)
		adminGroup.POST("/search-config/hot-keywords", catalogSvc.HandleAdminCreateHotKeyword, idempotencyMW)
		adminGroup.PATCH("/search-config/hot-keywords/:keywordId", catalogSvc.HandleAdminUpdateHotKeyword, idempotencyMW)
		adminGroup.GET("/search-config/autocomplete", catalogSvc.HandleAdminListAutocomplete)
		adminGroup.POST("/search-config/autocomplete", catalogSvc.HandleAdminCreateAutocomplete, idempotencyMW)
		adminGroup.PATCH("/search-config/autocomplete/:termId", catalogSvc.HandleAdminUpdateAutocomplete, idempotencyMW)

		// Alert rules and alerts
		alertsSvc := alerts.NewService(db, auditSvc)
		adminGroup.GET("/alert-rules", alertsSvc.HandleListRules)
		adminGroup.POST("/alert-rules", alertsSvc.HandleCreateRule, idempotencyMW)
		adminGroup.PATCH("/alert-rules/:ruleId", alertsSvc.HandleUpdateRule, idempotencyMW)

		// On-call schedules
		adminGroup.GET("/on-call", alertsSvc.HandleListOnCall)
		adminGroup.POST("/on-call", alertsSvc.HandleCreateOnCall, idempotencyMW)

		adminGroup.GET("/alerts", alertsSvc.HandleListAlerts)
		adminGroup.GET("/alerts/:alertId", alertsSvc.HandleGetAlert)
		adminGroup.POST("/alerts/:alertId/assign", alertsSvc.HandleAssignAlert, idempotencyMW)
		adminGroup.POST("/alerts/:alertId/acknowledge", alertsSvc.HandleAcknowledgeAlert, idempotencyMW)

		// Work orders
		workordersSvc := workorders.NewService(db, auditSvc)
		adminGroup.POST("/work-orders", workordersSvc.HandleCreate, idempotencyMW)
		adminGroup.GET("/work-orders", workordersSvc.HandleList)
		adminGroup.GET("/work-orders/:workOrderId", workordersSvc.HandleGet)
		adminGroup.POST("/work-orders/:workOrderId/dispatch", workordersSvc.HandleDispatch, idempotencyMW)
		adminGroup.POST("/work-orders/:workOrderId/acknowledge", workordersSvc.HandleAcknowledge, idempotencyMW)
		adminGroup.POST("/work-orders/:workOrderId/start", workordersSvc.HandleStart, idempotencyMW)
		adminGroup.POST("/work-orders/:workOrderId/resolve", workordersSvc.HandleResolve, idempotencyMW)
		adminGroup.POST("/work-orders/:workOrderId/post-incident-review", workordersSvc.HandlePostIncidentReview, idempotencyMW)
		adminGroup.POST("/work-orders/:workOrderId/close", workordersSvc.HandleClose, idempotencyMW)
		adminGroup.POST("/work-orders/:workOrderId/evidence", workordersSvc.HandleUploadEvidence, idempotencyMW)
		adminGroup.GET("/work-orders/:workOrderId/evidence", workordersSvc.HandleListEvidence)

		// Document checksum verification
		adminGroup.POST("/documents/:documentId/verify-checksum", func(c echo.Context) error {
			docID := c.Param("documentId")
			if err := uploadsSvc.VerifyChecksum(c.Request().Context(), docID); err != nil {
				return httpx.NewAPIError(http.StatusConflict, "checksum_mismatch", err.Error())
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"message": "Checksum verified.", "status": "ok"})
		})
		adminGroup.POST("/evidence/:evidenceId/verify-checksum", func(c echo.Context) error {
			evID := c.Param("evidenceId")
			if err := uploads.VerifyEvidenceChecksum(c.Request().Context(), db, auditSvc, evID); err != nil {
				return httpx.NewAPIError(http.StatusConflict, "checksum_mismatch", err.Error())
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"message": "Evidence checksum verified.", "status": "ok"})
		})
	}

	return e
}

// getBlockedUserIDs returns all user IDs that have a block relationship with the
// given user (both directions).
func getBlockedUserIDs(ctx context.Context, db *sql.DB, userID string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT blocked_id FROM blocks WHERE blocker_id=$1 UNION SELECT blocker_id FROM blocks WHERE blocked_id=$1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// writeOnlyRateLimit wraps a rate limiter so it only counts write requests
// (POST, PATCH, PUT, DELETE). GET and other read methods pass through freely.
func writeOnlyRateLimit(rl *httpx.RateLimiter, keyFn func(c echo.Context) string) echo.MiddlewareFunc {
	inner := rl.Middleware(keyFn)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := c.Request().Method
			if method == "POST" || method == "PATCH" || method == "PUT" || method == "DELETE" {
				return inner(next)(c)
			}
			return next(c)
		}
	}
}
