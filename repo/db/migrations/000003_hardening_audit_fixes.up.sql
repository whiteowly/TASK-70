-- Hardening: add notes_encrypted to profiles, on_call_schedules, checksum to evidence

-- Notes encrypted fields for sensitive-at-rest coverage
ALTER TABLE customer_profiles ADD COLUMN IF NOT EXISTS notes_encrypted BYTEA;
ALTER TABLE provider_profiles ADD COLUMN IF NOT EXISTS notes_encrypted BYTEA;

-- On-call schedule model for alert assignment eligibility
CREATE TABLE IF NOT EXISTS on_call_schedules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tier       INT         NOT NULL DEFAULT 1 CHECK (tier BETWEEN 1 AND 3),
    start_time TIMESTAMPTZ NOT NULL,
    end_time   TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT on_call_time_range CHECK (end_time > start_time)
);

CREATE INDEX IF NOT EXISTS idx_on_call_schedules_active
    ON on_call_schedules (user_id, start_time, end_time);

-- Checksum column for evidence files
ALTER TABLE work_order_evidence ADD COLUMN IF NOT EXISTS checksum_sha256 VARCHAR(64);
