### 1. Fix Verification Verdict
- **Overall: Fixed**

### 2. Issue-by-issue status

1) **Sensitive-field masking is not implemented end to end**  
- **Status: Fixed**  
- **Rationale:** Customer/provider profile GET handlers now decrypt and return masked phone/notes; masking helpers are centralized; hardening tests validate masked output.  
- **Evidence:** `backend/cmd/api/main.go:157`, `backend/cmd/api/main.go:175`, `backend/cmd/api/main.go:182`, `backend/cmd/api/main.go:328`, `backend/cmd/api/main.go:346`, `backend/cmd/api/main.go:353`, `backend/internal/platform/crypto/crypto.go:43`, `backend/internal/platform/crypto/crypto.go:62`, `backend/cmd/api/hardening_test.go:533`, `backend/cmd/api/hardening_test.go:659`, `README.md:392`, `../docs/api-spec.md:69`  
- **Smallest remaining blocker:** None.

2) **On-call escalation model is not implemented**  
- **Status: Fixed**  
- **Rationale:** Active on-call filtering exists, assignment enforces active on-call eligibility, auto-assign uses lowest tier, escalation to next tier is implemented and invoked by worker, and tests cover these behaviors.  
- **Evidence:** `backend/internal/alerts/alerts.go:347`, `backend/internal/alerts/alerts.go:450`, `backend/internal/alerts/alerts.go:479`, `backend/internal/alerts/alerts.go:541`, `backend/internal/alerts/alerts.go:571`, `backend/internal/alerts/alerts.go:628`, `backend/cmd/worker/main.go:70`, `backend/cmd/api/hardening_test.go:927`, `backend/cmd/api/hardening_test.go:982`, `frontend/src/pages/admin/AlertCenterPage.tsx:177`, `README.md:443`  
- **Smallest remaining blocker:** None.

3) **Sensitive-at-rest scope (“phone numbers and notes”) is only partially implemented**  
- **Status: Fixed**  
- **Rationale:** Profile PATCH handlers encrypt both phone and notes before persistence; tests verify encrypted notes in DB and masked reads.  
- **Evidence:** `backend/cmd/api/main.go:208`, `backend/cmd/api/main.go:220`, `backend/cmd/api/main.go:379`, `backend/cmd/api/main.go:391`, `backend/internal/platform/crypto/crypto.go:24`, `backend/cmd/api/hardening_test.go:635`, `backend/cmd/api/hardening_test.go:692`, `README.md:400`, `../docs/api-spec.md:70`, `../docs/api-spec.md:74`  
- **Smallest remaining blocker:** None.

4) **Idempotency scope is narrower than the prompt implies**  
- **Status: Fixed**  
- **Rationale:** Idempotency middleware now covers authenticated write endpoints across customer/provider/admin route groups (with only read-only checksum verify endpoints excluded), and additional hardening tests were added for expanded paths.  
- **Evidence:** `backend/internal/platform/httpx/idempotency.go:18`, `backend/internal/platform/httpx/idempotency.go:61`, `backend/cmd/api/main.go:232`, `backend/cmd/api/main.go:285`, `backend/cmd/api/main.go:403`, `backend/cmd/api/main.go:415`, `backend/cmd/api/main.go:491`, `backend/cmd/api/main.go:513`, `backend/cmd/api/hardening_test.go:1128`, `backend/cmd/api/hardening_test.go:1173`, `backend/cmd/api/hardening_test.go:1243`, `README.md:237`, `../docs/api-spec.md:6`  
- **Smallest remaining blocker:** None.

5) **Checksum tamper detection is only recorded, not actively verified**  
- **Status: Fixed**  
- **Rationale:** Explicit admin verification endpoints exist for documents/evidence; backend recomputes SHA-256 and returns mismatch errors; hardening tests verify tamper detection path.  
- **Evidence:** `backend/cmd/api/main.go:517`, `backend/cmd/api/main.go:524`, `backend/internal/uploads/uploads.go:281`, `backend/internal/uploads/uploads.go:305`, `backend/internal/uploads/uploads.go:320`, `backend/internal/uploads/uploads.go:348`, `backend/cmd/api/hardening_test.go:773`, `backend/cmd/api/hardening_test.go:819`, `README.md:343`, `../docs/api-spec.md:79`  
- **Smallest remaining blocker:** None.

6) **Cached-query 300 ms SLA is not backed by executable verification**  
- **Status: Fixed**  
- **Rationale:** Executable benchmark-style API test asserts average cached-query latency under 300 ms; docs and coverage matrix now map this test explicitly.  
- **Evidence:** `backend/cmd/api/hardening_test.go:831`, `backend/cmd/api/hardening_test.go:870`, `backend/cmd/api/hardening_test.go:871`, `README.md:510`, `../docs/test-coverage.md:131`  
- **Smallest remaining blocker:** None.

### 3. Residual concerns
- Minor docs consistency issue introduced during idempotency expansion: route counts in README headings appear mismatched with listed items (e.g., “Customer routes (7)” lists more than 7).  
  - Evidence: `README.md:239`, `README.md:250`, `README.md:265`  
- No additional code-level blockers found for the six tracked issues.

### 4. Final recommendation
- These **6 previously reported issues should now be considered resolved** for the static audit.
