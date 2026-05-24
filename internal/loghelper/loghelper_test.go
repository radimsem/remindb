package loghelper

import (
	"log/slog"
	"testing"
)

func TestOrDiscard(t *testing.T) {
	if got := OrDiscard(nil); got == nil {
		t.Fatal("OrDiscard(nil) returned nil")
	}
	OrDiscard(nil).Info("must not panic")

	l := slog.New(slog.DiscardHandler)
	if got := OrDiscard(l); got != l {
		t.Errorf("OrDiscard(l) = %p, want identity %p", got, l)
	}
}

func TestOrDefault(t *testing.T) {
	if got := OrDefault(nil); got != slog.Default() {
		t.Errorf("OrDefault(nil) = %p, want slog.Default() %p", got, slog.Default())
	}

	l := slog.New(slog.DiscardHandler)
	if got := OrDefault(l); got != l {
		t.Errorf("OrDefault(l) = %p, want identity %p", got, l)
	}
}
