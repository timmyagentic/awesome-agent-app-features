// Command updater-demo runs a complete update transaction against temporary
// files and an in-memory release source. It is safe to execute remotely with
// go run package@commit and never touches an installed product.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/timmyagentic/awesome-agent-app-features/updater"
)

const (
	product        = "demo-agent"
	currentVersion = "v1.0.0"
	targetVersion  = "v1.1.0"
)

type memorySource struct {
	release updater.Release
	assets  map[string][]byte
}

func (source memorySource) LatestStable(context.Context) (updater.Release, error) {
	return source.release, nil
}

func (source memorySource) Download(ctx context.Context, asset updater.Asset, destination io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, ok := source.assets[asset.Name]
	if !ok {
		return fmt.Errorf("demo asset %q is unavailable", asset.Name)
	}
	_, err := destination.Write(data)
	return err
}

func main() {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		fatal(fmt.Errorf("the standalone updater demo supports macOS and Linux"))
	}
	directory, err := os.MkdirTemp("", "agent-app-features-updater-demo-")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(directory)

	target := filepath.Join(directory, product)
	if err := os.WriteFile(target, versionScript(currentVersion), 0o755); err != nil {
		fatal(err)
	}
	archiveName := updater.ReleaseArchiveName(product)(targetVersion, runtime.GOOS, runtime.GOARCH)
	archive, err := tarGz(product, versionScript(targetVersion))
	if err != nil {
		fatal(err)
	}
	digest := sha256.Sum256(archive)
	checksums := []byte(fmt.Sprintf("%x  %s\n", digest, archiveName))
	source := memorySource{
		release: updater.Release{
			Tag: targetVersion,
			URL: "https://example.invalid/releases/tag/" + targetVersion,
			Assets: []updater.Asset{
				{Name: archiveName, Size: int64(len(archive))},
				{Name: "checksums.txt", Size: int64(len(checksums))},
			},
		},
		assets: map[string][]byte{
			archiveName:     archive,
			"checksums.txt": checksums,
		},
	}
	service, err := updater.New(updater.Config{
		Product:        product,
		CurrentVersion: currentVersion,
		ExecutablePath: target,
		BinaryName:     product,
		AssetName:      updater.ReleaseArchiveName(product),
		Source:         source,
		Verifier:       updater.ExactVersionLine(product),
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
	if !plan.Available() {
		fatal(fmt.Errorf("demo release was unexpectedly up to date"))
	}
	fmt.Printf("Preview exact plan: %s (%s)\n", plan.Release().Tag, plan.ArchiveAsset().Name)
	result, err := service.Apply(context.Background(), plan)
	if err != nil {
		fatal(err)
	}
	if !result.Updated {
		fatal(fmt.Errorf("demo transaction did not install the update"))
	}
	if err := updater.ExactVersionLine(product).Verify(context.Background(), target, targetVersion); err != nil {
		fatal(fmt.Errorf("verify final demo executable: %w", err))
	}
	fmt.Printf("Temporary transaction complete: %s -> %s; installed products were not touched.\n", currentVersion, result.Release.Tag)
}

func versionScript(version string) []byte {
	return []byte("#!/bin/sh\nprintf '" + product + " " + version + "\\n'\n")
}

func tarGz(name string, body []byte) ([]byte, error) {
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(body)),
	}); err != nil {
		return nil, err
	}
	if _, err := tarWriter.Write(body); err != nil {
		return nil, err
	}
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
