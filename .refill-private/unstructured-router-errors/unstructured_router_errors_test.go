package unstructured_router_errors_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/certificate"
	"cleanroom-monitor-release/internal/storage/sqlite"
	"cleanroom-monitor-release/internal/transport/httpapi"
)

func TestMethodNotAllowedUsesStructuredError(t *testing.T) {
	repo, err := sqlite.Open(context.Background(), "file:router-errors?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	handler := httpapi.New(application.NewService(repo, certificate.NewGenerator(), nil)).Handler()
	request := httptest.NewRequest(http.MethodPut, "/healthz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"requestId"`
		} `json:"error"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("405 响应不是结构化 JSON: %v; body=%q", err, response.Body.String())
	}
	if body.Error.Code != "method_not_allowed" || body.Error.RequestID == "" {
		t.Fatalf("405 响应缺少稳定错误码或 requestId: %#v", body)
	}
}
