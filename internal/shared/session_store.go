package shared

import (
	"context"
	"encoding/json"
)

// SessionStoreEntry is a single transcript entry. Entries are opaque,
// JSON-safe values: a store must persist them in order and return them from
// Load in the same order. Deep-equal round-tripping is required; byte-equal
// serialization is not (backends that reorder object keys are fine).
type SessionStoreEntry = json.RawMessage

// SessionKey addresses one transcript in a SessionStore.
//
// ProjectKey is a filesystem-safe encoding of the working directory, SessionID
// is the session UUID, and Subpath is set when the entry belongs to a subagent
// transcript or sidecar file rather than the main conversation (e.g.
// "subagents/agent-<id>"). An empty Subpath refers to the main transcript.
type SessionKey struct {
	ProjectKey string
	SessionID  string
	Subpath    string
}

// SessionStore mirrors session JSONL transcripts to an external backend (S3,
// Redis, a database, ...) so a session created on one host can be resumed on
// another. The Claude CLI always writes transcripts to local disk first; the
// SDK then forwards each batch to Append. Load is called once before the
// subprocess spawns when a session is being resumed.
//
// This is the required MVP surface (Append + Load). The optional management
// methods from the TypeScript/Python SDKs (listSessions, delete, listSubkeys,
// ...) are intentionally omitted.
type SessionStore interface {
	// Append persists a batch of transcript entries for key, preserving order.
	Append(ctx context.Context, key SessionKey, entries []SessionStoreEntry) error
	// Load returns the entries previously persisted for key, in order, or
	// (nil, nil) when the session is unknown.
	Load(ctx context.Context, key SessionKey) ([]SessionStoreEntry, error)
}
