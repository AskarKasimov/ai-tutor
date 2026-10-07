package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

type IDGenerator struct{}

func (IDGenerator) New(prefix string) (string, error) { return ID(prefix) }

func Token() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func ID(prefix string) (string, error) { s, err := Token(); return prefix + "_" + s, err }
func Hash(token string) []byte         { h := sha256.Sum256([]byte(token)); return h[:] }
