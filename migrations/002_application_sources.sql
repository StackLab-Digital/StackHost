-- StackHost migration 2: secure application source configuration.
ALTER TABLE applications ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE applications ADD COLUMN configuration_status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE applications ADD COLUMN configured_at TEXT;
ALTER TABLE applications ADD COLUMN last_validated_at TEXT;
ALTER TABLE applications ADD COLUMN source_revision INTEGER NOT NULL DEFAULT 0;
UPDATE applications SET configuration_status = 'draft' WHERE status = 'unknown';
CREATE TABLE IF NOT EXISTS application_sources (
  id INTEGER PRIMARY KEY,
  application_id INTEGER NOT NULL UNIQUE REFERENCES applications(id) ON DELETE CASCADE,
  source_type TEXT NOT NULL,
  encrypted_payload BLOB NOT NULL,
  encryption_nonce BLOB NOT NULL,
  payload_version INTEGER NOT NULL DEFAULT 1,
  checksum TEXT NOT NULL,
  validation_status TEXT NOT NULL DEFAULT 'draft',
  validation_errors TEXT NOT NULL DEFAULT '[]',
  validation_warnings TEXT NOT NULL DEFAULT '[]',
  summary_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_application_sources_application ON application_sources(application_id);
