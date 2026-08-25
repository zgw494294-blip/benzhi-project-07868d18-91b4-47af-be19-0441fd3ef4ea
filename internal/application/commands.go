package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"cleanroom-monitor-release/internal/domain/monitoring"
)

type CommandMeta struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
	Actor           string `json:"actor"`
	Role            string `json:"role"`
}

type CreateCampaignCommand struct {
	CommandMeta
	ID               string                     `json:"id"`
	FacilityName     string                     `json:"facilityName"`
	RoomCode         string                     `json:"roomCode"`
	CleanlinessClass string                     `json:"cleanlinessClass"`
	PlannedDate      time.Time                  `json:"plannedDate"`
	Points           []monitoring.SamplingPoint `json:"points"`
}

type RegisterInstrumentsCommand struct {
	CommandMeta
	CampaignID  string                          `json:"campaignId"`
	Instruments []monitoring.InstrumentEvidence `json:"instruments"`
}
type AddRoundCommand struct {
	CommandMeta
	CampaignID string                      `json:"campaignId"`
	Round      monitoring.MeasurementRound `json:"round"`
}
type SubmitReviewCommand struct {
	CommandMeta
	CampaignID string `json:"campaignId"`
}
type DecideFindingCommand struct {
	CommandMeta
	CampaignID      string                     `json:"campaignId"`
	FindingID       string                     `json:"findingId"`
	Decision        monitoring.FindingDecision `json:"decision"`
	Note            string                     `json:"note"`
	RemediationNote string                     `json:"remediationNote"`
}
type FreezeCommand struct {
	CommandMeta
	CampaignID       string `json:"campaignId"`
	CandidateVersion int64  `json:"candidateVersion"`
	ManifestHash     string `json:"manifestHash"`
}
type IssueCommand struct {
	CommandMeta
	CampaignID string `json:"campaignId"`
	IssuedBy   string `json:"issuedBy"`
}

func validateMeta(meta CommandMeta, expectedRequired bool) error {
	if strings.TrimSpace(meta.IdempotencyKey) == "" {
		return &ValidationError{Message: "idempotencyKey 不能为空"}
	}
	if strings.TrimSpace(meta.Actor) == "" {
		return &ValidationError{Message: "actor 不能为空"}
	}
	if expectedRequired && meta.ExpectedVersion < 1 {
		return &ValidationError{Message: "expectedVersion 必须大于零"}
	}
	return nil
}

func requestHash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
