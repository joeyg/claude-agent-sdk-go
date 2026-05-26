// Package main provides a reference S3-backed SessionStore for the Claude Agent
// SDK. It is an example/reference adapter (own module, own AWS dependency) — the
// core SDK stays dependency-free. Copy this file into your project and adapt it.
//
// Storage model: append-only. Each Append writes one newline-delimited-JSON
// part object under <prefix>/<projectKey>/<sessionId>/part-<epochMs>-<rand>.jsonl.
// Load lists those parts, sorts them lexically (the timestamped names make that
// chronological), fetches each, and concatenates the entries. Append-only means
// concurrent appends never race on a read-modify-write.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/severity1/claude-agent-sdk-go"
)

// S3SessionStore is a reference SessionStore backed by an S3 bucket.
type S3SessionStore struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3SessionStore builds a store from a pre-configured S3 client, bucket, and
// key prefix. The caller owns credentials, region, TLS, and pooling via the
// client.
func NewS3SessionStore(client *s3.Client, bucket, prefix string) *S3SessionStore {
	return &S3SessionStore{client: client, bucket: bucket, prefix: strings.Trim(prefix, "/")}
}

// keyPrefix returns the S3 prefix for one transcript (main or subagent).
func (s *S3SessionStore) keyPrefix(key claudecode.SessionKey) string {
	parts := []string{}
	if s.prefix != "" {
		parts = append(parts, s.prefix)
	}
	parts = append(parts, key.ProjectKey, key.SessionID)
	if key.Subpath != "" {
		parts = append(parts, key.Subpath)
	}
	return path.Join(parts...) + "/"
}

// Append writes one part object containing the batch as NDJSON.
func (s *S3SessionStore) Append(ctx context.Context, key claudecode.SessionKey, entries []claudecode.SessionStoreEntry) error {
	if len(entries) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, e := range entries {
		buf.Write(e)
		buf.WriteByte('\n')
	}

	objectKey := s.keyPrefix(key) + fmt.Sprintf("part-%013d-%s.jsonl", time.Now().UnixMilli(), randHex())
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		return fmt.Errorf("put %s: %w", objectKey, err)
	}
	return nil
}

// Load lists, sorts, and concatenates all part objects for the transcript.
func (s *S3SessionStore) Load(ctx context.Context, key claudecode.SessionKey) ([]claudecode.SessionStoreEntry, error) {
	prefix := s.keyPrefix(key)

	var keys []string
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", prefix, err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	sort.Strings(keys) // lexical == chronological thanks to the timestamped names

	var entries []claudecode.SessionStoreEntry
	for _, k := range keys {
		out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(k),
		})
		if err != nil {
			return nil, fmt.Errorf("get %s: %w", k, err)
		}
		data, err := io.ReadAll(out.Body)
		_ = out.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", k, err)
		}
		for _, line := range bytes.Split(data, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			entries = append(entries, claudecode.SessionStoreEntry(append([]byte(nil), line...)))
		}
	}
	return entries, nil
}

func randHex() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "000000000000"
	}
	return hex.EncodeToString(b[:])
}

// Ensure S3SessionStore satisfies the interface at compile time.
var _ claudecode.SessionStore = (*S3SessionStore)(nil)
