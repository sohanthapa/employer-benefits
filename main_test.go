package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sohanthapa/employer-benefits/agent"
	"github.com/sohanthapa/employer-benefits/tools"
)

func runChat(t *testing.T, llm agent.LLM, input string) string {
	t.Helper()
	store, err := tools.Load("docs/mock_data.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var out bytes.Buffer
	if err := chat(context.Background(), agent.New(llm, store), strings.NewReader(input), &out); err != nil {
		t.Fatalf("chat: %v", err)
	}
	t.Logf("\n%s", out.String())
	return out.String()
}

// Scenario 4 end to end: the agent asks for the zip, the user answers on the next line.
func TestChat_MultiTurn(t *testing.T) {
	llm := &agent.ScriptedLLM{Steps: []agent.Step{
		agent.Answer("What is your zip code?"),
		agent.Call(agent.ToolGetPlanDetails, "employee_id", "E123"),
		agent.Call(agent.ToolSearchProviders, "zip_code", "94107", "specialty", "family_counseling"),
		agent.Answer("Covered. Two providers found."),
	}}
	out := runChat(t, llm, "Find me a family counseling provider. I am E123.\n\n94107\nexit\nnot read\n")

	for _, want := range []string{
		"Agent:  What is your zip code?",
		`1. get_plan_details(employee_id="E123")`,
		`2. search_providers(specialty="family_counseling", zip_code="94107")`,
		"Agent:  Covered. Two providers found.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q", want)
		}
	}
	// Blank line skipped, and nothing after "exit" is handled: two answers only.
	if got := strings.Count(out, "Agent:"); got != 2 {
		t.Errorf("want 2 answers, got %d", got)
	}
}

func TestChat_QuitWords(t *testing.T) {
	for _, word := range []string{"exit", "quit", "EXIT", " Quit "} {
		out := runChat(t, &agent.ScriptedLLM{Steps: []agent.Step{agent.Answer("Hello.")}}, word+"\nhi\n")
		if strings.Contains(out, "Agent:") {
			t.Errorf("%q should end the chat before any answer, got %q", word, out)
		}
	}
}

// A ticket on one turn does not show on the next.
func TestChat_TicketThenNormalTurn(t *testing.T) {
	llm := &agent.ScriptedLLM{Steps: []agent.Step{
		agent.Call(agent.ToolSearchProviders, "zip_code", "94107", "specialty", "anxiety"),
		agent.Answer("No provider nearby. I opened a ticket."),
		agent.Call(agent.ToolGetPlanDetails, "employee_id", "E123"),
		agent.Answer("Family counseling is covered."),
	}}
	out := runChat(t, llm, "Find an anxiety provider near 94107.\nIs family counseling covered? I am E123.\n")

	if got := strings.Count(out, "Ticket: "); got != 1 {
		t.Errorf("want exactly one Ticket line, got %d", got)
	}
	if !strings.Contains(out, "Agent:  Family counseling is covered.") {
		t.Errorf("second turn should be answered normally, got %q", out)
	}
}

func TestChat_EndOfInput(t *testing.T) {
	out := runChat(t, &agent.ScriptedLLM{Steps: []agent.Step{agent.Answer("Hello.")}}, "hi\n")
	if !strings.Contains(out, "Agent:  Hello.") {
		t.Errorf("unexpected output %q", out)
	}
}

// failOnce fails the first call, then answers.
type failOnce struct{ calls int }

func (f *failOnce) Next(context.Context, []agent.Message) (agent.Step, error) {
	f.calls++
	if f.calls == 1 {
		return agent.Step{}, errors.New("model unavailable")
	}
	return agent.Answer("Hello."), nil
}

func TestChat_ErrorKeepsChatOpen(t *testing.T) {
	out := runChat(t, &failOnce{}, "hi\nhi\n")
	if !strings.Contains(out, "error: llm: model unavailable") || !strings.Contains(out, "Agent:  Hello.") {
		t.Errorf("want an error line, then an answer on retry, got %q", out)
	}
}
