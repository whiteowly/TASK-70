-- FieldServe baseline schema
-- Generated for the scaffold; all UUIDs use gen_random_uuid().

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ============================================================
-- Core identity
-- ============================================================

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(128) NOT NULL UNIQUE,
    password_hash TEXT         NOT NULL,
    email         VARCHAR(256) NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(64) NOT NULL UNIQUE,
    description TEXT
);

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

-- ============================================================
-- Profile tables
-- ============================================================

CREATE TABLE customer_profiles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    display_name    VARCHAR(256),
    phone_encrypted BYTEA,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE provider_profiles (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    business_name      VARCHAR(256) NOT NULL,
    phone_encrypted    BYTEA,
    service_area_miles INT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_profiles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    display_name VARCHAR(256),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- Catalog
-- ============================================================

CREATE TABLE categories (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id  UUID REFERENCES categories(id) ON DELETE SET NULL,
    name       VARCHAR(256) NOT NULL,
    slug       VARCHAR(256) NOT NULL UNIQUE,
    sort_order INT          NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE tags (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(128) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE services (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id      UUID         NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    category_id      UUID         REFERENCES categories(id) ON DELETE SET NULL,
    title            VARCHAR(512) NOT NULL,
    description      TEXT,
    price_cents      INT          NOT NULL,
    rating_avg       NUMERIC(3,2) NOT NULL DEFAULT 0.00,
    popularity_score INT          NOT NULL DEFAULT 0,
    status           VARCHAR(32)  NOT NULL DEFAULT 'active',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE service_tags (
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    tag_id     UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (service_id, tag_id)
);

CREATE TABLE service_availability_windows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id  UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    day_of_week INT  NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time  TIME NOT NULL,
    end_time    TIME NOT NULL
);

-- ============================================================
-- Customer interactions
-- ============================================================

CREATE TABLE favorites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customer_profiles(id) ON DELETE CASCADE,
    service_id  UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (customer_id, service_id)
);

CREATE TABLE interests (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID        NOT NULL REFERENCES customer_profiles(id) ON DELETE CASCADE,
    provider_id UUID        NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    service_id  UUID        NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    status      VARCHAR(32) NOT NULL DEFAULT 'submitted',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE interest_status_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interest_id UUID        NOT NULL REFERENCES interests(id) ON DELETE CASCADE,
    old_status  VARCHAR(32),
    new_status  VARCHAR(32) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- Messaging
-- ============================================================

CREATE TABLE messages (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id    UUID        NOT NULL,
    sender_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body         TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE message_receipts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID        NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     VARCHAR(32) NOT NULL DEFAULT 'sent',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE blocks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    blocker_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (blocker_id, blocked_id)
);

-- ============================================================
-- Provider documents
-- ============================================================

CREATE TABLE provider_documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     UUID         NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    filename        VARCHAR(512) NOT NULL,
    mime_type       VARCHAR(128) NOT NULL,
    size_bytes      INT          NOT NULL,
    checksum_sha256 VARCHAR(64)  NOT NULL,
    storage_path    TEXT         NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ============================================================
-- Search infrastructure
-- ============================================================

CREATE TABLE search_keyword_config (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    keyword    VARCHAR(256) NOT NULL,
    is_hot     BOOLEAN      NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE autocomplete_terms (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    term       VARCHAR(256) NOT NULL,
    weight     INT          NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ============================================================
-- Auth sessions & idempotency
-- ============================================================

CREATE TABLE auth_sessions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash     TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL,
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE idempotency_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash        VARCHAR(128) NOT NULL UNIQUE,
    user_id         UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    response_status INT,
    response_body   TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ  NOT NULL
);

-- ============================================================
-- Audit & analytics
-- ============================================================

CREATE TABLE audit_event_index (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type    VARCHAR(128) NOT NULL,
    actor_id      UUID,
    resource_type VARCHAR(128),
    resource_id   UUID,
    metadata      JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE search_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    query_text   TEXT,
    filters      JSONB,
    result_count INT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE search_history (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    query_text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE recommendation_snapshots (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_date DATE  NOT NULL,
    data          JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE analytics_daily_rollups (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rollup_date  DATE         NOT NULL,
    metric_name  VARCHAR(128) NOT NULL,
    metric_value NUMERIC      NOT NULL,
    metadata     JSONB,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ============================================================
-- Export jobs
-- ============================================================

CREATE TABLE export_jobs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id     UUID        NOT NULL REFERENCES admin_profiles(id) ON DELETE CASCADE,
    export_type  VARCHAR(64) NOT NULL,
    status       VARCHAR(32) NOT NULL DEFAULT 'pending',
    file_path    TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

-- ============================================================
-- Alerting & work orders
-- ============================================================

CREATE TABLE alert_rules (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              VARCHAR(256) NOT NULL,
    condition         JSONB        NOT NULL,
    severity          VARCHAR(32)  NOT NULL,
    quiet_hours_start TIME,
    quiet_hours_end   TIME,
    enabled           BOOLEAN      NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE alerts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id     UUID        NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    severity    VARCHAR(32) NOT NULL,
    status      VARCHAR(32) NOT NULL DEFAULT 'new',
    data        JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);

CREATE TABLE alert_assignments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id        UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    assignee_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_at TIMESTAMPTZ
);

CREATE TABLE work_orders (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id    UUID        REFERENCES alerts(id) ON DELETE SET NULL,
    status      VARCHAR(32) NOT NULL DEFAULT 'new',
    assigned_to UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE work_order_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    work_order_id UUID        NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    old_status    VARCHAR(32),
    new_status    VARCHAR(32) NOT NULL,
    actor_id      UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE work_order_evidence (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    work_order_id        UUID NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    file_path            TEXT NOT NULL,
    uploaded_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    retention_expires_at TIMESTAMPTZ
);

-- ============================================================
-- Indexes: trigram (GIN) for full-text-like search
-- ============================================================

CREATE INDEX idx_services_title_trgm       ON services       USING GIN (title       gin_trgm_ops);
CREATE INDEX idx_services_description_trgm ON services       USING GIN (description gin_trgm_ops);
CREATE INDEX idx_categories_name_trgm      ON categories     USING GIN (name        gin_trgm_ops);
CREATE INDEX idx_tags_name_trgm            ON tags           USING GIN (name        gin_trgm_ops);

-- ============================================================
-- Indexes: B-tree for sorting / filtering
-- ============================================================

CREATE INDEX idx_services_price_cents      ON services (price_cents);
CREATE INDEX idx_services_rating_avg       ON services (rating_avg);
CREATE INDEX idx_services_created_at       ON services (created_at);
CREATE INDEX idx_services_popularity_score ON services (popularity_score);
