# FieldServe Implementation Plan

## 1. Objective

Deliver an entirely offline/local service-marketplace platform with:

- React role-based portals for Administrators, Service Providers, and Customers
- Go + Echo REST APIs
- PostgreSQL persistence and search support
- Local caching, local file storage, local audit storage, and local scheduled jobs
- Real security, audit, anti-abuse, analytics, and alert-management behavior rather than mocked shortcuts

## 2. Non-negotiable requirements

### Product
- Hierarchical service catalog with controlled tags
- Multi-field search with fuzzy matching
- Filters: keyword, price range (USD), service area radius (miles), availability dates, rating
- Sorting: newest, price, distance, popularity
- Pagination
- Favorites
- Compare up to 3 services
- Search history
- Trending recommendations from locally aggregated activity
- Interest workflow: submitted -> accepted/declined
- In-app messaging with read receipts
- Duplicate-interest prevention: max 1 active interest per customer/provider within 7 days
- Two-way blocking with immediate hiding and message stop, but prior records retained

### Backend / Ops / Security
- Echo REST APIs with RBAC enforcement
- PostgreSQL for transactional and search-support data
- Query-result cache with 10-minute TTL + LRU eviction
- Cached-query response target under 300 ms
- Audit log for logins, permission changes, service publishing, exports, and API access
- Immutable append-only audit storage with daily rotation
- AES-256 at-rest encryption for sensitive fields such as phone numbers and notes
- Rate limit: 60 requests/minute/account
- Idempotency token window: 5 minutes
- Upload validation: allowlisted MIME + extension, 10 MB max, checksum fingerprinting, executable rejection
- Admin dashboards: user growth, search-to-interest conversion, provider utilization, time filters, CSV export
- Exports restricted to Administrators and always logged
- Alert Center with scheduled evaluation, quiet hours, severity, escalation, SLA timers, dispatch-to-resolution workflow, post-incident review, 180-day evidence retention

## 3. Architecture decisions

## 3.1 Runtime model
- Primary runtime command: `docker compose up --build`
- Broad test command: `./run_tests.sh`
- DB bootstrap command: `./init_db.sh`
- Entire stack runs locally with Docker services for:
  - frontend
  - api
  - worker
  - postgres

## 3.2 Repo/runtime constraints
- No checked-in `.env` files
- No hardcoded secrets
- No hardcoded database bootstrap credentials in repo logic
- A dev-only bootstrap script generates runtime-only local values at container start and injects them into containers/volumes
- `README.md` inside `repo/` is the only repo-local doc and must explain runtime/test flow clearly

## 3.3 Frontend stack
- React + TypeScript + Vite
- React Router for nested portal routing
- TanStack Query for API data caching/invalidation
- Tailwind CSS + `shadcn/ui` primitives for fast, consistent portal UX
- Zustand or a thin context layer only for local UI-only state (compare tray, filters draft, auth shell state)

Why:
- React Router supports nested role-based layouts and portal separation cleanly
- TanStack Query fits cached REST reads plus optimistic/rollback mutation flows for favorites, messaging, and interest actions
- Vite keeps local development fast in a Dockerized offline setup

## 3.4 Backend stack
- Go 1.22+
- Echo route groups and middleware for API versioning, auth, RBAC, throttling, idempotency, and audit hooks
- PostgreSQL with:
  - relational tables for core entities
  - `pg_trgm` for fuzzy search support
  - full-text search support where useful for keyword/autocomplete ranking
- Local disk volumes for:
  - provider uploads
  - CSV exports
  - append-only audit files
  - alert/work-order evidence

Why:
- Echo route groups and middleware fit `/api/v1`, `/admin`, `/provider`, `/customer` boundary enforcement well
- PostgreSQL can satisfy both transactional consistency and local fuzzy search/indexing without external services

## 4. System overview

## 4.1 Frontend portals

### Customer portal
- Service discovery home
- Search results + filters + compare tray
- Service detail
- Favorites
- Interest tracker
- Messaging inbox
- Search history / trending recommendations
- Blocked relationships view

### Provider portal
- Service management CRUD
- Availability management
- Interest inbox and decisioning
- Messaging inbox
- Upload center for provider documents
- Block list management
- Performance summary widgets

