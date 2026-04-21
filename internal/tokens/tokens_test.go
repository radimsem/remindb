package tokens

import "testing"

func TestEstimate_Empty(t *testing.T) {
	if got := Estimate(""); got != 0 {
		t.Errorf("Estimate(\"\") = %d, want 0", got)
	}
}

func TestEstimate_Short(t *testing.T) {
	// "hello" = 5 bytes * 0.25 = 1.25 → ceil → 2
	if got := Estimate("hello"); got != 2 {
		t.Errorf("Estimate(\"hello\") = %d, want 2", got)
	}
}
