// Package sessionstore provides helpers for the SessionStore transcript-mirror
// feature: resolving the CLI config/projects directory, encoding a working
// directory into a project key, and mapping an emitted transcript file path
// back to a SessionKey.
package sessionstore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// ResolveConfigDir returns the CLI config directory (CLAUDE_CONFIG_DIR),
// matching the CLI's own resolution order: an explicit CLAUDE_CONFIG_DIR in
// extraEnv, then the process environment, then <home>/.claude.
func ResolveConfigDir(extraEnv map[string]string) string {
	if extraEnv != nil {
		if dir := extraEnv["CLAUDE_CONFIG_DIR"]; dir != "" {
			return dir
		}
	}
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to a relative ".claude"; the CLI applies the same default.
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

// ProjectsDir returns the projects subdirectory of a config directory, where
// the CLI stores per-project session transcripts.
func ProjectsDir(configDir string) string {
	return filepath.Join(configDir, "projects")
}

// EncodeProjectKey encodes a working directory into the CLI's filesystem-safe
// project key by replacing every non-alphanumeric rune with '-'. For example
// "/Users/joe/dev/hyperscout" becomes "-Users-joe-dev-hyperscout".
func EncodeProjectKey(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	for _, r := range cwd {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// ParseSessionKey maps an absolute transcript file path to a SessionKey,
// relative to projectsDir. It returns ok=false when the path escapes
// projectsDir or does not match the expected layout.
//
// Layout:
//   - <projectKey>/<sessionId>.jsonl                  -> main transcript
//   - <projectKey>/<sessionId>/<a>/<b>[/...].jsonl    -> subagent/sidecar (subpath set)
func ParseSessionKey(filePath, projectsDir string) (shared.SessionKey, bool) {
	rel, err := filepath.Rel(projectsDir, filePath)
	if err != nil {
		return shared.SessionKey{}, false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return shared.SessionKey{}, false
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		return shared.SessionKey{}, false
	}

	projectKey := parts[0]
	second := parts[1]

	if len(parts) == 2 && strings.HasSuffix(second, ".jsonl") {
		return shared.SessionKey{
			ProjectKey: projectKey,
			SessionID:  strings.TrimSuffix(second, ".jsonl"),
		}, true
	}

	if len(parts) >= 4 {
		sub := append([]string(nil), parts[2:]...)
		sub[len(sub)-1] = strings.TrimSuffix(sub[len(sub)-1], ".jsonl")
		return shared.SessionKey{
			ProjectKey: projectKey,
			SessionID:  second,
			Subpath:    strings.Join(sub, "/"),
		}, true
	}

	return shared.SessionKey{}, false
}