### Administrator portal
- User/account management
- Category/tag taxonomy management
- Search configuration management for autocomplete terms and hot keywords
- Permission and security oversight
- Audit log viewer
- Export center
- Analytics dashboards
- Alert Center and work-order operations

## 4.2 Backend services/modules
- Auth + RBAC module
- Service catalog/search module
- Interest + messaging module
- Favorites + compare/history/recommendations module
- Blocking/compliance module
- Upload validation/storage module
- Analytics/export module
- Audit/security module
- Alert/work-order scheduler module

## 5. Domain model

## 5.1 Core tables
- `users`
- `roles`
- `user_roles`
- `customer_profiles`
- `provider_profiles`
- `admin_profiles`
- `categories` (self-referential hierarchy)
- `tags`
- `services`
- `service_tags`
- `service_availability_windows`
- `service_media` (optional later if prompt-safe)
- `favorites`
- `interests`
- `interest_status_events`
- `messages`
- `message_receipts`
- `blocks`
- `provider_documents`
- `search_keyword_config`
- `autocomplete_terms`
- `auth_sessions`
- `idempotency_keys`
- `audit_event_index`
- `search_events`
- `search_history`
- `recommendation_snapshots`
- `analytics_daily_rollups`
- `export_jobs`
- `alert_rules`
- `alerts`
- `alert_assignments`
- `work_orders`
- `work_order_events`
- `work_order_evidence`

## 5.2 Sensitive/encrypted fields
- phone numbers
- private notes
- document metadata notes
- internal alert/work-order notes where sensitive

Encrypted at application level with AES-256 before DB write; only masked values are returned to standard UI surfaces.

## 5.3 Key state machines

### Interest
- `submitted`
- `accepted`
- `declined`
- `withdrawn` (safe extension for customer-side closure)

Rule: only one active interest (`submitted` or `accepted`) per customer/provider pair within 7 days.

### Message receipt
- `sent`
- `delivered`
- `read`

### Work order
- `new`
- `dispatched`
- `acknowledged`
- `in_progress`
- `resolved`
- `post_incident_review`
- `closed`

## 6. Search and discovery design

## 6.1 Query pipeline
1. Normalize incoming search params
2. Build cache key from normalized filters/sort/page
3. Check in-memory LRU cache (10-minute TTL)
4. On miss, query PostgreSQL using indexed filters and trigram/full-text ranking
5. Store result page payload and aggregate metadata in cache
6. Log search event for analytics and trending

## 6.2 Search storage/indexing
- GIN/GIST support for text search fields where appropriate
- `pg_trgm` indexes on service title, provider name, tag text, category labels
- B-tree indexes for price, rating, created_at, popularity counters
- Availability filtering through indexed time-window joins
- Service area filtering via stored provider/service radius plus locality metadata; if geocoordinates are available, add point-distance support later without changing API shape

## 6.3 Performance strategy
- Debounced frontend search requests
- Stable query key normalization to maximize cache hits
- Page-sized cached payloads
- Warm-cache response budget under 300 ms
- Periodic cache invalidation on service publish/update, favorites/interest activity affecting popularity, and taxonomy changes

## 6.4 Trending recommendations
- Weighted local signals:
  - searches
  - result clicks/profile views
  - favorites
  - compare actions
  - submitted interests
- Time-window decay favors recent activity
- Daily and rolling-short-window aggregates feed recommendation cards

## 7. Frontend architecture

## 7.1 Route map
- `/login`
- `/customer/*`
- `/provider/*`
- `/admin/*`
- shared routes for unauthorized / blocked / not found

Each portal gets its own layout shell, nav, route guard, and permission-aware menu.

## 7.2 Frontend module breakdown

### Shell and auth
- app bootstrap
- router
- auth/session store
- role-aware route guards
- common API client + error normalization

### Discovery experience
- search page
- filters panel
- result cards/grid
- compare drawer (max 3)
- favorites actions
- search history / trending modules

### Engagement flows
- interest submission form
- interest status timeline
- messaging thread list/detail
- read receipt indicators
- blocking controls and blocked-state UX

### Provider workspace
- service CRUD forms
- availability editor
- provider document uploader
- interest decision screen

