package builders

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

// HttpGet performs a context-aware HTTP GET request.
func HttpGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return http.DefaultClient.Do(req)
}

// DownloadAndExtractArchive fetches a tar archive from archiveUrl and extracts it
// into dstDir. Supported formats are .tar.gz, .tar.bz2, and .tar.xz.
func DownloadAndExtractArchive(ctx context.Context, archiveUrl, dstDir string) error {
	resp, err := HttpGet(ctx, archiveUrl)
	if err != nil {
		return fmt.Errorf("cannot get archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cannot get archive: HTTP %d", resp.StatusCode)
	}

	r, err := openCompressedReader(resp.Body, archiveUrl)
	if err != nil {
		return fmt.Errorf("cannot open archive: %w", err)
	}
	defer r.Close()

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("mkdir dst: %w", err)
	}

	tr := tar.NewReader(r)

	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if h == nil {
			continue
		}

		name, ok := tarNameToValidFSPath(h.Name)
		if !ok {
			continue
		}

		target := filepath.Join(dstDir, filepath.FromSlash(name))

		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(h.Mode)); err != nil {
				return fmt.Errorf("mkdir %q: %w", target, err)
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("mkdir parent for %q: %w", target, err)
			}
			if err := writeFile(target, h.Mode, tr); err != nil {
				return fmt.Errorf("write %q: %w", target, err)
			}

		default:
			return fmt.Errorf("unsupported tar entry type %v for %q", h.Typeflag, h.Name)
		}
	}
}

// FakeCloser wraps an io.Reader with a no-op Close method to satisfy io.ReadCloser.
type FakeCloser struct {
	io.Reader
}

// Close is a no-op that always returns nil.
func (FakeCloser) Close() error { return nil }

func openCompressedReader(r io.Reader, archiveUrl string) (io.ReadCloser, error) {
	l := strings.ToLower(archiveUrl)

	switch {
	case strings.HasSuffix(l, ".tar.gz"), strings.HasSuffix(l, ".tgz"):
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		return gr, nil

	case strings.HasSuffix(l, ".tar.bz2"), strings.HasSuffix(l, ".tbz2"), strings.HasSuffix(l, ".tbz"):
		return FakeCloser{bzip2.NewReader(r)}, nil

	case strings.HasSuffix(l, ".tar.xz"), strings.HasSuffix(l, ".txz"):
		xr, err := xz.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("xz reader: %w", err)
		}

		return FakeCloser{xr}, nil

	default:
		return nil, fmt.Errorf("unsupported archive extension: %q", archiveUrl)
	}
}

func tarNameToValidFSPath(name string) (string, bool) {
	// Tar spec uses forward slashes, but some archives may contain backslashes.
	// Normalize to forward slashes so fs.ValidPath works consistently.
	name = strings.ReplaceAll(name, "\\", "/")

	// Strip common prefix.
	name = strings.TrimPrefix(name, "./")

	// Strip trailing slashes (directories may be "dir/").
	name = strings.TrimRight(name, "/")

	if name == "" || name == "." {
		return "", false
	}

	if !fs.ValidPath(name) {
		return "", false
	}

	return name, true
}

func writeFile(filename string, mode int64, source io.Reader) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := f.Chmod(os.FileMode(mode)); err != nil {
		return err
	}

	if _, err := io.Copy(f, source); err != nil {
		return err
	}

	return nil
}
