package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const schemaVersion = 3

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS schema_meta (id INTEGER PRIMARY KEY CHECK (id=1), version INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS campaigns (id TEXT PRIMARY KEY, facility_name TEXT NOT NULL, room_code TEXT NOT NULL, cleanliness_class TEXT NOT NULL, planned_date TEXT NOT NULL, status TEXT NOT NULL, version INTEGER NOT NULL, aggregate_json BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, manifest_hash TEXT NOT NULL DEFAULT '', frozen_version INTEGER NOT NULL DEFAULT 0)`,
	`CREATE TABLE IF NOT EXISTS sampling_points (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, label TEXT NOT NULL, metric TEXT NOT NULL, unit TEXT NOT NULL, required_replicates INTEGER NOT NULL, lower_limit REAL, upper_limit REAL, PRIMARY KEY (campaign_id, id))`,
	`CREATE TABLE IF NOT EXISTS instrument_evidence (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, instrument_type TEXT NOT NULL, serial_number TEXT NOT NULL, certificate_ref TEXT NOT NULL, calibrated_at TEXT NOT NULL, expires_at TEXT NOT NULL, covered_metrics_json BLOB NOT NULL, PRIMARY KEY (campaign_id, id))`,
	`CREATE TABLE IF NOT EXISTS measurement_rounds (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, round_number INTEGER NOT NULL, kind TEXT NOT NULL, samples_json BLOB NOT NULL, recorded_by TEXT NOT NULL, recorded_at TEXT NOT NULL, supersedes_round_id TEXT NOT NULL DEFAULT '', PRIMARY KEY (campaign_id, id))`,
	`CREATE TABLE IF NOT EXISTS inspection_batches (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, source_version INTEGER NOT NULL, checked_at TEXT NOT NULL, effective_round_ids_json BLOB NOT NULL, point_stats_json BLOB NOT NULL, finding_count INTEGER NOT NULL, PRIMARY KEY (campaign_id, id))`,
	`CREATE TABLE IF NOT EXISTS findings (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, code TEXT NOT NULL, point_id TEXT NOT NULL DEFAULT '', round_id TEXT NOT NULL DEFAULT '', check_batch_id TEXT NOT NULL DEFAULT '', finding_key TEXT NOT NULL DEFAULT '', evidence_round_ids_json BLOB NOT NULL DEFAULT '[]', evidence_sample_ids_json BLOB NOT NULL DEFAULT '[]', missing_replicates_json BLOB NOT NULL DEFAULT '[]', message TEXT NOT NULL, decision TEXT NOT NULL, decided_by TEXT NOT NULL DEFAULT '', decision_note TEXT NOT NULL DEFAULT '', remediation_note TEXT NOT NULL DEFAULT '', remediation_round_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, decided_at TEXT, PRIMARY KEY (campaign_id, id))`,
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
				// Tolerate columns that already exist when migrating an intermediate database.
				if isDuplicateColumnErr(err) {
					continue
				}
				return fmt.Errorf("upgrade schema to version 2: %w", err)
			}
		}
	}
	if version < 3 {
		if err = migrateScopedEvidencePrimaryKey(ctx, tx); err != nil {
			return fmt.Errorf("upgrade schema to version 3: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE schema_meta SET version=? WHERE id=1`, schemaVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// migrateScopedEvidencePrimaryKey rebuilds the campaign-scoped evidence tables so
// their primary key becomes (campaign_id, id). This allows different campaigns to
// reuse sampling point, instrument and round IDs while still rejecting duplicates
// within a single campaign. Existing rows are preserved.
func migrateScopedEvidencePrimaryKey(ctx context.Context, tx *sql.Tx) error {
	rebuilds := []struct {
		table   string
		columns string
	}{
		{"sampling_points", "campaign_id,id,label,metric,unit,required_replicates,lower_limit,upper_limit"},
		{"instrument_evidence", "campaign_id,id,instrument_type,serial_number,certificate_ref,calibrated_at,expires_at,covered_metrics_json"},
		{"measurement_rounds", "campaign_id,id,round_number,kind,samples_json,recorded_by,recorded_at,supersedes_round_id"},
		{"inspection_batches", "campaign_id,id,source_version,checked_at,effective_round_ids_json,point_stats_json,finding_count"},
		{"findings", "campaign_id,id,code,point_id,round_id,check_batch_id,finding_key,evidence_round_ids_json,evidence_sample_ids_json,missing_replicates_json,message,decision,decided_by,decision_note,remediation_note,remediation_round_id,created_at,decided_at"},
	}
	for _, item := range rebuilds {
		if err := rebuildTablePrimaryKey(ctx, tx, item.table, item.columns); err != nil {
			return err
		}
	}
	// Recreate the round-number index dropped alongside the rebuilt table.
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_round_campaign_number ON measurement_rounds(campaign_id,round_number)`); err != nil {
		return fmt.Errorf("recreate measurement_rounds index: %w", err)
	}
	return nil
}

func rebuildTablePrimaryKey(ctx context.Context, tx *sql.Tx, table, columns string) error {
	tempName := table + "_v3_rebuild"
	// Drop a leftover temp table from a previously interrupted migration attempt.
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS ` + tempName); err != nil {
		return fmt.Errorf("drop temp table %s: %w", tempName, err)
	}
	// Build the replacement table with the campaign-scoped composite primary key.
	def, ok := v3TableDefinitions[table]
	if !ok {
		return fmt.Errorf("missing v3 definition for table %s", table)
	}
	if _, err := tx.ExecContext(ctx, def); err != nil {
		return fmt.Errorf("create replacement table %s: %w", table, err)
	}
	// Preserve all existing rows.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s(%s) SELECT %s FROM %s`, tempName, columns, columns, table)); err != nil {
		return fmt.Errorf("copy rows for table %s: %w", table, err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE ` + table); err != nil {
		return fmt.Errorf("drop legacy table %s: %w", table, err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE ` + tempName + ` RENAME TO ` + table); err != nil {
		return fmt.Errorf("rename replacement table %s: %w", table, err)
	}
	return nil
}

