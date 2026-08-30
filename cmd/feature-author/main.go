package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/timmyagentic/awesome-agent-app-features/internal/clioutput"
	"github.com/timmyagentic/awesome-agent-app-features/internal/featureauthor"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		return authorArgumentFailure(false, stdout, stderr, "expected a command")
	}
	switch arguments[0] {
	case "validate":
		flags := flag.NewFlagSet("validate", flag.ContinueOnError)
		jsonMode := hasArgument(arguments[1:], "--json")
		setFlagOutput(flags, jsonMode, stderr)
		root := flags.String("root", ".", "repository root")
		jsonOutput := flags.Bool("json", false, "emit the stable Agent-readable JSON result")
		if err := flags.Parse(arguments[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return authorArgumentFailure(jsonMode, stdout, stderr, err.Error())
		}
		if flags.NArg() != 0 {
			return authorArgumentFailure(*jsonOutput, stdout, stderr, "validate accepts no positional arguments")
		}
		if err := featureauthor.Validate(*root); err != nil {
			return authorFailure(*jsonOutput, stdout, stderr, clioutput.New(false, "feature-author validate", "feature_catalog_invalid", "The Feature catalog or generated documentation is invalid.", err.Error(), "Fix the reported manifest or run feature-author sync-docs when the generated README catalog drifted.", "feature-author validate --json"))
		}
		if *jsonOutput {
			return authorSuccess(stdout, stderr, clioutput.New(true, "feature-author validate", "feature_catalog_valid", "The Feature catalog and generated documentation are valid.", "Every registered manifest, delivery path, example, and README catalog block matches the repository source of truth.", "No remediation is required.", "make verify"))
		}
		fmt.Fprintln(stdout, "Feature catalog valid")
		return 0
	case "new":
		flags := flag.NewFlagSet("new", flag.ContinueOnError)
		flags.SetOutput(stderr)
		root := flags.String("root", ".", "repository root")
		id := flags.String("id", "", "lowercase kebab-case Feature id")
		name := flags.String("name", "", "human-readable Feature name")
		kind := flags.String("kind", string(featureauthor.KindGo), "go or source-subtree")
		runtimeName := flags.String("runtime", "", "runtime label")
		if err := flags.Parse(arguments[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			fmt.Fprintln(stderr, "feature-author:", err)
			return 2
		}
		if flags.NArg() != 0 {
			fmt.Fprintln(stderr, "feature-author: new accepts no positional arguments")
			return 2
		}
		if err := featureauthor.Scaffold(featureauthor.ScaffoldOptions{Root: *root, ID: *id, Name: *name, Kind: featureauthor.Kind(*kind), Runtime: *runtimeName}); err != nil {
			fmt.Fprintln(stderr, "feature-author:", err)
			return 1
		}
		fmt.Fprintf(stdout, "Feature %s scaffolded\n", *id)
		return 0
	case "sync-docs":
		flags := flag.NewFlagSet("sync-docs", flag.ContinueOnError)
		flags.SetOutput(stderr)
		root := flags.String("root", ".", "repository root")
		if err := flags.Parse(arguments[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			fmt.Fprintln(stderr, "feature-author:", err)
			return 2
		}
		if flags.NArg() != 0 {
			fmt.Fprintln(stderr, "feature-author: sync-docs accepts no positional arguments")
			return 2
		}
		if err := featureauthor.SyncReadmes(*root); err != nil {
			fmt.Fprintln(stderr, "feature-author:", err)
			return 1
		}
		fmt.Fprintln(stdout, "Generated README Feature catalogs synchronized")
		return 0
	case "release-check":
		flags := flag.NewFlagSet("release-check", flag.ContinueOnError)
		jsonMode := hasArgument(arguments[1:], "--json")
		setFlagOutput(flags, jsonMode, stderr)
		root := flags.String("root", ".", "repository root")
		tag := flags.String("tag", "", "release tag")
		jsonOutput := flags.Bool("json", false, "emit the stable Agent-readable JSON result")
		if err := flags.Parse(arguments[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return authorArgumentFailure(jsonMode, stdout, stderr, err.Error())
		}
		if flags.NArg() != 0 || strings.TrimSpace(*tag) == "" {
			return authorArgumentFailure(*jsonOutput, stdout, stderr, "release-check requires --tag")
		}
		currentCommit, err := gitOutput(*root, "rev-parse", *tag+"^{commit}")
		if err != nil {
			return authorFailure(*jsonOutput, stdout, stderr, clioutput.New(false, "feature-author release-check", "feature_release_invalid", "The Feature release metadata is invalid.", fmt.Sprintf("resolve release tag: %v", err), "Create or select the exact annotated release tag and rerun the release check.", "feature-author release-check --json --tag "+*tag))
		}
		resolver := func(candidate, relativePath string) (featureauthor.TagResolution, error) {
			candidateCommit, err := gitOutput(*root, "rev-parse", "--verify", "refs/tags/"+candidate+"^{commit}")
			if err != nil {
				return featureauthor.TagResolution{}, nil
			}
			command := exec.Command("git", "-C", *root, "merge-base", "--is-ancestor", candidateCommit, currentCommit)
			if err := command.Run(); err != nil {
				if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
					return featureauthor.TagResolution{Exists: true}, nil
				}
				return featureauthor.TagResolution{Exists: true}, err
			}
			file, err := gitOutput(*root, "show", "refs/tags/"+candidate+":"+relativePath)
			if err != nil {
				return featureauthor.TagResolution{Exists: true, AncestorOfRelease: true}, nil
			}
			return featureauthor.TagResolution{Exists: true, AncestorOfRelease: true, File: []byte(file)}, nil
		}
		if err := featureauthor.ValidateRelease(*root, *tag, resolver); err != nil {
			return authorFailure(*jsonOutput, stdout, stderr, clioutput.New(false, "feature-author release-check", "feature_release_invalid", "The Feature release metadata is invalid.", err.Error(), "Correct release_status, since history, manifests, or generated documentation before publishing.", "feature-author release-check --json --tag "+*tag))
		}
		if *jsonOutput {
			return authorSuccess(stdout, stderr, clioutput.New(true, "feature-author release-check", "feature_release_valid", "The Feature release metadata is valid.", "Published and unreleased Features have consistent introduction history at the requested tag.", "No remediation is required.", "make verify"))
		}
		fmt.Fprintf(stdout, "Feature release metadata valid for %s\n", *tag)
		return 0
	default:
		return authorArgumentFailure(hasArgument(arguments[1:], "--json"), stdout, stderr, fmt.Sprintf("unknown command %q", arguments[0]))
	}
}

func setFlagOutput(flags *flag.FlagSet, jsonMode bool, stderr io.Writer) {
	if jsonMode {
		flags.SetOutput(io.Discard)
	} else {
		flags.SetOutput(stderr)
	}
}

func authorSuccess(stdout, stderr io.Writer, result clioutput.Result) int {
	if err := clioutput.Write(stdout, result); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func authorFailure(jsonMode bool, stdout, stderr io.Writer, result clioutput.Result) int {
	if jsonMode {
		_ = clioutput.Write(stdout, result)
	} else {
		fmt.Fprintln(stderr, "feature-author:", result.Why)
	}
	return 1
}

func authorArgumentFailure(jsonMode bool, stdout, stderr io.Writer, why string) int {
	if jsonMode {
		result := clioutput.New(false, "feature-author", "invalid_arguments", "The Feature author command arguments are invalid.", why, "Use new, sync-docs, validate, or release-check with the documented flags.", "feature-author --help")
		_ = clioutput.Write(stdout, result)
	} else {
		fmt.Fprintln(stderr, "feature-author:", why)
		fmt.Fprintln(stderr, "usage: feature-author <new|sync-docs|validate|release-check>")
	}
	return 2
}

func hasArgument(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted {
			return true
		}
	}
	return false
}

func gitOutput(root string, arguments ...string) (string, error) {
	commandArguments := append([]string{"-C", root}, arguments...)
	output, err := exec.Command("git", commandArguments...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}
