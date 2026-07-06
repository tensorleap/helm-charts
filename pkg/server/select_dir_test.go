package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirsAndSplit(t *testing.T) {
	tmp := t.TempDir()
	for _, d := range []string{"data", "datasets", "logs"} {
		if err := os.Mkdir(filepath.Join(tmp, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("x"), 0644)

	// trailing sep -> list this dir's children (dirs only, with trailing sep)
	dir, prefix := splitForListing(tmp + string(os.PathSeparator))
	if prefix != "" || dir != filepath.Clean(tmp) {
		t.Fatalf("trailing-sep split: got dir=%q prefix=%q", dir, prefix)
	}
	if got := listDirs(dir, prefix); len(got) != 3 {
		t.Fatalf("list-all: want 3 dirs, got %v", got)
	}

	// partial name -> dir is parent, prefix is base; substring-matches 2 dirs
	dir, prefix = splitForListing(filepath.Join(tmp, "dat"))
	if dir != tmp || prefix != "dat" {
		t.Fatalf("partial split: got dir=%q prefix=%q", dir, prefix)
	}
	got := listDirs(dir, prefix)
	if len(got) != 2 {
		t.Fatalf("prefix match: want 2 dirs, got %v", got)
	}
	for _, g := range got {
		if !strings.HasSuffix(g, string(os.PathSeparator)) {
			t.Fatalf("dir suggestion missing trailing sep: %q", g)
		}
	}

	// no match -> empty (caller then accepts the typed value as a new path)
	if got := listDirs(tmp, "nope"); len(got) != 0 {
		t.Fatalf("no match: want 0, got %v", got)
	}
}
