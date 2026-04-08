# FieldServe

Offline-first local service marketplace with role-based portals for Administrators, Service Providers, and Customers.

## Stack

| Layer | Technology |
|-------|-----------|
| Frontend | React 18, TypeScript, Vite, React Router, TanStack Query, Zustand, Tailwind CSS |
| API | Go 1.22, Echo v4, bcrypt |
| Worker | Go 1.22 (background job runner) |
| Database | PostgreSQL 16 with pg_trgm for fuzzy search |
| Runtime | Docker Compose |

## Quick start

```bash
docker compose up --build
```

This is the primary runtime command. On first run a `bootstrap` init container generates ephemeral dev secrets into a Docker volume. No `.env` file is created anywhere in the repo tree. Secrets persist in the `secrets` volume across restarts and are cleared by `docker compose down -v`.

Host ports are bound to `127.0.0.1` and assigned automatically by the OS to avoid collisions. After startup, discover the actual URLs:

```bash
./scripts/show-urls.sh
```

Or use Docker directly:

```bash
docker compose port frontend 80    # e.g. 127.0.0.1:32771
docker compose port api 8080       # e.g. 127.0.0.1:32770
```

To pin specific host ports instead of random assignment:

```bash
API_PORT=8080 FRONTEND_PORT=3000 docker compose up --build
```

## Database initialization

```bash
./init_db.sh
```

This is the only project-standard database initialization path. It:
1. Starts the bootstrap and postgres services
2. Creates a `schema_migrations` tracking table
3. Applies pending migrations from `db/migrations/` in sorted order
4. Wraps each migration in a single transaction with `ON_ERROR_STOP=1` — SQL errors cause an immediate rollback and script failure
5. Skips already-applied migrations on rerun
6. Seeds three dev accounts and the three roles (see below)

To drop and recreate the database from scratch:

```bash
./init_db.sh --reset
```

## Authentication

### Seeded dev accounts

After running `./init_db.sh`, three accounts are available:

| Username | Password | Role | Portal |
|----------|----------|------|--------|
| `admin` | `admin123` | Administrator | `/admin` |
| `provider` | `provider123` | Service Provider | `/provider` |
| `customer` | `customer123` | Customer | `/customer` |

### Bootstrap admin

For a fresh database without seeded accounts, `POST /api/v1/auth/bootstrap-admin` creates the first administrator. This endpoint is locked once any admin exists.

### Session behavior

- Sessions use server-side opaque tokens stored in `auth_sessions` with SHA-256 hashing
- The session cookie (`fieldserve_session`) is `HttpOnly`, `SameSite=Strict`, `Path=/`, with environment-aware `Secure` flag
- Idle timeout: 8 hours
- Absolute lifetime: 7 days
- Logout invalidates the session immediately
- Passwords are hashed with bcrypt (pgcrypto `gen_salt('bf')` for seeds, Go `golang.org/x/crypto/bcrypt` at runtime)
- **Cookie `Secure` flag**: controlled by the `COOKIE_SECURE` environment variable:
  - `true` — always set `Secure` (for production behind TLS termination)
  - `false` — never set `Secure` (for local HTTP development)
  - `auto` or unset (default) — set `Secure` when the request arrived over TLS or has `X-Forwarded-Proto: https`

### Auth endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/auth/login` | No | Login with username/password |
| POST | `/api/v1/auth/logout` | Yes | Invalidate current session |
| GET | `/api/v1/auth/me` | Yes | Get current user and roles |
| POST | `/api/v1/auth/bootstrap-admin` | No | Create first admin (once only) |

### Route protection

- `/api/v1/customer/*` requires `customer` role
- `/api/v1/provider/*` requires `provider` role
- `/api/v1/admin/*` requires `administrator` role
- Unauthenticated requests to protected routes return `401`
- Wrong-role requests return `403` and log a `privilege_escalation` audit event

### Frontend auth

- Auth state is bootstrapped on page load via `GET /auth/me`
- No tokens are stored in JavaScript — the HttpOnly cookie handles session identity
- Route guards enforce role-based portal access
- Wrong-role access shows a 403 page inline (user is authenticated but lacks permission)
- Unauthenticated access redirects to `/login`
- After login, the user is redirected to their role's portal

## Catalog, taxonomy, and services

### Admin taxonomy management

Administrators manage the service catalog taxonomy through:
- **Categories** (`/admin/categories`): Hierarchical categories with parent/child support, slugs, and sort order
- **Tags** (`/admin/tags`): Controlled tag vocabulary applied to services

