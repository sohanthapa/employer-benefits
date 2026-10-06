package agent

import (
	"fmt"
	"sort"
	"strings"
)

// Format renders a response for the terminal: the tools called,
// the ticket if any, and the answer.
func (r Response) Format() string {
	var b strings.Builder
	if len(r.ToolsCalled) == 0 {
		b.WriteString("Tools:  (none)\n")
	}
	for i, c := range r.ToolsCalled {
		label := "       "
		if i == 0 {
			label = "Tools: "
		}
		fmt.Fprintf(&b, "%s %d. %s(%s)\n", label, i+1, c.Name, formatArgs(c.Args))
	}
	if r.TicketID != "" {
		fmt.Fprintf(&b, "Ticket: %s\n", r.TicketID)
	}
	fmt.Fprintf(&b, "Agent:  %s\n", r.Answer)
	return b.String()
}

// formatArgs prints arguments in a stable order.
func formatArgs(args map[string]string) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, args[k]))
	}
	return strings.Join(parts, ", ")
}
