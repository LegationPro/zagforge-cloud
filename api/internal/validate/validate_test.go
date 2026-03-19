package validate_test

import (
	"strings"
	"testing"

	"github.com/LegationPro/zagforge-mvp-impl/api/internal/validate"
)

type sampleRequest struct {
	ID     string `validate:"required,uuid"`
	Name   string `validate:"required,min=1,max=100"`
	Status string `validate:"required,oneof=active inactive"`
}

func TestStruct_valid(t *testing.T) {
	req := sampleRequest{
		ID:     "550e8400-e29b-41d4-a716-446655440000",
		Name:   "test",
		Status: "active",
	}
	if err := validate.Struct(req); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestStruct_missingRequired(t *testing.T) {
	req := sampleRequest{
		ID:     "550e8400-e29b-41d4-a716-446655440000",
		Status: "active",
	}
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected error for missing Name")
	}
	if !strings.Contains(err.Error(), "Name is required") {
		t.Errorf("expected 'Name is required', got: %v", err)
	}
}

func TestStruct_invalidUUID(t *testing.T) {
	req := sampleRequest{
		ID:     "not-a-uuid",
		Name:   "test",
		Status: "active",
	}
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}
	if !strings.Contains(err.Error(), "ID must be a valid UUID") {
		t.Errorf("expected UUID error, got: %v", err)
	}
}

func TestStruct_invalidOneof(t *testing.T) {
	req := sampleRequest{
		ID:     "550e8400-e29b-41d4-a716-446655440000",
		Name:   "test",
		Status: "unknown",
	}
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if !strings.Contains(err.Error(), "Status must be one of") {
		t.Errorf("expected oneof error, got: %v", err)
	}
}

func TestStruct_multipleErrors(t *testing.T) {
	req := sampleRequest{}
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected errors for all missing fields")
	}
	// Should contain multiple errors joined by ";"
	if !strings.Contains(err.Error(), ";") {
		t.Errorf("expected multiple errors, got: %v", err)
	}
}

type conditionalRequest struct {
	Status  string `validate:"required,oneof=succeeded failed"`
	Message string `validate:"required_if=Status failed"`
}

func TestStruct_requiredIf_satisfied(t *testing.T) {
	req := conditionalRequest{Status: "failed", Message: "something broke"}
	if err := validate.Struct(req); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestStruct_requiredIf_missing(t *testing.T) {
	req := conditionalRequest{Status: "failed"}
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected error for missing Message when Status=failed")
	}
	if !strings.Contains(err.Error(), "Message is required") {
		t.Errorf("expected required_if error, got: %v", err)
	}
}

func TestStruct_requiredIf_notTriggered(t *testing.T) {
	req := conditionalRequest{Status: "succeeded"}
	if err := validate.Struct(req); err != nil {
		t.Fatalf("expected no error when Status=succeeded, got: %v", err)
	}
}
