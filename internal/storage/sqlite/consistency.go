package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"cleanroom-monitor-release/internal/domain/monitoring"
)

func (r *Repository) detectPersistenceMismatch(ctx context.Context, c *monitoring.Campaign) (bool, bool, error) {
	expected, err := expectedEvidenceRows(c)
	if err != nil {
		return false, false, err
	}
	actual, err := r.actualEvidenceRows(ctx, c.ID)
	if err != nil {
		return false, false, err
	}
	credentialMismatch, err := r.credentialMismatch(ctx, c)
	if err != nil {
		return false, false, err
	}
	return strings.Join(expected, "\n") != strings.Join(actual, "\n"), credentialMismatch, nil
}

func expectedEvidenceRows(c *monitoring.Campaign) ([]string, error) {
	rows := []string{}
	for _, point := range c.Points {
		rows = append(rows, fmt.Sprintf("point|%q|%q|%q|%q|%q|%d|%s|%s", point.ID, point.CampaignID, point.Label, point.Metric, point.Unit, point.RequiredReplicates, pointFloat(point.LowerLimit), pointFloat(point.UpperLimit)))
	}
	for _, item := range c.Instruments {
		metrics, err := json.Marshal(item.CoveredMetrics)
		if err != nil {
			return nil, err
		}
		rows = append(rows, fmt.Sprintf("instrument|%q|%q|%q|%q|%q|%q|%q|%s", item.ID, item.CampaignID, item.InstrumentType, item.SerialNumber, item.CertificateRef, item.CalibratedAt.UTC().Format(timeLayout), item.ExpiresAt.UTC().Format(timeLayout), metrics))
	}
	for _, round := range c.Rounds {
		samples, err := json.Marshal(round.Samples)
		if err != nil {
			return nil, err
		}
		rows = append(rows, fmt.Sprintf("round|%q|%q|%d|%q|%s|%q|%q|%q", round.ID, round.CampaignID, round.RoundNumber, round.Kind, samples, round.RecordedBy, round.RecordedAt.UTC().Format(timeLayout), round.SupersedesRoundID))
	}
	for _, batch := range c.InspectionBatches {
		roundIDs, err := json.Marshal(batch.EffectiveRoundIDs)
		if err != nil {
			return nil, err
		}
		stats, err := json.Marshal(batch.PointStats)
		if err != nil {
			return nil, err
		}
		rows = append(rows, fmt.Sprintf("batch|%q|%q|%d|%q|%s|%s|%d", batch.ID, batch.CampaignID, batch.SourceVersion, batch.CheckedAt.UTC().Format(timeLayout), roundIDs, stats, batch.FindingCount))
	}
	for _, finding := range c.Findings {
		roundIDs, err := json.Marshal(finding.EvidenceRoundIDs)
		if err != nil {
			return nil, err
		}
		sampleIDs, err := json.Marshal(finding.EvidenceSampleIDs)
		if err != nil {
			return nil, err
		}
		missing, err := json.Marshal(finding.MissingReplicates)
		if err != nil {
			return nil, err
		}
		decided := ""
		if finding.DecidedAt != nil {
			decided = finding.DecidedAt.UTC().Format(timeLayout)
		}
		rows = append(rows, fmt.Sprintf("finding|%q|%q|%q|%q|%q|%q|%q|%s|%s|%s|%q|%q|%q|%q|%q|%q|%q|%q", finding.ID, c.ID, finding.Code, finding.PointID, finding.RoundID, finding.CheckBatchID, finding.FindingKey, roundIDs, sampleIDs, missing, finding.Message, finding.Decision, finding.DecidedBy, finding.DecisionNote, finding.RemediationNote, finding.RemediationRound, finding.CreatedAt.UTC().Format(timeLayout), decided))
	}
	sort.Strings(rows)
	return rows, nil
}

