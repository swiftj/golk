package lk

import (
	"crypto/rand"
	"encoding/hex"
)

const (
	idAlphabet  = "abcdefghijklmnopqrstuvwxyz0123456789"
	idLength    = 24
	maxKeyBytes = 255
)

// NewID returns a new 24-character resource ID in the format the API accepts
// for client-supplied ids: a lowercase letter followed by 23 lowercase
// letters or digits. Generate one per source record and store it with the
// record, so a later sync finds the resource with Resources.Get.
func NewID() string {
	b := make([]byte, 0, idLength)
	b = append(b, randomChar(idAlphabet[:26]))
	for len(b) < idLength {
		b = append(b, randomChar(idAlphabet))
	}
	return string(b)
}

// randomChar picks a uniformly random byte of alphabet by rejection sampling.
func randomChar(alphabet string) byte {
	n := byte(len(alphabet))
	limit := 255 - 255%n
	var buf [1]byte
	for {
		_, _ = rand.Read(buf[:]) // crypto/rand.Read never fails (Go ≥ 1.24)
		if buf[0] < limit {
			return alphabet[buf[0]%n]
		}
	}
}

// newUUID returns a random (version 4) UUID in its canonical text form, which
// fits the idempotency key alphabet.
func newUUID() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	var b [36]byte
	hex.Encode(b[0:8], u[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], u[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], u[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], u[8:10])
	b[23] = '-'
	hex.Encode(b[24:], u[10:])
	return string(b[:])
}

// validIdempotencyKey reports whether k is 1–255 characters from [A-Za-z0-9._-].
func validIdempotencyKey(k string) bool {
	if len(k) == 0 || len(k) > maxKeyBytes {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
