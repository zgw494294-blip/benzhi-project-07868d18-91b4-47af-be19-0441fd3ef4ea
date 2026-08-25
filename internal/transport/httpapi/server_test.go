package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/storage/sqlite"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	repo, err := sqlite.Open(context.Background(), "file:http-test?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return New(application.NewService(repo, certificate.NewGenerator(), nil)).Handler()
}

func TestStrictJSONAndStableErrors(t *testing.T) {
	handler := testHandler(t)
	body := []byte(`{"id":"c1","facilityName":"设施","roomCode":"R1","cleanlinessClass":"ISO 5","plannedDate":"2026-08-26T00:00:00Z","actor":"e","role":"monitoring_engineer","idempotencyKey":"k1","points":[{"id":"p1","label":"点位","metric":"m","unit":"u","requiredReplicates":1}],"unknown":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if res.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request id")
	}
	missing := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/not-found", nil)
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, missing)
	if out.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", out.Code, out.Body.String())
	}
}
