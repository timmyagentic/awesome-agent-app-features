package feedback

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultErrorMaxAge is the default freshness window for optional errors.
	DefaultErrorMaxAge = 30 * time.Minute
	// MaxDescriptionBytes is the Feedback v1 description limit.
	MaxDescriptionBytes = 4000
	// MaxErrorBytes is the Feedback v1 recent-error text limit.
	MaxErrorBytes = 4000
	// MaxMetadataBytes is the Feedback v1 limit for each environment field.
	MaxMetadataBytes = 160
	// MaxCapabilityGapBytes is the Feedback v1 limit for one capability gap.
	MaxCapabilityGapBytes = 160
	// MaxCapabilityGaps is the Feedback v1 number-of-gaps limit.
	MaxCapabilityGaps = 20
)

// Builder turns host context into a deterministic, redacted structured Draft.
// Its zero value is ready to use.
type Builder struct {
	Now              func() time.Time
	AdditionalRedact func(string) string
	ErrorMaxAge      time.Duration
}

// Build creates one provider-neutral report shape regardless of whether the
// triggering signal was user prose, a recent error, or an unsupported
// capability.
func (b Builder) Build(input Input) (Draft, error) {
	now := b.Now
	if now == nil {
		now = time.Now
	}
	redact := func(value string) string {
		value = Redact(value)
		if b.AdditionalRedact != nil {
			value = b.AdditionalRedact(value)
		}
		return Redact(value)
	}
	errorMaxAge := b.ErrorMaxAge
	if errorMaxAge <= 0 {
		errorMaxAge = DefaultErrorMaxAge
	}

	environment := input.Environment
	environment.Product = boundedLine(redact(environment.Product), MaxMetadataBytes)
	if environment.Product == "" {
		return Draft{}, fmt.Errorf("feedback product is required")
	}
	environment.Version = boundedLine(redact(environment.Version), MaxMetadataBytes)
	environment.Agent = boundedLine(redact(environment.Agent), MaxMetadataBytes)
	if strings.TrimSpace(environment.OS) == "" {
		environment.OS = runtime.GOOS
	}
	if strings.TrimSpace(environment.Arch) == "" {
		environment.Arch = runtime.GOARCH
	}
	environment.OS = boundedLine(redact(environment.OS), MaxMetadataBytes)
	environment.Arch = boundedLine(redact(environment.Arch), MaxMetadataBytes)

	description := truncateUTF8(strings.TrimSpace(cleanText(redact(input.Description))), MaxDescriptionBytes, "\n\n[truncated]")
	var recentError *RecentError
	if input.RecentError != nil && strings.TrimSpace(input.RecentError.Text) != "" {
		age := now().Sub(input.RecentError.At)
		if !input.RecentError.At.IsZero() && age >= 0 && age <= errorMaxAge {
			text := truncateUTF8(strings.TrimSpace(cleanText(redact(input.RecentError.Text))), MaxErrorBytes, "\n[truncated]")
			if text != "" {
				recentError = &RecentError{
					Text: text,
					At:   input.RecentError.At.UTC(),
				}
			}
		}
	}
	gaps := cleanGaps(input.CapabilityGaps, redact, MaxCapabilityGaps, MaxCapabilityGapBytes)
	if description == "" && recentError == nil && len(gaps) == 0 {
		return Draft{}, ErrNothingToReport
	}

	return Draft{report: Report{
		Environment:    environment,
		Description:    description,
		RecentError:    recentError,
		CapabilityGaps: gaps,
	}}, nil
}

func cleanGaps(values []string, redact func(string) string, limit, maxLength int) []string {
	seen := make(map[string]struct{})
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = truncateUTF8(boundedLine(redact(value), maxLength), maxLength, "…")
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

func boundedLine(value string, maxBytes int) string {
	return truncateUTF8(cleanLine(value), maxBytes, "…")
}

func cleanLine(value string) string {
	value = strings.ToValidUTF8(value, "�")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}

func cleanText(value string) string {
	value = strings.ToValidUTF8(value, "�")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
			return -1
		}
		return r
	}, value)
}
