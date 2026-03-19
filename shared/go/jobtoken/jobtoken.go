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
//
// For key rotation, an optional previous key can be set. Sign always uses the
// current key, but Validate tries the current key first and falls back to the
// previous key. This gives a grace period for in-flight jobs during rotation.
type Signer struct {
	key     []byte
	prevKey []byte // optional; nil means no fallback
	ttl     time.Duration
}

// NewSigner creates a Signer with the given secret key and token TTL.
func NewSigner(key []byte, ttl time.Duration) *Signer {
	return &Signer{key: key, ttl: ttl}
}

// WithPreviousKey returns a copy of the Signer that also accepts tokens signed
// with the previous key during validation. Sign still uses the current key.
func (s *Signer) WithPreviousKey(prev []byte) *Signer {
	return &Signer{key: s.key, prevKey: prev, ttl: s.ttl}
}

// Sign generates a signed token for the given job ID.
func (s *Signer) Sign(jobID string) string {
	expiry := time.Now().Add(s.ttl).Unix()
	return sign(s.key, jobID, expiry)
}

// Validate checks the token for the given job ID.
// Returns ErrInvalidToken if the signature doesn't match,
// or ErrTokenExpired if the token is past its expiry.
// If a previous key is configured, it falls back to validating with that key.
func (s *Signer) Validate(jobID, token string) error {
	err := validate(s.key, jobID, token)
	if err == nil {
		return nil
	}

	// If the current key failed and we have a previous key, try that.
	if s.prevKey != nil && errors.Is(err, ErrInvalidToken) {
		return validate(s.prevKey, jobID, token)
	}

	return err
}

func validate(key []byte, jobID, token string) error {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return ErrInvalidToken
	}

	_, expiryStr := parts[0], parts[1]

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return ErrInvalidToken
	}

	expected := sign(key, jobID, expiry)
	if !hmac.Equal([]byte(token), []byte(expected)) {
		return ErrInvalidToken
	}

	if time.Now().Unix() > expiry {
		return ErrTokenExpired
	}

	return nil
}

func sign(key []byte, jobID string, expiry int64) string {
	payload := fmt.Sprintf("%s:%d", jobID, expiry)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s:%d", sig, expiry)
}
