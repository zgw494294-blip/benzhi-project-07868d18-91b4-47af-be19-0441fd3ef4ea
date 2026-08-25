package httpapi

import (
	"net/http"

	"cleanroom-monitor-release/internal/domain/monitoring"
)

type mutationResponse struct {
	Campaign         any                          `json:"campaign"`
	Replayed         bool                         `json:"replayed"`
	PlanSummary      *monitoring.PlanSummary      `json:"planSummary,omitempty"`
	Readiness        *monitoring.Readiness        `json:"readiness,omitempty"`
	SamplingProgress *monitoring.SamplingProgress `json:"samplingProgress,omitempty"`
	CheckBatch       *monitoring.InspectionBatch  `json:"checkBatch,omitempty"`
	FindingCount     *int                         `json:"findingCount,omitempty"`
}

func (s *Server) CreateCampaignHandler(w http.ResponseWriter, r *http.Request) {
	var input createCampaignRequest
	if err := decodeStrict(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if len(input.Points) == 0 || len(input.Points) > monitoring.MaxSamplingPoints {
		writeError(w, r, http.StatusBadRequest, "validation_error", "points 数量必须在 1 到 256 之间")
		return
	}
	campaign, replayed, err := s.service.CreateCampaign(r.Context(), input.command())
	if err != nil {
		mapError(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, mutationResponse{Campaign: campaign, Replayed: replayed, PlanSummary: &campaign.PlanSummary})
}

func (s *Server) RegisterInstrumentsHandler(w http.ResponseWriter, r *http.Request) {
	var input instrumentsRequest
	if err := decodeStrict(w, r, &input); err != nil {
		writeError(w, r, 400, "invalid_json", err.Error())
		return
	}
	campaign, replayed, err := s.service.RegisterInstruments(r.Context(), input.command(r.PathValue("campaignId")))
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, 200, mutationResponse{Campaign: campaign, Replayed: replayed, Readiness: &campaign.Readiness})
}

func (s *Server) AddRoundHandler(w http.ResponseWriter, r *http.Request) {
	var input roundRequest
	if err := decodeStrict(w, r, &input); err != nil {
		writeError(w, r, 400, "invalid_json", err.Error())
		return
	}
	campaign, replayed, err := s.service.AddRound(r.Context(), input.command(r.PathValue("campaignId")))
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, 200, mutationResponse{Campaign: campaign, Replayed: replayed, SamplingProgress: &campaign.SamplingProgress})
}

func (s *Server) SubmitReviewHandler(w http.ResponseWriter, r *http.Request) {
	var input submitReviewRequest
	if err := decodeStrict(w, r, &input); err != nil {
		writeError(w, r, 400, "invalid_json", err.Error())
		return
	}
	campaign, replayed, err := s.service.SubmitReview(r.Context(), input.command(r.PathValue("campaignId")))
	if err != nil {
		mapError(w, r, err)
		return
	}
	batch, _, _ := campaign.CurrentAndHistoricalFindings()
	findingCount := 0
	if batch != nil {
		findingCount = batch.FindingCount
	}
	writeJSON(w, 200, mutationResponse{Campaign: campaign, Replayed: replayed, CheckBatch: batch, FindingCount: &findingCount})
}

func (s *Server) DecideFindingHandler(w http.ResponseWriter, r *http.Request) {
	var input decideFindingRequest
	if err := decodeStrict(w, r, &input); err != nil {
		writeError(w, r, 400, "invalid_json", err.Error())
		return
	}
	campaign, replayed, err := s.service.DecideFinding(r.Context(), input.command(r.PathValue("campaignId"), r.PathValue("findingId")))
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, 200, mutationResponse{Campaign: campaign, Replayed: replayed})
}

func (s *Server) FreezeHandler(w http.ResponseWriter, r *http.Request) {
	var input freezeRequest
	if err := decodeStrict(w, r, &input); err != nil {
		writeError(w, r, 400, "invalid_json", err.Error())
		return
	}
	campaign, replayed, err := s.service.Freeze(r.Context(), input.command(r.PathValue("campaignId")))
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, 200, mutationResponse{Campaign: campaign, Replayed: replayed})
}

func (s *Server) IssueCredentialHandler(w http.ResponseWriter, r *http.Request) {
	var input issueCredentialRequest
	if err := decodeStrict(w, r, &input); err != nil {
		writeError(w, r, 400, "invalid_json", err.Error())
		return
	}
	campaign, replayed, err := s.service.Issue(r.Context(), input.command(r.PathValue("campaignId")))
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, 200, mutationResponse{Campaign: campaign, Replayed: replayed})
}
