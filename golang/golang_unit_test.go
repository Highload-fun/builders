package golang

import (
	"testing"

	"github.com/Highload-fun/builders"
)

// TestBuildArgs_NoLdflagsLinkshared verifies that buildArgs does NOT pass
// `-linkshared` as a value to `-ldflags`. The previous code emitted
// `-ldflags -linkshared`, which made the Go linker receive a literal
// "-linkshared" string as an ldflag argument (not a valid linker flag).
func TestBuildArgs_NoLdflagsLinkshared(t *testing.T) {
	args := buildArgs(nil)

	// First arg must be the build verb.
	if len(args) == 0 || args[0] != "build" {
		t.Fatalf("expected args[0]=build, got %v", args)
	}

	// Regression: the pair "-ldflags <something>-linkshared<something>" must
	// not appear.
	for i, a := range args {
		if a == "-ldflags" {
			// The value immediately following must not be exactly "-linkshared".
			if i+1 < len(args) && args[i+1] == "-linkshared" {
				t.Fatalf("-ldflags -linkshared regression: args=%v", args)
			}
		}
	}

	// The output flag must be followed by a clean path (no trailing "./").
	var sawO bool
	for i, a := range args {
		if a == "-o" {
			sawO = true
			if i+1 >= len(args) {
				t.Fatalf("-o without a value: args=%v", args)
			}
			want := builders.OutDir + "/main"
			if args[i+1] != want {
				t.Fatalf("expected -o %q, got %q", want, args[i+1])
			}
		}
	}
	if !sawO {
		t.Fatalf("expected -o in args, got %v", args)
	}

	// Package arg "./" must be present (otherwise `go build` uses CWD which is
	// fine here but we prefer explicit).
	last := args[len(args)-1]
	if last != "./" {
		t.Fatalf("expected last arg to be package './', got %q", last)
	}
}

// TestBuildArgs_PreservesCallerFlags verifies that user-supplied flags are
// preserved in order between "build" and the fixed trailing args.
func TestBuildArgs_PreservesCallerFlags(t *testing.T) {
	in := []string{"-gcflags=-m", "-race"}
	args := buildArgs(in)

	for _, flag := range in {
		var found bool
		for _, a := range args {
			if a == flag {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("caller flag %q missing from args %v", flag, args)
		}
	}
}

// TestBuildArgs_DoesNotMutateCallerFlags ensures that the caller's slice
// underlying array is not written to - a subtle bug if buildArgs used
// `append(flags, ...)` on a slice with extra capacity.
func TestBuildArgs_DoesNotMutateCallerFlags(t *testing.T) {
	flags := make([]string, 2, 16)
	flags[0] = "-x"
	flags[1] = "-v"

	_ = buildArgs(flags)

	// Peek past len into capacity to see if buildArgs wrote there.
	extended := flags[:cap(flags)]
	for i := 2; i < len(extended); i++ {
		if extended[i] != "" {
			t.Fatalf("caller's backing array was mutated at index %d: %q", i, extended[i])
		}
	}
}