### Admin workspace
- dashboard widgets and charts
- taxonomy management
- audit log table
- export jobs UI
- alert center / work-order console

## 7.3 Required UI states
For each prompt-critical flow define and implement:
- loading
- empty
- submitting
- disabled
- success
- inline validation error
- backend error
- duplicate-action prevention
- blocked-state message

## 7.4 Comparison feature
- Local compare tray persisted in local storage
- Strict cap at 3 services
- Shared comparison schema: price, rating, distance/service area, category, availability, provider summary, tags

## 8. Backend architecture

## 8.1 API route groups
- `/api/v1/auth`
- `/api/v1/catalog`
- `/api/v1/customer`
- `/api/v1/provider`
- `/api/v1/admin`
- `/api/v1/system`

## 8.2 Middleware chain
- request ID
- structured request logging with redaction
- auth/session resolution
- RBAC / object authorization
- rate limiting (60 RPM/account)
- idempotency-token enforcement on write endpoints
- audit event emission
- normalized error handling

## 8.3 Core backend packages
- `cmd/api`
- `cmd/worker`
- `internal/auth`
- `internal/rbac`
- `internal/catalog`
- `internal/search`
- `internal/interests`
- `internal/messages`
- `internal/favorites`
- `internal/blocks`
- `internal/uploads`
- `internal/analytics`
- `internal/audit`
- `internal/alerts`
- `internal/workorders`
- `internal/platform/cache`
- `internal/platform/crypto`
- `internal/platform/httpx`
- `internal/platform/storage`
- `internal/platform/scheduler`

## 8.4 Error contract
Use one normalized JSON error shape:

```json
{
  "error": {
    "code": "duplicate_interest",
    "message": "You already have an active interest with this provider.",
    "field_errors": {
      "providerId": ["Only one active interest is allowed within 7 days."]
    },
    "request_id": "..."
  }
}
```

This keeps inline frontend feedback predictable.

## 8.5 Authentication/session contract
- Use server-side local sessions with opaque session IDs stored in `auth_sessions`
- Deliver the session identifier in an `HttpOnly`, `SameSite=Strict`, secure cookie in production-like local runtime; use local-safe dev cookie settings when HTTPS is not present during development
- Frontend runtime proxies API requests to the backend so auth remains same-origin by default
- Session policy:
  - idle timeout: 8 hours
  - absolute lifetime: 7 days
  - explicit logout invalidates the server-side session immediately
  - role/permission changes invalidate affected sessions
- Every login, logout, failed login, bootstrap-admin action, and privilege-escalation attempt produces an audit event
- Frontend keeps only derived auth state in memory and never stores raw session tokens in local storage

## 9. Security and compliance plan

## 9.1 Authentication / authorization
- Local username/password auth with seeded dev accounts for each role and a bootstrap path for the first admin
- Password hashing with Argon2id or bcrypt (final choice during scaffold)
- Role checks at route level plus object ownership checks in service layer
- Admin-only endpoints isolated under dedicated route groups
- Privilege-escalation attempts logged and flagged

## 9.2 Sensitive data handling
- AES-256 application-layer encryption for sensitive persisted fields
- Redacted logging; never log plaintext phone numbers, notes, auth secrets, or token values
- Masked UI rendering for sensitive values by role

## 9.3 Anti-abuse controls
- Duplicate-interest constraint in service layer + DB uniqueness/support table logic
- 5-minute idempotency token storage and replay protection
- 60 RPM/account limiter
- Two-way block checks injected into search visibility, profile fetch, interest submission, and messaging send flows

## 9.4 Upload safety
- MIME sniffing + extension allowlist
- file size cap enforcement before persistence
- SHA-256 checksum capture
- executable-type denylist
- quarantine/reject path for invalid uploads

## 10. Audit and observability plan

## 10.1 Audit model
- Dual-path audit design:
  - append-only daily rotated file as immutable source of record
  - indexed summary rows in PostgreSQL for admin browsing/filtering
- Every authenticated API request emits an `api_access` audit event summary with request ID, actor ID, route, method, result class, and redacted metadata
- Rotation creates a new daily file and seals the previous file in read-only mode for the app write path

