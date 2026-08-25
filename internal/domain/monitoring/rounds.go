package monitoring

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (c *Campaign) AddRound(round MeasurementRound, now time.Time) error {
	return c.AddRoundByActor(round, round.RecordedBy, now)
}

func (c *Campaign) AddRoundByActor(round MeasurementRound, actor string, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if err := RequireStatus(c.Status, StatusReady, StatusExecuting, StatusRemediation); err != nil {
		return err
	}
	if round.ID == "" || strings.TrimSpace(round.RecordedBy) == "" || len(round.Samples) == 0 {
		return NewRuleError("validation_error", "轮次标识、记录人和样本不能为空")
	}
	if strings.TrimSpace(actor) == "" || round.RecordedBy != actor {
		return NewRuleError("recorded_by_mismatch", "轮次 recordedBy 必须与当前 actor 一致")
	}
	for _, existing := range c.Rounds {
		if existing.ID == round.ID {
			return NewRuleError("duplicate_round", "测量轮次 ID 不得重复")
		}
	}
	remediationMode := c.Status == StatusRemediation || c.hasOpenRemediation()
	if remediationMode {
		if round.Kind != RoundRemediation || round.SupersedesRoundID == "" {
			return NewRuleError("remediation_link_required", "整改补测必须关联被替代轮次")
		}
		supersededIndex := c.roundIndex(round.SupersedesRoundID)
		if supersededIndex < 0 {
			return NewRuleError("remediation_link_invalid", "关联的原轮次不存在")
		}
		if supersededIndex >= len(c.Rounds) {
			return NewRuleError("remediation_link_invalid", "整改轮次只能关联更早的轮次")
		}
	} else if round.Kind != RoundRoutine {
		return NewRuleError("invalid_round_kind", "首次采样必须为 Routine")
	}
	seen := map[string]bool{}
	seenIDs := map[string]bool{}
	for _, existingRound := range c.Rounds {
		for _, existingSample := range existingRound.Samples {
			seenIDs[existingSample.ID] = true
			if round.Kind == RoundRoutine {
				seen[fmt.Sprintf("%s/%d", existingSample.PointID, existingSample.Replicate)] = true
			}
		}
	}
	for _, sample := range round.Samples {
		if err := validateSample(sample); err != nil {
			return err
		}
		point, ok := c.pointByID(sample.PointID)
		if !ok {
			return NewRuleError("unknown_point", "样本引用未知采样点")
		}
		if sample.ID == "" || sample.Replicate < 1 {
			return NewRuleError("validation_error", "样本 ID 和重复序号无效")
		}
		if sample.Replicate > point.RequiredReplicates {
			return NewRuleError("replicate_out_of_plan", fmt.Sprintf("点位 %s 的重复序号不得超过 %d", point.ID, point.RequiredReplicates))
		}
		if seenIDs[sample.ID] {
			return NewRuleError("duplicate_sample", "样本 ID 不得与既有证据重复")
		}
		key := fmt.Sprintf("%s/%d", sample.PointID, sample.Replicate)
		if seen[key] {
			return NewRuleError("duplicate_sample", "同一点位的重复序号不得重复")
		}
		if sample.Unit != point.Unit {
			return NewRuleError("unit_mismatch", "样本单位与采样方案不匹配")
		}
		seen[key] = true
		seenIDs[sample.ID] = true
	}
	if round.Kind == RoundRemediation {
		if err := c.validateRemediationCoverage(round); err != nil {
			return err
		}
	}
	round.CampaignID = c.ID
	round.RoundNumber = len(c.Rounds) + 1
	round.RecordedAt = now.UTC()
	sort.Slice(round.Samples, func(i, j int) bool {
		if round.Samples[i].PointID == round.Samples[j].PointID {
			return round.Samples[i].Replicate < round.Samples[j].Replicate
		}
		return round.Samples[i].PointID < round.Samples[j].PointID
	})
	c.Rounds = append(c.Rounds, round)
	if round.Kind == RoundRemediation {
		c.closeCoveredRemediation(round)
	}
	if c.hasOpenRemediation() {
		c.Status = StatusRemediation
	} else {
		c.Status = StatusExecuting
	}
	c.touch(now)
	c.refreshDerivedViews()
	return nil
}

func (c *Campaign) hasRound(id string) bool {
	for _, round := range c.Rounds {
		if round.ID == id {
			return true
		}
	}
	return false
}

