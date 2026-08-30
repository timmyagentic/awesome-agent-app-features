package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/timmyagentic/awesome-agent-app-features/internal/lockcheck"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		usage()
		return
	}
	if len(os.Args) < 2 || os.Args[1] != "validate" {
		usage()
		os.Exit(2)
	}
	flags := flag.NewFlagSet("feature-lock validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	lockPath := flags.String("lock", "agent-app-features.lock.json", "path to the host lock")
	hostRoot := flags.String("host", ".", "target project root")
	sourceRoot := flags.String("source", "", "temporary exact-commit foundation source root")
	sourceCommit := flags.String("source-commit", "", "resolved 40-character foundation commit SHA")
	if err := flags.Parse(os.Args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		os.Exit(2)
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "unexpected argument %q\n", flags.Arg(0))
		os.Exit(2)
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
		fmt.Fprintln(os.Stderr, "feature lock validation failed:", err)
		os.Exit(1)
	}
	fmt.Printf("feature lock valid: %d feature(s), %d file(s), %d Go module(s), %d source subtree(s)\n",
		report.Features, report.Files, report.GoModules, report.SourceSubtrees)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: feature-lock validate --source <exact-commit-source> --source-commit <sha> [--host <project>] [--lock <file>]")
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
