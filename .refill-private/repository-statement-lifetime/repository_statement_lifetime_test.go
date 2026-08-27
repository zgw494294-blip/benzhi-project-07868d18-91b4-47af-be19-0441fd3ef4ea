package repository_statement_lifetime_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/domain/monitoring"
	"cleanroom-monitor-release/internal/storage/sqlite"
)

func TestTimelineStatementDoesNotOutliveRepository(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)

	firstRepo, err := sqlite.Open(ctx, "file:statement-owner-first?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	firstService := application.NewService(firstRepo, certificate.NewGenerator(), func() time.Time { return now })
	createCampaign(t, ctx, firstService, "campaign-first", "point-first", now)
	if _, err = firstService.Timeline(ctx, "campaign-first"); err != nil {
		t.Fatalf("prime timeline statement: %v", err)
	}
	if err = firstRepo.Close(); err != nil {
		t.Fatalf("close first repository: %v", err)
	}

	secondRepo, err := sqlite.Open(ctx, "file:statement-owner-second?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondRepo.Close() })
	secondService := application.NewService(secondRepo, certificate.NewGenerator(), func() time.Time { return now })
	createCampaign(t, ctx, secondService, "campaign-second", "point-second", now)

	events, err := secondService.Timeline(ctx, "campaign-second")
	if err != nil {
		t.Fatalf("second timeline used closed repository statement: %v", err)
	}
	if len(events) != 1 || events[0].CampaignID != "campaign-second" {
		t.Fatalf("second repository returned wrong timeline: %#v", events)
	}
}

func createCampaign(t *testing.T, ctx context.Context, service *application.Service, campaignID, pointID string, now time.Time) {
	t.Helper()
	upper := float64(10)
	command := application.CreateCampaignCommand{
		CommandMeta: application.CommandMeta{Actor: "engineer", Role: application.RoleEngineer, IdempotencyKey: fmt.Sprintf("create-%s", campaignID)},
		ID:          campaignID, FacilityName: "设施", RoomCode: "R1", CleanlinessClass: "ISO 5", PlannedDate: now.Add(time.Hour),
		Points: []monitoring.SamplingPoint{{ID: pointID, Label: pointID, Metric: "particles", Unit: "count", RequiredReplicates: 1, UpperLimit: &upper}},
	}
	if _, _, err := service.CreateCampaign(ctx, command); err != nil {
		t.Fatalf("create %s: %v", campaignID, err)
	}
}
