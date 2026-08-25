package monitoring

import (
	"sort"
	"strings"
	"time"
)

type Campaign struct {
	ID                            string               `json:"id"`
	FacilityName                  string               `json:"facilityName"`
	RoomCode                      string               `json:"roomCode"`
	CleanlinessClass              string               `json:"cleanlinessClass"`
	PlannedDate                   time.Time            `json:"plannedDate"`
	Status                        Status               `json:"status"`
	Version                       int64                `json:"version"`
	CreatedAt                     time.Time            `json:"createdAt"`
	UpdatedAt                     time.Time            `json:"updatedAt"`
	Points                        []SamplingPoint      `json:"points"`
	PlanSummary                   PlanSummary          `json:"planSummary"`
	Instruments                   []InstrumentEvidence `json:"instruments"`
	Readiness                     Readiness            `json:"readiness"`
	Rounds                        []MeasurementRound   `json:"rounds"`
	SamplingProgress              SamplingProgress     `json:"samplingProgress"`
	Findings                      []Finding            `json:"findings"`
	InspectionBatches             []InspectionBatch    `json:"inspectionBatches"`
	CurrentCheckBatchID           string               `json:"currentCheckBatchId,omitempty"`
	ManifestHash                  string               `json:"manifestHash,omitempty"`
	FrozenVersion                 int64                `json:"frozenVersion,omitempty"`
	Credential                    *ReleaseCredential   `json:"credential,omitempty"`
	PersistenceEvidenceMismatch   bool                 `json:"-"`
	PersistenceCredentialMismatch bool                 `json:"-"`
}

const (
	MaxSamplingPoints      = 256
	MaxRequiredReplicates  = 100
	MaxCampaignSampleCount = 10000
)

type CreateSpec struct {
	ID               string
	FacilityName     string
	RoomCode         string
	CleanlinessClass string
	PlannedDate      time.Time
	Points           []SamplingPoint
	Now              time.Time
}

func NewCampaign(spec CreateSpec) (*Campaign, error) {
	if strings.TrimSpace(spec.ID) == "" || strings.TrimSpace(spec.FacilityName) == "" || strings.TrimSpace(spec.RoomCode) == "" {
		return nil, NewRuleError("validation_error", "活动 ID、设施名称和房间编码不能为空")
	}
	if err := validateIdentifier("活动 ID", spec.ID); err != nil {
		return nil, err
	}
	if err := validateText("设施名称", spec.FacilityName, 200); err != nil {
		return nil, err
	}
	if err := validateText("房间编码", spec.RoomCode, 100); err != nil {
		return nil, err
	}
	if err := validateText("洁净等级", spec.CleanlinessClass, 80); err != nil {
		return nil, err
	}
	if strings.TrimSpace(spec.CleanlinessClass) == "" || spec.PlannedDate.IsZero() {
		return nil, NewRuleError("validation_error", "洁净等级和计划日期不能为空")
	}
	if len(spec.Points) == 0 {
		return nil, NewRuleError("validation_error", "至少需要一个采样点")
	}
	if len(spec.Points) > MaxSamplingPoints {
		return nil, NewRuleError("campaign_capacity_exceeded", "单活动采样点数量超过上限")
	}
	pointIDs := map[string]bool{}
	pointLabels := map[string]string{}
	metricUnits := map[string]string{}
	totalSamples := 0
	for i := range spec.Points {
		point := &spec.Points[i]
		point.CampaignID = spec.ID
		point.Label = strings.TrimSpace(point.Label)
		point.Metric = strings.TrimSpace(point.Metric)
		point.Unit = strings.TrimSpace(point.Unit)
		if err := validatePoint(*point); err != nil {
			return nil, err
		}
		if pointIDs[point.ID] {
			return nil, NewRuleError("duplicate_point", "采样点 ID 不得重复")
		}
		if conflict, exists := pointLabels[point.Label]; exists {
			return nil, NewRuleError("duplicate_point_name", "采样点名称冲突："+conflict+" 与 "+point.ID)
		}
		if unit, exists := metricUnits[point.Metric]; exists && unit != point.Unit {
			return nil, NewRuleError("inconsistent_metric_unit", "指标 "+point.Metric+" 在不同点位使用了不一致单位")
		}
		pointIDs[point.ID] = true
		pointLabels[point.Label] = point.ID
		metricUnits[point.Metric] = point.Unit
		totalSamples += point.RequiredReplicates
		if totalSamples > MaxCampaignSampleCount {
			return nil, NewRuleError("campaign_capacity_exceeded", "单活动计划样本总数超过上限")
		}
	}
	sort.Slice(spec.Points, func(i, j int) bool { return spec.Points[i].ID < spec.Points[j].ID })
	now := spec.Now.UTC()
	campaign := &Campaign{ID: spec.ID, FacilityName: strings.TrimSpace(spec.FacilityName), RoomCode: strings.TrimSpace(spec.RoomCode), CleanlinessClass: strings.TrimSpace(spec.CleanlinessClass), PlannedDate: spec.PlannedDate.UTC(), Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now, Points: spec.Points}
	campaign.refreshDerivedViews()
	return campaign, nil
}