All taxonomy changes emit audit events.

### Provider service management

Providers manage their own services through:
- **Service list** (`/provider/services`): View, create, edit, and delete own services
- **Service form**: Title, description, category, price, tags, and status (active/inactive)
- **Availability** (`/provider/services/:id/availability`): Weekly time windows per service

Providers can only see and modify their own services regardless of status (active or inactive). The provider service detail endpoint (`GET /provider/services/:id`) returns owned services in any status, while the public catalog endpoint only returns active services. Attempts to access another provider's service return 404 (not 403) to avoid leaking existence.

### Search and discovery

The customer discovery surface (`/customer/catalog`) provides real fuzzy search and multi-field filtering:

**Search params** on `GET /api/v1/catalog/services`:
| Param | Description |
|-------|-------------|
| `q` | Fuzzy keyword search via pg_trgm `similarity()` on service title and provider business name |
| `category_id` | Exact category UUID filter |
| `tag_ids` | Comma-separated tag UUIDs (service has at least one) |
| `min_price` / `max_price` | Price range filter in cents |
| `min_rating` | Minimum rating_avg filter |
| `radius_miles` | Service area filter — returns services where `provider_profiles.service_area_miles >= value`. This is a local-schema proxy; the API shape is forward-compatible with future geo support. |
| `available_date` | Availability date filter (YYYY-MM-DD). The backend resolves the date to its day of week and checks `service_availability_windows` for matching schedule windows. |
| `available_time` | Time filter (HH:MM) — when combined with `available_date`, narrows to windows ending at or after this time on the resolved day. Requires `available_date`. |
| `sort` | `newest`, `price_asc`, `price_desc`, `popularity`, `rating`, `distance`, `relevance` (default: `newest` without q, `relevance` with q) |
| `page` / `page_size` | Pagination (default 1/20, max page_size 100) |

Both camelCase (`categoryId`, `minPrice`, `pageSize`, `availableDate`, `availableTime`) and snake_case (`category_id`, `min_price`, `page_size`, `available_date`, `available_time`) param names are accepted.

