package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

func TestCollectionCRUD(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	item, err := st.PutItem(ctx, "notes", "note-1", json.RawMessage(`{"title":"Hello CORTEX","tags":["init"]}`))
	if err != nil {
		t.Fatalf("put item: %v", err)
	}
	if item.Collection != "notes" || item.ID != "note-1" {
		t.Fatalf("unexpected item metadata: %+v", item)
	}

	got, err := st.GetItem(ctx, "notes", "note-1")
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if string(got.Data) != `{"title":"Hello CORTEX","tags":["init"]}` {
		t.Fatalf("unexpected data: %s", string(got.Data))
	}

	cols, err := st.ListCollections(ctx)
	if err != nil || len(cols) != 1 || cols[0] != "notes" {
		t.Fatalf("unexpected collections: %v (%v)", cols, err)
	}

	items, err := st.ListItems(ctx, "notes")
	if err != nil || len(items) != 1 {
		t.Fatalf("unexpected items count: %d (%v)", len(items), err)
	}

	if err := st.DeleteItem(ctx, "notes", "note-1"); err != nil {
		t.Fatalf("delete item: %v", err)
	}

	_, err = st.GetItem(ctx, "notes", "note-1")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
