package certificate

import (
	"testing"
	"time"

	"cleanroom-monitor-release/internal/domain/monitoring"
)

func TestManifestDeterministicAndCredentialTamperDetection(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	upper := float64(10)
	c := &monitoring.Campaign{ID: "c-1", FacilityName: "设施", RoomCode: "R1", CleanlinessClass: "ISO 5", PlannedDate: now, Status: monitoring.StatusReviewPending, Version: 4, Points: []monitoring.SamplingPoint{{ID: "b", CampaignID: "c-1", Label: "B", Metric: "m", Unit: "u", RequiredReplicates: 1, UpperLimit: &upper}, {ID: "a", CampaignID: "c-1", Label: "A", Metric: "m", Unit: "u", RequiredReplicates: 1, UpperLimit: &upper}}, Instruments: []monitoring.InstrumentEvidence{{ID: "i", CampaignID: "c-1", InstrumentType: "计数器", SerialNumber: "s", CertificateRef: "cal", CalibratedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), CoveredMetrics: []string{"m"}}}}
	g := NewGenerator()
	first, err := g.Hash(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Points[0], c.Points[1] = c.Points[1], c.Points[0]
	second, err := g.Hash(c)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("hash not deterministic: %s %s", first, second)
	}
	if err = c.Freeze(first, now); err != nil {
		t.Fatal(err)
	}
	credential, err := g.Issue(c, "compliance", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Certify(credential, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	report := g.VerifyDetailed(c)
	if !report.Valid {
		t.Fatalf("verification failed: %s", report.Reason)
	}
	c.PersistenceEvidenceMismatch = true
	report = g.VerifyDetailed(c)
	if report.Valid || report.ReasonCode != "manifest_hash_mismatch" {
		t.Fatalf("规范化证据表差异未被识别：%#v", report)
	}
	c.PersistenceEvidenceMismatch = false
	c.PersistenceCredentialMismatch = true
	report = g.VerifyDetailed(c)
	if report.Valid || report.ReasonCode != "credential_digest_mismatch" {
		t.Fatalf("凭据存储差异未被识别：%#v", report)
	}
	c.PersistenceCredentialMismatch = false
	c.Credential.CredentialDigest = "sha256:tampered"
	report = g.VerifyDetailed(c)
	if report.Valid {
		t.Fatal("tampered credential accepted")
	}
}
