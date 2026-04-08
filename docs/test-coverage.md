# FieldServe Test Coverage Matrix — Final State

## Coverage summary
- **201 automated tests** (129 backend, 72 frontend)
- Broad test entrypoint: `./run_tests.sh`
- Backend: Go integration tests against real PostgreSQL + unit tests for cache/crypto/rate-limiter
- Frontend: Vitest with jsdom, mocked API layer, React Testing Library

## 1. Auth / RBAC / security boundaries

| Requirement | Test file(s) | Status | Notes |
| --- | --- | --- | --- |
| Login succeeds for seeded accounts | `auth_test.go:TestLoginSuccess` | Covered | All 3 seeded roles |
| Login failure returns 401 | `auth_test.go:TestLoginFailure` | Covered | |
| GET /auth/me with session | `auth_test.go:TestMeWithSession` | Covered | |
| GET /auth/me without session → 401 | `auth_test.go:TestMeWithoutSession` | Covered | |
| Logout invalidates session | `auth_test.go:TestLogout` | Covered | |
| Protected route rejects unauthenticated | `auth_test.go:TestProtectedRouteUnauthenticated` | Covered | |
| Admin route rejects non-admin + audit | `auth_test.go:TestAdminRouteAsCustomer` | Covered | Privilege escalation event verified |
| Frontend route guards | `App.test.tsx` | Covered | Redirect to /login, 403 for wrong role |
| Frontend login flow | `login.test.tsx` | Covered | Error display, redirect |

## 2. Catalog / taxonomy / search

| Requirement | Test file(s) | Status | Notes |
| --- | --- | --- | --- |
| Admin category CRUD | `catalog_test.go:TestAdminCreate/UpdateCategory` | Covered | |
| Admin tag CRUD | `catalog_test.go:TestAdminCreate/UpdateTag` | Covered | |
| Provider service CRUD | `catalog_test.go:TestProviderCreate/List/Update/Delete` | Covered | |
| Provider ownership enforcement | `catalog_test.go:TestProviderCannotMutateOtherService` | Covered | Returns 404 |
| Inactive service edit | `catalog_test.go:TestProviderGetOwnInactiveService` | Covered | |
| Availability management | `catalog_test.go:TestProviderSetAvailability` | Covered | |
| Fuzzy search (pg_trgm) | `search_test.go:TestFuzzySearch` | Covered | Typo "plumbng" matches |
| Search filters | `search_test.go:TestSearchFilters` | Covered | Price filter |
| Search sorting | `search_test.go:TestSearchSorting` | Covered | price_asc verified |
| Distance sorting | `search_test.go:TestSearchSortingDistance` | Covered | service_area_miles ASC ordering |
| Availability date filter | `search_test.go:TestSearchAvailabilityDateFilter` | Covered | Real YYYY-MM-DD date → weekday resolution, correct service matched |
| Availability date + time filter | `search_test.go:TestSearchAvailabilityDateWithTime` | Covered | Date + HH:MM time constraint, match/no-match/wrong-day |
| Frontend date-based availability | `search.test.tsx` | Covered | Date picker sends `available_date`, time filter sends `available_time` |
| Hot-keywords requires auth | `search_test.go:TestHotKeywordsRequiresAuth` | Covered | 401 for unauthenticated |
| Autocomplete requires auth | `search_test.go:TestAutocompleteRequiresAuth` | Covered | 401 for unauthenticated |
| Search pagination | `search_test.go:TestSearchPagination` | Covered | page_size + total |
| Search history | `search_test.go:TestSearchHistory` | Covered | |
| Favorites add/remove/no-dup | `search_test.go:TestFavorites*` | Covered | |
| Trending | `search_test.go:TestTrending` | Covered | |
| Hot keywords CRUD | `search_test.go:TestHotKeywordsAdminCRUD` | Covered | |
| Autocomplete CRUD | `search_test.go:TestAutocompleteCRUD` | Covered | |
| Cache behavior | `search_test.go:TestCacheBehavior` | Covered | Hit/invalidation verified |
| Field error rendering | `catalog_test.go:TestFieldErrorRendering` | Covered | 422 field_errors object map |
| Frontend search/filter/favorites/compare | `search.test.tsx` | Covered | 19 tests (incl. distance sort, date availability filter) |
| Frontend catalog management | `catalog.test.tsx` | Covered | 5 tests |

## 3. Interests / messaging / blocking

