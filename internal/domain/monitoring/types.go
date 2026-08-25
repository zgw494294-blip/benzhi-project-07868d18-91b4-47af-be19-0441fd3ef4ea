package monitoring

import "time"

type SamplingPoint struct {
	ID                 string   `json:"id"`
	CampaignID         string   `json:"campaignId"`
	Label              string   `json:"label"`
	Metric             string   `json:"metric"`
	Unit               string   `json:"unit"`
	RequiredReplicates int      `json:"requiredReplicates"`
	LowerLimit         *float64 `json:"lowerLimit,omitempty"`
	UpperLimit         *float64 `json:"upperLimit,omitempty"`
}

type MetricPlanSummary struct {
	Metric         string `json:"metric"`
	Unit           string `json:"unit"`
	PointCount     int    `json:"pointCount"`
	PlannedSamples int    `json:"plannedSamples"`
}

type PlanSummary struct {
	PointCount         int                 `json:"pointCount"`
	PlannedSampleCount int                 `json:"plannedSampleCount"`
	Metrics            []MetricPlanSummary `json:"metrics"`
}

type Readiness struct {
	RequiredMetrics []string `json:"requiredMetrics"`
	CoveredMetrics  []string `json:"coveredMetrics"`
	MissingMetrics  []string `json:"missingMetrics"`
	Ready           bool     `json:"ready"`
}

type PointSamplingProgress struct {
	PointID           string `json:"pointId"`
	PointLabel        string `json:"pointLabel"`
	CollectedSamples  int    `json:"collectedSamples"`
	RequiredSamples   int    `json:"requiredSamples"`
	MissingReplicates []int  `json:"missingReplicates"`
	Complete          bool   `json:"complete"`
}

type SamplingProgress struct {
	Points           []PointSamplingProgress `json:"points"`
	CollectedSamples int                     `json:"collectedSamples"`
	RequiredSamples  int                     `json:"requiredSamples"`
	CompletionRatio  float64                 `json:"completionRatio"`
	Complete         bool                    `json:"complete"`
}

type InstrumentEvidence struct {
	ID             string    `json:"id"`
	CampaignID     string    `json:"campaignId"`
	InstrumentType string    `json:"instrumentType"`
	SerialNumber   string    `json:"serialNumber"`
	CertificateRef string    `json:"certificateRef"`
	CalibratedAt   time.Time `json:"calibratedAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
	CoveredMetrics []string  `json:"coveredMetrics"`
}

type Sample struct {
	ID          string             `json:"id"`
	PointID     string             `json:"pointId"`
	Replicate   int                `json:"replicate"`
	Value       float64            `json:"value"`
	Unit        string             `json:"unit"`
	Environment map[string]float64 `json:"environment,omitempty"`
}

type RoundKind string

const (
	RoundRoutine     RoundKind = "Routine"
	RoundRemediation RoundKind = "Remediation"
)

type MeasurementRound struct {
	ID                string    `json:"id"`
	CampaignID        string    `json:"campaignId"`
	RoundNumber       int       `json:"roundNumber"`
	Kind              RoundKind `json:"kind"`
	Samples           []Sample  `json:"samples"`
	RecordedBy        string    `json:"recordedBy"`
	RecordedAt        time.Time `json:"recordedAt"`
	SupersedesRoundID string    `json:"supersedesRoundId,omitempty"`
}

type FindingDecision string

const (
	DecisionPending          FindingDecision = "Pending"
	DecisionAccepted         FindingDecision = "Accepted"
	DecisionNeedsRemediation FindingDecision = "NeedsRemediation"
	DecisionRemediated       FindingDecision = "Remediated"
)

type Finding struct {
	ID                string          `json:"id"`
	Code              string          `json:"code"`
	PointID           string          `json:"pointId,omitempty"`
	RoundID           string          `json:"roundId,omitempty"`
	CheckBatchID      string          `json:"checkBatchId"`
	FindingKey        string          `json:"findingKey"`
	EvidenceRoundIDs  []string        `json:"evidenceRoundIds,omitempty"`
	EvidenceSampleIDs []string        `json:"evidenceSampleIds,omitempty"`
	MissingReplicates []int           `json:"missingReplicates,omitempty"`
	Message           string          `json:"message"`
	Decision          FindingDecision `json:"decision"`
	DecidedBy         string          `json:"decidedBy,omitempty"`
	DecisionNote      string          `json:"decisionNote,omitempty"`
	RemediationNote   string          `json:"remediationNote,omitempty"`
	RemediationRound  string          `json:"remediationRoundId,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
	DecidedAt         *time.Time      `json:"decidedAt,omitempty"`
}

type CheckPointStat struct {
	PointID          string `json:"pointId"`
	CollectedSamples int    `json:"collectedSamples"`
	RequiredSamples  int    `json:"requiredSamples"`
}

type InspectionBatch struct {
	ID                string           `json:"id"`
	CampaignID        string           `json:"campaignId"`
	SourceVersion     int64            `json:"sourceVersion"`
	CheckedAt         time.Time        `json:"checkedAt"`
	EffectiveRoundIDs []string         `json:"effectiveRoundIds"`
	PointStats        []CheckPointStat `json:"pointStats"`
	FindingCount      int              `json:"findingCount"`
}

type FreezeBlocker struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	FindingID         string `json:"findingId,omitempty"`
	PointID           string `json:"pointId,omitempty"`
	MissingReplicates []int  `json:"missingReplicates,omitempty"`
}

type EvidenceCounts struct {
	PlanPoints  int `json:"planPoints"`
	Instruments int `json:"instruments"`
	Rounds      int `json:"rounds"`
	Decisions   int `json:"decisions"`
}

type ReleaseCredential struct {
	ID               string    `json:"id"`
	CampaignID       string    `json:"campaignId"`
	FrozenVersion    int64     `json:"frozenVersion"`
	ManifestHash     string    `json:"manifestHash"`
	IssuedBy         string    `json:"issuedBy"`
	IssuedAt         time.Time `json:"issuedAt"`
	CredentialDigest string    `json:"credentialDigest"`
}

type VerificationCheck struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	ReasonCode string `json:"reasonCode"`
}

type CredentialVerificationReport struct {
	CampaignID       string              `json:"campaignId"`
	CredentialID     string              `json:"credentialId,omitempty"`
	FrozenVersion    int64               `json:"frozenVersion,omitempty"`
	StoredHash       string              `json:"storedHash,omitempty"`
	RecalculatedHash string              `json:"recalculatedHash,omitempty"`
	Checks           []VerificationCheck `json:"checks"`
	Valid            bool                `json:"valid"`
	ReasonCode       string              `json:"reasonCode"`
	Reason           string              `json:"reason"`
}

type AuditEvent struct {
	Sequence        int64     `json:"sequence"`
	CampaignID      string    `json:"campaignId"`
	EventType       string    `json:"eventType"`
	Actor           string    `json:"actor"`
	FromStatus      Status    `json:"fromStatus,omitempty"`
	ToStatus        Status    `json:"toStatus,omitempty"`
	CampaignVersion int64     `json:"campaignVersion"`
	OccurredAt      time.Time `json:"occurredAt"`
	Details         string    `json:"details,omitempty"`
}
