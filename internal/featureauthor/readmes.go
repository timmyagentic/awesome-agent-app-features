package featureauthor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	catalogStart = "<!-- generated-feature-catalog:start -->"
	catalogEnd   = "<!-- generated-feature-catalog:end -->"
)

type catalogLanguage string

const (
	languageChinese catalogLanguage = "zh"
	languageEnglish catalogLanguage = "en"
)

type readmeTarget struct {
	Path     string
	Language catalogLanguage
}

var readmeTargets = []readmeTarget{
	{Path: "README.md", Language: languageChinese},
	{Path: "README.en.md", Language: languageEnglish},
}

// SyncReadmes updates only the generated catalog block in each public README.
func SyncReadmes(root string) error {
	root, err := normalizedRoot(root)
	if err != nil {
		return err
	}
	for _, target := range readmeTargets {
		path := filepath.Join(root, target.Path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		block, err := renderReadmeCatalog(root, target.Language)
		if err != nil {
			return err
		}
		updated, err := replaceGeneratedBlock(string(content), block)
		if err != nil {
			return fmt.Errorf("%s: %w", target.Path, err)
		}
		if updated != string(content) {
			if err := writeText(path, updated); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateReadmeCatalogs(root string) error {
	for _, target := range readmeTargets {
		content, err := os.ReadFile(filepath.Join(root, target.Path))
		if err != nil {
			return err
		}
		actual, err := generatedBlock(string(content))
		if err != nil {
			return fmt.Errorf("%s: %w", target.Path, err)
		}
		expected, err := renderReadmeCatalog(root, target.Language)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("%s generated Feature catalog drifted; run feature-author sync-docs", target.Path)
		}
	}
	return nil
}

func renderReadmeCatalog(root string, language catalogLanguage) (string, error) {
	var entry entryDocument
	if err := readJSON(filepath.Join(root, "features", "index.json"), &entry); err != nil {
		return "", err
	}
	var lines []string
	if language == languageChinese {
		lines = []string{
			catalogStart,
			"## Feature Catalog（由 manifest 生成）",
			"",
			"下表由 `features/index.json` 及同一 revision 的 Feature manifests 生成；请运行 `go run ./cmd/feature-author sync-docs --root .` 更新，`validate` 会拒绝漂移。",
			"",
			"| Feature | 状态 | 首发版本 | Delivery | Quick Start |",
			"| --- | --- | --- | --- | --- |",
		}
	} else {
		lines = []string{
			catalogStart,
			"## Feature catalog (generated from manifests)",
			"",
			"This table is generated from `features/index.json` and the Feature manifests at the same revision. Run `go run ./cmd/feature-author sync-docs --root .` to update it; `validate` rejects drift.",
			"",
			"| Feature | Status | Since | Delivery | Quick start |",
			"| --- | --- | --- | --- | --- |",
		}
	}
	for _, feature := range entry.Features {
		var manifest manifestDocument
		if err := readJSON(filepath.Join(root, filepath.FromSlash(feature.Manifest)), &manifest); err != nil {
			return "", err
		}
		modes := make([]string, 0, len(manifest.Delivery))
		seenModes := make(map[string]struct{}, len(manifest.Delivery))
		for _, delivery := range manifest.Delivery {
			if _, seen := seenModes[delivery.Mode]; seen {
				continue
			}
			seenModes[delivery.Mode] = struct{}{}
			modes = append(modes, delivery.Mode)
		}
		sort.Strings(modes)
		since := "—"
		if manifest.Since != nil {
			since = *manifest.Since
		}
		status := manifest.ReleaseStatus
		if manifest.Maturity != "" {
			status += " / " + manifest.Maturity
		}
		lines = append(lines, fmt.Sprintf("| [%s](%s) | %s | %s | %s | %s |",
			markdownCell(manifest.Name), feature.Readme, markdownCell(status), markdownCell(since), markdownCell(strings.Join(modes, " + ")), quickStart(manifest)))
	}
	lines = append(lines, catalogEnd)
	return strings.Join(lines, "\n"), nil
}

func quickStart(manifest manifestDocument) string {
	for _, example := range manifest.RemoteExamples {
		if example.Mode == "go-run" && example.Package != "" {
			return "`GOWORK=off go run " + markdownCell(example.Package) + "@<resolved-commit-sha>`"
		}
	}
	for _, delivery := range manifest.Delivery {
		if delivery.Mode == "source-subtree" && delivery.Path != "" && delivery.Verify != "" {
			return "extract `" + markdownCell(delivery.Path) + "`, then run `./" + markdownCell(delivery.Verify) + "`"
		}
	}
	return "read the pinned manifest"
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "\n", " ")
}

func generatedBlock(content string) (string, error) {
	start := strings.Index(content, catalogStart)
	end := strings.Index(content, catalogEnd)
	if start < 0 || end < start {
		return "", fmt.Errorf("generated Feature catalog markers are missing or invalid")
	}
	end += len(catalogEnd)
	return content[start:end], nil
}

func replaceGeneratedBlock(content, block string) (string, error) {
	actual, err := generatedBlock(content)
	if err != nil {
		return "", err
	}
	return strings.Replace(content, actual, block, 1), nil
}

func writeText(path, content string) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".feature-docs-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