| Requirement | Test file(s) | Status | Notes |
| --- | --- | --- | --- |
| Interest submit | `engagement_test.go:TestCustomerSubmitInterest` | Covered | |
| Duplicate interest → 409 | `engagement_test.go:TestDuplicateInterestRule` | Covered | field_errors verified |
| Withdraw | `engagement_test.go:TestCustomerWithdraw` | Covered | |
| Provider accept/decline | `engagement_test.go:TestProviderAccept/Decline` | Covered | |
| Interest object authorization | `engagement_test.go:TestInterestObjectAuthorization` | Covered | |
| Message send | `engagement_test.go:TestMessageSend` | Covered | |
| Message read receipts | `engagement_test.go:TestMessageRead` | Covered | |
| Blocked message send → 403 | `engagement_test.go:TestBlockedMessageSend` | Covered | |
| Blocked interest submit → 403 | `engagement_test.go:TestBlockedInterestSubmit` | Covered | |
| Blocked provider hidden from search | `engagement_test.go:TestBlockedProviderHiddenFromSearch` | Covered | |
| Blocked service detail → 404 | `engagement_test.go:TestBlockedServiceDetailAccess` | Covered | |
| Idempotency interest | `engagement_test.go:TestIdempotencyInterest` | Covered | Same key → same response |
| Idempotency message | `engagement_test.go:TestIdempotencyMessage` | Covered | |
| Idempotency scoping | `engagement_test.go:TestIdempotencyScopedByUserAndPath` | Covered | Cross-path non-collision |
| Idempotency concurrent same-key | `engagement_test.go:TestIdempotencyConcurrentSameKey` | Covered | 5 concurrent sends → 1 message created |
| Service-provider mismatch rejected | `engagement_test.go:TestInterestRejectsMismatchedServiceProvider` | Covered | 422 with field_errors.service_id |
| Rate limiting | `engagement_test.go:TestRateLimiting` | Covered | |
| Frontend engagement flow | `engagement.test.tsx` | Covered | 10 tests including full workflow |

## 4. Uploads / analytics / exports

| Requirement | Test file(s) | Status | Notes |
| --- | --- | --- | --- |
| Valid upload succeeds | `uploads_test.go:TestValidUpload` | Covered | Metadata + checksum |
| Executable rejected → 415 | `uploads_test.go:TestExecutableRejected` | Covered | |
| Oversized rejected → 413 | `uploads_test.go:TestOversizedRejected` | Covered | |
| Disallowed extension → 415 | `uploads_test.go:TestDisallowedExtension` | Covered | |
| Path confinement | `uploads_test.go:TestStoragePathConfinement` | Covered | 4 traversal filenames |
| Document list/delete | `uploads_test.go:TestDocumentListDelete` | Covered | |
| Analytics user growth | `uploads_test.go:TestAnalyticsUserGrowth` | Covered | |
| Analytics conversion | `uploads_test.go:TestAnalyticsConversion` | Covered | |
| Analytics utilization | `uploads_test.go:TestAnalyticsProviderUtilization` | Covered | |
| Non-admin export → 403 | `uploads_test.go:TestNonAdminExportRejected` | Covered | |
| Export creation + CSV | `uploads_test.go:TestExportCreation` | Covered | |
| Export audit event | `uploads_test.go:TestExportAuditEvent` | Covered | |
| API access audit | `uploads_test.go:TestAPIAccessAudit` | Covered | |
| Frontend uploads/analytics/exports | `operations.test.tsx` | Covered | 6 tests |

## 5. Alert center / work orders

| Requirement | Test file(s) | Status | Notes |
| --- | --- | --- | --- |
| Alert rule create/update | `alerts_test.go:TestAlertRuleCreate/Update` | Covered | |
| Unsupported metric → 422 | `alerts_test.go:TestUnsupportedMetricRejected*` | Covered | Create + update |
| Rule evaluation | `alerts_test.go:TestAlertEvaluation` | Covered | threshold=0 fires |
| Quiet hours suppress | `alerts_test.go:TestQuietHoursSuppress` | Covered | Medium suppressed |
| Critical bypasses quiet hours | `alerts_test.go:TestQuietHoursCriticalNotSuppressed` | Covered | |
| Alert assign | `alerts_test.go:TestAlertAssign` | Covered | |
| Alert acknowledge | `alerts_test.go:TestAlertAcknowledge` | Covered | |
| Work order full lifecycle | `alerts_test.go:TestWorkOrderFullLifecycle` | Covered | 7 transitions |
| Invalid WO transition → 422 | `alerts_test.go:TestInvalidWorkOrderTransition` | Covered | |
| SLA overdue check | `alerts_test.go:TestSLAOverdueCheck` | Covered | 48h old → critical |
| Evidence upload + retention | `alerts_test.go:TestEvidenceUpload` | Covered | 180-day verified |
| Evidence rejected extension | `alerts_test.go:TestEvidenceRejectedExtension` | Covered | .exe → 415 |
| Admin-only enforcement | `alerts_test.go:TestAdminOnlyAlertEndpoints` | Covered | Customer → 403 |
| Frontend alert/WO flow | `alerting.test.tsx` | Covered | 7 tests including full lifecycle |
| Frontend on-call assignment UI | `alerting.test.tsx` | Covered | On-call select dropdown populated |

## 6. Hardening / security