func validatePoint(point SamplingPoint) error {
	if point.ID == "" || strings.TrimSpace(point.Label) == "" || strings.TrimSpace(point.Metric) == "" || strings.TrimSpace(point.Unit) == "" {
		return NewRuleError("validation_error", "采样点标识、名称、指标和单位不能为空")
	}
	if err := validateIdentifier("采样点 ID", point.ID); err != nil {
		return err
	}
	if err := validateText("采样点名称", point.Label, 160); err != nil {
		return err
	}
	if err := validateText("采样指标", point.Metric, 80); err != nil {
		return err
	}
	if err := validateText("采样单位", point.Unit, 32); err != nil {
		return err
	}
	if point.RequiredReplicates < 1 || point.RequiredReplicates > MaxRequiredReplicates {
		return NewRuleError("replicate_limit_exceeded", "采样点重复测量数必须在 1 到 100 之间")
	}
	if point.LowerLimit == nil && point.UpperLimit == nil {
		return NewRuleError("threshold_required", "采样点必须至少设置一个阈值")
	}
	if point.LowerLimit != nil && point.UpperLimit != nil && *point.LowerLimit > *point.UpperLimit {
		return NewRuleError("validation_error", "阈值下限不得大于上限")
	}
	if point.LowerLimit != nil {
		if err := validateFinite("阈值下限", *point.LowerLimit); err != nil {
			return err
		}
	}
	if point.UpperLimit != nil {
		if err := validateFinite("阈值上限", *point.UpperLimit); err != nil {
			return err
		}
	}
	return nil
}

func (c *Campaign) refreshDerivedViews() {
	c.PlanSummary = c.buildPlanSummary()
	c.Readiness = c.buildReadiness()
	c.SamplingProgress = c.buildSamplingProgress()
}

func (c *Campaign) buildPlanSummary() PlanSummary {
	byMetric := map[string]*MetricPlanSummary{}
	result := PlanSummary{PointCount: len(c.Points), Metrics: []MetricPlanSummary{}}
	for _, point := range c.Points {
		entry := byMetric[point.Metric]
		if entry == nil {
			entry = &MetricPlanSummary{Metric: point.Metric, Unit: point.Unit}
			byMetric[point.Metric] = entry
		}
		entry.PointCount++
		entry.PlannedSamples += point.RequiredReplicates
		result.PlannedSampleCount += point.RequiredReplicates
	}
	for _, entry := range byMetric {
		result.Metrics = append(result.Metrics, *entry)
	}
	sort.Slice(result.Metrics, func(i, j int) bool { return result.Metrics[i].Metric < result.Metrics[j].Metric })
	return result
}

func (c *Campaign) ensureMutable() error {
	if IsFrozen(c.Status) {
		return NewRuleError("evidence_frozen", "冻结后禁止修改监测证据")
	}
	return nil
}

func (c *Campaign) touch(now time.Time) { c.Version++; c.UpdatedAt = now.UTC() }

func (c *Campaign) pointByID(id string) (SamplingPoint, bool) {
	for _, point := range c.Points {
		if point.ID == id {
			return point, true
		}
	}
	return SamplingPoint{}, false
}
