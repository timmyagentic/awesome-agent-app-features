// Command updater-example demonstrates safe wiring. It checks only by default;
// applying an update requires both -apply and an explicit terminal confirmation.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/timmyagentic/awesome-agent-app-features/updater"
	updatergithub "github.com/timmyagentic/awesome-agent-app-features/updater/github"
)

func main() {
	repository := flag.String("repo", "", "required GitHub owner/repository")
	current := flag.String("current", "v1.0.0", "current product version")
	executable := flag.String("executable", "", "installed executable path; defaults to this example executable")
	apply := flag.Bool("apply", false, "install the update after an explicit confirmation")
	flag.Parse()
	if strings.TrimSpace(*repository) == "" {
		fatal(fmt.Errorf("-repo owner/repository is required; run the offline updater-demo example for a zero-configuration transaction"))
	}

	service, err := updater.New(updater.Config{
		Product:        "example-agent",
		CurrentVersion: *current,
		ExecutablePath: *executable,
		BinaryName:     "example-agent",
		AssetName:      updater.ReleaseArchiveName("example-agent"),
		Source: updatergithub.Source{
			Repository: *repository,
		},
		Verifier: updater.ExactVersionLine("example-agent"),
		Progress: func(event updater.Event) {
			fmt.Printf("[%s] %s %s\n", event.Stage, event.TargetVersion, event.Asset)
		},
	})
	if err != nil {
		fatal(err)
	}
	plan, err := service.Prepare(context.Background())
	if err != nil {
		fatal(err)
	}
	release := plan.Release()
	if !plan.Available() {
		fmt.Printf("Already current at %s.\n", release.Tag)
		return
	}
	fmt.Printf("Stable update available: %s (%s)\n", release.Tag, release.URL)
	if !*apply {
		fmt.Println("Check only. Pass -apply to enter the install confirmation flow.")
		return
	}
	if strings.TrimSpace(*executable) == "" {
		fatal(fmt.Errorf("-executable is required with -apply; do not replace the go-run helper"))
	}
	fmt.Print("Replace the configured executable with this stable release? Type update: ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fatal(err)
	}
	if strings.TrimSpace(strings.ToLower(answer)) != "update" {
		fmt.Println("Not updated.")
		return
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Updated to %s. Host restart policy runs next.\n", result.Release.Tag)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