## 10.2 Audit event categories
- authentication
- api_access
- permission change
- service lifecycle
- export lifecycle
- admin access
- suspicious access / privilege escalation
- alert/work-order actions

## 10.3 Logging rules
- Structured JSON logs
- request IDs on every request
- audit logs separate from application logs
- redaction helpers centralized in shared package

## 11. Analytics and alert-center plan

## 11.1 Analytics
- Daily rollups plus ad hoc filtered reads
- Dashboards for:
  - user growth
  - search -> interest conversion
  - provider utilization
- CSV exports written to local export directory and downloaded through admin-only flow

## 11.1.1 Search configuration surface
- Admins can manage locally stored autocomplete terms and hot keywords from the admin portal
- Config changes are persisted in PostgreSQL, invalidate search caches, and emit audit events
- Catalog read endpoints only consume active configuration; write access is admin-only

## 11.2 Scheduled jobs
Worker process handles:
- trending refresh
- analytics rollups
- alert threshold evaluation
- SLA deadline checks
- evidence retention pruning after 180 days where permitted
- audit rotation coordination hooks if needed

## 11.3 Alert workflow
- rules are configurable by admins
- quiet-hour aware evaluation
- severity assignment
- on-call assignment inside app
- work-order creation/dispatch
- resolution and post-incident review tracking

## 12. File/storage design

## 12.1 Local directories/volumes
- `uploads/`
- `exports/`
- `audit/`
- `evidence/`

Backed by Docker volumes so data survives container restarts.

## 12.2 Compliance retention
- blocked relationships preserve prior records
- message/history records preserved for compliance review
- evidence retained 180 days

## 13. Verification and test strategy

## 13.1 Backend tests
- unit tests for business rules and validators
- repository/integration tests against PostgreSQL
- HTTP tests for auth/RBAC/error contracts
- scheduler tests for alert and retention logic

## 13.2 Frontend tests
- component tests for filter controls, compare tray, inline validation, blocked states
- page/route integration tests for customer/provider/admin flows
- Playwright E2E for synchronized frontend/backend paths

## 13.3 Required high-risk coverage
- 401, 403, 404
- duplicate interest conflict
- blocked-user visibility and messaging denial
- export admin-only enforcement
- idempotency replay handling
- rate-limit enforcement
- encryption-at-rest verification
- audit append-only + daily rotation verification
- API access audit verification
- privilege-escalation detection verification
- audit redaction
- search filter/sort/pagination correctness
- read receipts
- alert escalation/SLA timing

See `docs/test-coverage.md` for the matrix.

## 14. API and integration planning notes
- Keep endpoint shapes REST-style and role-scoped
- Put prompt-critical contracts in `docs/api-spec.md`
- Avoid frontend invention of backend state names; frontend must consume the backend lifecycle literals directly

## 15. Delivery slices

## Slice 1: Scaffold and platform foundation
- Docker Compose, frontend app shell, Echo API shell, worker shell, PostgreSQL, `./init_db.sh`, `./run_tests.sh`, bootstrap path, base README

## Slice 2: Auth, RBAC, and role portals
- login, seeded roles, route guards, middleware, audit hooks, privilege-escalation detection baseline

## Slice 3: Catalog, taxonomy, and provider service management
- categories, tags, service CRUD, availability, provider workspace, admin taxonomy screens

## Slice 4: Search/discovery experience
- fuzzy search, filters, sorting, pagination, search cache, history, trending, compare, favorites

## Slice 5: Interest + messaging workflow
- interest creation/status, duplicate protection, in-app messaging, read receipts, block rules

## Slice 6: Uploads, analytics, and exports
- provider docs, validation pipeline, dashboards, CSV export, export audit trail

## Slice 7: Alert center and work orders
- rules, scheduled evaluation, quiet hours, escalation, SLA timers, evidence retention, post-incident review

## Slice 8: Hardening
- security review, performance tuning, redaction review, README finalization, end-to-end verification

## 16. Definition of done

Planning is only considered implemented correctly when the delivered system has:
- real role-separated UI surfaces
- real backend enforcement for RBAC/object ownership
- real duplicate/blocked/idempotent behavior
- real offline-local audit/export/alert behavior
- documented Docker-first runtime and `./run_tests.sh`
- strong automated coverage across the critical flows and failure paths
