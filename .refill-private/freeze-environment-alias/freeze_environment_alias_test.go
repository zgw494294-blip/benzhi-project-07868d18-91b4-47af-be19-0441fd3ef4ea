package freeze_environment_alias_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

type recordingRepository struct {
	campaign *monitoring.Campaign
}

func (r *recordingRepository) Run(_ context.Context, mutation application.Mutation) (*monitoring.Campaign, bool, error) {
	if mutation.ExpectedVersion != r.campaign.Version {
		return nil, false, application.ErrVersionConflict
	}
	if err := mutation.Change(r.campaign); err != nil {
		return nil, false, err
	}
	if err := r.campaign.ValidateIntegrity(); err != nil {
		return nil, false, err
	}
	return r.campaign, false, nil
}

func (r *recordingRepository) Get(context.Context, string) (*monitoring.Campaign, error) {
	return r.campaign, nil
}

func (r *recordingRepository) Timeline(context.Context, string) ([]monitoring.AuditEvent, error) {
	return nil, nil
}

func (r *recordingRepository) Close() error { return nil }

func TestFreezeHashingDoesNotMutateEnvironmentalEvidence(t *testing.T) {
	now := time.Date(2029, 4, 5, 6, 7, 8, 0, time.UTC)
	planned := now.AddDate(0, 1, 0)
	upper := 10.0
	campaign, err := monitoring.NewCampaign(monitoring.CreateSpec{
		ID:               "campaign-negative-zero",
		FacilityName:     "一号设施",
		RoomCode:         "CR-07",
		CleanlinessClass: "ISO 7",
		PlannedDate:      planned,
		Now:              now,
		Points: []monitoring.SamplingPoint{{
			ID: "point-a", Label: "送风口", Metric: "particle", Unit: "count/m3", RequiredReplicates: 1, UpperLimit: &upper,
		}},
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	err = campaign.RegisterInstruments([]monitoring.InstrumentEvidence{{
		ID: "instrument-a", InstrumentType: "粒子计数器", SerialNumber: "SN-07", CertificateRef: "CAL-07",
		CalibratedAt: planned.AddDate(-1, 0, 0), ExpiresAt: planned.AddDate(1, 0, 0), CoveredMetrics: []string{"particle"},
	}}, "engineer-a", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("register instruments: %v", err)
	}
	err = campaign.AddRoundByActor(monitoring.MeasurementRound{
		ID: "round-a", Kind: monitoring.RoundRoutine, RecordedBy: "engineer-a",
		Samples: []monitoring.Sample{{
			ID: "sample-a", PointID: "point-a", Replicate: 1, Value: 5, Unit: "count/m3",
			Environment: map[string]float64{"temperatureOffset": math.Copysign(0, -1)},
		}},
	}, "engineer-a", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("add round: %v", err)
	}
	if err = campaign.SubmitForReview(now.Add(3 * time.Minute)); err != nil {
		t.Fatalf("submit review: %v", err)
	}

	data, err := json.Marshal(campaign)
	if err != nil {
		t.Fatalf("marshal preflight snapshot: %v", err)
	}
	var preflightSnapshot monitoring.Campaign
	if err = json.Unmarshal(data, &preflightSnapshot); err != nil {
		t.Fatalf("unmarshal preflight snapshot: %v", err)
	}
	generator := certificate.NewGenerator()
	manifestHash, err := generator.Hash(&preflightSnapshot)
	if err != nil {
		t.Fatalf("hash preflight snapshot: %v", err)
	}
	if !math.Signbit(campaign.Rounds[0].Samples[0].Environment["temperatureOffset"]) {
		t.Fatal("precondition failed: repository aggregate lost its signed zero")
	}

	repository := &recordingRepository{campaign: campaign}
	service := application.NewService(repository, generator, func() time.Time { return now.Add(4 * time.Minute) })
	_, _, err = service.Freeze(context.Background(), application.FreezeCommand{
		CommandMeta: application.CommandMeta{
			ExpectedVersion: campaign.Version,
			IdempotencyKey:  "freeze-negative-zero",
			Actor:           "reviewer-a",
			Role:            application.RoleReviewer,
		},
		CampaignID:       campaign.ID,
		CandidateVersion: campaign.Version,
		ManifestHash:     manifestHash,
	})
	if err != nil {
		t.Fatalf("freeze campaign: %v", err)
	}
	if !math.Signbit(repository.campaign.Rounds[0].Samples[0].Environment["temperatureOffset"]) {
		t.Fatalf("freeze hashing rewrote signed-zero environmental evidence before persistence")
	}
}
