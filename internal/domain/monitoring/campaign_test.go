package monitoring

import (
	"errors"
	"testing"
	"time"
)

func float(value float64) *float64 { return &value }

func newTestCampaign(t *testing.T) (*Campaign, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	campaign, err := NewCampaign(CreateSpec{
		ID:               "campaign-1",
		FacilityName:     "一号设施",
		RoomCode:         "CR-01",
		CleanlinessClass: "ISO 5",
		PlannedDate:      now.Add(24 * time.Hour),
		Now:              now,
		Points:           []SamplingPoint{{ID: "point-1", Label: "灌装点", Metric: "particles", Unit: "count/m3", RequiredReplicates: 2, UpperLimit: float(100)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return campaign, now
}

func readyCampaign(t *testing.T) (*Campaign, time.Time) {
	t.Helper()
	campaign, now := newTestCampaign(t)
	err := campaign.RegisterInstruments([]InstrumentEvidence{{ID: "instrument-1", InstrumentType: "粒子计数器", SerialNumber: "SN-1", CertificateRef: "CAL-1", CalibratedAt: now, ExpiresAt: now.Add(365 * 24 * time.Hour), CoveredMetrics: []string{"particles"}}}, "engineer", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if campaign.Status != StatusReady {
		t.Fatalf("status = %s", campaign.Status)
	}
	return campaign, now
}

func TestCampaignRemediationAndFreeze(t *testing.T) {
	campaign, now := readyCampaign(t)
	err := campaign.AddRound(MeasurementRound{ID: "round-1", Kind: RoundRoutine, RecordedBy: "engineer", Samples: []Sample{{ID: "sample-1", PointID: "point-1", Replicate: 1, Value: 150, Unit: "count/m3"}, {ID: "sample-2", PointID: "point-1", Replicate: 2, Value: 90, Unit: "count/m3"}}}, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = campaign.SubmitForReview(now.Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(campaign.Findings) != 1 || campaign.Findings[0].Code != "above_limit" {
		t.Fatalf("findings = %#v", campaign.Findings)
	}
	findingID := campaign.Findings[0].ID
	if err = campaign.DecideFinding(findingID, DecisionNeedsRemediation, "reviewer", "需要补测", "更换仪器后补测", now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if campaign.Status != StatusRemediation {
		t.Fatalf("status = %s", campaign.Status)
	}
	err = campaign.AddRound(MeasurementRound{ID: "round-2", Kind: RoundRemediation, SupersedesRoundID: "round-1", RecordedBy: "engineer", Samples: []Sample{{ID: "sample-3", PointID: "point-1", Replicate: 1, Value: 80, Unit: "count/m3"}, {ID: "sample-4", PointID: "point-1", Replicate: 2, Value: 85, Unit: "count/m3"}}}, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = campaign.SubmitForReview(now.Add(6 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(campaign.Findings) != 1 || campaign.Findings[0].Decision != DecisionRemediated {
		t.Fatalf("remediated finding not retained: %#v", campaign.Findings)
	}
	if err = campaign.Freeze("sha256:test", now.Add(7*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if campaign.Status != StatusFrozen || campaign.FrozenVersion != campaign.Version {
		t.Fatalf("invalid frozen campaign: %#v", campaign)
	}
	if err = campaign.AddRound(MeasurementRound{}, now.Add(8*time.Minute)); ErrorCode(err) != "evidence_frozen" {
		t.Fatalf("expected evidence_frozen, got %v", err)
	}
}

func TestRoundRejectsInvalidEvidence(t *testing.T) {
	tests := []struct {
		name  string
		round MeasurementRound
		code  string
	}{
		{"unknown point", MeasurementRound{ID: "r-unknown", Kind: RoundRoutine, RecordedBy: "e", Samples: []Sample{{ID: "s-1", PointID: "other", Replicate: 1, Value: 1, Unit: "count/m3"}}}, "unknown_point"},
		{"unit mismatch", MeasurementRound{ID: "r-unit", Kind: RoundRoutine, RecordedBy: "e", Samples: []Sample{{ID: "s-2", PointID: "point-1", Replicate: 1, Value: 1, Unit: "pcs"}}}, "unit_mismatch"},
		{"duplicate replicate", MeasurementRound{ID: "r-dup", Kind: RoundRoutine, RecordedBy: "e", Samples: []Sample{{ID: "s-3", PointID: "point-1", Replicate: 1, Value: 1, Unit: "count/m3"}, {ID: "s-4", PointID: "point-1", Replicate: 1, Value: 2, Unit: "count/m3"}}}, "duplicate_sample"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			campaign, now := readyCampaign(t)
			err := campaign.AddRound(test.round, now)
			if ErrorCode(err) != test.code {
				t.Fatalf("code=%s err=%v", ErrorCode(err), err)
			}
		})
	}
}

func TestCampaignValidation(t *testing.T) {
	_, err := NewCampaign(CreateSpec{})
	var rule *RuleError
	if !errors.As(err, &rule) || rule.Code != "validation_error" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlanNormalizationSummaryAndCrossValidation(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	upper := float64(10)
	campaign, err := NewCampaign(CreateSpec{ID: "plan-1", FacilityName: " 设施 ", RoomCode: " R1 ", CleanlinessClass: " ISO 5 ", PlannedDate: now.Add(time.Hour), Now: now, Points: []SamplingPoint{
		{ID: "p2", Label: " 点位二 ", Metric: " particle ", Unit: " count ", RequiredReplicates: 2, UpperLimit: &upper},
		{ID: "p1", Label: " 点位一 ", Metric: " particle ", Unit: " count ", RequiredReplicates: 3, UpperLimit: &upper},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if campaign.Version != 1 || campaign.Points[0].ID != "p1" || campaign.Points[0].Metric != "particle" {
		t.Fatalf("方案未规范化或排序：%#v", campaign)
	}
	if campaign.PlanSummary.PlannedSampleCount != 5 || len(campaign.PlanSummary.Metrics) != 1 || campaign.PlanSummary.Metrics[0].PointCount != 2 {
		t.Fatalf("方案摘要错误：%#v", campaign.PlanSummary)
	}
	_, err = NewCampaign(CreateSpec{ID: "plan-2", FacilityName: "设施", RoomCode: "R1", CleanlinessClass: "ISO 5", PlannedDate: now, Now: now, Points: []SamplingPoint{
		{ID: "a", Label: "相同点位", Metric: "m1", Unit: "u", RequiredReplicates: 1, UpperLimit: &upper},
		{ID: "b", Label: " 相同点位 ", Metric: "m2", Unit: "u", RequiredReplicates: 1, UpperLimit: &upper},
	}})
	if ErrorCode(err) != "duplicate_point_name" {
		t.Fatalf("预期 duplicate_point_name，得到 %v", err)
	}
}

func TestReadinessGapAndSamplingProgress(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	upper := float64(10)
	campaign, err := NewCampaign(CreateSpec{ID: "progress-1", FacilityName: "设施", RoomCode: "R1", CleanlinessClass: "ISO 5", PlannedDate: now.Add(time.Hour), Now: now, Points: []SamplingPoint{
		{ID: "p1", Label: "点位一", Metric: "particle", Unit: "count", RequiredReplicates: 3, UpperLimit: &upper},
		{ID: "p2", Label: "点位二", Metric: "pressure", Unit: "Pa", RequiredReplicates: 1, UpperLimit: &upper},
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = campaign.RegisterInstruments([]InstrumentEvidence{{ID: "i1", InstrumentType: "计数器", SerialNumber: "sn", CertificateRef: "cal", CalibratedAt: now, ExpiresAt: now.Add(2 * time.Hour), CoveredMetrics: []string{"particle"}}}, "engineer", now)
	if err != nil {
		t.Fatal(err)
	}
	if campaign.Status != StatusDraft || len(campaign.Readiness.MissingMetrics) != 1 || campaign.Readiness.MissingMetrics[0] != "pressure" {
		t.Fatalf("就绪缺口错误：%#v", campaign.Readiness)
	}
	err = campaign.RegisterInstruments([]InstrumentEvidence{{ID: "i2", InstrumentType: "压差计", SerialNumber: "sn2", CertificateRef: "cal2", CalibratedAt: now, ExpiresAt: now.Add(2 * time.Hour), CoveredMetrics: []string{"pressure"}}}, "engineer", now)
	if err != nil {
		t.Fatal(err)
	}
	err = campaign.AddRoundByActor(MeasurementRound{ID: "r1", Kind: RoundRoutine, RecordedBy: "engineer", Samples: []Sample{{ID: "s1", PointID: "p1", Replicate: 1, Value: 1, Unit: "count"}, {ID: "s3", PointID: "p1", Replicate: 3, Value: 1, Unit: "count"}}}, "engineer", now)
	if err != nil {
		t.Fatal(err)
	}
	progress := campaign.SamplingProgress.Points[0]
	if progress.CollectedSamples != 2 || len(progress.MissingReplicates) != 1 || progress.MissingReplicates[0] != 2 {
		t.Fatalf("采样进度错误：%#v", campaign.SamplingProgress)
	}
	version := campaign.Version
	err = campaign.AddRoundByActor(MeasurementRound{ID: "r2", Kind: RoundRoutine, RecordedBy: "other", Samples: []Sample{{ID: "s2", PointID: "p1", Replicate: 2, Value: 1, Unit: "count"}}}, "engineer", now)
	if ErrorCode(err) != "recorded_by_mismatch" || campaign.Version != version {
		t.Fatalf("来源冲突未原子拒绝：version=%d err=%v", campaign.Version, err)
	}
}

func TestRemediationClosesOnlyCompletelyCoveredFinding(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	upper := float64(10)
	campaign, err := NewCampaign(CreateSpec{ID: "remediation-precise", FacilityName: "设施", RoomCode: "R1", CleanlinessClass: "ISO 5", PlannedDate: now.Add(time.Hour), Now: now, Points: []SamplingPoint{
		{ID: "p1", Label: "点位一", Metric: "m", Unit: "u", RequiredReplicates: 1, UpperLimit: &upper},
		{ID: "p2", Label: "点位二", Metric: "m", Unit: "u", RequiredReplicates: 1, UpperLimit: &upper},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err = campaign.RegisterInstruments([]InstrumentEvidence{{ID: "i1", InstrumentType: "仪器", SerialNumber: "sn", CertificateRef: "cal", CalibratedAt: now, ExpiresAt: now.Add(2 * time.Hour), CoveredMetrics: []string{"m"}}}, "engineer", now); err != nil {
		t.Fatal(err)
	}
	if err = campaign.AddRoundByActor(MeasurementRound{ID: "r1", Kind: RoundRoutine, RecordedBy: "engineer", Samples: []Sample{{ID: "s1", PointID: "p1", Replicate: 1, Value: 20, Unit: "u"}, {ID: "s2", PointID: "p2", Replicate: 1, Value: 20, Unit: "u"}}}, "engineer", now); err != nil {
		t.Fatal(err)
	}
	if err = campaign.SubmitForReview(now); err != nil {
		t.Fatal(err)
	}
	for _, finding := range append([]Finding(nil), campaign.Findings...) {
		if err = campaign.DecideFinding(finding.ID, DecisionNeedsRemediation, "reviewer", "", "补测", now); err != nil {
			t.Fatal(err)
		}
	}
	if err = campaign.AddRoundByActor(MeasurementRound{ID: "r2", Kind: RoundRemediation, SupersedesRoundID: "r1", RecordedBy: "engineer", Samples: []Sample{{ID: "s3", PointID: "p1", Replicate: 1, Value: 5, Unit: "u"}}}, "engineer", now); err != nil {
		t.Fatal(err)
	}
	if campaign.Findings[0].Decision == campaign.Findings[1].Decision {
		t.Fatalf("部分补测错误地关闭了全部发现：%#v", campaign.Findings)
	}
	if err = campaign.SubmitForReview(now); ErrorCode(err) != "remediation_evidence_incomplete" {
		t.Fatalf("预期返回剩余整改缺口，得到 %v", err)
	}
}