**Availability filter behavior**: `availableDate` / `available_date` accepts a real date (YYYY-MM-DD). The backend resolves the date to its day of week (using Go's `time.Weekday()`: 0=Sunday through 6=Saturday) and checks the `service_availability_windows` table for matching schedule windows. When used alone, it matches any service with at least one availability window on that day. When combined with `availableTime` / `available_time` (HH:MM), the filter further narrows to windows where `end_time >= available_time` on the resolved day. The frontend exposes a date picker and a conditional "Available At" time input. Providers define weekly recurring schedules; customers query by real dates.

Sorting is deterministic — all sort modes use `created_at DESC` as a tiebreaker. Relevance sort uses `GREATEST(similarity(title, q), similarity(business_name, q)) DESC`.

**Search cache**: An in-memory LRU cache (capacity 500, 10-minute TTL) caches page-sized search results keyed by a SHA-256 hash of normalized search params. Cache is invalidated on taxonomy changes, provider service mutations, and favorite changes.

**Cached-query SLA**: The target for warm cached search queries is under 300 ms. This is verified by `TestCachedQueryPerformance` in the backend test suite, which warms the cache and measures 10 iterations of a cached query, asserting the average is under 300 ms. The benchmark can also be run standalone via `./scripts/bench-cached-query.sh`.

### Favorites

Customers can favorite services from the catalog or detail pages. Favorites are optimistic in the UI with rollback on failure.

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/customer/favorites` | Customer | List customer's favorites |
| POST | `/api/v1/customer/favorites/:serviceId` | Customer | Add favorite (idempotent) |
| DELETE | `/api/v1/customer/favorites/:serviceId` | Customer | Remove favorite |

### Compare

The compare tray is persisted in localStorage (key `fieldserve-compare`), hard-capped at 3 services. Adding a 4th service shows a visible inline alert on both the catalog page and service detail page. The comparison page (`/customer/compare`) shows aligned side-by-side data:
- **Provider** — business name
- **Price** — formatted as dollars
- **Rating** — X.XX / 5.00
- **Service Area** — `provider_profiles.service_area_miles` displayed as "X miles" (or "Not specified" if null); forward-compatible with future geo data
- **Category** — category name
- **Availability** — summarized from `service_availability_windows` data (e.g., "Mon: 09:00-17:00; Wed: 10:00-15:00"), fetched via service detail endpoint
- **Tags** — comma-separated tag names
- **Popularity** — popularity score

### Search history and trending

- `GET /api/v1/customer/search-history` returns the customer's recent unique search queries
- `GET /api/v1/catalog/trending` returns services ranked by recent favorites (7-day window) + popularity_score
- Search events and history are recorded in `search_events` and `search_history` tables on each keyword search

### Admin search config

Administrators manage search configuration through:
- **Hot keywords** (`/admin/hot-keywords`): Keywords surfaced as suggestion chips on the search page
- **Autocomplete terms** (`/admin/autocomplete`): Weighted terms for autocomplete suggestions

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/catalog/hot-keywords` | Any authenticated | Hot keywords for search suggestions |
| GET | `/api/v1/catalog/autocomplete?q=prefix` | Any authenticated | Autocomplete term prefix search |
| GET/POST/PATCH | `/api/v1/admin/search-config/hot-keywords[/:id]` | Admin | Hot keyword CRUD |
| GET/POST/PATCH | `/api/v1/admin/search-config/autocomplete[/:id]` | Admin | Autocomplete term CRUD |

### Interest lifecycle

Customers express interest in services. The lifecycle:
- **submitted** → initial state on creation
- **accepted** → provider accepts the interest
- **declined** → provider declines the interest
- **withdrawn** → customer withdraws their own interest

**Duplicate-interest rule**: Only one active interest (`submitted` or `accepted`) per customer/provider pair within 7 days. Attempts to create a duplicate return `409` with `field_errors.provider_id`.

Status transitions are recorded in `interest_status_events` for a full audit timeline.

### Messaging

Message threads are anchored to interests — the `interest.id` is the `thread_id`. Both customer and provider can send messages in a thread once an interest exists.

- Message bodies persist in `messages`
- Read state persists in `message_receipts` with statuses: `sent`, `delivered`, `read`
- Thread lists include unread count and last message preview
- Marking a thread as read updates all unread receipts for that user

### Blocking

Two-way blocking via the `blocks` table. If either party has blocked the other:
- Blocked providers are **hidden from search results** (filtered via `ExcludeProviderUserIDs`)
- **Interest submission** is rejected with `403`
- **Message sends** are rejected with `403`
- Historical interests and message threads **remain visible read-only** — they are not deleted, but new engagement is prevented
- The customer service detail page shows "Blocked" state with disabled interest button and an Unblock option

### Idempotency

Every authenticated write endpoint (POST, PATCH, DELETE) accepts an optional `Idempotency-Key` header for replay protection. When present, the middleware hashes the key with the user ID, HTTP method, and route path, and stores the response for a 5-minute replay window. Duplicate requests with the same key return the cached response without re-executing the handler.

**Customer routes (7):**
- `PATCH /customer/profile` — profile update
- `POST /customer/favorites/:serviceId` — add favorite
- `DELETE /customer/favorites/:serviceId` — remove favorite
- `POST /customer/interests` — interest submission
- `POST /customer/interests/:id/withdraw` — interest withdrawal
- `POST /customer/messages/:threadId` — message send
- `POST /customer/messages/:threadId/read` — mark thread read
- `POST /customer/blocks/:providerId` — block provider
- `DELETE /customer/blocks/:providerId` — unblock provider

**Provider routes (12):**
- `PATCH /provider/profile` — profile update
- `POST /provider/documents` — document upload
- `DELETE /provider/documents/:id` — document delete
- `POST /provider/services` — service creation
- `PATCH /provider/services/:id` — service update
- `DELETE /provider/services/:id` — service delete
- `POST /provider/services/:id/availability` — set availability
- `POST /provider/interests/:id/accept` — interest accept
- `POST /provider/interests/:id/decline` — interest decline
- `POST /provider/messages/:threadId` — message send
- `POST /provider/messages/:threadId/read` — mark thread read
- `POST /provider/blocks/:customerId` — block customer
- `DELETE /provider/blocks/:customerId` — unblock customer

**Admin routes (22):**
- `POST /admin/categories` — category create
- `PATCH /admin/categories/:id` — category update
- `POST /admin/tags` — tag create
- `PATCH /admin/tags/:id` — tag update
- `POST /admin/analytics/rollup` — trigger rollup
- `POST /admin/exports` — export creation
- `POST /admin/search-config/hot-keywords` — hot keyword create
- `PATCH /admin/search-config/hot-keywords/:id` — hot keyword update
- `POST /admin/search-config/autocomplete` — autocomplete create
- `PATCH /admin/search-config/autocomplete/:id` — autocomplete update
- `POST /admin/alert-rules` — alert rule creation
- `PATCH /admin/alert-rules/:id` — alert rule update
- `POST /admin/on-call` — on-call schedule creation
- `POST /admin/alerts/:id/assign` — alert assignment
- `POST /admin/alerts/:id/acknowledge` — alert acknowledgment
- `POST /admin/work-orders` — work order creation
- `POST /admin/work-orders/:id/{dispatch,acknowledge,start,resolve,post-incident-review,close}` — all 6 transitions
- `POST /admin/work-orders/:id/evidence` — evidence upload

**Intentionally excluded (2 read-only verification endpoints):**
- `POST /admin/documents/:id/verify-checksum` — read-only integrity check, no state mutation
- `POST /admin/evidence/:id/verify-checksum` — read-only integrity check, no state mutation

**Unauthenticated routes (not applicable — middleware requires auth context):**
- `POST /auth/login`, `POST /auth/bootstrap-admin`, `POST /auth/logout`

The key is SHA-256 hashed together with the user ID, HTTP method, and route path, then stored in `idempotency_keys` with a 5-minute replay window. This scoping ensures:
- The same key used by different users does not collide
- The same key reused by one user on a different endpoint does not replay the wrong cached response
- Replay is correct only for the exact user + method + path combination within the window

### Rate limiting

All authenticated write requests (POST/PATCH/PUT/DELETE) across customer, provider, and admin route groups are rate-limited at 60 requests per minute per user. Interest submission and message sends additionally carry per-route idempotency enforcement. Exceeding the rate limit returns `429 Too Many Requests`.

### Engagement API endpoints

| Method | Path | Role | Description |
|--------|------|------|-------------|
| POST | `/api/v1/customer/interests` | Customer | Submit interest (idempotent) |
| GET | `/api/v1/customer/interests` | Customer | List customer's interests |
| GET | `/api/v1/customer/interests/:id` | Customer | Interest detail with timeline |
| POST | `/api/v1/customer/interests/:id/withdraw` | Customer | Withdraw interest |
| GET | `/api/v1/provider/interests` | Provider | List incoming interests |
| POST | `/api/v1/provider/interests/:id/accept` | Provider | Accept interest |
| POST | `/api/v1/provider/interests/:id/decline` | Provider | Decline interest |
| GET | `/api/v1/customer/messages` | Customer | List message threads |
| GET | `/api/v1/customer/messages/:threadId` | Customer | Thread messages |
| POST | `/api/v1/customer/messages/:threadId` | Customer | Send message (idempotent) |
| POST | `/api/v1/customer/messages/:threadId/read` | Customer | Mark thread as read |
| GET | `/api/v1/provider/messages` | Provider | List message threads |
| GET | `/api/v1/provider/messages/:threadId` | Provider | Thread messages |
| POST | `/api/v1/provider/messages/:threadId` | Provider | Send message (idempotent) |
| POST | `/api/v1/provider/messages/:threadId/read` | Provider | Mark thread as read |
| POST | `/api/v1/customer/blocks/:providerId` | Customer | Block provider |
| DELETE | `/api/v1/customer/blocks/:providerId` | Customer | Unblock provider |
| POST | `/api/v1/provider/blocks/:customerId` | Provider | Block customer |
| DELETE | `/api/v1/provider/blocks/:customerId` | Provider | Unblock customer |

### Provider document uploads

Providers manage documents through `/provider/documents`:

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/provider/documents` | Provider | List provider's documents |
| POST | `/api/v1/provider/documents` | Provider | Upload document (multipart) |
| DELETE | `/api/v1/provider/documents/:id` | Provider | Delete own document |

**Upload validation pipeline**:
- **Allowed extensions**: `.pdf`, `.jpg`, `.jpeg`, `.png`, `.gif`, `.txt`, `.csv`, `.doc`, `.docx`
- **Allowed MIME types**: `application/pdf`, `image/jpeg`, `image/png`, `image/gif`, `text/plain`, `text/csv`, `application/msword`, Word OOXML
- **Blocked extensions (denylist)**: `.exe`, `.bat`, `.cmd`, `.sh`, `.ps1`, `.com`, `.scr`, `.msi`, `.dll`, `.so`, `.dylib`, `.bin`, `.js`, `.vbs`, `.wsf`, `.jar`
- Both the file extension AND the detected MIME type must pass validation. A file with an allowed extension but wrong MIME content is rejected, and vice versa.
- **Size cap**: 10 MB maximum
- **MIME sniffing**: `http.DetectContentType()` validates actual file content, not just extension
- **Checksum**: SHA-256 computed and stored per document and per evidence file
- **Checksum verification**: Admin endpoints `POST /admin/documents/:id/verify-checksum` and `POST /admin/evidence/:id/verify-checksum` recompute the file's SHA-256 and compare against the stored value. Mismatches return `409` and emit an audit event (`checksum_mismatch` / `evidence_checksum_mismatch`). This provides active tamper detection for stored files.
- **Storage**: Files stored under `/app/data/uploads/` with UUID-based filenames to prevent path traversal
- Rejections: `413` for oversized, `415` for disallowed type, `422` for other validation failures

### Analytics dashboards

Admin-only analytics with real DB-derived metrics:

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/admin/analytics/user-growth` | Admin | Users created per day by role |
| GET | `/api/v1/admin/analytics/conversion` | Admin | Search-to-interest conversion |
| GET | `/api/v1/admin/analytics/provider-utilization` | Admin | Per-provider activity |
| POST | `/api/v1/admin/analytics/rollup` | Admin | Generate daily rollups |

**Metric definitions**:
- **User growth**: Count of `users` created per day, grouped by role via `user_roles`+`roles`. Query params: `from`, `to` (YYYY-MM-DD).
- **Search-to-interest conversion**: Count of `search_events` vs `interests` per day. Rate = interests/searches. Query params: `from`, `to`.
- **Provider utilization**: Per provider: active services count, total interests received, messages sent.

**Rollups**: `analytics_daily_rollups` stores computed daily metrics. The worker generates rollups on its 30-second tick (idempotent — skips if today's exist). Admin can also trigger via `POST /admin/analytics/rollup`.

### CSV exports

Admin-only CSV export flow:

| Method | Path | Role | Description |
|--------|------|------|-------------|
| POST | `/api/v1/admin/exports` | Admin | Request export |
| GET | `/api/v1/admin/exports` | Admin | List export jobs |
| GET | `/api/v1/admin/exports/:id` | Admin | Export job detail |
| GET | `/api/v1/admin/exports/:id/download` | Admin | Download CSV file |

- Export types: `user_growth`, `conversion`, `provider_utilization`
- Jobs are stored in `export_jobs` with status transitions: `pending` → `completed` (or `failed`)
- CSV files are written to `/app/data/exports/` with UUID filenames
- Generation is synchronous within the request
- All export lifecycle events are audited

### Audit system (dual-path)

**DB-indexed audit**: Every authenticated API request emits an `api_access` audit event to `audit_event_index` containing: request_id, actor_id, HTTP method, URL path (no query params), and response status. All write operations (auth, catalog, interests, messages, blocks, uploads, exports, alerts, work orders) also emit specific lifecycle audit events.

**Append-only file audit**: Audit events are simultaneously written to daily-rotated JSONL files under `/app/data/audit/`. Files are named `audit-YYYY-MM-DD.jsonl`. On daily rotation, the previous day's file is sealed as read-only (chmod 0444). The file sink uses append-only semantics — the application write path never truncates or modifies existing entries.

**Redaction**: Audit events never log raw tokens, cookies, passwords, session IDs, or plaintext sensitive field values. The API access middleware logs only the URL path (excluding query parameters to prevent leaking sensitive query values). Metadata fields are limited to method, path, status, and request_id.

### Sensitive-field masking

Profile API endpoints (`GET /customer/profile`, `GET /provider/profile`) return **masked** values for sensitive fields by default:
- **Phone**: last 4 digits visible (e.g. `***5309`)
- **Notes**: first 3 characters plus `***` (e.g. `All***`)

Full plaintext sensitive values are never exposed in normal product-facing API responses. This policy is enforced by shared masking helpers in `internal/platform/crypto/` (`MaskPhone`, `MaskNote`). Audit logs also never contain plaintext sensitive values.

### Encryption at rest

Sensitive fields are encrypted at the application layer using AES-256-GCM before database persistence:
- `customer_profiles.phone_encrypted` — customer phone numbers
- `provider_profiles.phone_encrypted` — provider phone numbers
- `customer_profiles.notes_encrypted` — customer private notes
- `provider_profiles.notes_encrypted` — provider private notes

The encryption key is derived from the `ENCRYPTION_KEY` environment variable (64 hex chars = 32 bytes), which is generated by the bootstrap container and stored in the Docker secrets volume. The key is never checked into the repo.

### Retention and cleanup

The worker runs scheduled cleanup on each tick (~30s), idempotently removing:
- **Expired auth sessions**: sessions past `expires_at` or idle for > 8 hours
- **Expired idempotency keys**: keys past `expires_at` (5-minute TTL)
- **Expired evidence files**: `work_order_evidence` rows where `retention_expires_at < NOW()`, with best-effort file deletion from `/app/data/evidence/`

### Worker responsibilities

The worker process (`cmd/worker`) runs on a 30-second tick cycle:
1. **Alert rule evaluation**: evaluates all enabled rules against live DB metrics, creates alerts when thresholds are exceeded (with quiet-hours suppression and 1-hour dedup), auto-assigns new alerts to lowest-tier on-call user
2. **SLA deadline checks**: creates critical alerts for work orders overdue by 24+ hours
3. **On-call escalation**: escalates unacknowledged alert assignments older than 30 minutes to next on-call tier (Tier 1 → 2 → 3)
4. **Daily analytics rollups**: computes and stores `analytics_daily_rollups` (idempotent)
4. **Session cleanup**: removes expired auth sessions
5. **Idempotency key cleanup**: removes expired idempotency keys
6. **Evidence retention cleanup**: removes expired evidence files and DB records

### On-call model

Alert assignment is restricted to users with an active on-call schedule. The on-call model uses a tiered structure (tiers 1-3) with time-bounded schedules stored in `on_call_schedules`:

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/api/v1/admin/on-call` | Admin | List active on-call schedules |
| POST | `/api/v1/admin/on-call` | Admin | Create on-call schedule |

**Tier model**: Tier 1 = primary responder, Tier 2 = secondary escalation, Tier 3 = tertiary/management escalation.

**Active on-call** means the current time is between `start_time` and `end_time` of a schedule. `GET /admin/on-call` returns only active schedules. The frontend assignment dropdown is populated exclusively from this filtered set.

**Assignment eligibility**: Only users with an active on-call schedule can be assigned to alerts. Manual assignment to a non-on-call user returns `422`.

**Auto-assignment**: When the worker creates a new alert from a rule evaluation, the alert is automatically assigned to the lowest-tier active on-call user. If no on-call user is active, the alert remains unassigned.

**Tier-aware escalation**: The worker checks on each tick (~30s) for assigned-but-unacknowledged alerts older than 30 minutes. These are escalated to the next tier: Tier 1 → Tier 2 → Tier 3. Escalation creates an additional assignment to a next-tier on-call user. If no next-tier user is available or the alert is already at Tier 3, escalation is a no-op. Duplicate escalations are prevented.

### Alert center

Admin-configurable alert rules with scheduled evaluation:

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET/POST/PATCH | `/api/v1/admin/alert-rules[/:id]` | Admin | Alert rule CRUD |
| GET | `/api/v1/admin/alerts` | Admin | List alerts |
| GET | `/api/v1/admin/alerts/:id` | Admin | Alert detail with assignment |
| POST | `/api/v1/admin/alerts/:id/assign` | Admin | Assign alert to user |
| POST | `/api/v1/admin/alerts/:id/acknowledge` | Admin | Acknowledge assigned alert |

**Alert rule conditions**: Rules define a `metric` and `threshold`. Supported metrics:
- `unresolved_interests` — count of submitted interests older than 3 days
- `low_provider_utilization` — count of providers with 0 active services
- `overdue_work_orders` — count of work orders in dispatched/acknowledged/in_progress for 24+ hours

**Severity model**: `low`, `medium`, `high`, `critical`. Validated on create/update.

**Quiet hours**: Rules can define `quiet_hours_start` and `quiet_hours_end` (TIME). During quiet hours, alerts with severity below `critical` are suppressed. Critical alerts always fire regardless of quiet hours. Overnight ranges (e.g. 22:00–07:00) are handled correctly.

**Evaluation**: The worker evaluates all enabled rules on each tick (~30s). If a rule's metric exceeds its threshold, an alert is created (with 1-hour deduplication to avoid spam).

### Work orders

Full lifecycle management for incident response:

| Method | Path | Role | Description |
|--------|------|------|-------------|
| POST | `/api/v1/admin/work-orders` | Admin | Create work order (optionally from alert) |
| GET | `/api/v1/admin/work-orders` | Admin | List work orders |
| GET | `/api/v1/admin/work-orders/:id` | Admin | Detail with events + evidence |
| POST | `.../dispatch` | Admin | new → dispatched |
| POST | `.../acknowledge` | Admin | dispatched → acknowledged |
| POST | `.../start` | Admin | acknowledged → in_progress |
| POST | `.../resolve` | Admin | in_progress → resolved |
| POST | `.../post-incident-review` | Admin | resolved → post_incident_review |
| POST | `.../close` | Admin | post_incident_review → closed |
| POST | `.../evidence` | Admin | Upload evidence file |
| GET | `.../evidence` | Admin | List evidence files |

**Status lifecycle**: `new` → `dispatched` → `acknowledged` → `in_progress` → `resolved` → `post_incident_review` → `closed`. Invalid transitions return 422. All transitions are recorded in `work_order_events`.

**SLA deadline**: Work orders in dispatched/acknowledged/in_progress status for more than 24 hours trigger a `critical` severity SLA breach alert. The worker checks this on each tick.

**Evidence**: Files stored under `/app/data/evidence/` with UUID-based filenames. Retention metadata: `retention_expires_at` set to 180 days from upload. Same upload safety rules as provider documents (MIME check, size cap, path confinement).

### Local data roots

| Path | Purpose | Managed by |
|------|---------|-----------|
| `/app/data/uploads/` | Provider document files | Upload service |
| `/app/data/exports/` | CSV export files | Analytics/export service |
| `/app/data/audit/` | Append-only audit JSONL files | Audit file sink |
| `/app/data/evidence/` | Work order evidence files | Work orders service + retention cleanup |

All paths are backed by Docker named volumes that persist across container restarts.

## Testing

```bash
./run_tests.sh
```

This is the broad test command. It initializes the database, then runs:
- **Backend**: 129 tests (across 6 packages) — auth, RBAC, taxonomy, provider services, search, favorites, trending, distance sorting, date-based availability filtering, interests, messaging, blocking, idempotency (including concurrent same-key, provider service create), rate limiting, uploads (valid/rejected/path-confinement), analytics, exports, API access audit, alert rules (create/update/evaluate/quiet-hours/critical-bypass/metric-validation), on-call assignment eligibility/rejection, alert assignment/acknowledge, work orders (full lifecycle/invalid transitions), SLA overdue escalation, evidence upload/retention/rejection, AES-256 encryption round-trip for phone and notes, sensitive-field masking (phone/notes), audit file output, session/idempotency cleanup, cookie Secure behavior, rate-limit on all write routes, LRU cache, admin-only enforcement, service-provider relationship validation, hot-keywords/autocomplete auth enforcement, document checksum verification with tamper detection, cached-query 300ms SLA benchmark
- **Frontend**: 72 Vitest tests (across 7 test files) — auth, routes, search, favorites, compare, distance sort, date-based availability filter, interests, messaging, blocking, uploads, analytics, exports, alert rules (create/edit), alert center (severity/on-call-select/assign/acknowledge), work order full lifecycle (dispatch through close), evidence upload, integration workflows

No host toolchain beyond Docker is required.

## Secret management

**No `.env` files exist anywhere in the repo tree.** Secrets are generated at container start by `scripts/generate-secrets.sh` running inside an Alpine init container. The generated values are written to a Docker named volume (`secrets`) and sourced by each service's entrypoint. This approach:
- Requires no manual setup or pre-steps before `docker compose up`
- Never writes secrets to the host filesystem or repo directory
- Persists secrets across container restarts (same volume)
- Clears secrets on `docker compose down -v`

This is for **local development only** and is not the production secret-management path.

## Repo structure

```
backend/
  cmd/api/          API server with auth routes and middleware
  cmd/worker/       Background worker (alert eval, SLA checks, rollups, cleanup)
  internal/
    auth/           Authentication, sessions, handlers, RBAC middleware
    audit/          Audit event logging to audit_event_index
    rbac/           (reserved — RBAC enforcement is in auth middleware)
    catalog/        Category/tag/service CRUD, catalog reads, search config, provider ownership
    search/         Fuzzy search, cache, trending, search history
    interests/      Interest lifecycle, duplicate prevention, status events
    messages/       Messaging threads, read receipts, blocked enforcement
    favorites/      Customer favorites CRUD
    blocks/         Two-way blocking with search/interest/message enforcement
    uploads/        Provider document upload, validation, storage
    analytics/      Dashboard metrics, rollups, CSV exports
    alerts/         Alert rules, evaluation, quiet hours, assignment
    workorders/     Work order lifecycle, events, evidence
    platform/
      cache/        In-memory LRU cache with TTL
      httpx/        Error contract, middleware, rate limiter, idempotency, API access audit
      crypto/       AES-256-GCM encryption for sensitive fields
      storage/      (reserved — local disk paths used directly)
      scheduler/    (reserved — worker tick loop handles scheduling)
  Dockerfile        Multi-stage build for api
  Dockerfile.worker Multi-stage build for worker
frontend/
  src/
    api/            API client, catalog/search/favorites/admin API functions
    stores/         Zustand auth store, compare tray store (localStorage)
    components/     RouteGuard with role enforcement
    layouts/        Portal layouts with nav and logout
    pages/          Login, dashboards, admin taxonomy/search-config/analytics/exports, provider services/documents, search/discovery, favorites, compare, interests, messages
  Dockerfile        Multi-stage build (Node -> Nginx)
  nginx.conf        SPA-friendly reverse proxy config
db/
  migrations/       SQL migration files (up/down)
scripts/
  generate-secrets.sh  Ephemeral secret generator (runs in container, dev only)
  show-urls.sh         Print actual host URLs for running services
docker-compose.yml  Full stack orchestration
init_db.sh          Database initialization with migration tracking
run_tests.sh        Broad test runner
```

## Delivery slices

All 8 delivery slices are implemented.

## Current state

- Docker Compose orchestration for all services with init-container secret bootstrap
- Real auth: login/logout, session management, bcrypt passwords, HttpOnly cookies
- Real RBAC: route-group enforcement with privilege-escalation audit logging
- Role-aware frontend: auth bootstrapping, route guards, role-based redirects, 403 handling
- Admin taxonomy management: categories (hierarchical) and tags with audit logging
- Admin search config: hot keywords and autocomplete terms with audit logging
- Provider service management: CRUD with ownership enforcement, tag assignment, availability windows
- Fuzzy search with pg_trgm similarity, multi-field filtering, deterministic sorting (including distance), stable pagination
- In-memory LRU search cache (500 entries, 10-min TTL) with invalidation on data changes
- Customer favorites with optimistic UI and rollback
- Compare tray (max 3) with localStorage persistence and side-by-side comparison page
- Search history recording and trending recommendations from local activity signals
- Three seeded dev accounts (admin/provider/customer) via migration
- PostgreSQL baseline schema (32 tables, extensions, trigram + B-tree indexes)
- Migration tracking with rerun safety and transactional rollback
- Normalized JSON error contract in the API
- Request ID and structured logging middleware
- 201 automated tests (129 backend, 72 frontend)

- Interest lifecycle with duplicate prevention, status events, and object-level authorization
- Messaging threads anchored to interests with read receipts (sent/delivered/read)
- Two-way blocking enforced on search visibility, interest submission, and message sends
- Idempotency-key enforcement on interest submission, message sends, service creation, work order creation, export creation, and alert rule creation (5-minute replay window, concurrency-safe single-flight via DB row locking)
- Per-user rate limiting (60 RPM) on all authenticated write routes

- Provider document uploads with MIME sniffing, size cap, checksum, and path safety
- Admin analytics dashboards (user growth, search-to-interest conversion, provider utilization)
- Admin CSV exports with job tracking and file download
- API access audit emission for all authenticated requests
- Worker-based daily analytics rollup generation

- Alert center with configurable rules, scheduled evaluation, quiet-hours suppression, severity model, and in-app assignment/acknowledgment
- Work order lifecycle (new → dispatched → acknowledged → in_progress → resolved → post_incident_review → closed) with event history
- SLA deadline enforcement (24-hour breach triggers critical alert)
- Evidence upload with 180-day retention metadata under `/app/data/evidence/`

- AES-256-GCM application-layer encryption for sensitive fields (phone + notes) via ENCRYPTION_KEY from bootstrap
- Sensitive-field masking on all profile API responses (phone shows last 4 digits, notes show first 3 chars)
- On-call schedule model with tiered assignment eligibility enforcement
- Checksum tamper-detection verification for uploaded documents and evidence files
- Cached-query 300ms SLA verified by executable benchmark test
- Dual-path audit: DB-indexed `audit_event_index` + append-only daily-rotated JSONL files under `/app/data/audit/`
- Worker-driven cleanup: expired sessions, expired idempotency keys, expired evidence with file deletion
- Rate limiting on all authenticated write routes (60 RPM per user)
- Audit redaction: no query params, tokens, cookies, or plaintext sensitive values in audit output

All planned domain modules and hardening targets are implemented. The system is ready for self-test.
