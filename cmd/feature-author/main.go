package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/timmyagentic/awesome-agent-app-features/internal/featureauthor"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "feature-author:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("usage: feature-author <new|validate|release-check>")
	}
	switch arguments[0] {
	case "validate":
		flags := flag.NewFlagSet("validate", flag.ContinueOnError)
		root := flags.String("root", ".", "repository root")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("validate accepts no positional arguments")
		}
		if err := featureauthor.Validate(*root); err != nil {
			return err
		}
		fmt.Println("Feature catalog valid")
		return nil
	case "new":
		flags := flag.NewFlagSet("new", flag.ContinueOnError)
		root := flags.String("root", ".", "repository root")
		id := flags.String("id", "", "lowercase kebab-case Feature id")
		name := flags.String("name", "", "human-readable Feature name")
		kind := flags.String("kind", string(featureauthor.KindGo), "go or source-subtree")
		runtimeName := flags.String("runtime", "", "runtime label")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("new accepts no positional arguments")
		}
		if err := featureauthor.Scaffold(featureauthor.ScaffoldOptions{Root: *root, ID: *id, Name: *name, Kind: featureauthor.Kind(*kind), Runtime: *runtimeName}); err != nil {
			return err
		}
		fmt.Printf("Feature %s scaffolded\n", *id)
		return nil
	case "release-check":
		flags := flag.NewFlagSet("release-check", flag.ContinueOnError)
		root := flags.String("root", ".", "repository root")
		tag := flags.String("tag", "", "release tag")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || strings.TrimSpace(*tag) == "" {
			return fmt.Errorf("release-check requires --tag")
		}
		currentCommit, err := gitOutput(*root, "rev-parse", *tag+"^{commit}")
		if err != nil {
			return fmt.Errorf("resolve release tag: %w", err)
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
			return err
		}
		fmt.Printf("Feature release metadata valid for %s\n", *tag)
		return nil
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func gitOutput(root string, arguments ...string) (string, error) {
	commandArguments := append([]string{"-C", root}, arguments...)
	output, err := exec.Command("git", commandArguments...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}
