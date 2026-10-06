// Command employer-benefits is a terminal chat with the Employer Benefits Agent,
// using the real Claude model. Needs ANTHROPIC_API_KEY.
//
// Run from the project folder:
//
//	go run .                                    # chat; type "exit" to quit
//	go run . "Is anxiety covered? I am E123."   # one question
//
// Set CLAUDE_MODEL to use a different model.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sohanthapa/employer-benefits/agent"
	"github.com/sohanthapa/employer-benefits/claude"
	"github.com/sohanthapa/employer-benefits/tools"
)

func main() {
	dataPath := flag.String("data", "docs/mock_data.json", "path to mock data file")
	flag.Parse()

	store, err := tools.Load(*dataPath)
	if err != nil {
		fail(err)
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fail(errors.New("ANTHROPIC_API_KEY is not set"))
	}
	llm := claude.New(apiKey)
	if model := os.Getenv("CLAUDE_MODEL"); model != "" {
		llm.Model = model
	}
	a := agent.New(llm, store)
	ctx := context.Background()

	// With arguments: answer that one question and exit.
	if question := strings.TrimSpace(strings.Join(flag.Args(), " ")); question != "" {
		resp, err := a.Ask(ctx, question)
		if err != nil {
			fail(err)
		}
		fmt.Printf("User:   %s\n", question)
		fmt.Print(resp.Format())
		return
	}

	fmt.Println(`Employer Benefits Agent. Type your question, or "exit" to quit.`)
	if err := chat(ctx, a, os.Stdin, os.Stdout); err != nil {
		fail(err)
	}
}

// chat reads one message per line and prints the agent's response,
// until "exit" or end of input. The same agent is used for every line,
// so it remembers earlier messages.
func chat(ctx context.Context, a *agent.Agent, in io.Reader, out io.Writer) error {
	lines := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "You:    ")
		if !lines.Scan() {
			fmt.Fprintln(out)
			return lines.Err()
		}
		msg := strings.TrimSpace(lines.Text())
		switch strings.ToLower(msg) {
		case "":
			continue
		case "exit", "quit":
			return nil
		}

		resp, err := a.Ask(ctx, msg)
		if err != nil {
			// Keep the chat open; the failed turn is not stored, so the user can retry.
			fmt.Fprintln(out, "error:", err)
			continue
		}
		fmt.Fprint(out, resp.Format())
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