// v3TableDefinitions holds the campaign-scoped replacement schemas used when
// rebuilding evidence tables that were created with a globally-unique id primary key.
var v3TableDefinitions = map[string]string{
	"sampling_points":     `CREATE TABLE sampling_points_v3_rebuild (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, label TEXT NOT NULL, metric TEXT NOT NULL, unit TEXT NOT NULL, required_replicates INTEGER NOT NULL, lower_limit REAL, upper_limit REAL, PRIMARY KEY (campaign_id, id))`,
	"instrument_evidence": `CREATE TABLE instrument_evidence_v3_rebuild (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, instrument_type TEXT NOT NULL, serial_number TEXT NOT NULL, certificate_ref TEXT NOT NULL, calibrated_at TEXT NOT NULL, expires_at TEXT NOT NULL, covered_metrics_json BLOB NOT NULL, PRIMARY KEY (campaign_id, id))`,
	"measurement_rounds":  `CREATE TABLE measurement_rounds_v3_rebuild (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, round_number INTEGER NOT NULL, kind TEXT NOT NULL, samples_json BLOB NOT NULL, recorded_by TEXT NOT NULL, recorded_at TEXT NOT NULL, supersedes_round_id TEXT NOT NULL DEFAULT '', PRIMARY KEY (campaign_id, id))`,
	"inspection_batches":  `CREATE TABLE inspection_batches_v3_rebuild (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, source_version INTEGER NOT NULL, checked_at TEXT NOT NULL, effective_round_ids_json BLOB NOT NULL, point_stats_json BLOB NOT NULL, finding_count INTEGER NOT NULL, PRIMARY KEY (campaign_id, id))`,
	"findings":            `CREATE TABLE findings_v3_rebuild (campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE, id TEXT NOT NULL, code TEXT NOT NULL, point_id TEXT NOT NULL DEFAULT '', round_id TEXT NOT NULL DEFAULT '', check_batch_id TEXT NOT NULL DEFAULT '', finding_key TEXT NOT NULL DEFAULT '', evidence_round_ids_json BLOB NOT NULL DEFAULT '[]', evidence_sample_ids_json BLOB NOT NULL DEFAULT '[]', missing_replicates_json BLOB NOT NULL DEFAULT '[]', message TEXT NOT NULL, decision TEXT NOT NULL, decided_by TEXT NOT NULL DEFAULT '', decision_note TEXT NOT NULL DEFAULT '', remediation_note TEXT NOT NULL DEFAULT '', remediation_round_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, decided_at TEXT, PRIMARY KEY (campaign_id, id))`,
}

func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate column name")
}
