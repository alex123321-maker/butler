package actionauth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// InputClaimHash pins a private ingress claim without putting its reusable
// identifier in a model-facing contract or owner approval. Only the owning
// ContextReader supplies it; model tool arguments cannot select a claim.
func InputClaimHash(inputID, claimID string) string {
	if inputID == "" || claimID == "" || len(inputID) > 512 || len(claimID) > 512 || strings.ContainsRune(inputID, 0) || strings.ContainsRune(claimID, 0) {
		return ""
	}
	sum := sha256.Sum256([]byte("butler-guarded-input-claim-v1\x00" + inputID + "\x00" + claimID))
	return hex.EncodeToString(sum[:])
}
