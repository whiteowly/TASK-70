ALTER TABLE work_order_evidence DROP COLUMN IF EXISTS checksum_sha256;
DROP TABLE IF EXISTS on_call_schedules;
ALTER TABLE provider_profiles DROP COLUMN IF EXISTS notes_encrypted;
ALTER TABLE customer_profiles DROP COLUMN IF EXISTS notes_encrypted;
