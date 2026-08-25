package monitoring

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (c *Campaign) SubmitForReview(now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if gaps := c.RemediationGaps(); len(gaps) > 0 {
		parts := make([]string, 0, len(gaps))
		for _, gap := range gaps {
			parts = append(parts, fmt.Sprintf("%s:%s:%v", gap.FindingID, gap.PointID, gap.MissingReplicates))
		}
		return NewRuleError("remediation_evidence_incomplete", "整改证据尚未完整关联："+strings.Join(parts, ";"))
	}
	if err := RequireStatus(c.Status, StatusExecuting); err != nil {
		return err
	}
	effective := c.effectiveSamples()
	roundSet := map[string]bool{}
	for _, samples := range effective {
		for _, item := range samples {
			roundSet[item.RoundID] = true
		}
	}
	roundIDs := make([]string, 0, len(roundSet))
	for roundID := range roundSet {
		roundIDs = append(roundIDs, roundID)
	}
	sort.Strings(roundIDs)
	batchID := StableID("check", fmt.Sprintf("%s:%d:%s", c.ID, c.Version, strings.Join(roundIDs, ",")))
	batch := InspectionBatch{ID: batchID, CampaignID: c.ID, SourceVersion: c.Version, CheckedAt: now.UTC(), EffectiveRoundIDs: roundIDs, PointStats: []CheckPointStat{}}
	findings := make([]Finding, 0)
	for _, point := range c.Points {
		items := effective[point.ID]
		byReplicate := map[int]effectiveSample{}
		for _, item := range items {
			byReplicate[item.Sample.Replicate] = item
		}
		missing := []int{}
		for replicate := 1; replicate <= point.RequiredReplicates; replicate++ {
			if _, ok := byReplicate[replicate]; !ok {
				missing = append(missing, replicate)
			}
		}
		batch.PointStats = append(batch.PointStats, CheckPointStat{PointID: point.ID, CollectedSamples: len(byReplicate), RequiredSamples: point.RequiredReplicates})
		if len(missing) > 0 {
			roundID := latestPointRound(items)
			findings = append(findings, newFinding(batchID, roundID, point.ID, "missing_replicates", nil, missing, fmt.Sprintf("点位 %s 缺少重复序号 %v", point.Label, missing), now))
		}
		for replicate := 1; replicate <= point.RequiredReplicates; replicate++ {
			item, ok := byReplicate[replicate]
			if !ok {
				continue
			}
			if point.LowerLimit != nil && item.Sample.Value < *point.LowerLimit {
				findings = append(findings, newFinding(batchID, item.RoundID, point.ID, "below_limit", []string{item.Sample.ID}, nil, fmt.Sprintf("样本 %s 低于下限", item.Sample.ID), now))
			}
			if point.UpperLimit != nil && item.Sample.Value > *point.UpperLimit {
				findings = append(findings, newFinding(batchID, item.RoundID, point.ID, "above_limit", []string{item.Sample.ID}, nil, fmt.Sprintf("样本 %s 超过上限", item.Sample.ID), now))
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].FindingKey < findings[j].FindingKey })
	deduped := make([]Finding, 0, len(findings))
	seen := map[string]bool{}
	for _, finding := range findings {
		if !seen[finding.FindingKey] {
			seen[finding.FindingKey] = true
			deduped = append(deduped, finding)
		}
	}
	batch.FindingCount = len(deduped)
	c.InspectionBatches = append(c.InspectionBatches, batch)
	c.CurrentCheckBatchID = batch.ID
	c.Findings = append(c.Findings, deduped...)
	c.Status = StatusReviewPending
	c.touch(now)
	c.refreshDerivedViews()
	return nil
}

func latestPointRound(items []effectiveSample) string {
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1].RoundID
}

