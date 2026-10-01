package argon2

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type Hasher struct{ passwordSlots chan struct{} }

func New() *Hasher { return &Hasher{passwordSlots: make(chan struct{}, 2)} }

const hashMemory = 64 * 1024
const hashTime = 3
const hashThreads = 2

func (a *Hasher) passwordSlot(ctx context.Context) (func(), error) {
	select {
	case a.passwordSlots <- struct{}{}:
		return func() { <-a.passwordSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (a *Hasher) Hash(ctx context.Context, password string) (string, error) {
	release, err := a.passwordSlot(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, hashTime, hashMemory, hashThreads, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", hashMemory, hashTime, hashThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func (a *Hasher) Verify(ctx context.Context, encoded, password string) (bool, error) {
	release, err := a.passwordSlot(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	// Unknown users still pay exactly the normal password verification cost.
	salt := make([]byte, 16)
	want := make([]byte, 32)
	if encoded != "" {
		fields := strings.Split(encoded, "$")
		if len(fields) != 6 || fields[1] != "argon2id" || fields[2] != "v=19" || fields[3] != fmt.Sprintf("m=%d,t=%d,p=%d", hashMemory, hashTime, hashThreads) {
			return false, fmt.Errorf("unsupported stored password hash")
		}
		salt, err = base64.RawStdEncoding.DecodeString(fields[4])
		if err != nil || len(salt) != 16 {
			return false, fmt.Errorf("invalid password salt")
		}
		want, err = base64.RawStdEncoding.DecodeString(fields[5])
		if err != nil || len(want) != 32 {
			return false, fmt.Errorf("invalid password hash")
		}
	}
	actual := argon2.IDKey([]byte(password), salt, hashTime, hashMemory, hashThreads, 32)
	return subtle.ConstantTimeCompare(actual, want) == 1 && encoded != "", nil
}
