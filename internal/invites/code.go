// Package invites provides the invite-code primitives used across the
// auth, participant, and invite-handler layers.
package invites

import (
	"crypto/rand"
	"math/big"
)

// Alphabet avoids confusable characters (0/O, 1/I, L, etc.) so
// invite codes stay readable when typed or dictated.
const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// CodeLength is the fixed number of characters in an invite code.
// 8 chars over this 31-char alphabet = ~40 bits, plenty of headroom
// for unique codes and resistant to guessing.
const CodeLength = 8

// Generate returns a fresh random invite code. Callers should retry
// on a unique-constraint violation (vanishingly rare) rather than
// trying to predict collisions here.
func Generate() (string, error) {
	buf := make([]byte, CodeLength)
	max := big.NewInt(int64(len(alphabet)))
	for i := range buf {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = alphabet[n.Int64()]
	}
	return string(buf), nil
}
