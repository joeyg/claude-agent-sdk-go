// Package main demonstrates the SessionStore transcript-mirror feature with the
// in-memory store. The first query mirrors its transcript into the store; the
// second resumes from the store on a fresh CLI config dir, so the agent retains
// context from the first turn even though nothing was read from local disk.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/severity1/claude-agent-sdk-go"
)

func main() {
	fmt.Println("Claude Agent SDK - Session Storage Example")
	fmt.Println("==========================================")

	ctx := context.Background()
	store := claudecode.NewInMemorySessionStore()

	// First turn: establish a fact and capture the CLI session UUID.
	sessionID, err := runTurn(ctx, store, "", "Remember the secret word is 'pineapple'. Reply with just 'ok'.")
	if err != nil {
		handleErr(err)
		return
	}
	fmt.Printf("\n[captured session: %s]\n", sessionID)

	if sessionID == "" {
		log.Fatal("no session id returned; cannot demonstrate resume")
	}

	// Second turn: resume from the store and verify the agent recalls the fact.
	if _, err := runTurn(ctx, store, sessionID, "What is the secret word?"); err != nil {
		handleErr(err)
		return
	}

	fmt.Println("\nDone.")
}

// runTurn runs one query. When resumeID is set, the transcript is loaded back
// from the store; otherwise a new session is started. The transcript is always
// mirrored to the store via WithSessionStore.
func runTurn(ctx context.Context, store claudecode.SessionStore, resumeID, prompt string) (string, error) {
	opts := []claudecode.Option{claudecode.WithSessionStore(store)}
	if resumeID != "" {
		opts = append(opts, claudecode.WithResume(resumeID))
	}

	fmt.Printf("\n> %s\n", prompt)
	iterator, err := claudecode.Query(ctx, prompt, opts...)
	if err != nil {
		return "", err
	}
	defer iterator.Close()

	var sessionID string
	for {
		message, err := iterator.Next(ctx)
		if err != nil {
			if errors.Is(err, claudecode.ErrNoMoreMessages) {
				break
			}
			return sessionID, err
		}
		if message == nil {
			break
		}

		switch msg := message.(type) {
		case *claudecode.AssistantMessage:
			for _, block := range msg.Content {
				if textBlock, ok := block.(*claudecode.TextBlock); ok {
					fmt.Print(textBlock.Text)
				}
			}
		case *claudecode.SystemMessage:
			if msg.Subtype == "mirror_error" {
				log.Printf("session mirror error: %v", msg.Data["error"])
			}
		case *claudecode.ResultMessage:
			sessionID = msg.SessionID
			if msg.IsError && msg.Result != nil {
				return sessionID, fmt.Errorf("query error: %s", *msg.Result)
			}
		}
	}
	fmt.Println()
	return sessionID, nil
}

func handleErr(err error) {
	if cliErr := claudecode.AsCLINotFoundError(err); cliErr != nil {
		fmt.Printf("Claude CLI not found: %v\n", cliErr)
		fmt.Println("Install with: npm install -g @anthropic-ai/claude-code")
		return
	}
	if connErr := claudecode.AsConnectionError(err); connErr != nil {
		fmt.Printf("Connection failed: %v\n", connErr)
		return
	}
	log.Fatalf("Example failed: %v", err)
}
