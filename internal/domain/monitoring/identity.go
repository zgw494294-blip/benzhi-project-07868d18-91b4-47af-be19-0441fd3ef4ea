package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
)

func StableID(prefix, seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return prefix + "_" + hex.EncodeToString(sum[:8])
}

func ValidateCampaignID(id string) error {
	return validateIdentifier("活动 ID", id)
}
