// Command feedback-example demonstrates the required preview/approval boundary.
// It previews locally by default and sends only when an endpoint is configured
// and the user types an explicit confirmation.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/timmyagentic/awesome-agent-app-features/feedback"
)

func main() {
	endpoint := flag.String("endpoint", "", "self-hosted relay URL; empty means preview only")
	description := flag.String("description", "Please improve startup diagnostics", "feedback description")
	flag.Parse()

	draft, err := (feedback.Builder{}).Build(feedback.Input{
		Description: *description,
		Environment: feedback.Environment{
			Product: "example-agent",
			Version: "v1.0.0",
			Agent:   "coding-agent",
		},
	})
	if err != nil {
		fatal(err)
	}
	fmt.Println(draft.Preview())
	if strings.TrimSpace(*endpoint) == "" {
		fmt.Println("Preview only: configure -endpoint to exercise submission.")
		return
	}

	fmt.Print("Submit this exact redacted report? Type yes to continue: ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fatal(err)
	}
	if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
		fmt.Println("Not submitted.")
		return
	}
	approved, err := draft.Approve(true)
	if err != nil {
		fatal(err)
	}
	receipt, err := (feedback.Client{Endpoint: *endpoint}).Submit(context.Background(), approved)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Submitted: %s\n", receipt.IssueURL)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
