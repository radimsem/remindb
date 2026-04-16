package testutil

import (
	"context"
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

func OpenTestDB(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	t.Cleanup(func() { _ = st.Close() })
	return st
}
