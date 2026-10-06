# Review Log

Feedback from the reviewer agent after each slice, and what was done about it.
For reference only. Nit comments are left out.

## Slice 1 — mock data + tools

| # | Severity | Feedback | Action |
|---|----------|----------|--------|
| 1 | Medium | `Load` does not validate the data. A plan missing a specialty silently reads as "not covered", a provider with a misspelled specialty can never be found, and `{}` loads fine. | Fixed. `Load` now fails if there are no plans, a plan is missing a specialty, or a provider has an unsupported specialty. Tests added. |
| 2 | Medium | Inputs are trimmed but matched case-sensitively. `e123` or `Family_Counseling` from the LLM would fail and cause a needless ticket. | Fixed. Employee ID is upper-cased and specialty lower-cased before matching. Tests added. |
| 3 | Low | "No provider found" returns an empty list with no error, so it is the one escalation case that is not an error. | No change here. Slice 2 agent loop must check for an empty list. |
| 4 | Low | `tickets` has no getter, so later slices can only see a ticket through the return value. | Deferred. Add a getter only if slice 2 needs it. |
| 5 | Low | Missing tests: malformed JSON, trimming, stored ticket reason. | Fixed. Tests added. |
| 6 | Low | `go.mod` has no `go` directive, so newer language features (`any`, generics) will not compile. | Deferred to slice 3, where adding the Claude SDK updates `go.mod` anyway. Current code does not need them. |

## Slice 2 — agent loop, fake LLM, 5-call cap, escalation

Also covers `cmd/tools/main.go` (manual tool runner from slice 1) and `cmd/demo/main.go`. No findings in either.

| # | Severity | Feedback | Action |
|---|----------|----------|--------|
| 1 | Medium | Ticket reason used only the current message. In a multi-turn request, a failure on turn 2 filed `User asked: "94107"` and lost the original question. | Fixed. The reason now includes every user message in the conversation. Test added. |
| 2 | Medium | A tool call with a missing argument created a ticket ("no provider found near ") instead of asking the user. | Fixed. A missing required argument is sent back to the LLM as an error telling it to ask the user. No ticket; the call still counts toward the cap. Test added. |
| 3 | Low | On an error return, the half-finished turn stayed in history: a retry would duplicate the user message, and a tool call could be left without a result. | Fixed. `Ask` drops the turn from history on any error. Test added. |
| 4 | Low | When the 5-call limit is hit, the agent returns a fixed fallback answer instead of giving the LLM one last chance to answer. | No change. The rule is "at 5, stop". A fixed answer is predictable and needs no extra LLM call. |
| 5 | Low | The test that every tool call has a matching result covered only the limit path. | Fixed. Now also covers a tool requested after a code-created ticket and after an LLM-created ticket. |

Notes for slice 3 (Claude client), from the same review:

- Tool arguments are `map[string]string`. Declare every parameter as a string in the tool schema, and convert non-string values (e.g. a numeric zip) when decoding.
- Tool-only steps have empty text. Do not send empty text blocks; treat an empty final answer as an error.
- Return an error if the model sends more than one tool call in a turn or stops at `max_tokens`, since a step holds one call.

## Slice 3 — Claude API client

Reviewer traced every path through `Agent.Ask` and found all of them produce a valid message sequence for the API.

| # | Severity | Feedback | Action |
|---|----------|----------|--------|
| 1 | Medium | System prompt rule 4 told the model to pass an unsupported specialty to a tool, but a coverage-only question ("Is dermatology covered?") has no search, so the model could answer "not covered" with no ticket. | Fixed. Rule 4 now says an unsupported specialty means call `create_support_ticket`, without asking for a zip or searching. Rule 6 is limited to supported specialties. |
| 2 | Medium | The fake server decoded requests with the client's own structs, so a wrong JSON tag (`input_schema`, `tool_choice`, ...) would pass on both sides. | Fixed. The request test now checks the raw body for the literal key names, plus method, path and headers. |
| 3 | Low | A `null` tool argument became the string `"<nil>"`, which passed the missing-argument check and led to a ticket instead of a question. | Fixed. Null arguments are dropped. Test added. |
| 4 | Low | An empty user message would be sent as a text block with no text, which the API rejects. | Fixed. `Ask` returns an error for an empty message and stores nothing. Test added. |
| 5 | Low | A `refusal` stop reason showed up as a generic "empty reply" error, or as a normal answer. | Fixed. It is now an explicit error. Test added. |
| 6 | Low | The end-to-end test covered only the no-provider path. | Fixed. Added a test for a missing argument, then the 5-call limit, then a second turn, with the same wire checks. |

Not done: formatting a numeric zip with a leading zero (`02139` sent as a number arrives as `2139`). The schema declares zip as a string, so this is unlikely; left as is to keep the code simple.

## Slice 4 — interactive chat, plus a whole-project pass

Whole-project result: all 7 scenarios behave to spec. "Not covered means no search" and "never substitute a specialty" are enforced by the system prompt only, not by code.

| # | Severity | Feedback | Action |
|---|----------|----------|--------|
| 1 | Medium | An old tool result saying "do not call more tools" stays in the chat history, and prompt rule 7 had no scope, so the model might refuse tools for a new question later in the same chat. | Fixed. Both texts now say "for this request", and rule 7 adds that a new user message starts a new request. Test added that a second turn runs tools normally. |
| 2 | Medium | The ticket reason joined every user message in the session, so a ticket on question 3 also listed questions 1 and 2. | Fixed. The agent tracks where the current request starts. A turn with tool calls ends the request; a turn with none (a clarifying question) continues it. Test added. |
| 3 | Low | If the model call failed after a ticket was created, the turn was dropped: the user never saw the ticket ID and a retry made a duplicate. | Fixed. The agent returns the fixed ticket answer instead of the error. Test added. |
| 4 | Low | A single input line over 64KB ends the chat with an error. | No change. The error is reported, not silent, and a 64KB chat message is not realistic. |
| 5 | Low | A missing API key was only reported after each message. | Fixed. `main` checks the key at startup and exits with a clear error. |
| 6 | Low | No tests for `quit` / upper-case `EXIT`, or for a ticket turn followed by a normal turn in the chat. | Fixed. Tests added. |
