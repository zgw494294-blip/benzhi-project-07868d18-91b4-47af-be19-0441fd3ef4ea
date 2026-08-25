package monitoring

import "fmt"

func (c *Campaign) ValidateIntegrity() error {
	if c == nil {
		return fmt.Errorf("campaign aggregate is nil")
	}
	if err := validateIdentifier("活动 ID", c.ID); err != nil {
		return err
	}
	if !c.Status.Valid() {
		return fmt.Errorf("invalid campaign status %q", c.Status)
	}
	if c.Version < 1 {
		return fmt.Errorf("invalid campaign version %d", c.Version)
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() || c.UpdatedAt.Before(c.CreatedAt) {
		return fmt.Errorf("invalid campaign timestamps")
	}
	if c.PlannedDate.IsZero() {
		return fmt.Errorf("planned date is missing")
	}
	if len(c.Points) == 0 {
		return fmt.Errorf("sampling plan is empty")
	}
	if err := c.validatePointIntegrity(); err != nil {
		return err
	}
	if err := c.validateInstrumentIntegrity(); err != nil {
		return err
	}
	if err := c.validateRoundIntegrity(); err != nil {
		return err
	}
	if err := c.validateFindingIntegrity(); err != nil {
		return err
	}
	if err := c.validateInspectionBatchIntegrity(); err != nil {
		return err
	}
	return c.validateReleaseIntegrity()
}

func (c *Campaign) validatePointIntegrity() error {
	seen := map[string]bool{}
	labels := map[string]bool{}
	units := map[string]string{}
	total := 0
	for _, point := range c.Points {
		if point.CampaignID != c.ID {
			return fmt.Errorf("sampling point %s has wrong campaign", point.ID)
		}
		if seen[point.ID] {
			return fmt.Errorf("duplicate sampling point %s", point.ID)
		}
		if labels[point.Label] {
			return fmt.Errorf("duplicate sampling point label %s", point.Label)
		}
		if unit, ok := units[point.Metric]; ok && unit != point.Unit {
			return fmt.Errorf("metric %s uses inconsistent units", point.Metric)
		}
		if err := validatePoint(point); err != nil {
			return err
		}
		seen[point.ID] = true
		labels[point.Label] = true
		units[point.Metric] = point.Unit
		total += point.RequiredReplicates
	}
	if len(c.Points) > MaxSamplingPoints || total > MaxCampaignSampleCount {
		return fmt.Errorf("sampling plan exceeds campaign capacity")
	}
	return nil
}

func (c *Campaign) validateInstrumentIntegrity() error {
	seen := map[string]bool{}
	certificates := map[string]bool{}
	naturalKeys := map[string]bool{}
	for _, item := range c.Instruments {
		if item.CampaignID != c.ID {
			return fmt.Errorf("instrument %s has wrong campaign", item.ID)
		}
		if seen[item.ID] {
			return fmt.Errorf("duplicate instrument %s", item.ID)
		}
		if certificates[item.CertificateRef] || naturalKeys[instrumentNaturalKey(item)] {
			return fmt.Errorf("duplicate instrument evidence %s", item.ID)
		}
		if err := validateInstrument(item); err != nil {
			return err
		}
		if item.CalibratedAt.IsZero() || item.ExpiresAt.IsZero() || !item.ExpiresAt.After(item.CalibratedAt) {
			return fmt.Errorf("instrument %s has invalid calibration period", item.ID)
		}
		seen[item.ID] = true
		certificates[item.CertificateRef] = true
		naturalKeys[instrumentNaturalKey(item)] = true
	}
	return nil
}

func (c *Campaign) validateRoundIntegrity() error {
	roundIDs := map[string]bool{}
	sampleIDs := map[string]bool{}
	for index, round := range c.Rounds {
		if round.CampaignID != c.ID {
			return fmt.Errorf("round %s has wrong campaign", round.ID)
		}
		if roundIDs[round.ID] {
			return fmt.Errorf("duplicate round %s", round.ID)
		}
		if round.RoundNumber != index+1 {
			return fmt.Errorf("round %s has non-contiguous number", round.ID)
		}
		if round.Kind != RoundRoutine && round.Kind != RoundRemediation {
			return fmt.Errorf("round %s has invalid kind", round.ID)
		}
		if round.RecordedAt.IsZero() || round.RecordedBy == "" {
			return fmt.Errorf("round %s has incomplete provenance", round.ID)
		}
		if round.Kind == RoundRemediation && (round.SupersedesRoundID == "" || !roundIDs[round.SupersedesRoundID]) {
			return fmt.Errorf("round %s has invalid remediation link", round.ID)
		}
		for _, sample := range round.Samples {
			if sampleIDs[sample.ID] {
				return fmt.Errorf("duplicate sample %s", sample.ID)
			}
			if _, ok := c.pointByID(sample.PointID); !ok {
				return fmt.Errorf("sample %s references unknown point", sample.ID)
			}
			if err := validateSample(sample); err != nil {
				return err
			}
			sampleIDs[sample.ID] = true
		}
		roundIDs[round.ID] = true
	}
	return nil
}

func (c *Campaign) validateFindingIntegrity() error {
	seen := map[string]bool{}
	batches := map[string]bool{}
	for _, batch := range c.InspectionBatches {
		batches[batch.ID] = true
	}
	for _, finding := range c.Findings {
		if seen[finding.ID] {
			return fmt.Errorf("duplicate finding %s", finding.ID)
		}
		if finding.ID == "" || finding.FindingKey == "" || finding.CheckBatchID == "" || !batches[finding.CheckBatchID] || finding.Code == "" || finding.Message == "" || finding.CreatedAt.IsZero() {
			return fmt.Errorf("finding has incomplete evidence")
		}
		if finding.PointID != "" {
			if _, ok := c.pointByID(finding.PointID); !ok {
				return fmt.Errorf("finding %s references unknown point", finding.ID)
			}
		}
		if finding.RoundID != "" && !c.hasRound(finding.RoundID) {
			return fmt.Errorf("finding %s references unknown round", finding.ID)
		}
		switch finding.Decision {
		case DecisionPending:
			if finding.DecidedAt != nil || finding.DecidedBy != "" {
				return fmt.Errorf("pending finding %s contains decision provenance", finding.ID)
			}
		case DecisionAccepted, DecisionNeedsRemediation, DecisionRemediated:
			if finding.DecidedAt == nil || finding.DecidedBy == "" {
				return fmt.Errorf("decided finding %s lacks provenance", finding.ID)
			}
		default:
			return fmt.Errorf("finding %s has invalid decision", finding.ID)
		}
		if finding.Decision == DecisionRemediated && !c.hasRound(finding.RemediationRound) {
			return fmt.Errorf("finding %s lacks remediation evidence", finding.ID)
		}
		seen[finding.ID] = true
	}
	return nil
}

func (c *Campaign) validateInspectionBatchIntegrity() error {
	seen := map[string]bool{}
	for _, batch := range c.InspectionBatches {
		if batch.ID == "" || batch.CampaignID != c.ID || batch.SourceVersion < 1 || batch.CheckedAt.IsZero() || seen[batch.ID] {
			return fmt.Errorf("invalid inspection batch %s", batch.ID)
		}
		for _, roundID := range batch.EffectiveRoundIDs {
			if !c.hasRound(roundID) {
				return fmt.Errorf("inspection batch %s references unknown round", batch.ID)
			}
		}
		seen[batch.ID] = true
	}
	if c.CurrentCheckBatchID != "" && !seen[c.CurrentCheckBatchID] {
		return fmt.Errorf("current inspection batch is missing")
	}
	return nil
}

func (c *Campaign) validateReleaseIntegrity() error {
	if IsFrozen(c.Status) {
		if c.ManifestHash == "" || c.FrozenVersion < 1 || c.FrozenVersion > c.Version {
			return fmt.Errorf("frozen campaign has invalid manifest binding")
		}
	} else if c.ManifestHash != "" || c.FrozenVersion != 0 || c.Credential != nil {
		return fmt.Errorf("mutable campaign contains release evidence")
	}
	if c.Status == StatusCertified {
		if c.Credential == nil {
			return fmt.Errorf("certified campaign has no credential")
		}
		credential := c.Credential
		if credential.ID == "" || credential.CampaignID != c.ID || credential.FrozenVersion != c.FrozenVersion || credential.ManifestHash != c.ManifestHash || credential.IssuedBy == "" || credential.IssuedAt.IsZero() || credential.CredentialDigest == "" {
			return fmt.Errorf("credential binding is invalid")
		}
	} else if c.Credential != nil {
		return fmt.Errorf("non-certified campaign contains credential")
	}
	return nil
}
