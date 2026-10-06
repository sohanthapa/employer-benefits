package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sohanthapa/employer-benefits/tools"
)

func newAgent(t *testing.T, steps ...Step) *Agent {
	t.Helper()
	store, err := tools.Load("../docs/mock_data.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return New(&ScriptedLLM{Steps: steps}, store)
}

func ask(t *testing.T, a *Agent, msg string) Response {
	t.Helper()
	resp, err := a.Ask(context.Background(), msg)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	t.Logf("answer=%q tools=%v ticket=%q", resp.Answer, toolNames(resp), resp.TicketID)
	return resp
}

func toolNames(r Response) []string {
	names := []string{}
	for _, c := range r.ToolsCalled {
		names = append(names, c.Name)
	}
	return names
}

func wantTools(t *testing.T, r Response, want ...string) {
	t.Helper()
	if got := strings.Join(toolNames(r), ","); got != strings.Join(want, ",") {
		t.Errorf("tools called: want %v, got %v", want, toolNames(r))
	}
}

var (
	planE123   = Call(ToolGetPlanDetails, "employee_id", "E123")
	searchFam  = Call(ToolSearchProviders, "zip_code", "94107", "specialty", "family_counseling")
	searchAnx  = Call(ToolSearchProviders, "zip_code", "94107", "specialty", "anxiety")
	searchDent = Call(ToolSearchProviders, "zip_code", "94107", "specialty", "dentistry")
)

// Scenario 1
func TestCoveredAndProvidersFound(t *testing.T) {
	a := newAgent(t, planE123, searchFam, Answer("Covered. Providers: Bay Family Counseling, Mission Family Center."))
	resp := ask(t, a, "Does my plan cover family counseling? Find a provider near 94107. I am E123.")

	wantTools(t, resp, ToolGetPlanDetails, ToolSearchProviders)
	if resp.TicketID != "" {
		t.Errorf("want no ticket, got %s", resp.TicketID)
	}
	if !strings.Contains(resp.Answer, "Bay Family Counseling") {
		t.Errorf("unexpected answer %q", resp.Answer)
	}
}

// Scenario 2
func TestNotCovered(t *testing.T) {
	a := newAgent(t, planE123, Answer("Your plan does not cover substance use."))
	resp := ask(t, a, "Does my plan cover substance use? I am E123.")

	wantTools(t, resp, ToolGetPlanDetails)
	if resp.TicketID != "" {
		t.Errorf("want no ticket, got %s", resp.TicketID)
	}
}

// Scenario 3
func TestCoveredButNoProvider(t *testing.T) {
	a := newAgent(t, planE123, searchAnx, Answer("Covered, but no provider nearby. I opened a ticket."))
	resp := ask(t, a, "Is anxiety covered? Find a provider near 94107. I am E123.")

	wantTools(t, resp, ToolGetPlanDetails, ToolSearchProviders, ToolCreateSupportTicket)
	if resp.TicketID != "TICKET-001" {
		t.Errorf("want TICKET-001, got %q", resp.TicketID)
	}
	// The LLM must be told about the ticket so it can tell the user.
	last := a.history[len(a.history)-2]
	if last.Role != RoleTool || !strings.Contains(last.Text, "TICKET-001") {
		t.Errorf("tool result should mention the ticket, got %+v", last)
	}
}

// Scenario 4
func TestMissingZipAsksUser(t *testing.T) {
	a := newAgent(t,
		Answer("What is your zip code?"),
		planE123, searchFam, Answer("Covered. Two providers found."),
	)

	first := ask(t, a, "Find me a family counseling provider. I am E123.")
	wantTools(t, first)
	if first.TicketID != "" || first.Answer != "What is your zip code?" {
		t.Errorf("want a question and no ticket, got %+v", first)
	}

	second := ask(t, a, "94107")
	wantTools(t, second, ToolGetPlanDetails, ToolSearchProviders)

	// Both turns are kept, so the LLM sees the earlier question.
	if a.history[0].Role != RoleUser || a.history[2].Text != "94107" {
		t.Errorf("history not kept across turns: %+v", a.history[:3])
	}
}

// A failure on a later turn must still report the original question.
func TestTicketReasonKeepsEarlierTurns(t *testing.T) {
	a := newAgent(t,
		Answer("What is your zip code?"),
		searchAnx, Answer("No provider nearby. I opened a ticket."),
	)
	ask(t, a, "Find me an anxiety provider. I am E123.")
	resp := ask(t, a, "94107")

	reason := resp.ToolsCalled[len(resp.ToolsCalled)-1].Args["reason"]
	if !strings.Contains(reason, "anxiety provider") || !strings.Contains(reason, "94107") {
		t.Errorf("reason should include both turns, got %q", reason)
	}
}

// A ticket for a later question must not include earlier, finished questions.
func TestTicketReasonExcludesEarlierRequests(t *testing.T) {
	a := newAgent(t,
		planE123, Answer("Family counseling is covered."),
		searchAnx, Answer("No provider nearby. I opened a ticket."),
	)
	ask(t, a, "Is family counseling covered? I am E123.")
	resp := ask(t, a, "Find an anxiety provider near 94107.")

	reason := resp.ToolsCalled[len(resp.ToolsCalled)-1].Args["reason"]
	if strings.Contains(reason, "family counseling") || !strings.Contains(reason, "anxiety provider") {
		t.Errorf("reason should cover only the current request, got %q", reason)
	}
}

// A ticket in one turn must not carry over to the next.
func TestTicketDoesNotCarryOver(t *testing.T) {
	a := newAgent(t,
		searchAnx, Answer("No provider nearby. I opened a ticket."),
		planE123, searchFam, Answer("Covered. Two providers found."),
	)
	ask(t, a, "Find an anxiety provider near 94107.")
	resp := ask(t, a, "Is family counseling covered? Find one near 94107. I am E123.")

	wantTools(t, resp, ToolGetPlanDetails, ToolSearchProviders)
	if resp.TicketID != "" {
		t.Errorf("want no ticket on the second turn, got %s", resp.TicketID)
	}
}

// A tool call with a missing argument is sent back to the LLM, not escalated.
func TestMissingArgumentIsNotEscalated(t *testing.T) {
	a := newAgent(t,
		Call(ToolSearchProviders, "zip_code", "", "specialty", "family_counseling"),
		Answer("What is your zip code?"),
	)
	resp := ask(t, a, "Find me a family counseling provider.")

	if resp.TicketID != "" {
		t.Errorf("want no ticket, got %s", resp.TicketID)
	}
	result := a.history[2]
	if !result.IsError || !strings.Contains(result.Text, "missing zip_code") {
		t.Errorf("want a missing zip_code error sent to the LLM, got %+v", result)
	}
}

// Scenario 5
func TestEmployeeNotFound(t *testing.T) {
	a := newAgent(t, Call(ToolGetPlanDetails, "employee_id", "E999"), Answer("I could not find you. I opened a ticket."))
	resp := ask(t, a, "Is family counseling covered? I am E999.")

	wantTools(t, resp, ToolGetPlanDetails, ToolCreateSupportTicket)
	if resp.TicketID == "" {
		t.Error("want a ticket")
	}
}

// Scenario 6
func TestUnsupportedSpecialty(t *testing.T) {
	a := newAgent(t, searchDent, Answer("We do not support dentistry. I opened a ticket."))
	resp := ask(t, a, "Find me a dentist near 94107.")

	wantTools(t, resp, ToolSearchProviders, ToolCreateSupportTicket)
	if resp.TicketID == "" {
		t.Error("want a ticket")
	}
	reason := resp.ToolsCalled[1].Args["reason"]
	if !strings.Contains(reason, "dentist near 94107") || !strings.Contains(reason, "unsupported specialty") {
		t.Errorf("ticket reason should say what was asked and why it failed, got %q", reason)
	}
}

// Scenario 7: the LLM never stops asking for tools.
func TestToolCallLimit(t *testing.T) {
	a := newAgent(t, searchFam)
	resp := ask(t, a, "Find a family counseling provider near 94107.")

	wantTools(t, resp,
		ToolSearchProviders, ToolSearchProviders, ToolSearchProviders, ToolSearchProviders, ToolSearchProviders,
		ToolCreateSupportTicket)
	if resp.TicketID == "" || !strings.Contains(resp.Answer, resp.TicketID) {
		t.Errorf("want a ticket named in the answer, got %+v", resp)
	}
}

func TestToolCallLimitResetsPerRequest(t *testing.T) {
	steps := []Step{}
	for i := 0; i < 2; i++ {
		steps = append(steps, searchFam, searchFam, searchFam, Answer("done"))
	}
	a := newAgent(t, steps...)

	for i := 0; i < 2; i++ {
		resp := ask(t, a, "Find a family counseling provider near 94107.")
		if resp.TicketID != "" {
			t.Errorf("request %d: 3 calls should not hit the limit, got ticket %s", i+1, resp.TicketID)
		}
	}
}

// The LLM may escalate on its own, e.g. for an out-of-scope question.
func TestLLMCreatesTicket(t *testing.T) {
	a := newAgent(t,
		planE123, planE123, planE123, planE123, planE123,
		Call(ToolCreateSupportTicket, "reason", "user asked about dental"),
		Answer("I opened a ticket."),
	)
	resp := ask(t, a, "Is dental covered? I am E123.")

	// The ticket call still works after 5 other calls.
	if resp.TicketID != "TICKET-001" || resp.Answer != "I opened a ticket." {
		t.Errorf("want TICKET-001 from the LLM's own call, got %+v", resp)
	}
	if len(resp.ToolsCalled) != 6 {
		t.Errorf("want 6 tool calls, got %d", len(resp.ToolsCalled))
	}
}

func TestNoToolsAfterTicket(t *testing.T) {
	a := newAgent(t, searchAnx, searchFam)
	resp := ask(t, a, "Find an anxiety provider near 94107.")

	// The second search is refused, and only one ticket exists.
	wantTools(t, resp, ToolSearchProviders, ToolCreateSupportTicket)
	if !strings.Contains(resp.Answer, "TICKET-001") {
		t.Errorf("want fallback answer naming the ticket, got %q", resp.Answer)
	}
}

func TestUnknownTool(t *testing.T) {
	a := newAgent(t, Call("delete_plan", "employee_id", "E123"), Answer("I opened a ticket."))
	resp := ask(t, a, "Delete my plan.")

	if resp.TicketID == "" {
		t.Error("want a ticket for an unknown tool")
	}
}

// Every tool call must be followed by its result, or the next LLM call is invalid.
func TestHistoryPairsToolCallsWithResults(t *testing.T) {
	tests := []struct {
		name  string
		steps []Step
	}{
		{"limit reached", []Step{searchFam}},
		{"tool requested after code-created ticket", []Step{searchAnx, searchFam}},
		{"tool requested after LLM-created ticket", []Step{Call(ToolCreateSupportTicket, "reason", "out of scope"), searchFam}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newAgent(t, tc.steps...)
			ask(t, a, "Find a provider near 94107.")

			for i, m := range a.history {
				if m.ToolCall == nil {
					continue
				}
				if i+1 >= len(a.history) || a.history[i+1].Role != RoleTool || a.history[i+1].ToolCallID != m.ToolCall.ID {
					t.Fatalf("history[%d]: tool call %s has no matching result", i, m.ToolCall.ID)
				}
			}
			if last := a.history[len(a.history)-1]; last.Role != RoleAssistant || last.ToolCall != nil {
				t.Errorf("history should end with the assistant's answer, got %+v", last)
			}
		})
	}
}

