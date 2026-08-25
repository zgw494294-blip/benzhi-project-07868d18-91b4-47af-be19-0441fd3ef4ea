package httpapi

import (
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

type commandMetadata struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
	Actor           string `json:"actor"`
	Role            string `json:"role"`
}

func (m commandMetadata) applicationMeta() application.CommandMeta {
	return application.CommandMeta{
		ExpectedVersion: m.ExpectedVersion,
		IdempotencyKey:  m.IdempotencyKey,
		Actor:           m.Actor,
		Role:            m.Role,
	}
}

type createCampaignRequest struct {
	commandMetadata
	ID               string                     `json:"id"`
	FacilityName     string                     `json:"facilityName"`
	RoomCode         string                     `json:"roomCode"`
	CleanlinessClass string                     `json:"cleanlinessClass"`
	PlannedDate      time.Time                  `json:"plannedDate"`
	Points           []monitoring.SamplingPoint `json:"points"`
}

func (v createCampaignRequest) command() application.CreateCampaignCommand {
	return application.CreateCampaignCommand{
		CommandMeta:      v.applicationMeta(),
		ID:               v.ID,
		FacilityName:     v.FacilityName,
		RoomCode:         v.RoomCode,
		CleanlinessClass: v.CleanlinessClass,
		PlannedDate:      v.PlannedDate,
		Points:           v.Points,
	}
}

type instrumentsRequest struct {
	commandMetadata
	Instruments []monitoring.InstrumentEvidence `json:"instruments"`
}

func (v instrumentsRequest) command(campaignID string) application.RegisterInstrumentsCommand {
	return application.RegisterInstrumentsCommand{
		CommandMeta: v.applicationMeta(),
		CampaignID:  campaignID,
		Instruments: v.Instruments,
	}
}

type roundRequest struct {
	commandMetadata
	Round monitoring.MeasurementRound `json:"round"`
}

func (v roundRequest) command(campaignID string) application.AddRoundCommand {
	return application.AddRoundCommand{
		CommandMeta: v.applicationMeta(),
		CampaignID:  campaignID,
		Round:       v.Round,
	}
}

type submitReviewRequest struct{ commandMetadata }

func (v submitReviewRequest) command(campaignID string) application.SubmitReviewCommand {
	return application.SubmitReviewCommand{CommandMeta: v.applicationMeta(), CampaignID: campaignID}
}

type decideFindingRequest struct {
	commandMetadata
	Decision        monitoring.FindingDecision `json:"decision"`
	Note            string                     `json:"note"`
	RemediationNote string                     `json:"remediationNote"`
}

func (v decideFindingRequest) command(campaignID, findingID string) application.DecideFindingCommand {
	return application.DecideFindingCommand{
		CommandMeta:     v.applicationMeta(),
		CampaignID:      campaignID,
		FindingID:       findingID,
		Decision:        v.Decision,
		Note:            v.Note,
		RemediationNote: v.RemediationNote,
	}
}

type freezeRequest struct {
	commandMetadata
	CandidateVersion int64  `json:"candidateVersion"`
	ManifestHash     string `json:"manifestHash"`
}

func (v freezeRequest) command(campaignID string) application.FreezeCommand {
	return application.FreezeCommand{CommandMeta: v.applicationMeta(), CampaignID: campaignID, CandidateVersion: v.CandidateVersion, ManifestHash: v.ManifestHash}
}

type issueCredentialRequest struct {
	commandMetadata
	IssuedBy string `json:"issuedBy"`
}

func (v issueCredentialRequest) command(campaignID string) application.IssueCommand {
	return application.IssueCommand{
		CommandMeta: v.applicationMeta(),
		CampaignID:  campaignID,
		IssuedBy:    v.IssuedBy,
	}
}
