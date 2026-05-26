// Command 22_s3_session_store wires the reference S3SessionStore into a query.
//
// It expects S3_SESSION_BUCKET (and optionally S3_SESSION_PREFIX) plus standard
// AWS credentials in the environment. Without a bucket it prints usage and
// exits, so the example still builds and runs in CI without S3 access.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/severity1/claude-agent-sdk-go"
)

func main() {
	bucket := os.Getenv("S3_SESSION_BUCKET")
	if bucket == "" {
		fmt.Println("Set S3_SESSION_BUCKET (and optionally S3_SESSION_PREFIX) to run this example.")
		fmt.Println("It mirrors session transcripts to S3 and resumes from them.")
		return
	}

	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load AWS config: %v", err)
	}
	store := NewS3SessionStore(s3.NewFromConfig(cfg), bucket, os.Getenv("S3_SESSION_PREFIX"))

	iterator, err := claudecode.Query(ctx, "Say hello in one word.", claudecode.WithSessionStore(store))
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer iterator.Close()

	for {
		message, err := iterator.Next(ctx)
		if err != nil {
			if errors.Is(err, claudecode.ErrNoMoreMessages) {
				break
			}
			log.Fatalf("next: %v", err)
		}
		if message == nil {
			break
		}
		switch msg := message.(type) {
		case *claudecode.AssistantMessage:
			for _, block := range msg.Content {
				if tb, ok := block.(*claudecode.TextBlock); ok {
					fmt.Print(tb.Text)
				}
			}
		case *claudecode.ResultMessage:
			fmt.Printf("\n[session %s mirrored to s3://%s]\n", msg.SessionID, bucket)
		}
	}
}
