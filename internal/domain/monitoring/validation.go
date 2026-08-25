package monitoring

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func validateIdentifier(field, value string) error {
	if !identifierPattern.MatchString(value) {
		return NewRuleError("validation_error", field+" 必须是 1 到 128 位的安全标识")
	}
	return nil
}

func validateText(field, value string, maximum int) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return NewRuleError("validation_error", field+" 不能为空")
	}
	if utf8.RuneCountInString(trimmed) > maximum {
		return NewRuleError("validation_error", field+" 长度超出限制")
	}
	return nil
}

func validateFinite(field string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return NewRuleError("validation_error", field+" 必须是有限数值")
	}
	return nil
}

func validateEnvironment(values map[string]float64) error {
	if len(values) > 32 {
		return NewRuleError("validation_error", "环境条件字段不得超过 32 个")
	}
	for name, value := range values {
		if err := validateIdentifier("环境条件名称", name); err != nil {
			return err
		}
		if err := validateFinite("环境条件数值", value); err != nil {
			return err
		}
	}
	return nil
}

func validateSample(sample Sample) error {
	if err := validateIdentifier("样本 ID", sample.ID); err != nil {
		return err
	}
	if err := validateIdentifier("采样点 ID", sample.PointID); err != nil {
		return err
	}
	if sample.Replicate < 1 || sample.Replicate > 1000 {
		return NewRuleError("validation_error", "重复序号必须在 1 到 1000 之间")
	}
	if err := validateFinite("样本值", sample.Value); err != nil {
		return err
	}
	if err := validateText("样本单位", sample.Unit, 32); err != nil {
		return err
	}
	return validateEnvironment(sample.Environment)
}

func validateInstrument(item InstrumentEvidence) error {
	if err := validateIdentifier("仪器证据 ID", item.ID); err != nil {
		return err
	}
	if err := validateText("仪器类型", item.InstrumentType, 80); err != nil {
		return err
	}
	if err := validateText("仪器序列号", item.SerialNumber, 120); err != nil {
		return err
	}
	if err := validateText("校准证书引用", item.CertificateRef, 240); err != nil {
		return err
	}
	if len(item.CoveredMetrics) == 0 || len(item.CoveredMetrics) > 64 {
		return NewRuleError("metric_not_covered", "仪器覆盖指标数量无效")
	}
	seen := map[string]bool{}
	for _, metric := range item.CoveredMetrics {
		if err := validateText("覆盖指标", metric, 80); err != nil {
			return err
		}
		if seen[metric] {
			return NewRuleError("validation_error", "仪器覆盖指标不得重复")
		}
		seen[metric] = true
	}
	return nil
}
