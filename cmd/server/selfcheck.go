package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type selfcheckClient struct {
	baseURL string
	client  *http.Client
}

func runSelfcheck(parent context.Context, listener net.Listener, server *http.Server) error {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	client := selfcheckClient{baseURL: "http://" + listener.Addr().String(), client: &http.Client{Timeout: 3 * time.Second}}
	if err := client.flow(ctx); err != nil {
		_ = server.Close()
		return fmt.Errorf("selfcheck 失败: %w", err)
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Println("selfcheck passed: Draft -> Ready -> Executing -> ReviewPending -> Frozen -> Certified，凭据有效")
	return nil
}

func (c selfcheckClient) flow(ctx context.Context) error {
	now := time.Now().UTC()
	planned := now.Add(24 * time.Hour).Truncate(time.Second)
	upperParticles := float64(3520)
	lowerVelocity := float64(0.2)
	upperVelocity := float64(0.6)
	lowerPressure := float64(10)
	create := map[string]any{"id": "selfcheck-campaign", "facilityName": "自检设施", "roomCode": "CR-01", "cleanlinessClass": "ISO 5", "plannedDate": planned, "actor": "engineer-selfcheck", "role": "monitoring_engineer", "idempotencyKey": "selfcheck-create", "points": []any{
		map[string]any{"id": "point-particle", "label": "灌装线", "metric": "particle_0_5um", "unit": "count/m3", "requiredReplicates": 2, "upperLimit": upperParticles},
		map[string]any{"id": "point-velocity", "label": "高效过滤器下方", "metric": "air_velocity", "unit": "m/s", "requiredReplicates": 2, "lowerLimit": lowerVelocity, "upperLimit": upperVelocity},
		map[string]any{"id": "point-pressure", "label": "缓冲间压差", "metric": "pressure_diff", "unit": "Pa", "requiredReplicates": 2, "lowerLimit": lowerPressure}}}
	var created struct {
		Campaign struct {
			Version int64 `json:"version"`
		} `json:"campaign"`
	}
	if err := c.post(ctx, "/api/v1/campaigns", create, &created); err != nil {
		return err
	}
	instruments := map[string]any{"expectedVersion": created.Campaign.Version, "idempotencyKey": "selfcheck-instruments", "actor": "engineer-selfcheck", "role": "monitoring_engineer", "instruments": []any{
		map[string]any{"id": "inst-particle", "instrumentType": "particle_counter", "serialNumber": "PC-001", "certificateRef": "CAL-PC-001", "calibratedAt": now.Add(-24 * time.Hour), "expiresAt": planned.Add(365 * 24 * time.Hour), "coveredMetrics": []string{"particle_0_5um"}},
		map[string]any{"id": "inst-air", "instrumentType": "anemometer", "serialNumber": "AV-001", "certificateRef": "CAL-AV-001", "calibratedAt": now.Add(-24 * time.Hour), "expiresAt": planned.Add(365 * 24 * time.Hour), "coveredMetrics": []string{"air_velocity"}},
		map[string]any{"id": "inst-pressure", "instrumentType": "differential_pressure_gauge", "serialNumber": "DP-001", "certificateRef": "CAL-DP-001", "calibratedAt": now.Add(-24 * time.Hour), "expiresAt": planned.Add(365 * 24 * time.Hour), "coveredMetrics": []string{"pressure_diff"}}}}
	var ready struct {
		Campaign struct {
			Version int64  `json:"version"`
			Status  string `json:"status"`
		} `json:"campaign"`
	}
	if err := c.post(ctx, "/api/v1/campaigns/selfcheck-campaign/instruments", instruments, &ready); err != nil {
		return err
	}
	if ready.Campaign.Status != "Ready" {
		return fmt.Errorf("预期 Ready，得到 %s", ready.Campaign.Status)
	}
	samples := []any{sample("s-p-1", "point-particle", 1, 1000, "count/m3"), sample("s-p-2", "point-particle", 2, 1100, "count/m3"), sample("s-v-1", "point-velocity", 1, 0.35, "m/s"), sample("s-v-2", "point-velocity", 2, 0.36, "m/s"), sample("s-d-1", "point-pressure", 1, 15, "Pa"), sample("s-d-2", "point-pressure", 2, 16, "Pa")}
	round := map[string]any{"expectedVersion": ready.Campaign.Version, "idempotencyKey": "selfcheck-round", "actor": "engineer-selfcheck", "role": "monitoring_engineer", "round": map[string]any{"id": "round-1", "kind": "Routine", "recordedBy": "engineer-selfcheck", "samples": samples}}
	var executing struct {
		Campaign struct {
			Version int64 `json:"version"`
		} `json:"campaign"`
	}
	if err := c.post(ctx, "/api/v1/campaigns/selfcheck-campaign/rounds", round, &executing); err != nil {
		return err
	}
	review := map[string]any{"expectedVersion": executing.Campaign.Version, "idempotencyKey": "selfcheck-review", "actor": "engineer-selfcheck", "role": "monitoring_engineer"}
	var pending struct {
		Campaign struct {
			Version  int64  `json:"version"`
			Status   string `json:"status"`
			Findings []any  `json:"findings"`
		} `json:"campaign"`
	}
	if err := c.post(ctx, "/api/v1/campaigns/selfcheck-campaign/submit-review", review, &pending); err != nil {
		return err
	}
	if pending.Campaign.Status != "ReviewPending" || len(pending.Campaign.Findings) != 0 {
		return fmt.Errorf("自动检查结果不符合预期")
	}
	var preflight struct {
		CandidateVersion int64  `json:"candidateVersion"`
		ManifestHash     string `json:"manifestHash"`
		Eligible         bool   `json:"eligible"`
	}
	if err := c.get(ctx, fmt.Sprintf("/api/v1/campaigns/selfcheck-campaign/freeze/preflight?candidateVersion=%d", pending.Campaign.Version), &preflight); err != nil {
		return err
	}
	if !preflight.Eligible || preflight.ManifestHash == "" {
		return fmt.Errorf("冻结预检未通过")
	}
	freeze := map[string]any{"expectedVersion": pending.Campaign.Version, "candidateVersion": preflight.CandidateVersion, "manifestHash": preflight.ManifestHash, "idempotencyKey": "selfcheck-freeze", "actor": "reviewer-selfcheck", "role": "quality_reviewer"}
	var frozen struct {
		Campaign struct {
			Version      int64  `json:"version"`
			ManifestHash string `json:"manifestHash"`
		} `json:"campaign"`
	}
	if err := c.post(ctx, "/api/v1/campaigns/selfcheck-campaign/freeze", freeze, &frozen); err != nil {
		return err
	}
	if frozen.Campaign.ManifestHash == "" {
		return fmt.Errorf("冻结清单哈希为空")
	}
	issue := map[string]any{"expectedVersion": frozen.Campaign.Version, "idempotencyKey": "selfcheck-issue", "actor": "compliance-selfcheck", "role": "facility_compliance", "issuedBy": "compliance-selfcheck"}
	var certified struct {
		Campaign struct {
			Status string `json:"status"`
		} `json:"campaign"`
	}
	if err := c.post(ctx, "/api/v1/campaigns/selfcheck-campaign/credential", issue, &certified); err != nil {
		return err
	}
	if certified.Campaign.Status != "Certified" {
		return fmt.Errorf("预期 Certified，得到 %s", certified.Campaign.Status)
	}
	var verification struct {
		Valid  bool   `json:"valid"`
		Reason string `json:"reason"`
	}
	if err := c.get(ctx, "/api/v1/campaigns/selfcheck-campaign/credential/verify", &verification); err != nil {
		return err
	}
	if !verification.Valid {
		return fmt.Errorf("凭据无效: %s", verification.Reason)
	}
	var timeline struct {
		Events []any `json:"events"`
	}
	if err := c.get(ctx, "/api/v1/campaigns/selfcheck-campaign/timeline", &timeline); err != nil {
		return err
	}
	if len(timeline.Events) != 6 {
		return fmt.Errorf("预期 6 条审计事件，得到 %d", len(timeline.Events))
	}
	return nil
}

func sample(id, point string, replicate int, value float64, unit string) map[string]any {
	return map[string]any{"id": id, "pointId": point, "replicate": replicate, "value": value, "unit": unit, "environment": map[string]float64{"temperatureC": 22, "humidityPct": 45}}
}
func (c selfcheckClient) post(ctx context.Context, path string, payload, response any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request, response)
}
func (c selfcheckClient) get(ctx context.Context, path string, response any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(request, response)
}
func (c selfcheckClient) do(request *http.Request, response any) error {
	res, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s %s 返回 %d: %s", request.Method, request.URL.Path, res.StatusCode, string(body))
	}
	if err = json.Unmarshal(body, response); err != nil {
		return fmt.Errorf("解析响应: %w", err)
	}
	return nil
}
