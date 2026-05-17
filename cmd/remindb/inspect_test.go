package main

import "testing"

func TestAbbrevTokens(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0k"},
		{1234, "1.2k"},
		{359297, "359.3k"},
		{1500000, "1.5M"},
	}

	for _, c := range cases {
		if got := abbrevTokens(c.in); got != c.want {
			t.Errorf("abbrevTokens(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
