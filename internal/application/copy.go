package application

import "cleanroom-monitor-release/internal/domain/monitoring"

func copyPoints(source []monitoring.SamplingPoint) []monitoring.SamplingPoint {
	if source == nil {
		return nil
	}
	result := make([]monitoring.SamplingPoint, len(source))
	copy(result, source)
	return result
}

func copyInstruments(source []monitoring.InstrumentEvidence) []monitoring.InstrumentEvidence {
	if source == nil {
		return nil
	}
	result := make([]monitoring.InstrumentEvidence, len(source))
	for i, item := range source {
		result[i] = item
		result[i].CoveredMetrics = append([]string(nil), item.CoveredMetrics...)
	}
	return result
}

func copyRound(source monitoring.MeasurementRound) monitoring.MeasurementRound {
	result := source
	result.Samples = make([]monitoring.Sample, len(source.Samples))
	for i, sample := range source.Samples {
		result.Samples[i] = sample
		if sample.Environment != nil {
			result.Samples[i].Environment = make(map[string]float64, len(sample.Environment))
			for name, value := range sample.Environment {
				result.Samples[i].Environment[name] = value
			}
		}
	}
	return result
}

func copyVerificationReport(source monitoring.CredentialVerificationReport) monitoring.CredentialVerificationReport {
	result := source
	result.Checks = append([]monitoring.VerificationCheck(nil), source.Checks...)
	return result
}
