// Command demo runs the agent loop on the 7 scenarios using the scripted
// fake LLM, and prints what the agent did. No API key needed.
//
// The LLM steps and final answers are scripted here; the tool calls,
// the 5-call cap and the ticket creation are done by the real agent code.
//
// Run from the project folder:
//
//	go run ./cmd/demo
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/sohanthapa/employer-benefits/agent"
	"github.com/sohanthapa/employer-benefits/tools"
)

type scenario struct {
	name  string
	turns []string     // user messages, one per request
	steps []agent.Step // what the fake LLM does
}

func scenarios() []scenario {
	plan := func(id string) agent.Step { return agent.Call(agent.ToolGetPlanDetails, "employee_id", id) }
	search := func(zip, specialty string) agent.Step {
		return agent.Call(agent.ToolSearchProviders, "zip_code", zip, "specialty", specialty)
	}

	return []scenario{
		{
			name:  "1. Covered and providers found",
			turns: []string{"Does my plan cover family counseling? Find a provider near 94107. I am E123."},
			steps: []agent.Step{plan("E123"), search("94107", "family_counseling"),
				agent.Answer("Yes, family counseling is covered. Providers near 94107: Bay Family Counseling, Mission Family Center.")},
		},
		{
			name:  "2. Not covered",
			turns: []string{"Does my plan cover substance use? I am E123."},
			steps: []agent.Step{plan("E123"), agent.Answer("No, your plan does not cover substance use.")},
		},
		{
			name:  "3. Covered but no provider found",
			turns: []string{"Is anxiety covered? Find a provider near 94107. I am E123."},
			steps: []agent.Step{plan("E123"), search("94107", "anxiety"),
				agent.Answer("Anxiety is covered, but I found no provider near 94107. I opened a support ticket for you.")},
		},
		{
			name:  "4. Missing zip code",
			turns: []string{"Find me a family counseling provider. I am E123.", "94107"},
			steps: []agent.Step{agent.Answer("What is your zip code?"),
				plan("E123"), search("94107", "family_counseling"),
				agent.Answer("Family counseling is covered. Providers near 94107: Bay Family Counseling, Mission Family Center.")},
		},
		{
			name:  "5. Employee not found",
			turns: []string{"Is family counseling covered? I am E999."},
			steps: []agent.Step{plan("E999"), agent.Answer("I could not find employee E999. I opened a support ticket for you.")},
		},
		{
			name:  "6. Specialty not supported",
			turns: []string{"Find me a dentist near 94107."},
			steps: []agent.Step{search("94107", "dentistry"),
				agent.Answer("Dentistry is not a supported specialty. I opened a support ticket for you.")},
		},
		{
			name:  "7. Tool call limit reached (LLM never stops calling tools)",
			turns: []string{"Find a family counseling provider near 94107."},
			steps: []agent.Step{search("94107", "family_counseling")},
		},
	}
}

func main() {
	dataPath := flag.String("data", "docs/mock_data.json", "path to mock data file")
	flag.Parse()

	store, err := tools.Load(*dataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	for _, sc := range scenarios() {
		fmt.Printf("=== %s ===\n", sc.name)
		a := agent.New(&agent.ScriptedLLM{Steps: sc.steps}, store)
		for _, msg := range sc.turns {
			resp, err := a.Ask(context.Background(), msg)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			fmt.Printf("User:   %s\n", msg)
			fmt.Print(resp.Format())
		}
		fmt.Println()
	}
}
