package concurrent_round_scratch_test

import (
	"context"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

type blockedRepository struct {
	entered chan string
	release chan struct{}
	now     time.Time
}

func (r *blockedRepository) Run(ctx context.Context, mutation application.Mutation) (*monitoring.Campaign, bool, error) {
	r.entered <- mutation.CampaignID
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	pointID := "point-a"
	if mutation.CampaignID == "campaign-b" {
		pointID = "point-b"
	}
	upper := 100.0
	campaign := &monitoring.Campaign{
		ID:          mutation.CampaignID,
		PlannedDate: r.now.Add(24 * time.Hour),
		Status:      monitoring.StatusReady,
		Version:     mutation.ExpectedVersion,
		CreatedAt:   r.now.Add(-time.Hour),
		UpdatedAt:   r.now,
		Points: []monitoring.SamplingPoint{{
			ID:                 pointID,
			CampaignID:         mutation.CampaignID,
			Label:              pointID,
			Metric:             "particles",
			Unit:               "count/m3",
			RequiredReplicates: 1,
			UpperLimit:         &upper,
		}},
	}
	if err := mutation.Change(campaign); err != nil {
		return nil, false, err
	}
	return campaign, false, nil
}

func (r *blockedRepository) Get(context.Context, string) (*monitoring.Campaign, error) {
	return nil, application.ErrNotFound
}

func (r *blockedRepository) Timeline(context.Context, string) ([]monitoring.AuditEvent, error) {
	return nil, application.ErrNotFound
}

func (r *blockedRepository) Close() error {
	return nil
}

type callResult struct {
	name     string
	campaign *monitoring.Campaign
	err      error
}

func roundCommand(campaignID, pointID, key string, value float64) application.AddRoundCommand {
	return application.AddRoundCommand{
		CommandMeta: application.CommandMeta{
			ExpectedVersion: 2,
			IdempotencyKey:  key,
			Actor:           "engineer",
			Role:            application.RoleEngineer,
		},
		CampaignID: campaignID,
		Round: monitoring.MeasurementRound{
			ID:         "round-" + key,
			Kind:       monitoring.RoundRoutine,
			RecordedBy: "engineer",
			Samples: []monitoring.Sample{{
				ID:        "sample-" + key,
				PointID:   pointID,
				Replicate: 1,
				Value:     value,
				Unit:      "count/m3",
			}},
		},
	}
}

func TestConcurrentRoundStagingKeepsRequestOwnership(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	repo := &blockedRepository{
		entered: make(chan string, 2),
		release: make(chan struct{}),
		now:     now,
	}
	service := application.NewService(repo, nil, func() time.Time { return now })
	results := make(chan callResult, 2)

	invoke := func(name string, command application.AddRoundCommand) {
		campaign, _, err := service.AddRound(context.Background(), command)
		results <- callResult{name: name, campaign: campaign, err: err}
	}
	go invoke("first", roundCommand("campaign-a", "point-a", "key-a", 11))
	if entered := <-repo.entered; entered != "campaign-a" {
		t.Fatalf("第一个进入仓储的活动为 %s", entered)
	}
	go invoke("second", roundCommand("campaign-b", "point-b", "key-b", 22))
	if entered := <-repo.entered; entered != "campaign-b" {
		t.Fatalf("第二个进入仓储的活动为 %s", entered)
	}
	close(repo.release)

	for range 2 {
		result := <-results
		if result.err != nil {
			t.Errorf("%s 合法轮次被另一请求污染: %v", result.name, result.err)
			continue
		}
		if len(result.campaign.Rounds) != 1 {
			t.Errorf("%s 保存轮次数量=%d", result.name, len(result.campaign.Rounds))
		}
	}
	if t.Failed() {
		t.Log("两个独立活动的并发轮次必须保持各自的 Samples 所有权")
	}
}
