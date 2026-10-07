package random

import (
	"crypto/rand"
	"math/big"
)

type Chooser struct{}

func (Chooser) Choose(count int) (int, error) {
	if count <= 1 {
		return 0, nil
	}
	index, err := rand.Int(rand.Reader, big.NewInt(int64(count)))
	if err != nil {
		return 0, err
	}
	return int(index.Int64()), nil
}
