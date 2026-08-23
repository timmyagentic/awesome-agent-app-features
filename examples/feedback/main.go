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
	"github.com/timmyagentic/awesome-agent-app-features/feedback/httpclient"
)

func main() {
	endpoint := flag.String("endpoint", "", "self-hosted /v1/feedback relay URL; empty means preview only")
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
	// This text is only the reference runner's renderer. Product integrations
	// should render the same fields in their own card, CLI, or web UI.
	renderReport(draft.Report())
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
	receipt, err := (httpclient.Client{Endpoint: *endpoint}).Submit(context.Background(), approved)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Submitted: %s\n", receipt.ReferenceURL)
}

func renderReport(report feedback.Report) {
	fmt.Printf("Product: %s\n", report.Environment.Product)
	fmt.Printf("Version: %s\n", report.Environment.Version)
	fmt.Printf("OS/Arch: %s/%s\n", report.Environment.OS, report.Environment.Arch)
	fmt.Printf("Agent: %s\n", report.Environment.Agent)
	fmt.Printf("Install ID: %s\n", report.InstallID)
	fmt.Printf("Description: %s\n", report.Description)
	if report.RecentError != nil {
		fmt.Printf("Recent error (%s): %s\n", report.RecentError.At.Format("2006-01-02T15:04:05Z07:00"), report.RecentError.Text)
	}
	for _, gap := range report.CapabilityGaps {
		fmt.Printf("Capability gap: %s\n", gap)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
