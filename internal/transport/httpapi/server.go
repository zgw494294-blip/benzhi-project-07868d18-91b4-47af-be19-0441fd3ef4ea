package httpapi

import (
	"net/http"
	"sync/atomic"
	"time"

	"cleanroom-monitor-release/internal/application"
)

type Server struct {
	service  *application.Service
	mux      *http.ServeMux
	sequence atomic.Uint64
}

func New(service *application.Service) *Server {
	s := &Server{service: service, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler {
	handler := withTimeout(15*time.Second, s.mux)
	handler = recoverPanics(handler)
	handler = requestLog(handler)
	handler = securityHeaders(handler)
	return s.withRequestID(handler)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.HealthHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns", s.CreateCampaignHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns/{campaignId}", s.GetCampaignHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{campaignId}/instruments", s.RegisterInstrumentsHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{campaignId}/rounds", s.AddRoundHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{campaignId}/submit-review", s.SubmitReviewHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns/{campaignId}/findings", s.FindingsHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{campaignId}/findings/{findingId}/decision", s.DecideFindingHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{campaignId}/freeze", s.FreezeHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns/{campaignId}/freeze/preflight", s.FreezePreflightHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{campaignId}/credential", s.IssueCredentialHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns/{campaignId}/credential/verify", s.VerifyCredentialHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns/{campaignId}/timeline", s.TimelineHandler)
}
