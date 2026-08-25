package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

var requiredTables = []string{
	"audit_events",
	"campaigns",
	"findings",
	"idempotency_records",
	"instrument_evidence",
	"measurement_rounds",
	"release_credentials",
	"sampling_points",
	"schema_meta",
}

func (r *Repository) verifySchema(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		return fmt.Errorf("inspect sqlite schema: %w", err)
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return err
		}
		found[name] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}
	missing := make([]string, 0)
	for _, table := range requiredTables {
		if !found[table] {
			missing = append(missing, table)
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return fmt.Errorf("database schema is incomplete: %s", strings.Join(missing, ","))
	}
	var integrity string
	if err = r.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("run sqlite quick_check: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("sqlite integrity check failed: %s", integrity)
	}
	return nil
}

func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		var table, parent string
		var rowID, foreignKey int64
		if err = rows.Scan(&table, &rowID, &parent, &foreignKey); err != nil {
			return err
		}
		return fmt.Errorf("foreign key violation table=%s row=%d parent=%s key=%d", table, rowID, parent, foreignKey)
	}
	return rows.Err()
}
