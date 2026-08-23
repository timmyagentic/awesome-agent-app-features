package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func extractBinary(archivePath, archiveName, binaryName, targetDirectory string, maximum int64) (string, error) {
	switch {
	case strings.HasSuffix(archiveName, ".zip"):
		return extractZipBinary(archivePath, binaryName, targetDirectory, maximum)
	case strings.HasSuffix(archiveName, ".tar.gz"), strings.HasSuffix(archiveName, ".tgz"):
		return extractTarGzBinary(archivePath, binaryName, targetDirectory, maximum)
	default:
		return "", fmt.Errorf("unsupported archive format %s", archiveName)
	}
}

func extractTarGzBinary(archivePath, binaryName, targetDirectory string, maximum int64) (string, error) {
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return "", fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	var staged string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			removeIfPresent(staged)
			return "", fmt.Errorf("read tar archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || !archiveEntryMatches(header.Name, binaryName) {
			continue
		}
		if staged != "" {
			removeIfPresent(staged)
			return "", fmt.Errorf("archive contains more than one %s binary", binaryName)
		}
		if header.Size < 0 || header.Size > maximum {
			return "", fmt.Errorf("release binary exceeds %d bytes", maximum)
		}
		staged, err = writeStagedBinary(targetDirectory, binaryName, io.LimitReader(reader, maximum+1), maximum)
		if err != nil {
			return "", err
		}
	}
	if staged == "" {
		return "", fmt.Errorf("archive does not contain %s", binaryName)
	}
	return staged, nil
}

func extractZipBinary(archivePath, binaryName, targetDirectory string, maximum int64) (string, error) {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("open zip archive: %w", err)
	}
	defer archive.Close()
	var staged string
	for _, entry := range archive.File {
		if !entry.Mode().IsRegular() || !archiveEntryMatches(entry.Name, binaryName) {
			continue
		}
		if staged != "" {
			removeIfPresent(staged)
			return "", fmt.Errorf("archive contains more than one %s binary", binaryName)
		}
		if entry.UncompressedSize64 > uint64(maximum) {
			return "", fmt.Errorf("release binary exceeds %d bytes", maximum)
		}
		source, err := entry.Open()
		if err != nil {
			return "", err
		}
		staged, err = writeStagedBinary(targetDirectory, binaryName, source, maximum)
		closeErr := source.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			removeIfPresent(staged)
			return "", fmt.Errorf("close zip entry: %w", closeErr)
		}
	}
	if staged == "" {
		return "", fmt.Errorf("archive does not contain %s", binaryName)
	}
	return staged, nil
}

func archiveEntryMatches(name, binaryName string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	cleaned := path.Clean(normalized)
	if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	return path.Base(cleaned) == binaryName
}

func writeStagedBinary(directory, binaryName string, source io.Reader, maximum int64) (string, error) {
	file, err := os.CreateTemp(directory, "."+filepath.Base(binaryName)+".staged-*")
	if err != nil {
		return "", err
	}
	staged := file.Name()
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(staged)
		}
	}()
	written, err := io.Copy(file, io.LimitReader(source, maximum+1))
	if err != nil {
		return "", fmt.Errorf("write staged binary: %w", err)
	}
	if written > maximum {
		return "", fmt.Errorf("release binary exceeds %d bytes", maximum)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync staged binary: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close staged binary: %w", err)
	}
	remove = false
	return staged, nil
}

func removeIfPresent(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
