package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

type rowScanner interface{ Scan(...any) error }

func scanCampaign(row rowScanner) (*monitoring.Campaign, error) {
	var data []byte
	if err := row.Scan(&data); err != nil {
		return nil, err
	}
	var c monitoring.Campaign
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
func loadCampaignTx(ctx context.Context, tx *sql.Tx, id string) (*monitoring.Campaign, error) {
	return scanCampaign(tx.QueryRowContext(ctx, `SELECT aggregate_json FROM campaigns WHERE id=?`, id))
}

func (r *Repository) Get(ctx context.Context, id string) (*monitoring.Campaign, error) {
	c, err := scanCampaign(r.db.QueryRowContext(ctx, `SELECT aggregate_json FROM campaigns WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, application.ErrNotFound
	}
	if err == nil {
		c.PersistenceEvidenceMismatch, c.PersistenceCredentialMismatch, err = r.detectPersistenceMismatch(ctx, c)
	}
	return c, err
}

func (r *Repository) Timeline(ctx context.Context, id string) ([]monitoring.AuditEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT sequence,campaign_id,event_type,actor,from_status,to_status,campaign_version,occurred_at,details FROM audit_events WHERE campaign_id=? ORDER BY sequence`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []monitoring.AuditEvent{}
	for rows.Next() {
		var e monitoring.AuditEvent
		var at string
		if err = rows.Scan(&e.Sequence, &e.CampaignID, &e.EventType, &e.Actor, &e.FromStatus, &e.ToStatus, &e.CampaignVersion, &at, &e.Details); err != nil {
			return nil, err
		}
		e.OccurredAt, _ = time.Parse(timeLayout, at)
		events = append(events, e)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		if _, err = r.Get(ctx, id); err != nil {
			return nil, err
		}
	}
	return events, nil
}
