package logger_test

import (
	"testing"

	"github.com/LegationPro/zagforge/shared/go/logger"
)

func TestNew_dev_returnsLogger(t *testing.T) {
	l, err := logger.New("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNew_production_returnsLogger(t *testing.T) {
	l, err := logger.New("production")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNew_empty_returnsProductionLogger(t *testing.T) {
	l, err := logger.New("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}
