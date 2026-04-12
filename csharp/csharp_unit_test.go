package csharp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestFetchChannelReleases_ClosesBody verifies that the per-channel fetch
// closes the HTTP response body exactly once. The original implementation
// deferred close inside a loop, so bodies stayed open until GetVersions
// returned; with many channels this leaked fds.
func TestFetchChannelReleases_ClosesBody(t *testing.T) {
	var opens, closes int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&opens, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"releases":[{"release-date":"2024-01-15","release-version":"8.0.1"}]}`)
	}))
	defer ts.Close()

	// Hook http.DefaultTransport so we can count Close() on the body.
	// Using a RoundTripper wrapper.
	origClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: &countingTransport{wrapped: http.DefaultTransport, closes: &closes}}
	defer func() { http.DefaultClient = origClient }()

	dst := map[string]time.Time{}
	if err := fetchChannelReleases(context.Background(), ts.URL, dst); err != nil {
		t.Fatalf("fetchChannelReleases: %v", err)
	}

	if got, want := atomic.LoadInt32(&opens), int32(1); got != want {
		t.Fatalf("expected %d opens, got %d", want, got)
	}
	if got, want := atomic.LoadInt32(&closes), int32(1); got != want {
		t.Fatalf("expected %d body closes, got %d", want, got)
	}
	if _, ok := dst["8.0.1"]; !ok {
		t.Fatalf("expected version 8.0.1 in result, got %v", dst)
	}
}

type countingTransport struct {
	wrapped http.RoundTripper
	closes  *int32
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := c.wrapped.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &countingCloser{ReadCloser: resp.Body, closes: c.closes}
	return resp, nil
}

type countingCloser struct {
	io.ReadCloser
	closes *int32
}

func (c *countingCloser) Close() error {
	atomic.AddInt32(c.closes, 1)
	return c.ReadCloser.Close()
}

// TestWriteBuilderPasswdFile_Mode0444 verifies that the sandbox /etc/passwd
// stand-in is written with mode 0444 (world-readable). The previous code used
// the surprising 06444 octal (setuid+setgid+0444), which has no functional
// effect on a regular file but is confusing and unexpected.
func TestWriteBuilderPasswdFile_Mode0444(t *testing.T) {
	path, err := writeBuilderPasswdFile()
	if err != nil {
		t.Fatalf("writeBuilderPasswdFile: %v", err)
	}
	defer os.Remove(path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Regression: setuid/setgid bits must be clear.
	mode := info.Mode()
	if mode&os.ModeSetuid != 0 {
		t.Errorf("setuid bit should be clear, got mode %v", mode)
	}
	if mode&os.ModeSetgid != 0 {
		t.Errorf("setgid bit should be clear, got mode %v", mode)
	}
	if perm := mode.Perm(); perm != 0444 {
		t.Errorf("expected perm 0444, got %o", perm)
	}

	// Sanity: file contains the expected entry.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "builder:x:65534:65534") {
		t.Errorf("passwd file missing builder entry, got %q", string(data))
	}
}

// TestParseChannelVersion_Valid confirms the happy path still works after the
// length guard is added.
func TestParseChannelVersion_Valid(t *testing.T) {
	cases := map[string]string{
		"8.0.0":      "8.0",
		"9.0.100":    "9.0",
		"10.0.0-pre": "10.0",
	}
	for in, want := range cases {
		got, err := parseChannelVersion(in)
		if err != nil {
			t.Errorf("parseChannelVersion(%q) returned error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseChannelVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestParseChannelVersion_DotlessVersionReturnsError verifies the regression:
// dotless version strings (e.g. "8", "abc") previously caused `parts[:2]` to
// panic with an index out of range. The caller must instead get a clean error.
func TestParseChannelVersion_DotlessVersionReturnsError(t *testing.T) {
	for _, v := range []string{"", "8", "abc", "v10"} {
		t.Run(v, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("parseChannelVersion(%q) panicked: %v", v, r)
				}
			}()
			_, err := parseChannelVersion(v)
			if err == nil {
				t.Fatalf("parseChannelVersion(%q) returned nil error", v)
			}
			if !strings.Contains(err.Error(), "invalid version") {
				t.Fatalf("error for %q should mention invalid version, got %q", v, err.Error())
			}
		})
	}
}
