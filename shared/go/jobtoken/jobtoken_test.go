package jobtoken

import (
	"testing"
	"time"
)

func TestSignAndValidate(t *testing.T) {
	s := NewSigner([]byte("test-secret"), 5*time.Minute)
	jobID := "550e8400-e29b-41d4-a716-446655440000"

	token := s.Sign(jobID)
	if err := s.Validate(jobID, token); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
}

func TestWrongJobID(t *testing.T) {
	s := NewSigner([]byte("test-secret"), 5*time.Minute)

	token := s.Sign("job-aaa")
	if err := s.Validate("job-bbb", token); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestWrongKey(t *testing.T) {
	s1 := NewSigner([]byte("key-one"), 5*time.Minute)
	s2 := NewSigner([]byte("key-two"), 5*time.Minute)
	jobID := "job-123"

	token := s1.Sign(jobID)
	if err := s2.Validate(jobID, token); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestExpiredToken(t *testing.T) {
	s := NewSigner([]byte("test-secret"), -1*time.Second) // already expired
	jobID := "job-123"

	token := s.Sign(jobID)
	if err := s.Validate(jobID, token); err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestMalformedToken(t *testing.T) {
	s := NewSigner([]byte("test-secret"), 5*time.Minute)

	cases := []string{
		"",
		"no-colon",
		"abc:not-a-number",
		"abc:123:extra",
	}
	for _, token := range cases {
		if err := s.Validate("job-123", token); err != ErrInvalidToken {
			t.Errorf("token %q: expected ErrInvalidToken, got %v", token, err)
		}
	}
}

func TestTamperedSignature(t *testing.T) {
	s := NewSigner([]byte("test-secret"), 5*time.Minute)
	jobID := "job-123"

	token := s.Sign(jobID)
	// Flip a character in the hex signature.
	tampered := "ff" + token[2:]
	if err := s.Validate(jobID, tampered); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for tampered token, got %v", err)
	}
}

// --- Dual-key rotation tests ---

func TestWithPreviousKey_acceptsOldToken(t *testing.T) {
	oldKey := []byte("old-secret")
	newKey := []byte("new-secret")
	jobID := "job-rotation"

	// Token signed with the old key.
	oldSigner := NewSigner(oldKey, 5*time.Minute)
	token := oldSigner.Sign(jobID)

	// New signer with previous key fallback should accept it.
	newSigner := NewSigner(newKey, 5*time.Minute).WithPreviousKey(oldKey)
	if err := newSigner.Validate(jobID, token); err != nil {
		t.Fatalf("expected old token to be accepted during rotation, got %v", err)
	}
}

func TestWithPreviousKey_acceptsNewToken(t *testing.T) {
	oldKey := []byte("old-secret")
	newKey := []byte("new-secret")
	jobID := "job-rotation"

	// Token signed with the new key.
	newSigner := NewSigner(newKey, 5*time.Minute).WithPreviousKey(oldKey)
	token := newSigner.Sign(jobID)

	if err := newSigner.Validate(jobID, token); err != nil {
		t.Fatalf("expected new token to be accepted, got %v", err)
	}
}

func TestWithPreviousKey_rejectsUnknownKey(t *testing.T) {
	oldKey := []byte("old-secret")
	newKey := []byte("new-secret")
	unknownKey := []byte("unknown-secret")
	jobID := "job-rotation"

	// Token signed with an unknown key.
	unknownSigner := NewSigner(unknownKey, 5*time.Minute)
	token := unknownSigner.Sign(jobID)

	// Should reject — neither current nor previous key matches.
	newSigner := NewSigner(newKey, 5*time.Minute).WithPreviousKey(oldKey)
	if err := newSigner.Validate(jobID, token); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for unknown key, got %v", err)
	}
}

func TestWithPreviousKey_expiredOldToken(t *testing.T) {
	oldKey := []byte("old-secret")
	newKey := []byte("new-secret")
	jobID := "job-rotation"

	// Expired token signed with old key.
	oldSigner := NewSigner(oldKey, -1*time.Second)
	token := oldSigner.Sign(jobID)

	newSigner := NewSigner(newKey, 5*time.Minute).WithPreviousKey(oldKey)
	if err := newSigner.Validate(jobID, token); err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired for expired old token, got %v", err)
	}
}

func TestWithPreviousKey_signAlwaysUsesCurrentKey(t *testing.T) {
	oldKey := []byte("old-secret")
	newKey := []byte("new-secret")
	jobID := "job-rotation"

	rotatedSigner := NewSigner(newKey, 5*time.Minute).WithPreviousKey(oldKey)
	token := rotatedSigner.Sign(jobID)

	// Token should validate with new key only (no fallback needed).
	newOnlySigner := NewSigner(newKey, 5*time.Minute)
	if err := newOnlySigner.Validate(jobID, token); err != nil {
		t.Fatalf("expected Sign to use current key, got %v", err)
	}

	// Token should NOT validate with old key only.
	oldOnlySigner := NewSigner(oldKey, 5*time.Minute)
	if err := oldOnlySigner.Validate(jobID, token); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken when validating new token with old key only, got %v", err)
	}
}

func TestWithoutPreviousKey_rejectsOldToken(t *testing.T) {
	oldKey := []byte("old-secret")
	newKey := []byte("new-secret")
	jobID := "job-123"

	oldSigner := NewSigner(oldKey, 5*time.Minute)
	token := oldSigner.Sign(jobID)

	// Without previous key, old tokens are rejected.
	newSigner := NewSigner(newKey, 5*time.Minute)
	if err := newSigner.Validate(jobID, token); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken without previous key fallback, got %v", err)
	}
}
