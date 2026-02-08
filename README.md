# github.com/Highload-fun/builders

Go library that provides sandboxed compilation for multiple programming languages. Each builder downloads compiler toolchains on demand (or uses system-installed compilers), then compiles user source code inside a Linux sandbox using [libsandbox](https://github.com/Highload-fun/libsandbox) with cgroups, mount namespaces, and network isolation.

```
go get github.com/Highload-fun/builders
```

## Supported Languages

| Package | Builder ID | Version source | Default version |
|---------|------------|----------------|-----------------|
| `cpp/` | `cpp` | System-installed g++/clang++ | `g++13.3.0` |
| `csharp/` | `csharp` | dotnet releases-index.json (>=8.x) | `8.0.0` |
| `golang/` | `go` | go.dev release page | `go1.13.8` |
| `rust/` | `rust` | GitHub RELEASES.md | `rust-1.47.0` |
| `zig/` | `zig` | ziglang.org JSON index | `0.13.0` |


## How It Works

Each builder implements the `Builder` interface:

```go
type Builder interface {
	GetVersions(ctx context.Context) ([]Version, error)
	Build(ctx context.Context, sb *sandbox.Sandbox, version string, flags []string) error
}
```

When `builders.Build` is called, it:

1. Creates a temporary directory for the sandbox root
2. Configures the sandbox with a 5 GB memory limit and network disabled
3. Mounts the source directory at `/src` and output directory at `/out`
4. Delegates to the language-specific builder, which downloads/locates the compiler and runs it inside the sandbox
5. The compiled binary is written to `/out/main`

Compiler toolchains are downloaded once and cached in `/tmp`.

## Adding a New Language

1. Create a subpackage (e.g., `mylang/`)
2. Implement the `Builder` interface
3. Register via `init()`:
   ```go
   func init() {
       builders.Register("mylang", &MyLang{})
   }
   ```
4. Add `test/src/` with a minimal source file that compiles to a binary
5. Write a test file (`mylang_test.go`) using the shared test harness:
   ```go
   package mylang_test

   import (
       "embed"
       "io/fs"
       "testing"

       "github.com/stretchr/testify/assert"

       "github.com/Highload-fun/builders"
       "github.com/Highload-fun/builders/mylang"
   )

   //go:embed test/*
   var testSrcFs embed.FS

   func TestMyLang(t *testing.T) {
       subFs, err := fs.Sub(testSrcFs, "test/src")
       if !assert.NoError(t, err) {
           t.FailNow()
       }

       // Validates GetVersions and builds with the default version
       builders.Check(t, mylang.BuilderId, subFs)

       // Optionally test a specific version
       builders.CheckBuilding(t, "with v1.2.3", mylang.BuilderId, "v1.2.3", []string{}, subFs)
   }
   ```
   `builders.Check` verifies that `GetVersions` returns a non-empty list and that the default version compiles the embedded source and produces a runnable binary. `builders.CheckBuilding` does the same for a specific version and flags.

## Requirements

- Linux with cgroup v2 support
- `gcc-13` (used as linker by Go, Rust, and C++ builders)
- Root or appropriate sandbox permissions
- Network access (for downloading compiler toolchains on first use)

## Testing

```bash
# Run all tests
go test ./...

# Run tests for a specific builder
go test ./golang/ -run TestGolang
go test ./rust/ -run TestRust
go test ./zig/ -run TestZig
go test ./csharp/ -run TestCSharp
go test ./cpp/ -run TestCpp
```

Tests download real compiler toolchains and execute sandboxed builds, so they require network access and can be slow on first run. Toolchains are cached in `/tmp` for subsequent runs.
