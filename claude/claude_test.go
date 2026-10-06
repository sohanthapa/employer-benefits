package claude

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sohanthapa/employer-benefits/agent"
	"github.com/sohanthapa/employer-benefits/tools"
)

// fakeClaude is a local server that replies with canned bodies in order
// and records every request it receives.
type fakeClaude struct {
	t        *testing.T
	replies  []string
	status   int
	requests []request
	headers  []http.Header
	bodies   []string // raw JSON, to check the real key names
	paths    []string
}

func (f *fakeClaude) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		f.t.Errorf("bad request body: %v", err)
	}
	f.requests = append(f.requests, req)
	f.headers = append(f.headers, r.Header)
	f.bodies = append(f.bodies, string(raw))
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)

	if len(f.requests) > len(f.replies) {
		f.t.Errorf("unexpected request %d", len(f.requests))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
	}
	io.WriteString(w, f.replies[len(f.requests)-1])
}

func newClient(t *testing.T, f *fakeClaude) *Client {
	t.Helper()
	f.t = t
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	c := New("test-key")
	c.BaseURL = srv.URL
	return c
}

func userMsg(text string) []agent.Message {
	return []agent.Message{{Role: agent.RoleUser, Text: text}}
}

const (
	replyPlan     = `{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_1","name":"get_plan_details","input":{"employee_id":"E123"}}]}`
	replySearch   = `{"stop_reason":"tool_use","content":[{"type":"text","text":"Let me look."},{"type":"tool_use","id":"toolu_2","name":"search_providers","input":{"zip_code":94107,"specialty":"family_counseling"}}]}`
	replySearchNo = `{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_2","name":"search_providers","input":{"zip_code":"94107","specialty":"anxiety"}}]}`
	replyAnswer   = `{"stop_reason":"end_turn","content":[{"type":"text","text":"Yes, it is covered."}]}`
)

func TestNext_Request(t *testing.T) {
	f := &fakeClaude{replies: []string{replyAnswer}}
	c := newClient(t, f)

	if _, err := c.Next(context.Background(), userMsg("Is anxiety covered?")); err != nil {
		t.Fatalf("Next: %v", err)
	}

	h, req := f.headers[0], f.requests[0]
	if f.paths[0] != "POST /v1/messages" {
		t.Errorf("want POST /v1/messages, got %s", f.paths[0])
	}
	if h.Get("x-api-key") != "test-key" || h.Get("anthropic-version") != "2023-06-01" || h.Get("content-type") != "application/json" {
		t.Errorf("missing auth, version or content-type header: %v", h)
	}
	// Check literal key names: the fake server decodes with our own structs,
	// so a wrong JSON tag would otherwise pass on both sides.
	for _, key := range []string{
		`"model":"`, `"max_tokens":1024`, `"system":"`, `"messages":[`,
		`"tools":[{"name":"get_plan_details","description":"`, `"input_schema":{"type":"object","properties":{`,
		`"required":["employee_id"]`,
		`"tool_choice":{"type":"auto","disable_parallel_tool_use":true}`,
	} {
		if !strings.Contains(f.bodies[0], key) {
			t.Errorf("request body is missing %s\nbody: %s", key, f.bodies[0])
		}
	}
	if req.Model != DefaultModel || req.MaxTokens != maxTokens || req.System == "" {
		t.Errorf("unexpected model/max_tokens/system: %+v", req)
	}
	if !req.ToolChoice.DisableParallelToolUse {
		t.Error("parallel tool use must be disabled")
	}

	names := []string{}
	for _, tl := range req.Tools {
		names = append(names, tl.Name)
	}
	if got := strings.Join(names, ","); got != "get_plan_details,search_providers,create_support_ticket" {
		t.Errorf("want exactly the three tools, got %s", got)
	}
}

func TestSystemPromptListsSpecialties(t *testing.T) {
	for _, sp := range tools.Specialties {
		if !strings.Contains(systemPrompt, sp) {
			t.Errorf("system prompt is missing specialty %q", sp)
		}
	}
}

