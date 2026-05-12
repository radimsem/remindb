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
		{"vendor", true},
		{"target", true},
		{"dist", true},
		{"__pycache__", true},
		{"Pods", true},
		{"bower_components", true},
		{"venv", true},
		{"notes", false},
		{"pods", false},
		{"Vendor", false},
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