func newFinding(batchID, roundID, pointID, code string, sampleIDs []string, missing []int, message string, now time.Time) Finding {
	sort.Strings(sampleIDs)
	evidence := append([]string(nil), sampleIDs...)
	if len(missing) > 0 {
		evidence = append(evidence, fmt.Sprint(missing))
	}
	key := StableID("finding-key", fmt.Sprintf("%s:%s:%s", pointID, code, strings.Join(evidence, ",")))
	roundIDs := []string{}
	if roundID != "" {
		roundIDs = append(roundIDs, roundID)
	}
	return Finding{ID: StableID("finding", batchID+":"+key), CheckBatchID: batchID, FindingKey: key, Code: code, PointID: pointID, RoundID: roundID, EvidenceRoundIDs: roundIDs, EvidenceSampleIDs: sampleIDs, MissingReplicates: missing, Message: message, Decision: DecisionPending, CreatedAt: now.UTC()}
}

func (c *Campaign) DecideFinding(findingID string, decision FindingDecision, actor, note, remediationNote string, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if err := RequireStatus(c.Status, StatusReviewPending); err != nil {
		return err
	}
	if actor == "" || (decision != DecisionAccepted && decision != DecisionNeedsRemediation) {
		return NewRuleError("validation_error", "裁决人或裁决值无效")
	}
	for i := range c.Findings {
		finding := &c.Findings[i]
		if finding.ID != findingID {
			continue
		}
		if finding.CheckBatchID != c.CurrentCheckBatchID {
			return NewRuleError("finding_not_current", "只能裁决当前检查批次的发现项")
		}
		if finding.Decision != DecisionPending {
			return NewRuleError("already_decided", "发现项已完成裁决")
		}
		if decision == DecisionNeedsRemediation && strings.TrimSpace(remediationNote) == "" {
			return NewRuleError("remediation_note_required", "不合格裁决必须填写整改说明")
		}
		at := now.UTC()
		finding.Decision, finding.DecidedBy, finding.DecisionNote, finding.RemediationNote, finding.DecidedAt = decision, actor, strings.TrimSpace(note), strings.TrimSpace(remediationNote), &at
		pending, needsRemediation := false, false
		for _, item := range c.Findings {
			if item.CheckBatchID != c.CurrentCheckBatchID {
				continue
			}
			pending = pending || item.Decision == DecisionPending
			needsRemediation = needsRemediation || item.Decision == DecisionNeedsRemediation
		}
		if !pending && needsRemediation {
			c.Status = StatusRemediation
		}
		c.touch(now)
		return nil
	}
	return NewRuleError("finding_not_found", "发现项不存在")
}

func (c *Campaign) RemediationGaps() []FreezeBlocker {
	result := []FreezeBlocker{}
	for _, finding := range c.Findings {
		if finding.Decision != DecisionNeedsRemediation {
			continue
		}
		point, ok := c.pointByID(finding.PointID)
		missing := []int{}
		if ok {
			for replicate := 1; replicate <= point.RequiredReplicates; replicate++ {
				missing = append(missing, replicate)
			}
		}
		result = append(result, FreezeBlocker{Code: "remediation_evidence_missing", Message: "整改发现尚未由完整补测证据闭环", FindingID: finding.ID, PointID: finding.PointID, MissingReplicates: missing})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].FindingID < result[j].FindingID })
	return result
}

