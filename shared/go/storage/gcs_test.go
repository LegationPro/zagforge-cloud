package storage

import (
	"testing"
)

func TestSnapshotPath(t *testing.T) {
	got := SnapshotPath(
		"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		"f9e8d7c6-b5a4-3210-fedc-ba0987654321",
		"3fa912e1abc456def789012345678901abcdef01",
	)
	want := "a1b2c3d4-e5f6-7890-abcd-ef1234567890/f9e8d7c6-b5a4-3210-fedc-ba0987654321/3fa912e1abc456def789012345678901abcdef01/snapshot.json"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNewClient_emptyBucket_returnsError(t *testing.T) {
	_, err := NewClient(t.Context(), Config{}, nil)
	if err != ErrBucketRequired {
		t.Fatalf("expected ErrBucketRequired, got %v", err)
	}
}
