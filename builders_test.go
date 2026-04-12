package builders

import (
	"context"
	"strings"
	"testing"
)

// TestBuild_RejectsDotDotVersion ensures that a version of ".." is rejected by the
// validator. Without this guard, `filepath.Join("/tmp", "zig", "..")` resolves to
// "/tmp" and the whole host /tmp directory gets mounted at /compiler inside the
// sandbox - any attacker-writable file there would run as the compiler.
func TestBuild_RejectsDotDotVersion(t *testing.T) {
	err := Build(context.Background(), "go", "..", nil, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatalf("Build accepted version '..' - expected an error")
	}
	if !strings.Contains(err.Error(), "invalid version") {
		t.Fatalf("Build error for '..' should mention invalid version, got %q", err.Error())
	}
}

// TestBuild_RejectsEmbeddedDotDot covers versions where the `..` traversal is
// embedded inside an otherwise valid-looking string. The current regex
// `^[a-zA-Z0-9._+-]*$` allows these because `.` is unrestricted.
func TestBuild_RejectsEmbeddedDotDot(t *testing.T) {
	cases := []string{
		"1..2",
		"..1.2",
		"1.2..",
		"abc..def",
	}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			err := Build(context.Background(), "go", v, nil, t.TempDir(), t.TempDir())
			if err == nil {
				t.Fatalf("Build accepted dangerous version %q - expected an error", v)
			}
			if !strings.Contains(err.Error(), "invalid version") {
				t.Fatalf("Build error for %q should mention invalid version, got %q", v, err.Error())
			}
		})
	}
}

// TestBuild_RejectsObviousInjection ensures slashes are still rejected (existing regex).
func TestBuild_RejectsSlash(t *testing.T) {
	err := Build(context.Background(), "go", "1.2.3/../evil", nil, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatalf("Build accepted slashed version")
	}
	if !strings.Contains(err.Error(), "invalid version") {
		t.Fatalf("expected invalid version error, got %q", err.Error())
	}
}

// TestBuild_UnknownBuilderStillFails is a sanity control: ensure that after the
// dotdot rejection, an unknown-builder error still surfaces normally (rules out
// accidental global short-circuits).
func TestBuild_UnknownBuilderStillFails(t *testing.T) {
	err := Build(context.Background(), "no-such-builder", "1.2.3", nil, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatalf("expected error for unknown builder")
	}
	if strings.Contains(err.Error(), "invalid version") {
		t.Fatalf("unknown-builder error should not mention invalid version, got %q", err.Error())
	}
}
