package stalefreezepreflightcache

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/domain/monitoring"
	"cleanroom-monitor-release/internal/transport/httpapi"
)

type stateRepository struct {
	mu       sync.Mutex
	campaign *monitoring.Campaign
}

func (r *stateRepository) Run(_ context.Context, mutation application.Mutation) (*monitoring.Campaign, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if mutation.ExpectedVersion != r.campaign.Version {
		return nil, false, application.ErrVersionConflict
	}
	if err := mutation.Change(r.campaign); err != nil {
		return nil, false, err
	}
	copy := *r.campaign
	return &copy, false, nil
}

func (r *stateRepository) Get(context.Context, string) (*monitoring.Campaign, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *r.campaign
	return &copy, nil
}

func (r *stateRepository) Timeline(context.Context, string) ([]monitoring.AuditEvent, error) {
	return nil, nil
}

func (r *stateRepository) Close() error { return nil }

func TestFreezePreflightCacheDoesNotCrossCampaignVersions(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	campaign := eligibleCampaign(t, now)
	repo := &stateRepository{campaign: campaign}
	service := application.NewService(repo, certificate.NewGenerator(), func() time.Time { return now.Add(10 * time.Minute) })
	handler := httpapi.New(service).Handler()

	first := getPreflight(t, handler, campaign.Version)
	if !first.Eligible || first.ManifestHash == "" {
		t.Fatalf("fixture preflight should be eligible: %+v", first)
	}

	payload := map[string]any{
		"expectedVersion":  first.CandidateVersion,
		"candidateVersion": first.CandidateVersion,
		"manifestHash":     first.ManifestHash,
		"idempotencyKey":   "freeze-after-preflight",
		"actor":            "reviewer-1",
		"role":             "quality_reviewer",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/cache-campaign/freeze", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("freeze returned %d: %s", response.Code, response.Body.String())
	}

	repo.mu.Lock()
	frozenVersion := repo.campaign.Version
	frozenStatus := repo.campaign.Status
	repo.mu.Unlock()
	if frozenStatus != monitoring.StatusFrozen {
		t.Fatalf("fixture did not freeze, got %s", frozenStatus)
	}

	second := getPreflight(t, handler, frozenVersion)
	if second.CandidateVersion != frozenVersion || second.Eligible || second.ManifestHash != "" {
		t.Fatalf("preflight reused version %d after campaign advanced to Frozen version %d: %+v", first.CandidateVersion, frozenVersion, second)
	}
}

func getPreflight(t *testing.T, handler http.Handler, version int64) application.FreezePreflight {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/cache-campaign/freeze/preflight?candidateVersion="+jsonNumber(version), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("preflight returned %d: %s", response.Code, response.Body.String())
	}
	var report application.FreezePreflight
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func jsonNumber(value int64) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func eligibleCampaign(t *testing.T, now time.Time) *monitoring.Campaign {
	t.Helper()
	upper := 10.0
	campaign, err := monitoring.NewCampaign(monitoring.CreateSpec{
		ID:               "cache-campaign",
		FacilityName:     "缓存复现设施",
		RoomCode:         "CR-CACHE",
		CleanlinessClass: "ISO 5",
		PlannedDate:      now.Add(24 * time.Hour),
		Now:              now,
		Points: []monitoring.SamplingPoint{{
			ID: "point-1", Label: "采样点", Metric: "particle", Unit: "count/m3", RequiredReplicates: 1, UpperLimit: &upper,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = campaign.RegisterInstruments([]monitoring.InstrumentEvidence{{
		ID: "instrument-1", InstrumentType: "particle_counter", SerialNumber: "PC-1", CertificateRef: "CAL-1",
		CalibratedAt: now.Add(-time.Hour), ExpiresAt: now.Add(48 * time.Hour), CoveredMetrics: []string{"particle"},
	}}, "engineer-1", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	err = campaign.AddRound(monitoring.MeasurementRound{
		ID: "round-1", Kind: monitoring.RoundRoutine, RecordedBy: "engineer-1",
		Samples: []monitoring.Sample{{ID: "sample-1", PointID: "point-1", Replicate: 1, Value: 5, Unit: "count/m3"}},
	}, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = campaign.SubmitForReview(now.Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if blockers := campaign.FreezeBlockers(); len(blockers) != 0 {
		t.Fatalf("fixture has blockers: %+v", blockers)
	}
	return campaign
}