func (c *Campaign) roundIndex(id string) int {
	for index, round := range c.Rounds {
		if round.ID == id {
			return index
		}
	}
	return -1
}

type effectiveSample struct {
	Sample  Sample
	RoundID string
}

func (c *Campaign) effectiveSamples() map[string][]effectiveSample {
	result := map[string][]effectiveSample{}
	for _, round := range c.Rounds {
		if round.Kind == RoundRemediation {
			covered := map[string]bool{}
			for _, sample := range round.Samples {
				covered[sample.PointID] = true
			}
			for pointID := range covered {
				result[pointID] = nil
			}
		}
		for _, sample := range round.Samples {
			result[sample.PointID] = append(result[sample.PointID], effectiveSample{Sample: sample, RoundID: round.ID})
		}
	}
	return result
}

func (c *Campaign) buildSamplingProgress() SamplingProgress {
	effective := c.effectiveSamples()
	result := SamplingProgress{Points: []PointSamplingProgress{}}
	for _, point := range c.Points {
		seen := map[int]bool{}
		for _, item := range effective[point.ID] {
			if item.Sample.Replicate <= point.RequiredReplicates {
				seen[item.Sample.Replicate] = true
			}
		}
		entry := PointSamplingProgress{PointID: point.ID, PointLabel: point.Label, CollectedSamples: len(seen), RequiredSamples: point.RequiredReplicates, MissingReplicates: []int{}}
		for replicate := 1; replicate <= point.RequiredReplicates; replicate++ {
			if !seen[replicate] {
				entry.MissingReplicates = append(entry.MissingReplicates, replicate)
			}
		}
		entry.Complete = len(entry.MissingReplicates) == 0
		result.Points = append(result.Points, entry)
		result.CollectedSamples += entry.CollectedSamples
		result.RequiredSamples += entry.RequiredSamples
	}
	if result.RequiredSamples > 0 {
		result.CompletionRatio = float64(result.CollectedSamples) / float64(result.RequiredSamples)
	}
	result.Complete = result.RequiredSamples > 0 && result.CollectedSamples == result.RequiredSamples
	return result
}

func (c *Campaign) hasOpenRemediation() bool {
	for _, finding := range c.Findings {
		if finding.Decision == DecisionNeedsRemediation {
			return true
		}
	}
	return false
}

func (c *Campaign) validateRemediationCoverage(round MeasurementRound) error {
	original := c.Rounds[c.roundIndex(round.SupersedesRoundID)]
	originalPoints := map[string]bool{}
	for _, sample := range original.Samples {
		originalPoints[sample.PointID] = true
	}
	eligible := map[string]bool{}
	for _, finding := range c.Findings {
		if finding.Decision == DecisionNeedsRemediation && finding.RoundID == round.SupersedesRoundID {
			eligible[finding.PointID] = true
		}
	}
	provided := map[string]map[int]bool{}
	for _, sample := range round.Samples {
		if !originalPoints[sample.PointID] || !eligible[sample.PointID] {
			return NewRuleError("remediation_scope_invalid", "整改轮次包含未进入本次整改范围的点位："+sample.PointID)
		}
		if provided[sample.PointID] == nil {
			provided[sample.PointID] = map[int]bool{}
		}
		provided[sample.PointID][sample.Replicate] = true
	}
	for pointID, replicates := range provided {
		point, _ := c.pointByID(pointID)
		missing := []int{}
		for replicate := 1; replicate <= point.RequiredReplicates; replicate++ {
			if !replicates[replicate] {
				missing = append(missing, replicate)
			}
		}
		if len(missing) > 0 {
			return NewRuleError("remediation_coverage_incomplete", fmt.Sprintf("整改点位 %s 缺少重复序号 %v", pointID, missing))
		}
	}
	return nil
}

func (c *Campaign) closeCoveredRemediation(round MeasurementRound) {
	covered := map[string]bool{}
	for _, sample := range round.Samples {
		covered[sample.PointID] = true
	}
	for index := range c.Findings {
		finding := &c.Findings[index]
		if finding.Decision == DecisionNeedsRemediation && finding.RoundID == round.SupersedesRoundID && covered[finding.PointID] {
			finding.Decision = DecisionRemediated
			finding.RemediationRound = round.ID
		}
	}
}
