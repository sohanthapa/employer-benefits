package claude

import (
	"strings"

	"github.com/sohanthapa/employer-benefits/agent"
	"github.com/sohanthapa/employer-benefits/tools"
)

// systemPrompt holds the rules the model must follow.
// The 5-call cap and ticket creation are enforced in code (agent package), not here.
var systemPrompt = `You are the Employer Benefits Agent. You answer employees' questions about their benefit coverage and help them find providers.

Rules:
1. Only use the tools you are given.
2. Answer coverage questions only from the output of get_plan_details. Never answer coverage from your own knowledge.
3. If the employee ID, zip code or specialty is missing, ask the user for it. Do not guess and do not call a tool with a missing value.
4. Supported specialties: ` + strings.Join(tools.Specialties, ", ") + `. Map the user's words to one of these. If none fits, the specialty is unsupported: call create_support_ticket. Do not ask for a zip code or search.
5. Search only the specialty the user asked for. Never substitute another specialty.
6. If the plan says a supported specialty is not covered, say so and do not search for providers.
7. If a tool result says a support ticket was created, tell the user the ticket ID and call no more tools for that request. A new user message starts a new request.
8. If the question cannot be answered with these tools, call create_support_ticket with a reason that says what the user asked and why it could not be resolved.
9. Do not give medical advice.
10. Keep answers short. Use only facts from the tool results.
11. Reply in plain text. Do not use Markdown (no **bold**, no # headings); the answer is shown in a terminal.`

// tool is one tool definition in the Messages API format.
type tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"input_schema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]property `json:"properties"`
	Required   []string            `json:"required"`
}

type property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// All parameters are strings, matching agent.ToolCall.Args.
var toolDefs = []tool{
	{
		Name:        agent.ToolGetPlanDetails,
		Description: "Returns the employee's benefit coverage: for each specialty, whether the plan covers it.",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]property{
				"employee_id": {Type: "string", Description: "The employee's ID, e.g. E123."},
			},
			Required: []string{"employee_id"},
		},
	},
	{
		Name:        agent.ToolSearchProviders,
		Description: "Finds providers for one specialty in one zip code. Returns an empty list if there are none.",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]property{
				"zip_code":  {Type: "string", Description: "5-digit zip code, e.g. 94107."},
				"specialty": {Type: "string", Description: "One of: " + strings.Join(tools.Specialties, ", ") + "."},
			},
			Required: []string{"zip_code", "specialty"},
		},
	},
	{
		Name:        agent.ToolCreateSupportTicket,
		Description: "Escalates a question that could not be resolved to a human support agent. Returns the ticket ID.",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]property{
				"reason": {Type: "string", Description: "What the user asked and why it could not be resolved."},
			},
			Required: []string{"reason"},
		},
	},
}
