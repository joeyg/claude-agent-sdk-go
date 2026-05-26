package parser

import (
	"testing"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

func TestParseTranscriptMirrorMessage(t *testing.T) {
	line := `{"type":"transcript_mirror","filePath":"/cfg/projects/-p/sess.jsonl","entries":[{"type":"user","text":"hi"},{"type":"assistant","text":"yo"}]}`

	msgs, err := New().ProcessLine(line)
	if err != nil {
		t.Fatalf("ProcessLine error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}

	tm, ok := msgs[0].(*shared.TranscriptMirrorMessage)
	if !ok {
		t.Fatalf("got %T, want *TranscriptMirrorMessage", msgs[0])
	}
	if tm.Type() != shared.MessageTypeTranscriptMirror {
		t.Errorf("Type() = %q, want %q", tm.Type(), shared.MessageTypeTranscriptMirror)
	}
	if tm.FilePath != "/cfg/projects/-p/sess.jsonl" {
		t.Errorf("FilePath = %q", tm.FilePath)
	}
	if len(tm.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(tm.Entries))
	}
}

func TestParseTranscriptMirrorMessageMissingFields(t *testing.T) {
	for _, line := range []string{
		`{"type":"transcript_mirror","entries":[]}`,
		`{"type":"transcript_mirror","filePath":"/x.jsonl"}`,
	} {
		if _, err := New().ProcessLine(line); err == nil {
			t.Errorf("expected error for %q, got nil", line)
		}
	}
}
