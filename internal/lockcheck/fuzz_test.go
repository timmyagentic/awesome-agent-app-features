package lockcheck

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzJoinedPathNeverEscapesRoot(f *testing.F) {
	for _, seed := range []string{
		"go.mod",
		"internal/feedback/flow.go",
		"../outside",
		"../../etc/passwd",
		"/absolute/path",
		"a/../../../outside",
		"",
	} {
		f.Add(seed)
	}
	root := f.TempDir()
	f.Fuzz(func(t *testing.T, relative string) {
		path, err := joinedPath(root, relative)
		if err != nil {
			return
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			t.Fatalf("joinedPath(%q) escaped root as %q", relative, path)
		}
	})
}
