package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 2

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS schema_meta (id INTEGER PRIMARY KEY CHECK (id=1), version INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS campaigns (id TEXT PRIMARY KEY, facility_name TEXT NOT NULL, room_code TEXT NOT NULL, cleanliness_class TEXT NOT NULL, planned_date TEXT NOT NULL, status TEXT NOT NULL, version INTEGER NOT NULL, aggregate_json BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, manifest_hash TEXT NOT NULL DEFAULT '', frozen_version INTEGER NOT NULL DEFAULT 0)`,
	`CREATE TABLE IF NOT EXISTS sampling_points (id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, label TEXT NOT NULL, metric TEXT NOT NULL, unit TEXT NOT NULL, required_replicates INTEGER NOT NULL, lower_limit REAL, upper_limit REAL)`,
	`CREATE TABLE IF NOT EXISTS instrument_evidence (id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, instrument_type TEXT NOT NULL, serial_number TEXT NOT NULL, certificate_ref TEXT NOT NULL, calibrated_at TEXT NOT NULL, expires_at TEXT NOT NULL, covered_metrics_json BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS measurement_rounds (id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, round_number INTEGER NOT NULL, kind TEXT NOT NULL, samples_json BLOB NOT NULL, recorded_by TEXT NOT NULL, recorded_at TEXT NOT NULL, supersedes_round_id TEXT NOT NULL DEFAULT '')`,
	`CREATE TABLE IF NOT EXISTS inspection_batches (id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, source_version INTEGER NOT NULL, checked_at TEXT NOT NULL, effective_round_ids_json BLOB NOT NULL, point_stats_json BLOB NOT NULL, finding_count INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS findings (id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, code TEXT NOT NULL, point_id TEXT NOT NULL DEFAULT '', round_id TEXT NOT NULL DEFAULT '', check_batch_id TEXT NOT NULL DEFAULT '', finding_key TEXT NOT NULL DEFAULT '', evidence_round_ids_json BLOB NOT NULL DEFAULT '[]', evidence_sample_ids_json BLOB NOT NULL DEFAULT '[]', missing_replicates_json BLOB NOT NULL DEFAULT '[]', message TEXT NOT NULL, decision TEXT NOT NULL, decided_by TEXT NOT NULL DEFAULT '', decision_note TEXT NOT NULL DEFAULT '', remediation_note TEXT NOT NULL DEFAULT '', remediation_round_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, decided_at TEXT)`,
	`CREATE TABLE IF NOT EXISTS release_credentials (id TEXT PRIMARY KEY, campaign_id TEXT NOT NULL UNIQUE REFERENCES campaigns(id) ON DELETE CASCADE, frozen_version INTEGER NOT NULL, manifest_hash TEXT NOT NULL, issued_by TEXT NOT NULL, issued_at TEXT NOT NULL, credential_digest TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS idempotency_records (key TEXT PRIMARY KEY, campaign_id TEXT NOT NULL, request_hash TEXT NOT NULL, response_json BLOB NOT NULL, created_at TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS audit_events (sequence INTEGER PRIMARY KEY AUTOINCREMENT, campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, event_type TEXT NOT NULL, actor TEXT NOT NULL, from_status TEXT NOT NULL, to_status TEXT NOT NULL, campaign_version INTEGER NOT NULL, occurred_at TEXT NOT NULL, details TEXT NOT NULL DEFAULT '')`,
	`CREATE INDEX IF NOT EXISTS idx_audit_campaign_sequence ON audit_events(campaign_id,sequence)`,
	`CREATE INDEX IF NOT EXISTS idx_round_campaign_number ON measurement_rounds(campaign_id,round_number)`,
}

func (r *Repository) migrate(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, schemaStatements[0]); err != nil {
		return fmt.Errorf("create schema metadata: %w", err)
	}
	var version int
	err = tx.QueryRowContext(ctx, `SELECT version FROM schema_meta WHERE id=1`).Scan(&version)
	if err == sql.ErrNoRows {
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_meta(id,version) VALUES(1,?)`, schemaVersion); err != nil {
			return err
		}
		version = schemaVersion
	} else if err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("unsupported database schema version %d", version)
	}
	for _, statement := range schemaStatements[1:] {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply schema migration: %w", err)
		}
	}
	if version < 2 {
		upgrades := []string{
			`ALTER TABLE findings ADD COLUMN check_batch_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE findings ADD COLUMN finding_key TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE findings ADD COLUMN evidence_round_ids_json BLOB NOT NULL DEFAULT '[]'`,
			`ALTER TABLE findings ADD COLUMN evidence_sample_ids_json BLOB NOT NULL DEFAULT '[]'`,
			`ALTER TABLE findings ADD COLUMN missing_replicates_json BLOB NOT NULL DEFAULT '[]'`,
		}
		for _, statement := range upgrades {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("upgrade schema to version 2: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE schema_meta SET version=? WHERE id=1`, schemaVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}
