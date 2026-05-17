package cpp

import (
	"path/filepath"
	"testing"
)

// TestResolveCompilerClangPlusPlusSuffix regresses the link bug where
// resolveCompiler ran filepath.EvalSymlinks on /usr/bin/clang++-<MAJOR>
// and returned the fully-resolved real binary path, which on Debian/Ubuntu
// is /usr/lib/llvm-<MAJOR>/bin/clang (NOT ...clang++). Invoking that path
// makes clang's argv[0] read "clang", which puts the driver in C mode and
// stops it from auto-linking libstdc++ - turning every C++ solution that
// uses std::cout into an undefined-reference link error at challenge time.
//
// The fix is to preserve the clang++ basename after symlink resolution.
func TestResolveCompilerClangPlusPlusSuffix(t *testing.T) {
	compiler, bin, err := resolveCompiler("clang++18.1.3")
	if err != nil {
		t.Skipf("clang++-18 not installed on host, skipping: %v", err)
	}
	if compiler != "clang++" {
		t.Errorf("compiler name = %q, want %q", compiler, "clang++")
	}
	if got := filepath.Base(bin); got != "clang++" {
		t.Errorf("resolved invocation path basename = %q, want %q (full path: %s)", got, "clang++", bin)
	}
}
