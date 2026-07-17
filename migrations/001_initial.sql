-- Version 1: initial StackHost schema.
-- The same migration is applied transactionally by cmd/stackhost at startup.
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
