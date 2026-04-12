package rust

import (
	"testing"
)

// TestRustcArgs_DoesNotMutateCallerFlags locks in the fix for the subtle bug
// where `args := append(flags, ...)` would write into the caller's backing
// array whenever flags had spare capacity - silently corrupting data the
// caller might later use.
func TestRustcArgs_DoesNotMutateCallerFlags(t *testing.T) {
	flags := make([]string, 2, 16)
	flags[0] = "-C"
	flags[1] = "opt-level=3"

	// Mark the capacity tail with known sentinels so we can detect overwrites.
	full := flags[:cap(flags)]
	for i := 2; i < cap(flags); i++ {
		full[i] = "SENTINEL"
	}

	_ = rustcArgs(flags)

	for i := 2; i < cap(flags); i++ {
		if full[i] != "SENTINEL" {
			t.Fatalf("caller's backing array mutated at index %d: got %q", i, full[i])
		}
	}
}

// TestRustcArgs_PreservesCallerFlags ensures user flags are preserved at the
// head of the argument list.
func TestRustcArgs_PreservesCallerFlags(t *testing.T) {
	in := []string{"-C", "opt-level=3", "-C", "target-cpu=native"}
	args := rustcArgs(in)
	for i, v := range in {
		if args[i] != v {
			t.Fatalf("arg %d: expected %q, got %q", i, v, args[i])
		}
	}
}

// TestRustcArgs_IncludesOutputAndMain ensures the output and main.rs arguments
// come through unchanged.
func TestRustcArgs_IncludesOutputAndMain(t *testing.T) {
	args := rustcArgs(nil)
	var hasO, hasMain bool
	for i, a := range args {
		if a == "-o" && i+1 < len(args) {
			hasO = true
		}
		if a == "main.rs" {
			hasMain = true
		}
	}
	if !hasO {
		t.Errorf("expected -o in args, got %v", args)
	}
	if !hasMain {
		t.Errorf("expected main.rs in args, got %v", args)
	}
}
