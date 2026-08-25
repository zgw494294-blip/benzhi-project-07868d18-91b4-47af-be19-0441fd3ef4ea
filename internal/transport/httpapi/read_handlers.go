package httpapi

import (
	"net/http"
	"strconv"

	"cleanroom-monitor-release/internal/domain/monitoring"
)

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) GetCampaignHandler(w http.ResponseWriter, r *http.Request) {
	campaign, err := s.service.GetCampaign(r.Context(), r.PathValue("campaignId"))
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, 200, campaign)
}
func (s *Server) FindingsHandler(w http.ResponseWriter, r *http.Request) {
	campaign, err := s.service.GetCampaign(r.Context(), r.PathValue("campaignId"))
	if err != nil {
		mapError(w, r, err)
		return
	}
	batch, pending, history := campaign.CurrentAndHistoricalFindings()
	writeJSON(w, 200, map[string]any{"campaignId": campaign.ID, "version": campaign.Version, "findings": campaign.Findings, "currentBatch": batch, "pending": pending, "history": history, "remediationRequirements": campaign.RemediationGaps()})
}
func (s *Server) TimelineHandler(w http.ResponseWriter, r *http.Request) {
	events, err := s.service.Timeline(r.Context(), r.PathValue("campaignId"))
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaignId": r.PathValue("campaignId"), "events": events})
}
func (s *Server) VerifyCredentialHandler(w http.ResponseWriter, r *http.Request) {
	report, err := s.service.VerifyCredential(r.Context(), r.PathValue("campaignId"))
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, 200, report)
}

func (s *Server) FreezePreflightHandler(w http.ResponseWriter, r *http.Request) {
	if err := monitoring.ValidateCampaignID(r.PathValue("campaignId")); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	query := r.URL.Query()
	for name, values := range query {
		if name != "candidateVersion" || len(values) != 1 {
			writeError(w, r, http.StatusBadRequest, "validation_error", "冻结预检仅接受单个 candidateVersion 查询参数")
			return
		}
	}
	var candidateVersion int64
	if values, exists := query["candidateVersion"]; exists {
		raw := values[0]
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1 {
			writeError(w, r, http.StatusBadRequest, "validation_error", "candidateVersion 必须是大于零的整数")
			return
		}
		candidateVersion = value
	}
	report, err := s.service.FreezePreflight(r.Context(), r.PathValue("campaignId"), candidateVersion)
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
