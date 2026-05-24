package store

import (
	"reflect"
	"testing"
)

func TestBindStrings(t *testing.T) {
	tests := []struct {
		name       string
		vals       []string
		wantClause string
		wantArgs   []any
	}{
		{"nil", nil, "", nil},
		{"empty", []string{}, "", nil},
		{"single", []string{"a"}, "?", []any{"a"}},
		{"three", []string{"a", "b", "c"}, "?,?,?", []any{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, args := bindStrings(tt.vals)

			if clause != tt.wantClause {
				t.Errorf("clause = %q, want %q", clause, tt.wantClause)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %#v, want %#v", args, tt.wantArgs)
			}
		})
	}
}
