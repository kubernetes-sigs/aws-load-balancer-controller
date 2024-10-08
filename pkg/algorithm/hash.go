package algorithm

import (
	"crypto/sha256"
	"encoding/base64"
)

func ComputeSha256(s string) string {
	checkpointHash := sha256.New()
	_, _ = checkpointHash.Write([]byte(s))
	return base64.RawURLEncoding.EncodeToString(checkpointHash.Sum(nil))
}
