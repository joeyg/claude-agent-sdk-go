package sessionstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// DefaultLoadTimeout bounds a single SessionStore.Load call during resume.
const DefaultLoadTimeout = 60 * time.Second

// PrepareResume loads a session's transcript from the store and materializes it
// into a fresh temporary CLAUDE_CONFIG_DIR so the CLI can resume from it on a
// host that never wrote the transcript locally.
//
// It returns the temp config dir to point the subprocess at (via
// CLAUDE_CONFIG_DIR), or "" when there is nothing to resume (unknown/empty
// session) — in which case the caller should start fresh. The temp dir, when
// non-empty, is the caller's responsibility to remove.
//
// srcConfigDir is the real config dir whose credential files are copied into
// the temp dir (best-effort) so the resumed subprocess can authenticate.
func PrepareResume(ctx context.Context, store shared.SessionStore, sessionID, cwd, srcConfigDir string, timeout time.Duration) (string, error) {
	if store == nil || sessionID == "" {
		return "", nil
	}
	if timeout <= 0 {
		timeout = DefaultLoadTimeout
	}

	key := shared.SessionKey{ProjectKey: EncodeProjectKey(cwd), SessionID: sessionID}

	entries, err := loadWithTimeout(ctx, store, key, timeout)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}

	tempDir, err := os.MkdirTemp("", "claude-resume-")
	if err != nil {
		return "", fmt.Errorf("create resume temp dir: %w", err)
	}

	projectDir := filepath.Join(ProjectsDir(tempDir), key.ProjectKey)
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("create resume project dir: %w", err)
	}

	transcriptPath := filepath.Join(projectDir, sessionID+".jsonl")
	if err := writeNDJSON(transcriptPath, entries); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("write resume transcript: %w", err)
	}

	// Best-effort: carry credentials so the resumed subprocess can authenticate
	// when callers rely on the config dir rather than env vars.
	for _, name := range []string{".credentials.json", ".claude.json"} {
		copyFileIfExists(filepath.Join(srcConfigDir, name), filepath.Join(tempDir, name))
	}

	return tempDir, nil
}

// loadWithTimeout runs store.Load with a bounded timeout.
func loadWithTimeout(ctx context.Context, store shared.SessionStore, key shared.SessionKey, timeout time.Duration) ([]shared.SessionStoreEntry, error) {
	loadCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		entries []shared.SessionStoreEntry
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		entries, err := store.Load(loadCtx, key)
		ch <- result{entries: entries, err: err}
	}()

	select {
	case <-loadCtx.Done():
		return nil, fmt.Errorf("SessionStore.Load timed out after %s for session %s: %w", timeout, key.SessionID, loadCtx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("SessionStore.Load failed for session %s: %w", key.SessionID, r.err)
		}
		return r.entries, nil
	}
}

// writeNDJSON writes entries as newline-delimited JSON.
func writeNDJSON(path string, entries []shared.SessionStoreEntry) error {
	//nolint:gosec // G304: path is an SDK-built temp transcript path, not user-tainted.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	for _, e := range entries {
		if _, err := f.Write(e); err != nil {
			return err
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return f.Sync()
}

// copyFileIfExists copies src to dst when src exists; missing src is not an error.
// src/dst are SDK-resolved config-dir paths (not user-tainted).
func copyFileIfExists(src, dst string) {
	data, err := os.ReadFile(src) //nolint:gosec // G304: src is a resolved config-dir path.
	if err != nil {
		return
	}
	_ = os.WriteFile(dst, data, 0o600) //nolint:gosec // G703: dst is an SDK-built temp path.
}