func TestEmptyMessage(t *testing.T) {
	a := newAgent(t, Answer("hi"))
	if _, err := a.Ask(context.Background(), "  "); err == nil {
		t.Fatal("want error for an empty message")
	}
	if len(a.history) != 0 {
		t.Errorf("empty message must not be stored, got %d messages", len(a.history))
	}
}

// failAfter answers n calls from the script, then fails.
type failAfter struct {
	ScriptedLLM
	n int
}

func (f *failAfter) Next(ctx context.Context, h []Message) (Step, error) {
	if f.n == 0 {
		return Step{}, errors.New("model unavailable")
	}
	f.n--
	return f.ScriptedLLM.Next(ctx, h)
}

// If the LLM fails after a ticket was created, the user still gets the ticket.
func TestLLMErrorAfterTicket(t *testing.T) {
	store, err := tools.Load("../docs/mock_data.json")
	if err != nil {
		t.Fatal(err)
	}
	a := New(&failAfter{ScriptedLLM: ScriptedLLM{Steps: []Step{searchAnx}}, n: 1}, store)

	resp, err := a.Ask(context.Background(), "Find an anxiety provider near 94107.")
	if err != nil {
		t.Fatalf("want the ticket answer, got error %v", err)
	}
	if resp.TicketID != "TICKET-001" || !strings.Contains(resp.Answer, "TICKET-001") {
		t.Errorf("want a fallback answer naming the ticket, got %+v", resp)
	}
}

type failingLLM struct{}

func (failingLLM) Next(context.Context, []Message) (Step, error) {
	return Step{}, errors.New("model unavailable")
}

func TestLLMError(t *testing.T) {
	store, err := tools.Load("../docs/mock_data.json")
	if err != nil {
		t.Fatal(err)
	}
	a := New(failingLLM{}, store)
	if _, err := a.Ask(context.Background(), "hi"); err == nil {
		t.Fatal("want error when the LLM fails")
	}
	// The failed turn is dropped, so a retry does not duplicate the message.
	if len(a.history) != 0 {
		t.Errorf("want empty history after a failed turn, got %d messages", len(a.history))
	}
}
