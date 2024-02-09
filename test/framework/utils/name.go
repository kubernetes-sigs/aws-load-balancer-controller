package utils

import (
	"crypto/rand"
	"encoding/hex"
	"io"
)

// RandomDNS1123Label generates a random DNS1123 compatible label with specified length
func RandomDNS1123Label(length int) string {
	seedLen := (length + 1) / 2
	seedBuf := make([]byte, seedLen)
	io.ReadFull(rand.Reader, seedBuf[:])

	labelBuf := make([]byte, seedLen*2)
	hex.Encode(labelBuf, seedBuf)
	return "a" + string(labelBuf[:length-1])
}
