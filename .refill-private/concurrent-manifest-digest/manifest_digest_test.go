package concurrent_manifest_digest_test

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

func TestConcurrentManifestHashingIsIsolated(t *testing.T) {
	previous := runtime.GOMAXPROCS(8)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })

	campaigns := []*monitoring.Campaign{
		manifestCampaign("campaign-alpha", 1),
		manifestCampaign("campaign-bravo", 1001),
	}
	expected := make([]string, len(campaigns))
	for i, campaign := range campaigns {
		value, err := certificate.NewGenerator().Hash(campaign)
		if err != nil {
			t.Fatalf("计算基准清单哈希失败: %v", err)
		}
		expected[i] = value
	}

	shared := certificate.NewGenerator()
	start := make(chan struct{})
	var workers sync.WaitGroup
	var corrupted atomic.Bool
	for worker := 0; worker < 24; worker++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			for iteration := 0; iteration < 30; iteration++ {
				selected := (index + iteration) % len(campaigns)
				actual, err := shared.Hash(campaigns[selected])
				if err != nil || actual != expected[selected] {
					corrupted.Store(true)
				}
			}
		}(worker)
	}
	close(start)
	workers.Wait()
	if corrupted.Load() {
		t.Fatal("共享 Generator 在并发请求间污染了清单哈希")
	}
}

func manifestCampaign(id string, offset int) *monitoring.Campaign {
	points := make([]monitoring.SamplingPoint, 192)
	samples := make([]monitoring.Sample, 192)
	for i := range points {
		pointID := fmt.Sprintf("point-%04d", offset+i)
		points[i] = monitoring.SamplingPoint{ID: pointID, CampaignID: id, Label: pointID, Metric: "particle", Unit: "count/m3", RequiredReplicates: 1}
		samples[i] = monitoring.Sample{ID: fmt.Sprintf("sample-%04d", offset+i), PointID: pointID, Replicate: 1, Value: float64(offset + i), Unit: "count/m3", Environment: map[string]float64{"humidity": 45, "pressure": 101.3, "temperature": 22}}
	}
	now := time.Date(2026, time.August, 25, 8, 0, 0, 0, time.UTC)
	return &monitoring.Campaign{
		ID: id, FacilityName: "一号设施", RoomCode: "CR-01", CleanlinessClass: "ISO 7",
		PlannedDate: now, Status: monitoring.StatusReviewPending, Version: 4, CreatedAt: now, UpdatedAt: now,
		Points: points,
		Rounds: []monitoring.MeasurementRound{{ID: "round-1", CampaignID: id, RoundNumber: 1, Kind: monitoring.RoundRoutine, Samples: samples, RecordedBy: "engineer", RecordedAt: now}},
	}
}
