# Employer Benefits Agent

A Go backend AI agent that answers employee benefit questions, such as:

> Does my plan cover family counseling, and can you help me find a provider near me?

The agent uses Claude to decide which tool to call, asks for missing information, and escalates to a support ticket when it cannot resolve a request.

## Tools

| Tool | Purpose |
|---|---|
| `get_plan_details(employee_id)` | Returns the employee's benefit coverage |
| `search_providers(zip_code, specialty)` | Finds matching providers |
| `create_support_ticket(reason)` | Escalates unresolved questions |

Supported specialties: `family_counseling`, `marriage_counseling`, `anxiety`, `depression`, `substance_use`, `child_therapy`.

Plan and provider data is mocked in `docs/mock_data.json`. Sample employees: `E123`, `E456`.

## Requirements

- Go
- `ANTHROPIC_API_KEY` set in your environment (chat only; tests and demo do not need it)

## Run

Run all commands from the project folder.

```
go run .                  # chat; type "exit" to quit
go run . "your question"  # one question
go run ./cmd/demo         # 7 scenarios with a fake LLM, no API key
go test ./...             # tests, no API key
```

## Example

```
You:    Does my plan cover family counseling? Find a provider near 94107. I am E123.
Tools:  1. get_plan_details(employee_id="E123")
        2. search_providers(specialty="family_counseling", zip_code="94107")
Agent:  Yes, your plan covers family counseling. Providers near 94107: ...
```

## Rules

- At most 5 tool calls per user message. `create_support_ticket` is not counted.
- A support ticket is created when a tool fails, the employee is not found, no provider is found, the specialty is not supported, or the 5-call limit is reached.
- Missing employee ID, zip code or specialty: the agent asks the user.
- Coverage answers come only from `get_plan_details`.

## Project layout

| Path | Contents |
|---|---|
| `main.go` | Terminal chat |
| `agent/` | Tool-calling loop, 5-call limit, ticket creation, fake LLM for tests |
| `claude/` | Claude API client, system prompt, tool definitions |
| `tools/` | The three tools |
| `docs/mock_data.json` | Mock plans and providers |
| `cmd/demo/` | Scenario demo using the fake LLM |
| `cmd/tools/` | Run a single tool by hand |
| `PROMPT.md` | The prompt used to build this project |
| `REVIEW_LOG.md` | Code review feedback per slice |
