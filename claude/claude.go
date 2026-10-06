// Package claude implements agent.LLM using the Claude Messages API.
// It uses plain net/http, so there is no SDK dependency.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sohanthapa/employer-benefits/agent"
)

const (
	DefaultModel   = "claude-haiku-4-5-20251001"
	defaultBaseURL = "https://api.anthropic.com"
	apiVersion     = "2023-06-01"
	maxTokens      = 1024
)

// Client calls the Claude Messages API.
type Client struct {
	APIKey  string
	Model   string
	BaseURL string // overridden in tests
	HTTP    *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		APIKey:  apiKey,
		Model:   DefaultModel,
		BaseURL: defaultBaseURL,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Wire types for the Messages API.

type request struct {
	Model      string     `json:"model"`
	MaxTokens  int        `json:"max_tokens"`
	System     string     `json:"system"`
	Tools      []tool     `json:"tools"`
	ToolChoice toolChoice `json:"tool_choice"`
	Messages   []message  `json:"messages"`
}

type toolChoice struct {
	Type                   string `json:"type"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use"`
}

type message struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

// block is a content block: text, tool_use or tool_result, depending on Type.
type block struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"` // text

	ID    string          `json:"id,omitempty"` // tool_use
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"` // JSON object; required by the API even when empty

	ToolUseID string `json:"tool_use_id,omitempty"` // tool_result
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type response struct {
	Content    []block `json:"content"`
	StopReason string  `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Next sends the conversation to Claude and returns its next step.
func (c *Client) Next(ctx context.Context, history []agent.Message) (agent.Step, error) {
	if c.APIKey == "" {
		return agent.Step{}, errors.New("ANTHROPIC_API_KEY is not set")
	}

	body, err := json.Marshal(request{
		Model:     c.Model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Tools:     toolDefs,
		// One tool call per turn: agent.Step holds a single call.
		ToolChoice: toolChoice{Type: "auto", DisableParallelToolUse: true},
		Messages:   toMessages(history),
	})
	if err != nil {
		return agent.Step{}, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return agent.Step{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", apiVersion)

	httpResp, err := c.HTTP.Do(req)
	if err != nil {
		return agent.Step{}, fmt.Errorf("call claude: %w", err)
	}
	defer httpResp.Body.Close()

	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return agent.Step{}, fmt.Errorf("read response: %w", err)
	}

	var resp response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return agent.Step{}, fmt.Errorf("claude returned status %d with a non-JSON body", httpResp.StatusCode)
	}
	if httpResp.StatusCode != http.StatusOK {
		if resp.Error != nil {
			return agent.Step{}, fmt.Errorf("claude returned status %d: %s: %s", httpResp.StatusCode, resp.Error.Type, resp.Error.Message)
		}
		return agent.Step{}, fmt.Errorf("claude returned status %d", httpResp.StatusCode)
	}
	return toStep(resp)
}

// toMessages converts our history to the API format.
// A tool result is sent as a user message with a tool_result block.
func toMessages(history []agent.Message) []message {
	msgs := make([]message, 0, len(history))
	for _, m := range history {
		switch m.Role {
		case agent.RoleUser:
			msgs = append(msgs, message{Role: "user", Content: []block{{Type: "text", Text: m.Text}}})

		case agent.RoleAssistant:
			var blocks []block
			// The API rejects empty text blocks; tool-only steps have no text.
			if m.Text != "" {
				blocks = append(blocks, block{Type: "text", Text: m.Text})
			}
			if m.ToolCall != nil {
				args := m.ToolCall.Args
				if args == nil {
					args = map[string]string{}
				}
				input, _ := json.Marshal(args) // a string map always encodes
				blocks = append(blocks, block{Type: "tool_use", ID: m.ToolCall.ID, Name: m.ToolCall.Name, Input: input})
			}
			msgs = append(msgs, message{Role: "assistant", Content: blocks})

		case agent.RoleTool:
			msgs = append(msgs, message{Role: "user", Content: []block{{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Text,
				IsError:   m.IsError,
			}}})
		}
	}
	return msgs
}

// toStep converts Claude's reply to a step for the agent loop.
func toStep(resp response) (agent.Step, error) {
	switch resp.StopReason {
	case "max_tokens":
		return agent.Step{}, errors.New("claude reply was cut off (max_tokens)")
	case "refusal":
		return agent.Step{}, errors.New("claude refused to answer (refusal)")
	}

	var step agent.Step
	var texts []string
	for _, b := range resp.Content {
		switch b.Type {
		case "text":
			texts = append(texts, b.Text)
		case "tool_use":
			if step.ToolCall != nil {
				return agent.Step{}, errors.New("claude returned more than one tool call")
			}
			var input map[string]interface{}
			if len(b.Input) > 0 {
				if err := json.Unmarshal(b.Input, &input); err != nil {
					return agent.Step{}, fmt.Errorf("decode tool input: %w", err)
				}
			}
			args := map[string]string{}
			for k, v := range input {
				// Skip null so the agent treats it as a missing argument.
				if v == nil {
					continue
				}
				// The model may send a number (e.g. zip code) despite the string schema.
				args[k] = fmt.Sprint(v)
			}
			step.ToolCall = &agent.ToolCall{ID: b.ID, Name: b.Name, Args: args}
		}
	}
	step.Text = strings.TrimSpace(strings.Join(texts, "\n"))

	if step.ToolCall == nil && step.Text == "" {
		return agent.Step{}, fmt.Errorf("claude returned an empty reply (stop_reason %q)", resp.StopReason)
	}
	return step, nil
}
