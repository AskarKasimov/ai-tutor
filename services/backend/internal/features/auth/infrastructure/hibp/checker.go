package hibp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Checker struct {
	client  *http.Client
	url     string
	timeout time.Duration
}

func New(client *http.Client, url string, timeout time.Duration) *Checker {
	return &Checker{client, url, timeout}
}
func Digest(password string) string {
	sum := sha1.Sum([]byte(password))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
func (a *Checker) Compromised(ctx context.Context, password string) (bool, error) {
	// HIBP k-anonymity: only the first five SHA-1 hex characters leave the API.
	digest := Digest(password)
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", a.url+digest[:5], nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Add-Padding", "true")
	req.Header.Set("User-Agent", "ai-tutor-api-v0")
	resp, err := a.client.Do(req)
	unavailable := func() error {
		return fault.New(fault.Unavailable, "PROCESSING_UNAVAILABLE", "Проверка безопасности пароля временно недоступна.")
	}
	if err != nil {
		return false, unavailable()
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, unavailable()
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(body) > 2*1024*1024 {
		return false, unavailable()
	}
	valid := false
	breached := false
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		suffix, n, ok := strings.Cut(line, ":")
		if !ok || len(suffix) != 35 {
			return false, unavailable()
		}
		if _, err = hex.DecodeString(digest[:5] + suffix); err != nil {
			return false, unavailable()
		}
		count, err := strconv.ParseInt(n, 10, 64)
		if err != nil || count < 0 {
			return false, unavailable()
		}
		valid = true
		if strings.EqualFold(suffix, digest[5:]) && count > 0 {
			breached = true
		}
	}
	if !valid {
		return false, unavailable()
	}
	if breached {
		return true, nil
	}
	return false, nil
}
