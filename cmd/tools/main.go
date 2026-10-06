// Command tools calls one tool by hand and prints its output as JSON.
// It is a manual check for the tools; the agent does not use it.
//
// Run from the project folder:
//
//	go run ./cmd/tools plan E123
//	go run ./cmd/tools providers 94107 family_counseling
//	go run ./cmd/tools ticket "no provider found near 94107"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/sohanthapa/employer-benefits/tools"
)

const usage = `usage:
  tools plan <employee_id>
  tools providers <zip_code> <specialty>
  tools ticket <reason>`

func main() {
	dataPath := flag.String("data", "docs/mock_data.json", "path to mock data file")
	flag.Parse()
	args := flag.Args()

	store, err := tools.Load(*dataPath)
	if err != nil {
		fail(err)
	}

	var out interface{}
	switch {
	case len(args) == 2 && args[0] == "plan":
		out, err = store.GetPlanDetails(args[1])
	case len(args) == 3 && args[0] == "providers":
		out, err = store.SearchProviders(args[1], args[2])
	case len(args) == 2 && args[0] == "ticket":
		out, err = store.CreateSupportTicket(args[1])
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fail(err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
