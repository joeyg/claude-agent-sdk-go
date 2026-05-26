package subprocess

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

type fakeStore struct {
	mu    sync.Mutex
	calls int
	err   error
	last  shared.SessionKey
}

func (f *fakeStore) Append(_ context.Context, key shared.SessionKey, _ []shared.SessionStoreEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = key
	return f.err
}

func (f *fakeStore) Load(_ context.Context, _ shared.SessionKey) ([]shared.SessionStoreEntry, error) {
	return nil, nil
}

func newTestBatcher(store shared.SessionStore, projectsDir string, msgChan chan shared.Message) *mirrorBatcher {
	return newMirrorBatcher(context.Background(), store, projectsDir, msgChan)
}

func TestMirrorBatcherAppendsWithMappedKey(t *testing.T) {
	store := &fakeStore{}
	projects := filepath.Join("/cfg", "projects")
	b := newTestBatcher(store, projects, make(chan shared.Message, 1))

	b.handle(mirrorFrame{
		filePath: filepath.Join(projects, "-p", "sess123.jsonl"),
		entries:  []json.RawMessage{json.RawMessage(`{"a":1}`)},
	})

	if store.calls != 1 {
		t.Fatalf("Append calls = %d, want 1", store.calls)
	}
	if store.last.ProjectKey != "-p" || store.last.SessionID != "sess123" {
		t.Errorf("mapped key = %+v", store.last)
	}
}

func TestMirrorBatcherDropsPathOutsideProjects(t *testing.T) {
	store := &fakeStore{}
	msgChan := make(chan shared.Message, 1)
	b := newTestBatcher(store, filepath.Join("/cfg", "projects"), msgChan)

	b.handle(mirrorFrame{
		filePath: filepath.Join("/elsewhere", "x.jsonl"),
		entries:  []json.RawMessage{json.RawMessage(`{}`)},
	})

	if store.calls != 0 {
		t.Errorf("Append calls = %d, want 0", store.calls)
	}
	select {
	case m := <-msgChan:
		t.Errorf("unexpected message: %T", m)
	default:
	}
}

func TestMirrorBatcherRetriesThenEmitsMirrorError(t *testing.T) {
	orig := mirrorBackoff
	mirrorBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { mirrorBackoff = orig }()

	store := &fakeStore{err: errors.New("boom")}
	msgChan := make(chan shared.Message, 1)
	projects := filepath.Join("/cfg", "projects")
	b := newTestBatcher(store, projects, msgChan)

	b.handle(mirrorFrame{
		filePath: filepath.Join(projects, "-p", "sess.jsonl"),
		entries:  []json.RawMessage{json.RawMessage(`{}`)},
	})

	if store.calls != len(mirrorBackoff)+1 {
		t.Errorf("Append calls = %d, want %d", store.calls, len(mirrorBackoff)+1)
	}

	select {
	case m := <-msgChan:
		sys, ok := m.(*shared.SystemMessage)
		if !ok || sys.Subtype != "mirror_error" {
			t.Fatalf("got %T (%v), want mirror_error system message", m, m)
		}
	default:
		t.Fatal("expected a mirror_error message")
	}
}
