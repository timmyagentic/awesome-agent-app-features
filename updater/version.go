package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	stableTagPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)
	versionPattern   = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(-([0-9A-Za-z.-]+))?$`)
)

// ValidateStableRelease rejects drafts, prereleases, whitespace variants, and
// any tag that is not an exact three-component stable semantic version.
func ValidateStableRelease(release Release) error {
	tag := strings.TrimSpace(release.Tag)
	if release.Tag != tag || release.Draft || release.Prerelease || !stableTagPattern.MatchString(tag) {
		return fmt.Errorf("refusing non-stable release %q", release.Tag)
	}
	return nil
}

// IsNewerStable compares a validated stable candidate with the current
// version. `dev` builds are treated as older than a stable release.
func IsNewerStable(candidate, current string) (bool, error) {
	if !stableTagPattern.MatchString(candidate) {
		return false, fmt.Errorf("candidate %q is not an exact stable version", candidate)
	}
	current = strings.TrimSpace(current)
	if strings.HasPrefix(current, "dev") {
		return true, nil
	}
	candidateVersion, err := parseVersion(candidate)
	if err != nil {
		return false, err
	}
	currentVersion, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("parse current version: %w", err)
	}
	for index := 0; index < 3; index++ {
		if candidateVersion.numbers[index] > currentVersion.numbers[index] {
			return true, nil
		}
		if candidateVersion.numbers[index] < currentVersion.numbers[index] {
			return false, nil
		}
	}
	return currentVersion.prerelease != "", nil
}

type parsedVersion struct {
	numbers    [3]int
	prerelease string
}

func parseVersion(value string) (parsedVersion, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if matches == nil {
		return parsedVersion{}, fmt.Errorf("%q is not a semantic version", value)
	}
	var parsed parsedVersion
	for index := 0; index < 3; index++ {
		number, err := strconv.Atoi(matches[index+1])
		if err != nil {
			return parsedVersion{}, fmt.Errorf("parse version component: %w", err)
		}
		parsed.numbers[index] = number
	}
	parsed.prerelease = matches[5]
	return parsed, nil
}

// CommandVersionVerifier runs an executable and requires its first output line
// to equal ExpectedLine(expectedTag).
type CommandVersionVerifier struct {
	Args         []string
	ExpectedLine func(expectedTag string) string
	Timeout      time.Duration
}

// Verify implements VersionVerifier.
func (verifier CommandVersionVerifier) Verify(ctx context.Context, path, expectedTag string) error {
	if verifier.ExpectedLine == nil {
		return fmt.Errorf("expected version line function is required")
	}
	timeout := verifier.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	verifyContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := verifier.Args
	if len(args) == 0 {
		args = []string{"--version"}
	}
	command := exec.CommandContext(verifyContext, path, args...)
	output := &limitedVersionOutput{maximum: 64 * 1024}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if output.exceeded || errors.Is(err, errVersionOutputTooLarge) {
		return fmt.Errorf("version probe output exceeded %d bytes", output.maximum)
	}
	if err != nil {
		return fmt.Errorf("run version probe: %w", err)
	}
	data := output.bytes()
	firstLine, _, _ := bytes.Cut(bytes.TrimSpace(data), []byte("\n"))
	expected := verifier.ExpectedLine(expectedTag)
	if string(bytes.TrimSpace(firstLine)) != expected {
		return fmt.Errorf("version output %q does not equal %q", string(firstLine), expected)
	}
	return nil
}

var errVersionOutputTooLarge = errors.New("version probe output is too large")

type limitedVersionOutput struct {
	mutex    sync.Mutex
	buffer   bytes.Buffer
	maximum  int
	exceeded bool
}

func (output *limitedVersionOutput) Write(data []byte) (int, error) {
	output.mutex.Lock()
	defer output.mutex.Unlock()
	remaining := output.maximum - output.buffer.Len()
	if remaining <= 0 {
		output.exceeded = true
		return 0, errVersionOutputTooLarge
	}
	if len(data) > remaining {
		written, err := output.buffer.Write(data[:remaining])
		if err != nil {
			return written, err
		}
		output.exceeded = true
		return written, errVersionOutputTooLarge
	}
	return output.buffer.Write(data)
}

func (output *limitedVersionOutput) bytes() []byte {
	output.mutex.Lock()
	defer output.mutex.Unlock()
	return append([]byte(nil), output.buffer.Bytes()...)
}

// ExactVersionLine returns a common verifier for `<product> v1.2.3` output.
func ExactVersionLine(product string) CommandVersionVerifier {
	product = strings.TrimSpace(product)
	return CommandVersionVerifier{
		Args: []string{"--version"},
		ExpectedLine: func(tag string) string {
			if !strings.HasPrefix(tag, "v") {
				tag = "v" + tag
			}
			return product + " " + tag
		},
	}
}