func TestToMessages(t *testing.T) {
	history := []agent.Message{
		{Role: agent.RoleUser, Text: "Is anxiety covered? I am E123."},
		{Role: agent.RoleAssistant, ToolCall: &agent.ToolCall{ID: "toolu_1", Name: "get_plan_details", Args: map[string]string{"employee_id": "E123"}}},
		{Role: agent.RoleTool, ToolCallID: "toolu_1", Text: "error: employee not found", IsError: true},
		{Role: agent.RoleAssistant, Text: "I opened a ticket."},
	}
	got, err := json.Marshal(toMessages(history))
	if err != nil {
		t.Fatal(err)
	}

	// Tool-only step has no text block; the tool result goes in a user message.
	want := `[` +
		`{"role":"user","content":[{"type":"text","text":"Is anxiety covered? I am E123."}]},` +
		`{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"get_plan_details","input":{"employee_id":"E123"}}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"error: employee not found","is_error":true}]},` +
		`{"role":"assistant","content":[{"type":"text","text":"I opened a ticket."}]}` +
		`]`
	if string(got) != want {
		t.Errorf("messages mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestToMessages_ToolCallWithoutArgsKeepsInput(t *testing.T) {
	history := []agent.Message{{Role: agent.RoleAssistant, ToolCall: &agent.ToolCall{ID: "toolu_1", Name: "x"}}}
	got, _ := json.Marshal(toMessages(history))
	if !strings.Contains(string(got), `"input":{}`) {
		t.Errorf("tool_use must always have an input object, got %s", got)
	}
}

func TestNext_Replies(t *testing.T) {
	tests := []struct {
		name     string
		reply    string
		wantText string
		wantTool string
		wantArgs map[string]string
	}{
		{"final answer", replyAnswer, "Yes, it is covered.", "", nil},
		{"tool call", replyPlan, "", "get_plan_details", map[string]string{"employee_id": "E123"}},
		{"text and tool call, numeric zip", replySearch, "Let me look.", "search_providers",
			map[string]string{"zip_code": "94107", "specialty": "family_counseling"}},
		// A null argument is dropped, so the agent sees it as missing.
		{"null argument", `{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_1","name":"get_plan_details","input":{"employee_id":null}}]}`,
			"", "get_plan_details", map[string]string{"employee_id": ""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, &fakeClaude{replies: []string{tc.reply}})
			step, err := c.Next(context.Background(), userMsg("hi"))
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			t.Logf("step: text=%q tool=%+v", step.Text, step.ToolCall)

			if step.Text != tc.wantText {
				t.Errorf("text: want %q, got %q", tc.wantText, step.Text)
			}
			if tc.wantTool == "" {
				if step.ToolCall != nil {
					t.Errorf("want no tool call, got %+v", step.ToolCall)
				}
				return
			}
			if step.ToolCall == nil || step.ToolCall.Name != tc.wantTool || step.ToolCall.ID == "" {
				t.Fatalf("want tool %s with an ID, got %+v", tc.wantTool, step.ToolCall)
			}
			for k, v := range tc.wantArgs {
				if step.ToolCall.Args[k] != v {
					t.Errorf("arg %s: want %q, got %q", k, v, step.ToolCall.Args[k])
				}
			}
		})
	}
}

func TestNext_Errors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		reply   string
		wantErr string
	}{
		{"api error", 401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`, "invalid x-api-key"},
		{"non-JSON body", 502, `<html>bad gateway</html>`, "status 502"},
		{"cut off", 0, `{"stop_reason":"max_tokens","content":[{"type":"text","text":"Yes, it"}]}`, "max_tokens"},
		{"two tool calls", 0, `{"stop_reason":"tool_use","content":[` +
			`{"type":"tool_use","id":"a","name":"get_plan_details","input":{"employee_id":"E123"}},` +
			`{"type":"tool_use","id":"b","name":"search_providers","input":{"zip_code":"94107","specialty":"anxiety"}}]}`, "more than one tool call"},
		{"empty reply", 0, `{"stop_reason":"end_turn","content":[]}`, "empty reply"},
		{"refusal", 0, `{"stop_reason":"refusal","content":[{"type":"text","text":"I can't help with that."}]}`, "refusal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, &fakeClaude{replies: []string{tc.reply}, status: tc.status})
			_, err := c.Next(context.Background(), userMsg("hi"))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestNext_MissingAPIKey(t *testing.T) {
	c := New("")
	if _, err := c.Next(context.Background(), userMsg("hi")); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("want an ANTHROPIC_API_KEY error, got %v", err)
	}
}

// The real client driving the real agent loop, against the fake server.
func TestAgentWithClaudeClient(t *testing.T) {
	store, err := tools.Load("../docs/mock_data.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeClaude{replies: []string{replyPlan, replySearchNo, replyAnswer}}
	a := agent.New(newClient(t, f), store)

	resp, err := a.Ask(context.Background(), "Is anxiety covered? Find a provider near 94107. I am E123.")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	t.Logf("answer=%q ticket=%q tools=%d", resp.Answer, resp.TicketID, len(resp.ToolsCalled))

	if resp.TicketID != "TICKET-001" || resp.Answer != "Yes, it is covered." {
		t.Errorf("unexpected response %+v", resp)
	}

	checkWire(t, f)

	// The last request carries the no-provider result with the ticket ID for the model to report.
	last := f.requests[2].Messages
	result := last[len(last)-1].Content[0]
	if result.Type != "tool_result" || !strings.Contains(result.Content, "TICKET-001") {
		t.Errorf("want the ticket ID in the last tool result, got %+v", result)
	}
}

// checkWire verifies every request is a valid sequence: roles alternate starting
// with user, and each tool_use is answered by a tool_result with the same ID
// in the next message.
func checkWire(t *testing.T, f *fakeClaude) {
	t.Helper()
	for n, req := range f.requests {
		for i, m := range req.Messages {
			wantRole := "user"
			if i%2 == 1 {
				wantRole = "assistant"
			}
			if m.Role != wantRole {
				t.Errorf("request %d message %d: want role %s, got %s", n+1, i, wantRole, m.Role)
			}
			for _, b := range m.Content {
				if b.Type != "tool_use" {
					continue
				}
				if i+1 >= len(req.Messages) || req.Messages[i+1].Content[0].ToolUseID != b.ID {
					t.Errorf("request %d: tool_use %s has no matching tool_result", n+1, b.ID)
				}
			}
		}
	}
}

// The limit path ends a turn with a refused tool call and a fixed answer.
// The next turn must still be a valid request.
func TestAgentWithClaudeClient_LimitThenNextTurn(t *testing.T) {
	store, err := tools.Load("../docs/mock_data.json")
	if err != nil {
		t.Fatal(err)
	}
	// A missing argument first (sent back as is_error), then calls until the limit.
	missingArg := `{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_0","name":"get_plan_details","input":{}}]}`
	f := &fakeClaude{replies: []string{missingArg, replyPlan, replyPlan, replyPlan, replyPlan, replyPlan, replyAnswer}}
	a := agent.New(newClient(t, f), store)

	first, err := a.Ask(context.Background(), "Is anxiety covered? I am E123.")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if first.TicketID == "" || !strings.Contains(first.Answer, first.TicketID) {
		t.Errorf("want the limit to create a ticket, got %+v", first)
	}
	if !strings.Contains(f.bodies[1], `"content":"error: missing employee_id. Ask the user for it.","is_error":true`) {
		t.Errorf("missing-argument result not sent as is_error: %s", f.bodies[1])
	}

	second, err := a.Ask(context.Background(), "Thanks.")
	if err != nil {
		t.Fatalf("second Ask: %v", err)
	}
	if second.Answer != "Yes, it is covered." {
		t.Errorf("unexpected second answer %q", second.Answer)
	}
	checkWire(t, f)
}
