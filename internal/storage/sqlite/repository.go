package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

func (r *Repository) Run(ctx context.Context, mutation application.Mutation) (*monitoring.Campaign, bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	if replay, ok, err := readIdempotent(ctx, tx, mutation.IdempotencyKey, mutation.RequestHash); err != nil {
		return nil, false, err
	} else if ok {
		return replay, true, nil
	}
	current := &monitoring.Campaign{}
	oldStatus := monitoring.Status("")
	if mutation.Create {
		var exists int
		err = tx.QueryRowContext(ctx, `SELECT 1 FROM campaigns WHERE id=?`, mutation.CampaignID).Scan(&exists)
		if err == nil {
			return nil, false, application.ErrVersionConflict
		}
		if err != sql.ErrNoRows {
			return nil, false, err
		}
	} else {
		current, err = loadCampaignTx(ctx, tx, mutation.CampaignID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, application.ErrNotFound
		}
		if err != nil {
			return nil, false, err
		}
		if current.Version != mutation.ExpectedVersion {
			return nil, false, application.ErrVersionConflict
		}
		oldStatus = current.Status
	}
	if err = mutation.Change(current); err != nil {
		return nil, false, err
	}
	if err = current.ValidateIntegrity(); err != nil {
		return nil, false, err
	}
	if mutation.Create {
		err = insertCampaign(ctx, tx, current)
	} else {
		err = updateCampaign(ctx, tx, current, mutation.ExpectedVersion)
	}
	if err != nil {
		return nil, false, err
	}
	if err = replaceEvidence(ctx, tx, current); err != nil {
		return nil, false, err
	}
	event := monitoring.AuditEvent{CampaignID: current.ID, EventType: mutation.EventType, Actor: mutation.Actor, FromStatus: oldStatus, ToStatus: current.Status, CampaignVersion: current.Version, OccurredAt: mutation.OccurredAt.UTC(), Details: mutation.Details}
	if err = insertAudit(ctx, tx, event); err != nil {
		return nil, false, err
	}
	if err = checkForeignKeys(ctx, tx); err != nil {
		return nil, false, err
	}
	response, err := json.Marshal(current)
	if err != nil {
		return nil, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO idempotency_records(key,campaign_id,request_hash,response_json,created_at) VALUES(?,?,?,?,?)`, mutation.IdempotencyKey, current.ID, mutation.RequestHash, response, mutation.OccurredAt.UTC().Format(timeLayout)); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return current, false, nil
}

func readIdempotent(ctx context.Context, tx *sql.Tx, key, hash string) (*monitoring.Campaign, bool, error) {
	var storedHash string
	var data []byte
	err := tx.QueryRowContext(ctx, `SELECT request_hash,response_json FROM idempotency_records WHERE key=?`, key).Scan(&storedHash, &data)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if storedHash != hash {
		return nil, false, application.ErrIdempotencyConflict
	}
	var campaign monitoring.Campaign
	if err = json.Unmarshal(data, &campaign); err != nil {
		return nil, false, fmt.Errorf("decode idempotent response: %w", err)
	}
	return &campaign, true, nil
}
