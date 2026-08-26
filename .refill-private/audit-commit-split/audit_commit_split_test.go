package audit_commit_split_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/storage/sqlite"
	"cleanroom-monitor-release/internal/transport/httpapi"

	_ "modernc.org/sqlite"
)

func TestAuditFailureRollsBackCampaignMutation(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "audit.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	repo, err := sqlite.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	control, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	_, err = control.ExecContext(ctx, `CREATE TRIGGER reject_campaign_created_audit
		BEFORE INSERT ON audit_events
		WHEN NEW.event_type = 'CampaignCreated'
		BEGIN
			SELECT RAISE(ABORT, 'deterministic audit sink failure');
		END`)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	service := application.NewService(repo, certificate.NewGenerator(), func() time.Time { return now })
	handler := httpapi.New(service).Handler()
	body := `{
		"id":"audit-atomicity-campaign",
		"facilityName":"一号设施",
		"roomCode":"CR-1",
		"cleanlinessClass":"ISO 5",
		"plannedDate":"2026-08-26T08:00:00Z",
		"idempotencyKey":"create-audit-atomicity",
		"actor":"engineer-a",
		"role":"monitoring_engineer",
		"points":[{"id":"point-a","label":"中央点","metric":"particles","unit":"count/m3","requiredReplicates":1,"upperLimit":10}]
	}`
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", strings.NewReader(body))
	createRequest.Header.Set("Content-Type", "application/json")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusInternalServerError {
		t.Fatalf("审计接收端拒绝写入时应返回 500，实际 status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}

	retryRequest := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", strings.NewReader(body))
	retryRequest.Header.Set("Content-Type", "application/json")
	retryResponse := httptest.NewRecorder()
	handler.ServeHTTP(retryResponse, retryRequest)
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/audit-atomicity-campaign", nil))
	timelineResponse := httptest.NewRecorder()
	handler.ServeHTTP(timelineResponse, httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/audit-atomicity-campaign/timeline", nil))
	if retryResponse.Code != http.StatusInternalServerError || getResponse.Code != http.StatusNotFound || timelineResponse.Code != http.StatusNotFound {
		t.Fatalf("审计失败必须回滚聚合、幂等结果和时间线：retry status=%d get status=%d timeline status=%d", retryResponse.Code, getResponse.Code, timelineResponse.Code)
	}
}
