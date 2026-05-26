package subprocess

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/sessionstore"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

const (
	// mirrorQueueSize bounds the in-flight transcript-mirror frames.
	mirrorQueueSize = 64
	// mirrorSendTimeout bounds a single SessionStore.Append call.
	mirrorSendTimeout = 30 * time.Second
)

// mirrorBackoff is the per-attempt backoff between Append retries. Its length
// plus one is the total number of attempts (matches the TS SDK: 3 attempts).
var mirrorBackoff = []time.Duration{200 * time.Millisecond, 500 * time.Millisecond}

// mirrorFrame is a single transcript_mirror batch awaiting persistence.
type mirrorFrame struct {
	filePath string
	entries  []json.RawMessage
}

// mirrorBatcher forwards transcript_mirror frames to a SessionStore on its own
// goroutine so a slow or failing store never blocks the stdout pump. Append is
// retried with bounded backoff; on terminal failure a mirror_error system
// message is emitted and the batch is dropped (the local transcript on disk
// remains the source of truth).
type mirrorBatcher struct {
	store       shared.SessionStore
	projectsDir string
	ctx         context.Context
	msgChan     chan<- shared.Message
	in          chan mirrorFrame
}

func newMirrorBatcher(ctx context.Context, store shared.SessionStore, projectsDir string, msgChan chan<- shared.Message) *mirrorBatcher {
	return &mirrorBatcher{
		store:       store,
		projectsDir: projectsDir,
		ctx:         ctx,
		msgChan:     msgChan,
		in:          make(chan mirrorFrame, mirrorQueueSize),
	}
}

// run drains queued frames until the context is cancelled.
func (b *mirrorBatcher) run() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case frame := <-b.in:
			b.handle(frame)
		}
	}
}

// enqueue hands a frame to the batcher goroutine, dropping it (with a
// mirror_error) only if the context is done.
func (b *mirrorBatcher) enqueue(filePath string, entries []json.RawMessage) {
	select {
	case <-b.ctx.Done():
	case b.in <- mirrorFrame{filePath: filePath, entries: entries}:
	}
}

func (b *mirrorBatcher) handle(frame mirrorFrame) {
	key, ok := sessionstore.ParseSessionKey(frame.filePath, b.projectsDir)
	if !ok {
		// Path outside the projects dir (e.g. a config-dir mismatch). Drop it;
		// surfacing every such frame as an error would be noisy.
		return
	}

	entries := make([]shared.SessionStoreEntry, len(frame.entries))
	copy(entries, frame.entries)

	var lastErr error
	attempts := len(mirrorBackoff) + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-b.ctx.Done():
				return
			case <-time.After(mirrorBackoff[attempt-1]):
			}
		}

		sendCtx, cancel := context.WithTimeout(b.ctx, mirrorSendTimeout)
		err := b.store.Append(sendCtx, key, entries)
		cancel()
		if err == nil {
			return
		}
		lastErr = err
	}

	b.emitMirrorError(key, lastErr)
}

// emitMirrorError pushes a mirror_error system message so consumers can detect
// transcript-mirror data loss without it interrupting the agent.
func (b *mirrorBatcher) emitMirrorError(key shared.SessionKey, err error) {
	data := map[string]any{
		"type":       shared.MessageTypeSystem,
		"subtype":    "mirror_error",
		"projectKey": key.ProjectKey,
		"sessionId":  key.SessionID,
	}
	if key.Subpath != "" {
		data["subpath"] = key.Subpath
	}
	if err != nil {
		data["error"] = err.Error()
	}

	msg := &shared.SystemMessage{Subtype: "mirror_error", Data: data}
	select {
	case <-b.ctx.Done():
	case b.msgChan <- msg:
	}
}

// effectiveCwd returns the working directory the CLI subprocess will run in,
// which determines the project key used for transcript paths.
func (t *Transport) effectiveCwd() string {
	if t.options != nil && t.options.Cwd != nil && *t.options.Cwd != "" {
		return *t.options.Cwd
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

// prepareSessionMirror materializes a resume transcript from the SessionStore
// into a temporary CLAUDE_CONFIG_DIR. It is a no-op unless both a SessionStore
// and Resume session ID are configured. On success it records the temp dir on
// the transport (removed during cleanup) and buildEnvironment points the
// subprocess at it.
func (t *Transport) prepareSessionMirror(ctx context.Context) error {
	if t.options == nil || t.options.SessionStore == nil || t.options.Resume == nil {
		return nil
	}
	src := sessionstore.ResolveConfigDir(t.options.ExtraEnv)
	tempDir, err := sessionstore.PrepareResume(
		ctx,
		t.options.SessionStore,
		*t.options.Resume,
		t.effectiveCwd(),
		src,
		t.options.SessionStoreLoadTimeout,
	)
	if err != nil {
		return shared.NewConnectionError("failed to load session for resume", err)
	}
	t.sessionTempDir = tempDir
	return nil
}

// sessionProjectsDir returns the projects directory whose transcript paths the
// batcher maps to SessionKeys: the resume temp dir when resuming, otherwise the
// resolved CLAUDE_CONFIG_DIR.
func (t *Transport) sessionProjectsDir() string {
	configDir := t.sessionTempDir
	if configDir == "" {
		var extraEnv map[string]string
		if t.options != nil {
			extraEnv = t.options.ExtraEnv
		}
		configDir = sessionstore.ResolveConfigDir(extraEnv)
	}
	return sessionstore.ProjectsDir(configDir)
}

// startMirrorBatcher constructs and starts the transcript-mirror batcher when a
// SessionStore is configured.
func (t *Transport) startMirrorBatcher() {
	if t.options == nil || t.options.SessionStore == nil {
		return
	}
	t.mirrorBatcher = newMirrorBatcher(t.ctx, t.options.SessionStore, t.sessionProjectsDir(), t.msgChan)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.mirrorBatcher.run()
	}()
}