func (c *Campaign) FreezeBlockers() []FreezeBlocker {
	blockers := []FreezeBlocker{}
	if c.PersistenceEvidenceMismatch {
		blockers = append(blockers, FreezeBlocker{Code: "persistence_evidence_mismatch", Message: "聚合快照与规范化证据表不一致"})
	}
	if c.Status != StatusReviewPending {
		blockers = append(blockers, FreezeBlocker{Code: "invalid_freeze_status", Message: "活动必须处于 ReviewPending 才能冻结"})
	}
	for _, finding := range c.Findings {
		if finding.CheckBatchID == c.CurrentCheckBatchID && finding.Decision == DecisionPending {
			blockers = append(blockers, FreezeBlocker{Code: "pending_finding", Message: "当前发现项尚未裁决", FindingID: finding.ID, PointID: finding.PointID})
		}
		if finding.Decision == DecisionNeedsRemediation {
			blockers = append(blockers, FreezeBlocker{Code: "unclosed_remediation", Message: "整改发现尚未闭环", FindingID: finding.ID, PointID: finding.PointID})
		}
	}
	if !c.buildReadiness().Ready {
		blockers = append(blockers, FreezeBlocker{Code: "calibration_coverage_incomplete", Message: "有效校准证据未覆盖全部计划指标"})
	}
	if err := c.validatePointIntegrity(); err != nil {
		blockers = append(blockers, FreezeBlocker{Code: "sampling_plan_reference_invalid", Message: "采样方案证据不完整"})
	}
	if err := c.validateInstrumentIntegrity(); err != nil {
		blockers = append(blockers, FreezeBlocker{Code: "calibration_evidence_invalid", Message: "校准证据引用不完整"})
	}
	if err := c.validateRoundIntegrity(); err != nil {
		blockers = append(blockers, FreezeBlocker{Code: "evidence_reference_invalid", Message: "测量证据引用不完整"})
	}
	if err := c.validateFindingIntegrity(); err != nil {
		blockers = append(blockers, FreezeBlocker{Code: "finding_reference_invalid", Message: "发现与裁决证据引用不完整"})
	}
	if err := c.validateInspectionBatchIntegrity(); err != nil {
		blockers = append(blockers, FreezeBlocker{Code: "check_batch_reference_invalid", Message: "检查批次证据引用不完整"})
	}
	sort.Slice(blockers, func(i, j int) bool {
		if blockers[i].Code == blockers[j].Code {
			return blockers[i].FindingID < blockers[j].FindingID
		}
		return blockers[i].Code < blockers[j].Code
	})
	return blockers
}

func (c *Campaign) CanFreeze() error {
	if blockers := c.FreezeBlockers(); len(blockers) > 0 {
		return NewRuleError("freeze_blocked", blockers[0].Message)
	}
	return nil
}

func (c *Campaign) EvidenceCounts() EvidenceCounts {
	decisions := 0
	for _, finding := range c.Findings {
		if finding.Decision != DecisionPending {
			decisions++
		}
	}
	return EvidenceCounts{PlanPoints: len(c.Points), Instruments: len(c.Instruments), Rounds: len(c.Rounds), Decisions: decisions}
}

func (c *Campaign) Freeze(hash string, now time.Time) error {
	if err := c.CanFreeze(); err != nil {
		return err
	}
	if hash == "" {
		return NewRuleError("validation_error", "冻结清单哈希不能为空")
	}
	c.ManifestHash = hash
	c.Status = StatusFrozen
	c.touch(now)
	c.FrozenVersion = c.Version
	return nil
}

func (c *Campaign) Certify(credential ReleaseCredential, now time.Time) error {
	if err := RequireStatus(c.Status, StatusFrozen); err != nil {
		return err
	}
	if credential.CampaignID != c.ID || credential.FrozenVersion != c.FrozenVersion || credential.ManifestHash != c.ManifestHash {
		return NewRuleError("credential_mismatch", "凭据与冻结修订不匹配")
	}
	if c.Credential != nil {
		return NewRuleError("credential_exists", "活动已签发凭据")
	}
	c.Credential = &credential
	c.Status = StatusCertified
	c.touch(now)
	return nil
}

func (c *Campaign) CurrentAndHistoricalFindings() (*InspectionBatch, []Finding, []Finding) {
	var batch *InspectionBatch
	for index := range c.InspectionBatches {
		if c.InspectionBatches[index].ID == c.CurrentCheckBatchID {
			copy := c.InspectionBatches[index]
			batch = &copy
			break
		}
	}
	current, history := []Finding{}, []Finding{}
	for _, finding := range c.Findings {
		if finding.CheckBatchID == c.CurrentCheckBatchID && finding.Decision == DecisionPending {
			current = append(current, finding)
		} else {
			history = append(history, finding)
		}
	}
	return batch, current, history
}
