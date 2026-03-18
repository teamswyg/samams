package shared

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateID produces a random 16-char hex string for aggregate IDs.
func GenerateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
