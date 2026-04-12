package zig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestPrepareCompiler_FailedDownloadLeavesNoCorruptDir verifies that if
// DownloadAndExtractArchive fails after writing some files, prepareCompiler
// does not leave the target dir in place. The original code wrote directly
// into the final dir, so a mid-download failure would satisfy the `os.Stat`
// check on the next call and return a corrupt compiler path.
func TestPrepareCompiler_FailedDownloadLeavesNoCorruptDir(t *testing.T) {
	prev := downloadAndExtract
	t.Cleanup(func() { downloadAndExtract = prev })

	origBase := compilersHostDir
	compilersHostDir = t.TempDir()
	t.Cleanup(func() { compilersHostDir = origBase })

	downloadAndExtract = func(ctx context.Context, url, dir string) error {
		// Simulate a partially-extracted archive in dir, then fail.
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "partial_file"), []byte("x"), 0644); err != nil {
			return err
		}
		return errors.New("network died mid-download")
	}

	branch := &Branch{AMD64: &Download{Tarball: "http://example/zig.tar"}}
	_, err := prepareCompiler(context.Background(), "0.13.0", branch)
	if err == nil {
		t.Fatal("expected error from failing downloadAndExtract")
	}

	finalDir := filepath.Join(compilersHostDir, "zig", "0.13.0")
	if _, err := os.Stat(finalDir); !os.IsNotExist(err) {
		t.Fatalf("finalDir should not exist after failed download: err=%v", err)
	}

	// And no orphan .tmp dir either.
	tmpDir := finalDir + ".tmp"
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("tmpDir should be cleaned up after failure: err=%v", err)
	}
}

// TestPrepareCompiler_SuccessAtomicallyPublishes verifies that on success the
// final dir contains the extracted content.
func TestPrepareCompiler_SuccessAtomicallyPublishes(t *testing.T) {
	prev := downloadAndExtract
	t.Cleanup(func() { downloadAndExtract = prev })

	origBase := compilersHostDir
	compilersHostDir = t.TempDir()
	t.Cleanup(func() { compilersHostDir = origBase })

	downloadAndExtract = func(ctx context.Context, url, dir string) error {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "zig"), []byte("fake-binary"), 0755)
	}

	branch := &Branch{AMD64: &Download{Tarball: "http://example/zig.tar"}}
	out, err := prepareCompiler(context.Background(), "0.13.0", branch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(compilersHostDir, "zig", "0.13.0")
	if out != expected {
		t.Fatalf("expected dir %q, got %q", expected, out)
	}
	if _, err := os.Stat(filepath.Join(out, "zig")); err != nil {
		t.Fatalf("expected zig binary inside final dir: %v", err)
	}
}
