package jobtoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

// Signer creates and validates HMAC-SHA256 job tokens.
// Token format: hex(HMAC(key, jobID + ":" + expiry)) + ":" + expiry
type Signer struct {
	key []byte
	ttl time.Duration
}

// NewSigner creates a Signer with the given secret key and token TTL.
func NewSigner(key []byte, ttl time.Duration) *Signer {
	return &Signer{key: key, ttl: ttl}
}

// Sign generates a signed token for the given job ID.
func (s *Signer) Sign(jobID string) string {
	expiry := time.Now().Add(s.ttl).Unix()
	return sign(s.key, jobID, expiry)
}

// Validate checks the token for the given job ID.
// Returns ErrInvalidToken if the signature doesn't match,
// or ErrTokenExpired if the token is past its expiry.
func (s *Signer) Validate(jobID, token string) error {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return ErrInvalidToken
	}

	sig, expiryStr := parts[0], parts[1]

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return ErrInvalidToken
	}

	expected := sign(s.key, jobID, expiry)
	if !hmac.Equal([]byte(token), []byte(expected)) {
		return ErrInvalidToken
	}

	if time.Now().Unix() > expiry {
		return ErrTokenExpired
	}

	_ = sig // used via the full token comparison above
	return nil
}

func sign(key []byte, jobID string, expiry int64) string {
	payload := fmt.Sprintf("%s:%d", jobID, expiry)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s:%d", sig, expiry)
}
