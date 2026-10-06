package agent

import (
	"context"
	"errors"
	"fmt"
)

// ScriptedLLM is a fake LLM that replays fixed steps in order.
// When the steps run out it keeps repeating the last one.
// Used by tests and the demo command; no network or API key needed.
type ScriptedLLM struct {
	Steps []Step
	next  int
}

func (s *ScriptedLLM) Next(_ context.Context, _ []Message) (Step, error) {
	if len(s.Steps) == 0 {
		return Step{}, errors.New("scripted llm: no steps")
	}
	i := s.next
	if i >= len(s.Steps) {
		i = len(s.Steps) - 1
	}
	s.next++

	step := s.Steps[i]
	if step.ToolCall != nil {
		// Copy so each call gets its own ID, like a real model.
		call := *step.ToolCall
		call.ID = fmt.Sprintf("call_%d", s.next)
		step.ToolCall = &call
	}
	return step, nil
}

// Call builds a tool-call step. Args are key, value pairs.
func Call(name string, kv ...string) Step {
	args := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		args[kv[i]] = kv[i+1]
	}
	return Step{ToolCall: &ToolCall{Name: name, Args: args}}
}

// Answer builds a final-answer step.
func Answer(text string) Step {
	return Step{Text: text}
}
