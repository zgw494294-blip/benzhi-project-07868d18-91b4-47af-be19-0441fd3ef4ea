package cross_campaign_id_collision_test

import (
	"context"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/domain/monitoring"
	"cleanroom-monitor-release/internal/storage/sqlite"
)

func TestAggregateScopedPointIDsDoNotCollide(t *testing.T) {
	ctx := context.Background()
	repo, err := sqlite.Open(ctx, "file:cross-campaign-id?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	service := application.NewService(repo, certificate.NewGenerator(), func() time.Time { return now })
	upper := float64(10)
	create := func(campaignID, key string) error {
		_, _, createErr := service.CreateCampaign(ctx, application.CreateCampaignCommand{
			CommandMeta: application.CommandMeta{Actor: "engineer", Role: application.RoleEngineer, IdempotencyKey: key},
			ID:          campaignID, FacilityName: "设施", RoomCode: campaignID, CleanlinessClass: "ISO 5", PlannedDate: now.Add(time.Hour),
			Points: []monitoring.SamplingPoint{{ID: "point-1", Label: "活动内点位", Metric: "particle", Unit: "count", RequiredReplicates: 1, UpperLimit: &upper}},
		})
		return createErr
	}
	if err = create("campaign-a", "create-a"); err != nil {
		t.Fatal(err)
	}
	if err = create("campaign-b", "create-b"); err != nil {
		t.Fatalf("不同聚合应允许复用活动内点位 ID，第二个活动创建失败: %v", err)
	}
}
