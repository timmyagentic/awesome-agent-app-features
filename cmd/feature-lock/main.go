package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/timmyagentic/awesome-agent-app-features/internal/clioutput"
	"github.com/timmyagentic/awesome-agent-app-features/internal/lockcheck"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	jsonMode := containsArgument(arguments, "--json")
	if len(arguments) == 1 && (arguments[0] == "-h" || arguments[0] == "--help") {
		usage(stderr)
		return 0
	}
	if len(arguments) < 1 || arguments[0] != "validate" {
		return writeArgumentFailure(jsonMode, stdout, stderr, "expected the validate command")
	}
	flags := flag.NewFlagSet("feature-lock validate", flag.ContinueOnError)
	if jsonMode {
		flags.SetOutput(io.Discard)
	} else {
		flags.SetOutput(stderr)
	}
	lockPath := flags.String("lock", "agent-app-features.lock.json", "path to the host lock")
	hostRoot := flags.String("host", ".", "target project root")
	sourceRoot := flags.String("source", "", "temporary exact-commit foundation source root")
	sourceCommit := flags.String("source-commit", "", "resolved 40-character foundation commit SHA")
	jsonOutput := flags.Bool("json", false, "emit the stable Agent-readable JSON result")
	if err := flags.Parse(arguments[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return writeArgumentFailure(jsonMode, stdout, stderr, err.Error())
	}
	if flags.NArg() != 0 {
		return writeArgumentFailure(*jsonOutput, stdout, stderr, fmt.Sprintf("unexpected argument %q", flags.Arg(0)))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := lockcheck.Validate(ctx, lockcheck.Options{
		LockPath:      *lockPath,
		HostRoot:      *hostRoot,
		SourceRoot:    *sourceRoot,
		SourceCommit:  *sourceCommit,
		ResolveModule: resolveModule,
		Now:           time.Now,
	})
	if err != nil {
		if *jsonOutput {
			result := clioutput.New(false, "feature-lock validate", "feature_lock_invalid", "The host Feature lock is invalid.", err.Error(), "Fix the reported source, delivery, or host-file mismatch and regenerate the lock only from current verified truth.", "feature-lock validate --json")
			_ = clioutput.Write(stdout, result)
		} else {
			fmt.Fprintln(stderr, "feature lock validation failed:", err)
		}
		return 1
	}
	if *jsonOutput {
		result := clioutput.New(true, "feature-lock validate", "feature_lock_valid", "The host Feature lock is valid.", "The exact source, declared deliveries, Go modules, source subtrees, and host files are consistent.", "No remediation is required.", "")
		result.Data = map[string]int{"features": report.Features, "files": report.Files, "go_modules": report.GoModules, "source_subtrees": report.SourceSubtrees}
		if err := clioutput.Write(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		fmt.Fprintf(stdout, "feature lock valid: %d feature(s), %d file(s), %d Go module(s), %d source subtree(s)\n",
			report.Features, report.Files, report.GoModules, report.SourceSubtrees)
	}
	return 0
}

func writeArgumentFailure(jsonMode bool, stdout, stderr io.Writer, why string) int {
	if jsonMode {
		result := clioutput.New(false, "feature-lock validate", "invalid_arguments", "The Feature lock command arguments are invalid.", why, "Use the validate command with an exact source root and 40-character source commit.", "feature-lock validate --help")
		_ = clioutput.Write(stdout, result)
	} else {
		fmt.Fprintln(stderr, why)
		usage(stderr)
	}
	return 2
}

func containsArgument(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted {
			return true
		}
	}
	return false
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage: feature-lock validate --source <exact-commit-source> --source-commit <sha> [--host <project>] [--lock <file>] [--json]")
}

func resolveModule(ctx context.Context, hostRoot, modulePath string) (lockcheck.ModuleInfo, error) {
	command := exec.CommandContext(ctx, "go", "list", "-mod=mod", "-m", "-json", modulePath)
	command.Dir = hostRoot
	command.Env = environmentWith(command.Environ(), "GOWORK", "off")
	output, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			detail := strings.TrimSpace(string(exitErr.Stderr))
			if detail == "" {
				detail = exitErr.Error()
			}
			return lockcheck.ModuleInfo{}, fmt.Errorf("go list: %s", detail)
		}
		return lockcheck.ModuleInfo{}, err
	}
	var value struct {
		Version string
		Dir     string
		Replace *struct {
			Path string
			Dir  string
		}
	}
	if err := json.Unmarshal(output, &value); err != nil {
		return lockcheck.ModuleInfo{}, fmt.Errorf("decode go list output: %w", err)
	}
	if value.Version == "" || value.Dir == "" {
		return lockcheck.ModuleInfo{}, fmt.Errorf("module %s is not resolved in the host graph", modulePath)
	}
	return lockcheck.ModuleInfo{Version: value.Version, Directory: value.Dir, Replaced: value.Replace != nil}, nil
}

func environmentWith(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, prefix) {
			result = append(result, item)
		}
	}
	return append(result, prefix+value)
}
