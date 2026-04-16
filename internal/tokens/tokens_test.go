package tokens

import "testing"

func TestEstimate_Empty(t *testing.T) {
	if got := Estimate(""); got != 0 {
		t.Errorf("Estimate(\"\") = %d, want 0", got)
	}
}

func TestEstimate_Short(t *testing.T) {
	// "hello" = 5 bytes * 0.75 = 3.75 → ceil → 4
	if got := Estimate("hello"); got != 4 {
		t.Errorf("Estimate(\"hello\") = %d, want 4", got)
	}
}
