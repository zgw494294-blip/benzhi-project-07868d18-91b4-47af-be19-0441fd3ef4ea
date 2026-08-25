package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

const timeLayout = time.RFC3339Nano

func campaignArgs(c *monitoring.Campaign) ([]byte, []any, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, nil, err
	}
	args := []any{c.FacilityName, c.RoomCode, c.CleanlinessClass, c.PlannedDate.UTC().Format(timeLayout), c.Status, c.Version, data, c.CreatedAt.UTC().Format(timeLayout), c.UpdatedAt.UTC().Format(timeLayout), c.ManifestHash, c.FrozenVersion}
	return data, args, nil
}

func insertCampaign(ctx context.Context, tx *sql.Tx, c *monitoring.Campaign) error {
	data, _, err := campaignArgs(c)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaigns(id,facility_name,room_code,cleanliness_class,planned_date,status,version,aggregate_json,created_at,updated_at,manifest_hash,frozen_version) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, c.FacilityName, c.RoomCode, c.CleanlinessClass, c.PlannedDate.UTC().Format(timeLayout), c.Status, c.Version, data, c.CreatedAt.UTC().Format(timeLayout), c.UpdatedAt.UTC().Format(timeLayout), c.ManifestHash, c.FrozenVersion)
	return err
}

func updateCampaign(ctx context.Context, tx *sql.Tx, c *monitoring.Campaign, expected int64) error {
	data, _, err := campaignArgs(c)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE campaigns SET facility_name=?,room_code=?,cleanliness_class=?,planned_date=?,status=?,version=?,aggregate_json=?,updated_at=?,manifest_hash=?,frozen_version=? WHERE id=? AND version=?`, c.FacilityName, c.RoomCode, c.CleanlinessClass, c.PlannedDate.UTC().Format(timeLayout), c.Status, c.Version, data, c.UpdatedAt.UTC().Format(timeLayout), c.ManifestHash, c.FrozenVersion, c.ID, expected)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return application.ErrVersionConflict
	}
	return nil
}

func replaceEvidence(ctx context.Context, tx *sql.Tx, c *monitoring.Campaign) error {
	for _, table := range []string{"sampling_points", "instrument_evidence", "measurement_rounds", "findings", "inspection_batches", "release_credentials"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE campaign_id=?`, c.ID); err != nil {
			return err
		}
	}
	for _, p := range c.Points {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sampling_points(id,campaign_id,label,metric,unit,required_replicates,lower_limit,upper_limit) VALUES(?,?,?,?,?,?,?,?)`, p.ID, c.ID, p.Label, p.Metric, p.Unit, p.RequiredReplicates, p.LowerLimit, p.UpperLimit); err != nil {
			return err
		}
	}
	for _, item := range c.Instruments {
		metrics, _ := json.Marshal(item.CoveredMetrics)
		if _, err := tx.ExecContext(ctx, `INSERT INTO instrument_evidence(id,campaign_id,instrument_type,serial_number,certificate_ref,calibrated_at,expires_at,covered_metrics_json) VALUES(?,?,?,?,?,?,?,?)`, item.ID, c.ID, item.InstrumentType, item.SerialNumber, item.CertificateRef, item.CalibratedAt.UTC().Format(timeLayout), item.ExpiresAt.UTC().Format(timeLayout), metrics); err != nil {
			return err
		}
	}
	for _, round := range c.Rounds {
		samples, _ := json.Marshal(round.Samples)
		if _, err := tx.ExecContext(ctx, `INSERT INTO measurement_rounds(id,campaign_id,round_number,kind,samples_json,recorded_by,recorded_at,supersedes_round_id) VALUES(?,?,?,?,?,?,?,?)`, round.ID, c.ID, round.RoundNumber, round.Kind, samples, round.RecordedBy, round.RecordedAt.UTC().Format(timeLayout), round.SupersedesRoundID); err != nil {
			return err
		}
	}
	for _, batch := range c.InspectionBatches {
		roundIDs, _ := json.Marshal(batch.EffectiveRoundIDs)
		pointStats, _ := json.Marshal(batch.PointStats)
		if _, err := tx.ExecContext(ctx, `INSERT INTO inspection_batches(id,campaign_id,source_version,checked_at,effective_round_ids_json,point_stats_json,finding_count) VALUES(?,?,?,?,?,?,?)`, batch.ID, c.ID, batch.SourceVersion, batch.CheckedAt.UTC().Format(timeLayout), roundIDs, pointStats, batch.FindingCount); err != nil {
			return err
		}
	}
	for _, f := range c.Findings {
		var decided any
		if f.DecidedAt != nil {
			decided = f.DecidedAt.UTC().Format(timeLayout)
		}
		roundIDs, _ := json.Marshal(f.EvidenceRoundIDs)
		sampleIDs, _ := json.Marshal(f.EvidenceSampleIDs)
		missing, _ := json.Marshal(f.MissingReplicates)
		if _, err := tx.ExecContext(ctx, `INSERT INTO findings(id,campaign_id,code,point_id,round_id,check_batch_id,finding_key,evidence_round_ids_json,evidence_sample_ids_json,missing_replicates_json,message,decision,decided_by,decision_note,remediation_note,remediation_round_id,created_at,decided_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, f.ID, c.ID, f.Code, f.PointID, f.RoundID, f.CheckBatchID, f.FindingKey, roundIDs, sampleIDs, missing, f.Message, f.Decision, f.DecidedBy, f.DecisionNote, f.RemediationNote, f.RemediationRound, f.CreatedAt.UTC().Format(timeLayout), decided); err != nil {
			return err
		}
	}
	if c.Credential != nil {
		v := c.Credential
		if _, err := tx.ExecContext(ctx, `INSERT INTO release_credentials(id,campaign_id,frozen_version,manifest_hash,issued_by,issued_at,credential_digest) VALUES(?,?,?,?,?,?,?)`, v.ID, c.ID, v.FrozenVersion, v.ManifestHash, v.IssuedBy, v.IssuedAt.UTC().Format(timeLayout), v.CredentialDigest); err != nil {
			return err
		}
	}
	return nil
}

type auditExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertAudit(ctx context.Context, executor auditExecutor, event monitoring.AuditEvent) error {
	_, err := executor.ExecContext(ctx, `INSERT INTO audit_events(campaign_id,event_type,actor,from_status,to_status,campaign_version,occurred_at,details) VALUES(?,?,?,?,?,?,?,?)`, event.CampaignID, event.EventType, event.Actor, event.FromStatus, event.ToStatus, event.CampaignVersion, event.OccurredAt.UTC().Format(timeLayout), event.Details)
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}
