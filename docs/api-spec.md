# FieldServe Planned API Surface

## Conventions
- Base path: `/api/v1`
- Authenticated endpoints require local server-side session auth via `HttpOnly` cookie
- Every authenticated write endpoint (POST/PATCH/DELETE) accepts an optional `Idempotency-Key` header. When present, the middleware replays the cached response for duplicate keys within a 5-minute window. Only two admin POST endpoints are excluded: `verify-checksum` routes (read-only checks with no state mutation)
- Sensitive fields (phone, notes) are returned **masked** in standard API responses — plaintext is never exposed in normal product-facing flows
- Errors use the normalized error contract defined in `docs/design.md`

## Auth
- `POST /auth/login`
- `POST /auth/logout`
- `GET /auth/me`
- `POST /auth/bootstrap-admin`

### Session contract
- `POST /auth/login` creates an `auth_sessions` record and sets a session cookie
- `POST /auth/logout` invalidates the session and clears the cookie
- `GET /auth/me` resolves the current actor and role set from the server-side session
- Session idle timeout is 8 hours; absolute lifetime is 7 days
- Role/permission changes revoke impacted sessions

## Catalog
- `GET /catalog/categories`
- `GET /catalog/tags`
- `GET /catalog/services`
  - query: `q`, `categoryId`, `tagIds[]`, `minPrice`, `maxPrice`, `radiusMiles`, `availableDate` (YYYY-MM-DD), `availableTime` (HH:MM), `minRating`, `sort` (`newest`, `price_asc`, `price_desc`, `popularity`, `rating`, `distance`, `relevance`), `page`, `pageSize`
  - Availability filtering is date-based. `availableDate` accepts a real date (YYYY-MM-DD); the backend resolves it to the day of week and checks `service_availability_windows`. `availableTime` optionally narrows to windows ending at or after that time. Providers define weekly recurring schedules; customers query by actual dates.
- `GET /catalog/services/:serviceId`
- `GET /catalog/trending`
- `GET /catalog/hot-keywords`
- `GET /catalog/autocomplete`

## Customer
- `GET /customer/favorites`
- `POST /customer/favorites/:serviceId`
- `DELETE /customer/favorites/:serviceId`
- `GET /customer/search-history`
- `POST /customer/interests`
- `GET /customer/interests`
- `GET /customer/interests/:interestId`
- `POST /customer/interests/:interestId/withdraw`
- `GET /customer/messages`
- `GET /customer/messages/:threadId`
- `POST /customer/messages/:threadId`
- `POST /customer/messages/:threadId/read`
- `POST /customer/blocks/:providerId`
- `DELETE /customer/blocks/:providerId`

## Provider
- `GET /provider/services`
- `POST /provider/services`
- `PATCH /provider/services/:serviceId`
- `DELETE /provider/services/:serviceId`
- `POST /provider/services/:serviceId/availability`
- `GET /provider/interests`
- `POST /provider/interests/:interestId/accept`
- `POST /provider/interests/:interestId/decline`
- `GET /provider/messages`
- `GET /provider/messages/:threadId`
- `POST /provider/messages/:threadId`
- `POST /provider/messages/:threadId/read`
- `POST /provider/documents`
- `GET /provider/documents`
- `POST /provider/blocks/:customerId`
- `DELETE /provider/blocks/:customerId`

## Customer Profile
- `GET /customer/profile` — returns masked phone and masked notes
- `PATCH /customer/profile` — update phone and/or notes (encrypted at rest)

## Provider Profile
- `GET /provider/profile` — returns masked phone and masked notes
- `PATCH /provider/profile` — update phone and/or notes (encrypted at rest)

## Admin
- `GET /admin/on-call` — list on-call schedules
- `POST /admin/on-call` — create on-call schedule (user_id, tier 1-3, start_time, end_time)
- `POST /admin/documents/:documentId/verify-checksum` — verify document file integrity
- `POST /admin/evidence/:evidenceId/verify-checksum` — verify evidence file integrity
- `GET /admin/users`
- `PATCH /admin/users/:userId/roles`
- `GET /admin/categories`
- `POST /admin/categories`
- `PATCH /admin/categories/:categoryId`
- `GET /admin/tags`
- `POST /admin/tags`
- `PATCH /admin/tags/:tagId`
- `GET /admin/search-config/hot-keywords`
- `POST /admin/search-config/hot-keywords`
- `PATCH /admin/search-config/hot-keywords/:keywordId`
- `GET /admin/search-config/autocomplete`
- `POST /admin/search-config/autocomplete`
- `PATCH /admin/search-config/autocomplete/:termId`
- `GET /admin/audit-events`
- `GET /admin/analytics/user-growth`
- `GET /admin/analytics/conversion`
- `GET /admin/analytics/provider-utilization`
- `POST /admin/exports`
- `GET /admin/exports`
- `GET /admin/exports/:exportId/download`
- `GET /admin/alert-rules`
- `POST /admin/alert-rules`
- `PATCH /admin/alert-rules/:ruleId`
- `GET /admin/alerts`
- `GET /admin/work-orders`
- `POST /admin/work-orders/:workOrderId/dispatch`
- `POST /admin/work-orders/:workOrderId/acknowledge`
- `POST /admin/work-orders/:workOrderId/resolve`
- `POST /admin/work-orders/:workOrderId/post-incident-review`

## Planned lifecycle literals

### Interest status
- `submitted`
- `accepted`
- `declined`
- `withdrawn`

### Message receipt status
- `sent`
- `delivered`
- `read`

### Work order status
- `new`
- `dispatched`
- `acknowledged`
- `in_progress`
- `resolved`
- `post_incident_review`
- `closed`

## Planned status/error mappings
- `401` unauthenticated
- `403` forbidden / privilege escalation attempt
- `404` missing resource or hidden due to block/ownership rules where appropriate
- `409` duplicate interest / idempotency replay conflict
- `413` upload too large
- `415` invalid MIME/extension
- `422` validation failure
- `429` rate limit exceeded
