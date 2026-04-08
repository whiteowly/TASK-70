-- Reverse of 000001_baseline_schema.up.sql
-- Tables dropped in reverse dependency order.

DROP TABLE IF EXISTS work_order_evidence   CASCADE;
DROP TABLE IF EXISTS work_order_events     CASCADE;
DROP TABLE IF EXISTS work_orders           CASCADE;
DROP TABLE IF EXISTS alert_assignments     CASCADE;
DROP TABLE IF EXISTS alerts                CASCADE;
DROP TABLE IF EXISTS alert_rules           CASCADE;
DROP TABLE IF EXISTS export_jobs           CASCADE;
DROP TABLE IF EXISTS analytics_daily_rollups CASCADE;
DROP TABLE IF EXISTS recommendation_snapshots CASCADE;
DROP TABLE IF EXISTS search_history        CASCADE;
DROP TABLE IF EXISTS search_events         CASCADE;
DROP TABLE IF EXISTS audit_event_index     CASCADE;
DROP TABLE IF EXISTS idempotency_keys      CASCADE;
DROP TABLE IF EXISTS auth_sessions         CASCADE;
DROP TABLE IF EXISTS autocomplete_terms    CASCADE;
DROP TABLE IF EXISTS search_keyword_config CASCADE;
DROP TABLE IF EXISTS provider_documents    CASCADE;
DROP TABLE IF EXISTS blocks                CASCADE;
DROP TABLE IF EXISTS message_receipts      CASCADE;
DROP TABLE IF EXISTS messages              CASCADE;
DROP TABLE IF EXISTS interest_status_events CASCADE;
DROP TABLE IF EXISTS interests             CASCADE;
DROP TABLE IF EXISTS favorites             CASCADE;
DROP TABLE IF EXISTS service_availability_windows CASCADE;
DROP TABLE IF EXISTS service_tags          CASCADE;
DROP TABLE IF EXISTS services              CASCADE;
DROP TABLE IF EXISTS tags                  CASCADE;
DROP TABLE IF EXISTS categories            CASCADE;
DROP TABLE IF EXISTS admin_profiles        CASCADE;
DROP TABLE IF EXISTS provider_profiles     CASCADE;
DROP TABLE IF EXISTS customer_profiles     CASCADE;
DROP TABLE IF EXISTS user_roles            CASCADE;
DROP TABLE IF EXISTS roles                 CASCADE;
DROP TABLE IF EXISTS users                 CASCADE;

DROP EXTENSION IF EXISTS pg_trgm;
DROP EXTENSION IF EXISTS pgcrypto;
