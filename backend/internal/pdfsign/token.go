package pdfsign

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// base62Alphabet mirrors the alphabet used by internal/base62 (kept here as a
// constant only so the test suite can assert all characters in a generated
// token come from a known set).
const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// NewToken returns a fresh 32-byte base62-encoded identifier. Used for
// session tokens, signature IDs, and any other externally-visible opaque ID
// in the pdfsign package.
func NewToken() (string, error) {
	return base62.NewID(), nil
}

// Sha256Hex returns the lowercase hex SHA-256 digest of the given bytes.
func Sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
