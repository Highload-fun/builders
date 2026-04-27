// Package cpp provides a C++ builder that uses system-installed g++ and clang++ compilers.
//
// Unlike other builders that download compiler toolchains, this package discovers
// compilers already present on the host (e.g. /usr/bin/g++-13, /usr/bin/clang++-18)
// and mounts them into the sandbox for compilation.
//
// Version IDs follow the format "g++<semver>" or "clang++<semver>", for example
// "g++13.3.0" or "clang++18.1.3". The default version is "g++13.3.0".
//
// Import this package for its side effects to register the builder:
//
//	import _ "github.com/Highload-fun/builders/cpp"
package cpp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	sandbox "github.com/Highload-fun/libsandbox"

	"github.com/Highload-fun/builders"
)

// BuilderId is the identifier used to register and look up this builder.
const BuilderId = "cpp"

// Cpp implements the builders.Builder interface for C++ using system-installed
// g++ and clang++ compilers.
type Cpp struct {
	versions   []builders.Version
	lastUpdate time.Time
	mtx        sync.Mutex
}

var (
	gppVersionRe   = regexp.MustCompile(`(\d+\.\d+\.\d+)`)
	clangVersionRe = regexp.MustCompile(`version\s+(\d+\.\d+\.\d+)`)
)

func init() {
	builders.Register(BuilderId, &Cpp{})
}

// GetVersions discovers available C++ compilers by scanning /usr/bin for g++-*
// and clang++-* binaries, running --version on each to extract the version string.
// Results are cached for 1 hour.
func (c *Cpp) GetVersions(_ context.Context) ([]builders.Version, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.lastUpdate.Add(1 * time.Hour).After(time.Now()) {
		return c.versions, nil
	}

	var versions []builders.Version

	// Scan for g++ compilers
	matches, _ := filepath.Glob("/usr/bin/g++-*")
	for _, bin := range matches {
		ver, date, err := getGppVersion(bin)
		if err != nil {
			continue
		}
		versions = append(versions, builders.Version{
			Id:          "g++" + ver,
			ReleaseDate: date,
		})
	}

	// Scan for clang++ compilers
	matches, _ = filepath.Glob("/usr/bin/clang++-*")
	for _, bin := range matches {
		ver, date, err := getClangVersion(bin)
		if err != nil {
			continue
		}
		versions = append(versions, builders.Version{
			Id:          "clang++" + ver,
			ReleaseDate: date,
		})
	}

	c.versions = versions
	c.lastUpdate = time.Now()

	return versions, nil
}

func getGppVersion(bin string) (string, time.Time, error) {
	out, err := runVersion(bin)
	if err != nil {
		return "", time.Time{}, err
	}

	m := gppVersionRe.FindString(out)
	if m == "" {
		return "", time.Time{}, fmt.Errorf("cannot parse g++ version from %q", out)
	}

	date, err := binaryMtime(bin)
	if err != nil {
		return "", time.Time{}, err
	}

	return m, date, nil
}

func getClangVersion(bin string) (string, time.Time, error) {
	out, err := runVersion(bin)
	if err != nil {
		return "", time.Time{}, err
	}

	sub := clangVersionRe.FindStringSubmatch(out)
	if len(sub) < 2 {
		return "", time.Time{}, fmt.Errorf("cannot parse clang++ version from %q", out)
	}

	date, err := binaryMtime(bin)
	if err != nil {
		return "", time.Time{}, err
	}

	return sub[1], date, nil
}

func runVersion(bin string) (string, error) {
	cmd := exec.Command(bin, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.SplitN(string(output), "\n", 2)
	return lines[0], nil
}

func binaryMtime(bin string) (time.Time, error) {
	info, err := os.Stat(bin)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// Build compiles main.cpp inside the sandbox using the specified compiler version.
// If version is empty, "g++13.3.0" is used. If flags is empty, "-O2 -std=c++17" is used.
// The compiled binary is written to /out/main.
func (c *Cpp) Build(ctx context.Context, sb *sandbox.Sandbox, version string, flags []string) error {
	if version == "" {
		version = "g++13.3.0"
	}

	compiler, bin, err := resolveCompiler(version)
	if err != nil {
		return err
	}

	for _, d := range []string{
		"/lib",
		"/usr/lib",
		"/usr/libexec",
		"/usr/include",
	} {
		sb.MountDir(d, d)
	}

	sb.AddFile("/usr/bin/x86_64-linux-gnu-ld.bfd", "/usr/bin/ld", true)
	sb.AddFile("/usr/bin/x86_64-linux-gnu-as", "/usr/bin/as", true)

	// Invocation path inside the sandbox. For g++ we hardlink it at
	// /usr/bin/<compiler> because /usr/bin isn't bind-mounted; for clang we
	// use its real path under /usr/lib/llvm-<MAJOR>/bin/clang directly,
	// reachable via the /usr/lib mount. This matters for clang -flto: clang
	// derives the LLVMgold plugin path from its own /proc/self/exe as
	// "<clang_dir>/../lib/LLVMgold.so". When clang is hardlinked at
	// /usr/bin/clang++, that resolves to /usr/lib/LLVMgold.so (missing -
	// the plugin lives at /usr/lib/llvm-<MAJOR>/lib/LLVMgold.so), and the
	// link step fails with:
	//   /usr/bin/ld: /usr/bin/../lib/LLVMgold.so: error loading plugin: ...
	//     cannot open shared object file: No such file or directory
	// Running clang from its real path makes the relative lookup land on
	// the actual plugin file. The hardlink-then-bind-mount order in
	// libsandbox/sandbox prevents the alternative fix (placing the plugin
	// at /usr/lib/LLVMgold.so) - the bind mount over /usr/lib hides any
	// file we put there beforehand.
	invokePath := "/usr/bin/" + compiler
	if compiler == "clang++" {
		invokePath = bin
	} else {
		sb.AddFile(bin, invokePath, true)
	}

	sb.AddEnv("LANG=C")
	sb.AddEnv("PATH=/usr/bin")

	if len(flags) == 0 {
		flags = []string{"-O2", "-std=c++17"}
	}

	args := []string{"main.cpp"}
	args = append(args, flags...)
	args = append(args, "-o", filepath.Join(builders.OutDir, "main"))

	if _, err := sb.CommandContext(ctx, invokePath, args...).Output(); err != nil {
		return err
	}

	return nil
}

// resolveCompiler parses a version string like "g++13.3.0" or "clang++18.1.3"
// and returns the compiler name, the host binary path, and any error.
func resolveCompiler(version string) (compiler, bin string, err error) {
	switch {
	case strings.HasPrefix(version, "g++"):
		ver := strings.TrimPrefix(version, "g++")
		major := strings.SplitN(ver, ".", 2)[0]
		compiler = "g++"
		bin = "/usr/bin/g++-" + major
	case strings.HasPrefix(version, "clang++"):
		ver := strings.TrimPrefix(version, "clang++")
		major := strings.SplitN(ver, ".", 2)[0]
		compiler = "clang++"
		bin = "/usr/bin/clang++-" + major
	default:
		return "", "", fmt.Errorf("unknown C++ compiler version: %q", version)
	}

	real, err := filepath.EvalSymlinks(bin)
	if err != nil {
		return "", "", fmt.Errorf("compiler binary not found: %s", bin)
	}

	return compiler, real, nil
}
