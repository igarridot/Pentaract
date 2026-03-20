package localfs

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceListDirFiltersAndSortsEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "b-folder"), 0o755); err != nil {
		t.Fatalf("mkdir b-folder: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "a-folder"), 0o755); err != nil {
		t.Fatalf("mkdir a-folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "z.txt"), []byte("z"), 0o600); err != nil {
		t.Fatalf("write z.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("aa"), 0o600); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitkeep"), nil, 0o600); err != nil {
		t.Fatalf("write .gitkeep: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "z.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("symlink link.txt: %v", err)
	}

	source := New(root)
	items, err := source.ListDir("")
	if err != nil {
		t.Fatalf("ListDir returned error: %v", err)
	}

	if len(items) != 4 {
		t.Fatalf("expected 4 visible entries, got %d", len(items))
	}
	if items[0].Name != "a-folder" || items[1].Name != "b-folder" {
		t.Fatalf("expected folders first in name order, got %+v", items[:2])
	}
	if items[2].Name != "a.txt" || items[3].Name != "z.txt" {
		t.Fatalf("expected files after folders in name order, got %+v", items[2:])
	}
}

func TestSourceExpandSelectionWalksDirectoriesAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir docs/nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write docs/a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "nested", "b.txt"), []byte("bb"), 0o600); err != nil {
		t.Fatalf("write docs/nested/b.txt: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "docs", "a.txt"), filepath.Join(root, "docs", "nested", "link.txt")); err != nil {
		t.Fatalf("symlink docs/nested/link.txt: %v", err)
	}

	source := New(root)
	files, err := source.ExpandSelection([]string{"docs", "docs/a.txt"})
	if err != nil {
		t.Fatalf("ExpandSelection returned error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 regular files, got %d", len(files))
	}
	if files[0].Path != "docs/a.txt" || files[1].Path != "docs/nested/b.txt" {
		t.Fatalf("unexpected expanded files: %+v", files)
	}
}

func TestSourceOpenFileRejectsTraversalAndDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}

	source := New(root)
	if _, err := source.OpenFile("../a.txt"); err == nil {
		t.Fatalf("expected traversal path to fail")
	}
	if _, err := source.OpenFile("dir"); err == nil {
		t.Fatalf("expected directory path to fail")
	}

	file, err := source.OpenFile("a.txt")
	if err != nil {
		t.Fatalf("OpenFile returned error: %v", err)
	}
	defer file.Reader.Close()

	data, err := io.ReadAll(file.Reader)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(data) != "hello" || file.Name != "a.txt" || file.Size != 5 {
		t.Fatalf("unexpected opened file: %+v data=%q", file, string(data))
	}
}

func TestSourceListDirRejectsSymlinkEscapes(t *testing.T) {
	source := newSourceWithEscapingSymlink(t)
	if _, err := source.ListDir("escape"); err == nil {
		t.Fatalf("expected symlinked directory outside root to fail")
	}
}

func TestSourceExpandSelectionRejectsSymlinkEscapes(t *testing.T) {
	source := newSourceWithEscapingSymlink(t)
	if _, err := source.ExpandSelection([]string{"escape/secret.txt"}); err == nil {
		t.Fatalf("expected symlinked path outside root to fail")
	}
}

func TestSourceOpenFileRejectsSymlinkEscapes(t *testing.T) {
	source := newSourceWithEscapingSymlink(t)
	if _, err := source.OpenFile("escape/secret.txt"); err == nil {
		t.Fatalf("expected symlinked path outside root to fail")
	}
}

func TestSourceRequiresConfiguredRoot(t *testing.T) {
	source := New("")
	if _, err := source.ListDir(""); err == nil {
		t.Fatalf("expected missing root to fail")
	}
}

func newSourceWithEscapingSymlink(t *testing.T) *Source {
	t.Helper()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write secret.txt: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("symlink escape: %v", err)
	}

	return New(root)
}
