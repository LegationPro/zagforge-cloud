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
