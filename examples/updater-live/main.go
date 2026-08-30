// Command updater-live validates a public GitHub Release against a temporary
// executable. It never touches an installed product. By default it prepares
// and prints the exact plan; -apply-temporary executes that plan in the temp
// directory after checksum and staged/installed version verification.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/timmyagentic/awesome-agent-app-features/updater"
	updatergithub "github.com/timmyagentic/awesome-agent-app-features/updater/github"
)

var safeProductPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func main() {
	repository := flag.String("repo", "", "required GitHub owner/repository")
	product := flag.String("product", "", "required release and version-output product name")
	current := flag.String("current", "", "required currently installed version")
	archiveBinary := flag.String("archive-binary", "plain", "archive entry naming: plain or release-qualified")
	applyTemporary := flag.Bool("apply-temporary", false, "apply the exact plan only to a temporary fake executable")
	flag.Parse()

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		fatal(fmt.Errorf("the standalone updater live probe supports macOS and Linux"))
	}
	if strings.TrimSpace(*repository) == "" || !safeProductPattern.MatchString(*product) || strings.TrimSpace(*current) == "" {
		fatal(fmt.Errorf("-repo, a safe -product, and -current are required"))
	}
	if *archiveBinary != "plain" && *archiveBinary != "release-qualified" {
		fatal(fmt.Errorf("-archive-binary must be plain or release-qualified"))
	}

	directory, err := os.MkdirTemp("", "agent-app-features-updater-live-")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(directory)
	target := filepath.Join(directory, *product)
	if err := os.WriteFile(target, versionScript(*product, *current), 0o755); err != nil {
		fatal(err)
	}

	config := updater.Config{
		Product:        *product,
		CurrentVersion: *current,
		ExecutablePath: target,
		AssetName:      updater.ReleaseArchiveName(*product),
		Source:         updatergithub.Source{Repository: *repository},
		Verifier:       updater.ExactVersionLine(*product),
	}
	if *archiveBinary == "plain" {
		config.BinaryName = *product
	} else {
		config.ArchiveBinaryName = func(tag, goos, goarch string) string {
			return fmt.Sprintf("%s-%s-%s-%s", *product, tag, goos, goarch)
		}
	}
	service, err := updater.New(config)
	if err != nil {
		fatal(err)
	}
	plan, err := service.Prepare(context.Background())
	if err != nil {
		fatal(err)
	}
	release := plan.Release()
	if !plan.Available() {
		fmt.Printf("Up to date at %s.\n", release.Tag)
		return
	}
	fmt.Printf("Exact temporary plan: %s (%s), release notes: %d bytes\n",
		release.Tag, plan.ArchiveAsset().Name, len(release.Notes))
	if !*applyTemporary {
		fmt.Println("Prepared only. Pass -apply-temporary to exercise the exact transaction in a temp directory.")
		return
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil {
		fatal(err)
	}
	if !result.Updated {
		fatal(fmt.Errorf("temporary transaction did not update"))
	}
	fmt.Printf("Temporary public Release transaction complete: %s -> %s via %s; installed products were not touched.\n",
		*current, result.Release.Tag, result.ArchiveAsset)
}

func versionScript(product, version string) []byte {
	return []byte("#!/bin/sh\nprintf '%s\\n' '" + product + " " + version + "'\n")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
