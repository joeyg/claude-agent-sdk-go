package sessionstore

import (
	"path/filepath"
	"testing"
)

func TestEncodeProjectKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/Users/joe/dev/hyperscout", "-Users-joe-dev-hyperscout"},
		{"/Users/joe/.claude/worktrees/x", "-Users-joe--claude-worktrees-x"},
		{"abc123", "abc123"},
		{"", ""},
		{"a_b.c/d", "a-b-c-d"},
	}
	for _, c := range cases {
		if got := EncodeProjectKey(c.in); got != c.want {
			t.Errorf("EncodeProjectKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseSessionKey(t *testing.T) {
	projects := filepath.Join("/cfg", "projects")
	j := func(parts ...string) string {
		return filepath.Join(append([]string{projects}, parts...)...)
	}

	cases := []struct {
		name        string
		filePath    string
		wantOK      bool
		wantProject string
		wantSession string
		wantSubpath string
	}{
		{
			name:        "main transcript",
			filePath:    j("-Users-joe", "sess123.jsonl"),
			wantOK:      true,
			wantProject: "-Users-joe",
			wantSession: "sess123",
		},
		{
			name:        "subagent transcript",
			filePath:    j("-p", "sess", "subagents", "agent-1.jsonl"),
			wantOK:      true,
			wantProject: "-p",
			wantSession: "sess",
			wantSubpath: "subagents/agent-1",
		},
		{
			name:     "outside projects dir",
			filePath: filepath.Join("/other", "x.jsonl"),
			wantOK:   false,
		},
		{
			name:     "too few segments",
			filePath: j("onlyone"),
			wantOK:   false,
		},
		{
			name:     "three segments unsupported",
			filePath: j("-p", "sess", "foo.jsonl"),
			wantOK:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key, ok := ParseSessionKey(c.filePath, projects)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if key.ProjectKey != c.wantProject || key.SessionID != c.wantSession || key.Subpath != c.wantSubpath {
				t.Errorf("got %+v, want {%q %q %q}", key, c.wantProject, c.wantSession, c.wantSubpath)
			}
		})
	}
}
