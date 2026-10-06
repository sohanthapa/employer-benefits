// Package agent runs the tool-calling loop: the LLM picks a tool,
// the agent runs it and feeds the result back, until the LLM answers.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sohanthapa/employer-benefits/tools"
)

// MaxToolCalls is the cap per user request. create_support_ticket is not counted.
const MaxToolCalls = 5

// Response is what one user request returns.
type Response struct {
	Answer      string
	ToolsCalled []ToolCall
	TicketID    string // empty if no ticket was created
}

// Agent keeps the conversation history, so the user can answer
// a follow-up question (e.g. "what is your zip code?") in the next message.
type Agent struct {
	llm     LLM
	store   *tools.Store
	history []Message

	// requestStart is where the current request begins in history.
	// A request can span turns: a turn with no tool call is a clarifying question.
	requestStart int
}

func New(llm LLM, store *tools.Store) *Agent {
	return &Agent{llm: llm, store: store}
}

// Ask handles one user message and returns the agent's response.
func (a *Agent) Ask(ctx context.Context, userMessage string) (resp Response, err error) {
	if strings.TrimSpace(userMessage) == "" {
		return resp, errors.New("empty message")
	}
	// On error, drop this turn so the history stays valid and a retry does not duplicate it.
	start := len(a.history)
	defer func() {
		switch {
		case err != nil:
			a.history = a.history[:start]
		case len(resp.ToolsCalled) > 0:
			// Tools ran, so this request is done; the next message starts a new one.
			a.requestStart = len(a.history)
		}
	}()
	a.history = append(a.history, Message{Role: RoleUser, Text: userMessage})

	calls := 0
	for {
		step, err := a.llm.Next(ctx, a.history)
		if err != nil {
			// A ticket already exists: report it rather than lose it and create a duplicate on retry.
			if resp.TicketID != "" {
				resp.Answer = fallbackAnswer(resp.TicketID)
				a.history = append(a.history, Message{Role: RoleAssistant, Text: resp.Answer})
				return resp, nil
			}
			return resp, fmt.Errorf("llm: %w", err)
		}
		a.history = append(a.history, Message{Role: RoleAssistant, Text: step.Text, ToolCall: step.ToolCall})

		if step.ToolCall == nil {
			resp.Answer = step.Text
			return resp, nil
		}
		call := *step.ToolCall
		isTicket := call.Name == ToolCreateSupportTicket

		// A ticket ends the request: no more tools after escalating.
		if resp.TicketID != "" {
			return a.stop(resp, call, "request already escalated"), nil
		}
		if !isTicket && calls == MaxToolCalls {
			if err := a.escalate(&resp, "tool call limit reached"); err != nil {
				return resp, err
			}
			return a.stop(resp, call, "tool call limit reached"), nil
		}
		if !isTicket {
			calls++
		}

		resp.ToolsCalled = append(resp.ToolsCalled, call)
		out := a.runTool(call)
		if out.ticketID != "" {
			resp.TicketID = out.ticketID
		}
		if out.problem != "" {
			if err := a.escalate(&resp, out.problem); err != nil {
				return resp, err
			}
			out.content += fmt.Sprintf("\nSupport ticket %s was created. Tell the user and do not call more tools for this request.", resp.TicketID)
		}
		a.history = append(a.history, Message{
			Role:       RoleTool,
			Text:       out.content,
			ToolCallID: call.ID,
			IsError:    out.isError,
		})
	}
}

var requiredArgs = map[string][]string{
	ToolGetPlanDetails:      {"employee_id"},
	ToolSearchProviders:     {"zip_code", "specialty"},
	ToolCreateSupportTicket: {"reason"},
}

// toolOutcome is the result of running one tool.
type toolOutcome struct {
	content  string // sent back to the LLM
	isError  bool
	problem  string // non-empty means escalate, with this as the reason
	ticketID string // set when the LLM itself called create_support_ticket
}

func (a *Agent) runTool(call ToolCall) toolOutcome {
	// A missing argument means the LLM should have asked the user.
	// Send it back as an error with no ticket; the call still counts toward the cap.
	for _, name := range requiredArgs[call.Name] {
		if strings.TrimSpace(call.Args[name]) == "" {
			return toolOutcome{content: fmt.Sprintf("error: missing %s. Ask the user for it.", name), isError: true}
		}
	}

	switch call.Name {
	case ToolGetPlanDetails:
		plan, err := a.store.GetPlanDetails(call.Args["employee_id"])
		if err != nil {
			return failed(err.Error())
		}
		return toolOutcome{content: toJSON(plan)}

	case ToolSearchProviders:
		zip, specialty := call.Args["zip_code"], call.Args["specialty"]
		providers, err := a.store.SearchProviders(zip, specialty)
		if err != nil {
			return failed(err.Error())
		}
		// No match is not a tool error, but it is still an escalation case.
		out := toolOutcome{content: toJSON(providers)}
		if len(providers) == 0 {
			out.problem = fmt.Sprintf("no %s provider found near %s", specialty, zip)
		}
		return out

	case ToolCreateSupportTicket:
		ticket, err := a.store.CreateSupportTicket(call.Args["reason"])
		if err != nil {
			return failed(err.Error())
		}
		return toolOutcome{content: toJSON(ticket), ticketID: ticket.ID}

	default:
		return failed(fmt.Sprintf("unknown tool %q", call.Name))
	}
}

func failed(msg string) toolOutcome {
	return toolOutcome{content: "error: " + msg, isError: true, problem: msg}
}

// escalate creates a support ticket from code, so escalation does not depend on the LLM.
// The reason includes every user message of this request, since it may span
// several turns (e.g. the question, then the zip code).
func (a *Agent) escalate(resp *Response, problem string) error {
	asked := []string{}
	for _, m := range a.history[a.requestStart:] {
		if m.Role == RoleUser {
			asked = append(asked, m.Text)
		}
	}
	reason := fmt.Sprintf("User asked: %q. Could not resolve: %s.", strings.Join(asked, " / "), problem)
	ticket, err := a.store.CreateSupportTicket(reason)
	if err != nil {
		return fmt.Errorf("create support ticket: %w", err)
	}
	resp.TicketID = ticket.ID
	resp.ToolsCalled = append(resp.ToolsCalled, ToolCall{
		Name: ToolCreateSupportTicket,
		Args: map[string]string{"reason": reason},
	})
	return nil
}

// stop ends the request without running the requested tool.
// The refused call still gets a result so the history stays valid for the next LLM call.
func (a *Agent) stop(resp Response, call ToolCall, why string) Response {
	resp.Answer = fallbackAnswer(resp.TicketID)
	a.history = append(a.history,
		Message{Role: RoleTool, Text: "not run: " + why, ToolCallID: call.ID, IsError: true},
		Message{Role: RoleAssistant, Text: resp.Answer},
	)
	return resp
}

func fallbackAnswer(ticketID string) string {
	return fmt.Sprintf(
		"I could not fully resolve your request, so I created support ticket %s. A support agent will follow up with you.",
		ticketID)
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "error: " + err.Error()
	}
	return string(b)
}
