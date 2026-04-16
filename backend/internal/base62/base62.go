package base62

import (
	"crypto/rand"
	"errors"
	"math/big"
)

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var (
	base       = big.NewInt(62)
	zero       = big.NewInt(0)
	charIndex  = buildCharIndex()
	ErrInvalid = errors.New("base62: invalid character")
)

func buildCharIndex() map[byte]int {
	m := make(map[byte]int, len(alphabet))
	for i := 0; i < len(alphabet); i++ {
		m[alphabet[i]] = i
	}
	return m
}

// Encode converts raw bytes to a base62 string. Big-endian, preserving leading zero bytes as '0'.
func Encode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// Count leading zero bytes so we can re-emit them as '0' prefix.
	leading := 0
	for leading < len(b) && b[leading] == 0 {
		leading++
	}
	n := new(big.Int).SetBytes(b)
	buf := make([]byte, 0, len(b)*2)
	mod := new(big.Int)
	for n.Cmp(zero) > 0 {
		n.QuoRem(n, base, mod)
		buf = append(buf, alphabet[mod.Int64()])
	}
	for i := 0; i < leading; i++ {
		buf = append(buf, alphabet[0])
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// Decode reverses Encode. Errors on unknown chars.
func Decode(s string) ([]byte, error) {
	if s == "" {
		return []byte{}, nil
	}
	leading := 0
	for leading < len(s) && s[leading] == alphabet[0] {
		leading++
	}
	n := new(big.Int)
	for i := 0; i < len(s); i++ {
		idx, ok := charIndex[s[i]]
		if !ok {
			return nil, ErrInvalid
		}
		n.Mul(n, base)
		n.Add(n, big.NewInt(int64(idx)))
	}
	raw := n.Bytes()
	if leading == 0 {
		return raw, nil
	}
	out := make([]byte, leading+len(raw))
	copy(out[leading:], raw)
	return out, nil
}

// NewID produces 32 random bytes encoded as base62. ~43 chars, 256 bits of entropy.
func NewID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is catastrophic; nothing sensible to do but panic.
		panic("base62: rand.Read failed: " + err.Error())
	}
	return Encode(b)
}
