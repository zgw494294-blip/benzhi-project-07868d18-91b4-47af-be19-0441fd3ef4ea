package monitoring

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (c *Campaign) RegisterInstruments(items []InstrumentEvidence, actor string, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if err := RequireStatus(c.Status, StatusDraft); err != nil {
		return err
	}
	if strings.TrimSpace(actor) == "" || len(items) == 0 {
		return NewRuleError("validation_error", "操作者和仪器证据不能为空")
	}
	known := map[string]bool{}
	certificateRefs := map[string]string{}
	naturalKeys := map[string]string{}
	for _, existing := range c.Instruments {
		known[existing.ID] = true
		certificateRefs[existing.CertificateRef] = existing.ID
		naturalKeys[instrumentNaturalKey(existing)] = existing.ID
	}
	for i := range items {
		item := &items[i]
		item.CampaignID = c.ID
		item.InstrumentType = strings.TrimSpace(item.InstrumentType)
		item.SerialNumber = strings.TrimSpace(item.SerialNumber)
		item.CertificateRef = strings.TrimSpace(item.CertificateRef)
		for metricIndex := range item.CoveredMetrics {
			item.CoveredMetrics[metricIndex] = strings.TrimSpace(item.CoveredMetrics[metricIndex])
		}
		sort.Strings(item.CoveredMetrics)
		if err := validateInstrument(*item); err != nil {
			return err
		}
		if item.ID == "" || item.InstrumentType == "" || item.SerialNumber == "" || item.CertificateRef == "" {
			return NewRuleError("validation_error", "仪器证据字段不完整")
		}
		if known[item.ID] {
			return NewRuleError("duplicate_instrument", "仪器证据 ID 不得重复")
		}
		if conflict, exists := certificateRefs[item.CertificateRef]; exists {
			return NewRuleError("duplicate_certificate_ref", "校准证书引用冲突："+conflict+" 与 "+item.ID)
		}
		if conflict, exists := naturalKeys[instrumentNaturalKey(*item)]; exists {
			return NewRuleError("duplicate_instrument_evidence", "校准证据自然键冲突："+conflict+" 与 "+item.ID)
		}
		if item.CalibratedAt.IsZero() || item.ExpiresAt.IsZero() || !item.ExpiresAt.After(item.CalibratedAt) {
			return NewRuleError("invalid_calibration", "校准有效期无效")
		}
		if item.ExpiresAt.Before(c.PlannedDate) {
			return NewRuleError("expired_calibration", "仪器校准在计划采样日前失效")
		}
		if item.CalibratedAt.After(c.PlannedDate) {
			return NewRuleError("invalid_calibration", "校准时间不得晚于计划采样日")
		}
		if len(item.CoveredMetrics) == 0 {
			return NewRuleError("metric_not_covered", "仪器必须声明覆盖指标")
		}
		known[item.ID] = true
		certificateRefs[item.CertificateRef] = item.ID
		naturalKeys[instrumentNaturalKey(*item)] = item.ID
	}
	c.Instruments = append(c.Instruments, items...)
	sort.Slice(c.Instruments, func(i, j int) bool { return c.Instruments[i].ID < c.Instruments[j].ID })
	c.Readiness = c.buildReadiness()
	if c.Readiness.Ready {
		c.Status = StatusReady
	}
	c.touch(now)
	c.refreshDerivedViews()
	return nil
}

func (c *Campaign) buildReadiness() Readiness {
	requiredSet := map[string]bool{}
	covered := map[string]bool{}
	for _, point := range c.Points {
		requiredSet[point.Metric] = true
	}
	for _, instrument := range c.Instruments {
		if instrument.ExpiresAt.Before(c.PlannedDate) || instrument.CalibratedAt.After(c.PlannedDate) {
			continue
		}
		for _, metric := range instrument.CoveredMetrics {
			metric = strings.TrimSpace(metric)
			if requiredSet[metric] {
				covered[metric] = true
			}
		}
	}
	result := Readiness{RequiredMetrics: []string{}, CoveredMetrics: []string{}, MissingMetrics: []string{}}
	for metric := range requiredSet {
		result.RequiredMetrics = append(result.RequiredMetrics, metric)
		if covered[metric] {
			result.CoveredMetrics = append(result.CoveredMetrics, metric)
		} else {
			result.MissingMetrics = append(result.MissingMetrics, metric)
		}
	}
	sort.Strings(result.RequiredMetrics)
	sort.Strings(result.CoveredMetrics)
	sort.Strings(result.MissingMetrics)
	result.Ready = len(result.RequiredMetrics) > 0 && len(result.MissingMetrics) == 0
	return result
}

func instrumentNaturalKey(item InstrumentEvidence) string {
	metrics := append([]string(nil), item.CoveredMetrics...)
	sort.Strings(metrics)
	return fmt.Sprintf("%s|%s|%s", strings.TrimSpace(item.SerialNumber), item.CalibratedAt.UTC().Format(time.RFC3339Nano), strings.Join(metrics, ","))
}