func (r *Repository) actualEvidenceRows(ctx context.Context, campaignID string) ([]string, error) {
	rows := []string{}
	pointRows, err := r.db.QueryContext(ctx, `SELECT id,campaign_id,label,metric,unit,required_replicates,lower_limit,upper_limit FROM sampling_points WHERE campaign_id=?`, campaignID)
	if err != nil {
		return nil, err
	}
	for pointRows.Next() {
		var id, owner, label, metric, unit string
		var replicates int
		var lower, upper sql.NullFloat64
		if err = pointRows.Scan(&id, &owner, &label, &metric, &unit, &replicates, &lower, &upper); err != nil {
			pointRows.Close()
			return nil, err
		}
		rows = append(rows, fmt.Sprintf("point|%q|%q|%q|%q|%q|%d|%s|%s", id, owner, label, metric, unit, replicates, nullFloat(lower), nullFloat(upper)))
	}
	if err = closeRows(pointRows); err != nil {
		return nil, err
	}

	instrumentRows, err := r.db.QueryContext(ctx, `SELECT id,campaign_id,instrument_type,serial_number,certificate_ref,calibrated_at,expires_at,covered_metrics_json FROM instrument_evidence WHERE campaign_id=?`, campaignID)
	if err != nil {
		return nil, err
	}
	for instrumentRows.Next() {
		var id, owner, instrumentType, serial, certificateRef, calibrated, expires string
		var metrics []byte
		if err = instrumentRows.Scan(&id, &owner, &instrumentType, &serial, &certificateRef, &calibrated, &expires, &metrics); err != nil {
			instrumentRows.Close()
			return nil, err
		}
		rows = append(rows, fmt.Sprintf("instrument|%q|%q|%q|%q|%q|%q|%q|%s", id, owner, instrumentType, serial, certificateRef, calibrated, expires, metrics))
	}
	if err = closeRows(instrumentRows); err != nil {
		return nil, err
	}

	roundRows, err := r.db.QueryContext(ctx, `SELECT id,campaign_id,round_number,kind,samples_json,recorded_by,recorded_at,supersedes_round_id FROM measurement_rounds WHERE campaign_id=?`, campaignID)
	if err != nil {
		return nil, err
	}
	for roundRows.Next() {
		var id, owner, kind, recordedBy, recordedAt, supersedes string
		var number int
		var samples []byte
		if err = roundRows.Scan(&id, &owner, &number, &kind, &samples, &recordedBy, &recordedAt, &supersedes); err != nil {
			roundRows.Close()
			return nil, err
		}
		rows = append(rows, fmt.Sprintf("round|%q|%q|%d|%q|%s|%q|%q|%q", id, owner, number, kind, samples, recordedBy, recordedAt, supersedes))
	}
	if err = closeRows(roundRows); err != nil {
		return nil, err
	}

	batchRows, err := r.db.QueryContext(ctx, `SELECT id,campaign_id,source_version,checked_at,effective_round_ids_json,point_stats_json,finding_count FROM inspection_batches WHERE campaign_id=?`, campaignID)
	if err != nil {
		return nil, err
	}
	for batchRows.Next() {
		var id, owner, checkedAt string
		var version int64
		var roundIDs, stats []byte
		var findingCount int
		if err = batchRows.Scan(&id, &owner, &version, &checkedAt, &roundIDs, &stats, &findingCount); err != nil {
			batchRows.Close()
			return nil, err
		}
		rows = append(rows, fmt.Sprintf("batch|%q|%q|%d|%q|%s|%s|%d", id, owner, version, checkedAt, roundIDs, stats, findingCount))
	}
	if err = closeRows(batchRows); err != nil {
		return nil, err
	}

	findingRows, err := r.db.QueryContext(ctx, `SELECT id,campaign_id,code,point_id,round_id,check_batch_id,finding_key,evidence_round_ids_json,evidence_sample_ids_json,missing_replicates_json,message,decision,decided_by,decision_note,remediation_note,remediation_round_id,created_at,COALESCE(decided_at,'') FROM findings WHERE campaign_id=?`, campaignID)
	if err != nil {
		return nil, err
	}
	for findingRows.Next() {
		var id, owner, code, pointID, roundID, batchID, key, message, decision, decidedBy, decisionNote, remediationNote, remediationRound, createdAt, decidedAt string
		var roundIDs, sampleIDs, missing []byte
		if err = findingRows.Scan(&id, &owner, &code, &pointID, &roundID, &batchID, &key, &roundIDs, &sampleIDs, &missing, &message, &decision, &decidedBy, &decisionNote, &remediationNote, &remediationRound, &createdAt, &decidedAt); err != nil {
			findingRows.Close()
			return nil, err
		}
		rows = append(rows, fmt.Sprintf("finding|%q|%q|%q|%q|%q|%q|%q|%s|%s|%s|%q|%q|%q|%q|%q|%q|%q|%q", id, owner, code, pointID, roundID, batchID, key, roundIDs, sampleIDs, missing, message, decision, decidedBy, decisionNote, remediationNote, remediationRound, createdAt, decidedAt))
	}
	if err = closeRows(findingRows); err != nil {
		return nil, err
	}
	sort.Strings(rows)
	return rows, nil
}

func (r *Repository) credentialMismatch(ctx context.Context, c *monitoring.Campaign) (bool, error) {
	var id, campaignID, manifestHash, issuedBy, issuedAt, credentialDigest string
	var frozenVersion int64
	err := r.db.QueryRowContext(ctx, `SELECT id,campaign_id,frozen_version,manifest_hash,issued_by,issued_at,credential_digest FROM release_credentials WHERE campaign_id=?`, c.ID).Scan(&id, &campaignID, &frozenVersion, &manifestHash, &issuedBy, &issuedAt, &credentialDigest)
	if err == sql.ErrNoRows {
		return c.Credential != nil, nil
	}
	if err != nil {
		return false, err
	}
	if c.Credential == nil {
		return true, nil
	}
	v := c.Credential
	return id != v.ID || campaignID != v.CampaignID || frozenVersion != v.FrozenVersion || manifestHash != v.ManifestHash || issuedBy != v.IssuedBy || issuedAt != v.IssuedAt.UTC().Format(timeLayout) || credentialDigest != v.CredentialDigest, nil
}

func closeRows(rows *sql.Rows) error {
	err := rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func pointFloat(value *float64) string {
	if value == nil {
		return "null"
	}
	return strconv.FormatFloat(*value, 'g', -1, 64)
}

func nullFloat(value sql.NullFloat64) string {
	if !value.Valid {
		return "null"
	}
	return strconv.FormatFloat(value.Float64, 'g', -1, 64)
}
