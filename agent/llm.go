package agent

import "context"

// Tool names the LLM may ask for.
const (
	ToolGetPlanDetails      = "get_plan_details"
	ToolSearchProviders     = "search_providers"
	ToolCreateSupportTicket = "create_support_ticket"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is one tool the LLM wants to run.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]string
}

// Message is one entry in the conversation history.
type Message struct {
	Role Role
	Text string

	ToolCall   *ToolCall // assistant only: the tool it asked for
	ToolCallID string    // tool only: the call this result answers
	IsError    bool      // tool only
}

// Step is the LLM's next move: a tool call, or a final answer when ToolCall is nil.
type Step struct {
	Text     string
	ToolCall *ToolCall
}

// LLM decides the next step from the conversation so far.
// It is an interface so tests can use a scripted fake instead of a real model.
type LLM interface {
	Next(ctx context.Context, history []Message) (Step, error)
}
