package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

func TestRepositoryVersionIdempotencyAndTimeline(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, "file:repo-test?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	service := application.NewService(repo, certificate.NewGenerator(), func() time.Time { return now })
	upper := float64(10)
	cmd := application.CreateCampaignCommand{CommandMeta: application.CommandMeta{Actor: "engineer", Role: application.RoleEngineer, IdempotencyKey: "create-1"}, ID: "campaign-1", FacilityName: "设施", RoomCode: "R1", CleanlinessClass: "ISO 5", PlannedDate: now.Add(time.Hour), Points: []monitoring.SamplingPoint{{ID: "p1", Label: "点位", Metric: "m", Unit: "u", RequiredReplicates: 1, UpperLimit: &upper}}}
	created, replayed, err := service.CreateCampaign(ctx, cmd)
	if err != nil || replayed {
		t.Fatalf("create: replayed=%v err=%v", replayed, err)
	}
	again, replayed, err := service.CreateCampaign(ctx, cmd)
	if err != nil || !replayed || again.Version != created.Version {
		t.Fatalf("replay: %#v %v", again, err)
	}
	changed := cmd
	changed.FacilityName = "其他设施"
	if _, _, err = service.CreateCampaign(ctx, changed); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	inst := application.RegisterInstrumentsCommand{CommandMeta: application.CommandMeta{ExpectedVersion: 99, Actor: "engineer", Role: application.RoleEngineer, IdempotencyKey: "inst-1"}, CampaignID: created.ID, Instruments: []monitoring.InstrumentEvidence{{ID: "i1", InstrumentType: "仪器", SerialNumber: "s", CertificateRef: "c", CalibratedAt: now, ExpiresAt: now.Add(24 * time.Hour), CoveredMetrics: []string{"m"}}}}
	if _, _, err = service.RegisterInstruments(ctx, inst); !errors.Is(err, application.ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
	events, err := service.Timeline(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].CampaignVersion != 1 {
		t.Fatalf("events=%#v", events)
	}
}
