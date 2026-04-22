package fileext

import "testing"

func TestShouldSkipDir(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{".git", true},
		{".obsidian", true},
		{".github", true},
		{".hidden", true},
		{"node_modules", true},
		{"notes", false},
		{"sub-dir", false},
		{"", false},
		{".", false},
		{"..", false},
	}

	for _, c := range cases {
		if got := ShouldSkipDir(c.name); got != c.want {
			t.Errorf("ShouldSkipDir(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