| Requirement | Test file(s) | Status | Notes |
| --- | --- | --- | --- |
| AES-256 encrypt/decrypt | `crypto_test.go` | Covered | 7 unit tests (incl. MaskPhone, MaskNote) |
| Customer phone encrypted at rest | `hardening_test.go:TestEncryptedCustomerPhone` | Covered | Real PATCH/GET path, DB has ciphertext, read returns plaintext |
| Provider phone encrypted at rest | `hardening_test.go:TestEncryptedProviderPhone` | Covered | Real PATCH/GET path, DB has ciphertext, read returns plaintext |
| Customer notes encrypted at rest | `hardening_test.go:TestEncryptedCustomerNotes` | Covered | Real PATCH/GET path, DB ciphertext verified, masked response |
| Provider notes encrypted at rest | `hardening_test.go:TestEncryptedProviderNotes` | Covered | Real PATCH/GET path, DB ciphertext verified |
| Customer profile returns masked phone | `hardening_test.go:TestCustomerProfileReturnsMaskedPhone` | Covered | Phone returned as `***XXXX` not plaintext |
| Provider profile returns masked phone | `hardening_test.go:TestProviderProfileReturnsMaskedPhone` | Covered | Phone returned as `***XXXX` not plaintext |
| On-call rejects non-eligible assignee | `hardening_test.go:TestAlertAssignRejectsNonOnCallUser` | Covered | 422 without on-call schedule |
| On-call allows eligible assignee | `hardening_test.go:TestAlertAssignSucceedsWithOnCallSchedule` | Covered | 200 with active on-call |
| Idempotency on provider service create | `hardening_test.go:TestIdempotencyProviderServiceCreate` | Covered | Same key → same response, 1 DB row |
| Document checksum verification (pass) | `hardening_test.go:TestDocumentChecksumVerification` | Covered | Verify OK then tamper → 409 |
| Document checksum mismatch detection | `hardening_test.go:TestDocumentChecksumVerification` | Covered | Tampered file detected |
| Cached-query 300ms SLA | `hardening_test.go:TestCachedQueryPerformance` | Covered | 10 iterations, avg < 300ms |
| On-call auto-assign to lowest tier | `hardening_test.go:TestAlertAutoAssignLowestTier` | Covered | Alert created by rule eval → auto-assigned to tier 1 |
| On-call escalation to next tier | `hardening_test.go:TestEscalateUnacknowledgedToNextTier` | Covered | Unacked 45-min assignment → escalated to tier 2 |
| On-call list returns only active | `hardening_test.go:TestOnCallListReturnsOnlyActive` | Covered | Expired schedule filtered out |
| Idempotency on WO dispatch | `hardening_test.go:TestIdempotencyWorkOrderDispatch` | Covered | Same key → replay, 1 dispatch event |
| Idempotency on alert assign | `hardening_test.go:TestIdempotencyAlertAssign` | Covered | Same key → replay, 1 assignment row |
| Idempotency on profile PATCH | `hardening_test.go:TestIdempotencyCustomerProfilePatch` | Covered | Same key → replay, same response |
| Idempotency on service PATCH | `hardening_test.go:TestIdempotencyServiceUpdate` | Covered | Same key → replay, same response |
| Idempotency on category create | `hardening_test.go:TestIdempotencyAdminCategoryCreate` | Covered | Same key → replay, 1 DB row |
| Audit file rotation + sealing | `hardening_test.go:TestAuditFileRotationAndSealing` | Covered | Controllable time, yesterday sealed read-only, today writable |
| Session cleanup (real function) | `hardening_test.go:TestSessionCleanupReal` | Covered | Uses `cleanup.ExpiredSessions()`, verifies deletion + idempotent rerun |
| Idempotency cleanup (real function) | `hardening_test.go:TestIdempotencyCleanupReal` | Covered | Uses `cleanup.ExpiredIdempotencyKeys()`, verifies deletion |
| Evidence cleanup (real function + file) | `hardening_test.go:TestEvidenceCleanupReal` | Covered | Uses `cleanup.ExpiredEvidence()`, file + DB row both removed |
| Cookie Secure default (HTTP) | `hardening_test.go:TestCookieSecureDefault` | Covered | Secure=false on plain HTTP |
| Cookie Secure explicit true | `hardening_test.go:TestCookieSecureExplicitTrue` | Covered | COOKIE_SECURE=true → Secure=true |
| Cookie Secure auto with HTTPS | `hardening_test.go:TestCookieSecureAutoWithForwardedProto` | Covered | X-Forwarded-Proto=https → Secure=true |
| Rate limit on write routes | `hardening_test.go:TestRateLimitOnAdminWriteRoute` | Covered | Admin POST not 429 under limit |
| LRU cache | `cache_test.go` | Covered | 6 unit tests |
| Rate limiter | `ratelimit_test.go` | Covered | 2 unit tests |

## Remaining known gaps

| Area | Gap | Risk |
| --- | --- | --- |
| Playwright/browser E2E | No browser tests; covered by route/page integration tests with mocked APIs | Low — UI behavior verified through Testing Library |
| Concurrent access stress | Idempotency concurrent test added; no broader load tests | Low — idempotency race fixed and tested; rate limiter and cache are thread-safe |
| Evidence retention pruning under load | Tested with single expired file | Low — same DELETE logic regardless of count |
| Audit file backup/archival | Not tested — operational concern | Low — files persist in Docker volume |
| Cached-query SLA under load | Tested locally with single-user warm cache; no multi-user load test | Low — LRU cache is thread-safe; the 300ms target is a local dataset budget |
| On-call schedule overlap validation | No constraint preventing overlapping schedules for same user | Low — admin-managed, UI-enforced |
