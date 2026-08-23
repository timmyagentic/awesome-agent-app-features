package updater

import (
	"encoding/hex"
	"path"
	"strings"
	"testing"
)

func FuzzChecksumManifestParser(f *testing.F) {
	asset := "example-agent.tar.gz"
	f.Add(strings.Repeat("a", 64)+"  "+asset+"\n", asset)
	f.Add("not-a-checksum  example-agent.tar.gz", asset)
	f.Fuzz(func(t *testing.T, manifest, assetName string) {
		checksum, err := parseChecksumManifest(manifest, assetName)
		if err != nil {
			return
		}
		decoded, err := hex.DecodeString(checksum)
		if err != nil || len(decoded) != 32 {
			t.Fatalf("accepted invalid checksum %q", checksum)
		}
	})
}

func FuzzArchiveEntryMatchNeverAcceptsTraversal(f *testing.F) {
	f.Add("bin/example-agent", "example-agent")
	f.Add("../../example-agent", "example-agent")
	f.Add(`..\..\example-agent`, "example-agent")
	f.Fuzz(func(t *testing.T, entry, binary string) {
		if !archiveEntryMatches(entry, binary) {
			return
		}
		normalized := strings.ReplaceAll(entry, "\\", "/")
		cleaned := path.Clean(normalized)
		if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.Base(cleaned) != binary {
			t.Fatalf("accepted unsafe entry %q for %q", entry, binary)
		}
	})
}

func FuzzStableVersionComparison(f *testing.F) {
	f.Add("v1.2.3", "v1.2.2")
	f.Add("v01.2.3", "dev")
	f.Add("v1.2.3", "v1.2.3-beta.1")
	f.Fuzz(func(t *testing.T, candidate, current string) {
		_, _ = IsNewerStable(candidate, current)
	})
}
