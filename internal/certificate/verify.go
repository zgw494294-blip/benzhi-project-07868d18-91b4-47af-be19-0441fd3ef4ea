package certificate

import "cleanroom-monitor-release/internal/domain/monitoring"

func (g *Generator) VerifyDetailed(c *monitoring.Campaign) monitoring.CredentialVerificationReport {
	report := monitoring.CredentialVerificationReport{CampaignID: c.ID, FrozenVersion: c.FrozenVersion, StoredHash: c.ManifestHash, Checks: []monitoring.VerificationCheck{}}
	if !monitoring.IsFrozen(c.Status) {
		report.ReasonCode, report.Reason = "not_frozen", "活动尚未冻结"
		return report
	}
	frozen := *c
	frozen.Version = c.FrozenVersion - 1
	recalculated, hashErr := g.Hash(&frozen)
	report.RecalculatedHash = recalculated
	manifestPassed := hashErr == nil && recalculated == c.ManifestHash && !c.PersistenceEvidenceMismatch
	if c.Credential == nil {
		report.Checks = append(report.Checks, monitoring.VerificationCheck{Name: "manifest_integrity", Passed: manifestPassed, ReasonCode: passOr("manifest_hash_mismatch", manifestPassed)})
		report.ReasonCode, report.Reason = "credential_not_issued", "活动已冻结但尚未签发凭据"
		return report
	}
	credential := *c.Credential
	report.CredentialID = credential.ID
	checks := []monitoring.VerificationCheck{
		{Name: "campaign_binding", Passed: credential.CampaignID == c.ID, ReasonCode: passOr("campaign_binding_mismatch", credential.CampaignID == c.ID)},
		{Name: "frozen_version_binding", Passed: credential.FrozenVersion == c.FrozenVersion, ReasonCode: passOr("frozen_version_mismatch", credential.FrozenVersion == c.FrozenVersion)},
		{Name: "manifest_hash_binding", Passed: credential.ManifestHash == c.ManifestHash, ReasonCode: passOr("manifest_hash_binding_mismatch", credential.ManifestHash == c.ManifestHash)},
	}
	digestPassed := digest(credential) == credential.CredentialDigest && !c.PersistenceCredentialMismatch
	checks = append(checks,
		monitoring.VerificationCheck{Name: "manifest_integrity", Passed: manifestPassed, ReasonCode: passOr("manifest_hash_mismatch", manifestPassed)},
		monitoring.VerificationCheck{Name: "credential_digest", Passed: digestPassed, ReasonCode: passOr("credential_digest_mismatch", digestPassed)},
	)
	report.Checks = checks
	report.Valid = true
	for _, check := range checks {
		if !check.Passed {
			report.Valid = false
			if report.ReasonCode == "" {
				report.ReasonCode = check.ReasonCode
			}
		}
	}
	if report.Valid {
		report.ReasonCode, report.Reason = "valid", "凭据有效"
	} else {
		report.Reason = "凭据校验未通过"
	}
	return report
}

func passOr(failure string, passed bool) string {
	if passed {
		return "passed"
	}
	return failure
}
