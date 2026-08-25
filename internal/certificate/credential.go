package certificate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"cleanroom-monitor-release/internal/domain/monitoring"
)

func digest(c monitoring.ReleaseCredential) string {
	payload := fmt.Sprintf("credential/v1\n%s\n%d\n%s\n%s\n%s", c.CampaignID, c.FrozenVersion, c.ManifestHash, c.IssuedBy, c.IssuedAt.UTC().Format(time.RFC3339Nano))
	sum := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (g *Generator) Issue(c *monitoring.Campaign, issuedBy string, at time.Time) (monitoring.ReleaseCredential, error) {
	if c.Status != monitoring.StatusFrozen {
		return monitoring.ReleaseCredential{}, monitoring.NewRuleError("not_frozen", "只有已冻结活动可以签发凭据")
	}
	if c.ManifestHash == "" || c.FrozenVersion < 1 || issuedBy == "" {
		return monitoring.ReleaseCredential{}, monitoring.NewRuleError("invalid_frozen_manifest", "冻结清单信息不完整")
	}
	credential := monitoring.ReleaseCredential{ID: monitoring.StableID("credential", fmt.Sprintf("%s:%d:%s", c.ID, c.FrozenVersion, c.ManifestHash)), CampaignID: c.ID, FrozenVersion: c.FrozenVersion, ManifestHash: c.ManifestHash, IssuedBy: issuedBy, IssuedAt: at.UTC()}
	credential.CredentialDigest = digest(credential)
	return credential, nil
}
