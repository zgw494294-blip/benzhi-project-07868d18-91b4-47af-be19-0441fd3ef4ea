package certificate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"

	"cleanroom-monitor-release/internal/domain/monitoring"
)

type Generator struct{}

func NewGenerator() *Generator { return &Generator{} }

type manifest struct {
	EncodingVersion  string                          `json:"encodingVersion"`
	CampaignID       string                          `json:"campaignId"`
	FacilityName     string                          `json:"facilityName"`
	RoomCode         string                          `json:"roomCode"`
	CleanlinessClass string                          `json:"cleanlinessClass"`
	PlannedDate      string                          `json:"plannedDate"`
	Revision         int64                           `json:"revision"`
	Points           []monitoring.SamplingPoint      `json:"points"`
	Instruments      []monitoring.InstrumentEvidence `json:"instruments"`
	Rounds           []monitoring.MeasurementRound   `json:"rounds"`
	Findings         []monitoring.Finding            `json:"findings"`
}

func canonical(c *monitoring.Campaign) manifest {
	points := append([]monitoring.SamplingPoint(nil), c.Points...)
	instruments := make([]monitoring.InstrumentEvidence, len(c.Instruments))
	for index, item := range c.Instruments {
		instruments[index] = item
		instruments[index].CoveredMetrics = append([]string(nil), item.CoveredMetrics...)
	}
	rounds := make([]monitoring.MeasurementRound, len(c.Rounds))
	for index, round := range c.Rounds {
		rounds[index] = round
		rounds[index].Samples = append([]monitoring.Sample(nil), round.Samples...)
	}
	findings := make([]monitoring.Finding, len(c.Findings))
	for index, finding := range c.Findings {
		findings[index] = finding
		findings[index].EvidenceRoundIDs = append([]string(nil), finding.EvidenceRoundIDs...)
		findings[index].EvidenceSampleIDs = append([]string(nil), finding.EvidenceSampleIDs...)
		findings[index].MissingReplicates = append([]int(nil), finding.MissingReplicates...)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].ID < points[j].ID })
	sort.Slice(instruments, func(i, j int) bool { return instruments[i].ID < instruments[j].ID })
	sort.Slice(rounds, func(i, j int) bool { return rounds[i].RoundNumber < rounds[j].RoundNumber })
	sort.Slice(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	for i := range instruments {
		sort.Strings(instruments[i].CoveredMetrics)
	}
	for i := range rounds {
		sort.Slice(rounds[i].Samples, func(a, b int) bool { return rounds[i].Samples[a].ID < rounds[i].Samples[b].ID })
		for sampleIndex := range rounds[i].Samples {
			env := rounds[i].Samples[sampleIndex].Environment
			if env == nil {
				continue
			}
			normalized := make(map[string]float64, len(env))
			for name, value := range env {
				if value == 0 && math.Signbit(value) {
					value = 0
				}
				normalized[name] = value
			}
			rounds[i].Samples[sampleIndex].Environment = normalized
		}
	}
	return manifest{EncodingVersion: "cleanroom-manifest/v1", CampaignID: c.ID, FacilityName: c.FacilityName, RoomCode: c.RoomCode, CleanlinessClass: c.CleanlinessClass, PlannedDate: c.PlannedDate.UTC().Format("2006-01-02T15:04:05.999999999Z"), Revision: c.Version, Points: points, Instruments: instruments, Rounds: rounds, Findings: findings}
}

func (g *Generator) Hash(c *monitoring.Campaign) (string, error) {
	data, err := json.Marshal(canonical(c))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
