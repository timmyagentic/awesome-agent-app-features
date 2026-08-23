package feedback

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultErrorMaxAge      = 30 * time.Minute
	defaultMaxDescription   = 4000
	defaultMaxError         = 4000
	defaultMaxBody          = 12000
	defaultMaxTitle         = 200
	defaultMaxCapabilityGap = 160
	defaultMaxGaps          = 20
)

// Builder turns host context into a deterministic, redacted Draft. Its zero
// value is ready to use.
type Builder struct {
	Now               func() time.Time
	Redact            func(string) string
	ErrorMaxAge       time.Duration
	TitlePrefix       string
	MaxDescription    int
	MaxError          int
	MaxBody           int
	MaxTitle          int
	MaxCapabilityGaps int
	MaxCapabilityGap  int
}

// Build creates one report shape regardless of whether the triggering signal
// was user prose, a recent error, or an unsupported capability.
func (b Builder) Build(input Input) (Draft, error) {
	now := b.Now
	if now == nil {
		now = time.Now
	}
	redact := b.Redact
	if redact == nil {
		redact = Redact
	}
	errorMaxAge := b.ErrorMaxAge
	if errorMaxAge <= 0 {
		errorMaxAge = defaultErrorMaxAge
	}
	maxDescription := positiveOr(b.MaxDescription, defaultMaxDescription)
	maxError := positiveOr(b.MaxError, defaultMaxError)
	maxBody := positiveOr(b.MaxBody, defaultMaxBody)
	maxTitle := positiveOr(b.MaxTitle, defaultMaxTitle)
	maxGaps := positiveOr(b.MaxCapabilityGaps, defaultMaxGaps)
	maxGap := positiveOr(b.MaxCapabilityGap, defaultMaxCapabilityGap)
	titlePrefix := b.TitlePrefix
	if titlePrefix == "" {
		titlePrefix = "[feedback] "
	}

	environment := input.Environment
	environment.Product = cleanLine(redact(environment.Product))
	if environment.Product == "" {
		return Draft{}, fmt.Errorf("feedback product is required")
	}
	environment.Version = cleanLine(redact(environment.Version))
	environment.Agent = cleanLine(redact(environment.Agent))
	if strings.TrimSpace(environment.OS) == "" {
		environment.OS = runtime.GOOS
	}
	if strings.TrimSpace(environment.Arch) == "" {
		environment.Arch = runtime.GOARCH
	}
	environment.OS = cleanLine(redact(environment.OS))
	environment.Arch = cleanLine(redact(environment.Arch))

	installID := strings.TrimSpace(input.InstallID)
	if installID != "" && !validInstallID(installID) {
		return Draft{}, fmt.Errorf("installation ID must use 1-64 letters, digits, dots, underscores, or hyphens")
	}

	description := truncateUTF8(strings.TrimSpace(redact(input.Description)), maxDescription, "\n\n_[truncated]_")
	var recentError *RecentError
	if input.RecentError != nil && strings.TrimSpace(input.RecentError.Text) != "" {
		age := now().Sub(input.RecentError.At)
		if !input.RecentError.At.IsZero() && age >= 0 && age <= errorMaxAge {
			recentError = &RecentError{
				Text: truncateUTF8(strings.TrimSpace(redact(input.RecentError.Text)), maxError, "\n[truncated]"),
				At:   input.RecentError.At,
			}
		}
	}
	gaps := cleanGaps(input.CapabilityGaps, redact, maxGaps, maxGap)
	if description == "" && recentError == nil && len(gaps) == 0 {
		return Draft{}, ErrNothingToReport
	}

	titleSeed := description
	if titleSeed == "" && recentError != nil {
		titleSeed = "error: " + firstLine(recentError.Text)
	}
	if titleSeed == "" {
		titleSeed = "unsupported capability: " + strings.Join(gaps, ", ")
	}
	title := truncateUTF8(titlePrefix+cleanLine(firstLine(titleSeed)), maxTitle, "…")

	var body strings.Builder
	if description != "" {
		body.WriteString(description)
	}
	if recentError != nil {
		separate(&body)
		fmt.Fprintf(&body, "**Most recent error** (%s):\n\n%s", recentError.At.UTC().Format(time.RFC3339), indentBlock(recentError.Text))
	}
	if len(gaps) > 0 {
		separate(&body)
		body.WriteString("**Capabilities not available in this build**:\n")
		for _, gap := range gaps {
			body.WriteString("\n- ")
			body.WriteString(markdownText(gap))
		}
	}
	separate(&body)
	body.WriteString("---\n**Environment (allowlist only)**\n\n")
	fmt.Fprintf(&body, "- Product: %s\n", markdownText(environment.Product))
	fmt.Fprintf(&body, "- Version: %s\n", markdownText(valueOrUnknown(environment.Version)))
	fmt.Fprintf(&body, "- OS/Arch: %s/%s\n", markdownText(environment.OS), markdownText(environment.Arch))
	fmt.Fprintf(&body, "- Agent: %s\n", markdownText(valueOrUnknown(environment.Agent)))
	body.WriteString("\n_Reported through an in-product feedback flow after explicit user approval._")

	submission := Submission{
		Schema:    1,
		InstallID: installID,
		Product:   environment.Product,
		Version:   environment.Version,
		OS:        environment.OS,
		Arch:      environment.Arch,
		Agent:     environment.Agent,
		Title:     title,
		Body:      truncateUTF8(body.String(), maxBody, "\n\n_[truncated]_"),
	}
	return Draft{submission: submission}, nil
}

// NewInstallID returns a random, non-identifying installation identifier for
// relay rate limiting and deduplication. Keeping it empty disables this field.
func NewInstallID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate installation ID: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func validInstallID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func cleanGaps(values []string, redact func(string) string, limit, maxLength int) []string {
	seen := make(map[string]struct{})
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = truncateUTF8(cleanLine(redact(value)), maxLength, "…")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	sort.Strings(cleaned)
	if len(cleaned) > limit {
		cleaned = cleaned[:limit]
	}
	return cleaned
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func truncateUTF8(value string, maxBytes int, suffix string) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	if len(suffix) >= maxBytes {
		suffix = ""
	}
	limit := maxBytes - len(suffix)
	value = value[:limit]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value + suffix
}

func firstLine(value string) string {
	if line, _, ok := strings.Cut(value, "\n"); ok {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(value)
}

func cleanLine(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}

func indentBlock(value string) string {
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = "    " + lines[index]
	}
	return strings.Join(lines, "\n")
}

func separate(builder *strings.Builder) {
	if builder.Len() > 0 {
		builder.WriteString("\n\n")
	}
}

func markdownText(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(cleanLine(value))
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
