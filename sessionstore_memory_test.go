package claudecode

import (
	"bytes"
	"context"
	"testing"
)

func TestInMemorySessionStoreRoundTrip(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()
	key := SessionKey{ProjectKey: "-p", SessionID: "sess"}

	in := []SessionStoreEntry{
		SessionStoreEntry(`{"type":"user","n":1}`),
		SessionStoreEntry(`{"type":"assistant","n":2}`),
	}
	if err := store.Append(ctx, key, in[:1]); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(ctx, key, in[1:]); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	for i := range in {
		if !bytes.Equal(got[i], in[i]) {
			t.Errorf("entry %d = %s, want %s", i, got[i], in[i])
		}
	}
}

func TestInMemorySessionStoreUnknownSession(t *testing.T) {
	store := NewInMemorySessionStore()
	got, err := store.Load(context.Background(), SessionKey{ProjectKey: "-p", SessionID: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got %v, want nil for unknown session", got)
	}
}

func TestInMemorySessionStoreSubpathIsolation(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()
	main := SessionKey{ProjectKey: "-p", SessionID: "s"}
	sub := SessionKey{ProjectKey: "-p", SessionID: "s", Subpath: "subagents/agent-1"}

	_ = store.Append(ctx, main, []SessionStoreEntry{SessionStoreEntry(`{"m":1}`)})
	_ = store.Append(ctx, sub, []SessionStoreEntry{SessionStoreEntry(`{"s":1}`)})

	gotMain, _ := store.Load(ctx, main)
	gotSub, _ := store.Load(ctx, sub)
	if len(gotMain) != 1 || len(gotSub) != 1 {
		t.Fatalf("main=%d sub=%d, want 1 and 1", len(gotMain), len(gotSub))
	}
	if bytes.Equal(gotMain[0], gotSub[0]) {
		t.Error("main and subpath entries should be isolated")
	}
}
