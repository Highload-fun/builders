// Package builders provides sandboxed compilation for multiple programming languages.
//
// Each language is implemented as a separate subpackage that registers itself via
// [Register] during init(). To use a builder, import its package for side effects:
//
//	import _ "github.com/Highload-fun/builders/golang"
//
// Then call [GetVersions] and [Build] with the builder's ID.
package builders

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"time"

	sandbox "github.com/Highload-fun/libsandbox"
)

const (
	// CompilersHostDir is the host directory where downloaded compiler toolchains are cached.
	CompilersHostDir = "/tmp"
	// SourceDir is the path where user source code is mounted inside the sandbox.
	SourceDir = "/src"
	// OutDir is the path where the compiled binary is written inside the sandbox.
	OutDir = "/out"
)

// Builder is the interface that each language compiler must implement.
type Builder interface {
	// GetVersions returns the available compiler versions for this language.
	GetVersions(ctx context.Context) ([]Version, error)
	// Build compiles source code inside the given sandbox, producing a "main" binary in [OutDir].
	Build(ctx context.Context, sb *sandbox.Sandbox, version string, flags []string) error
}

// Version represents a single compiler release.
type Version struct {
	Id          string
	ReleaseDate time.Time
}

var (
	builders     = map[string]Builder{}
	validVersion = regexp.MustCompile(`^[a-zA-Z0-9._+-]*$`)
)

// Register adds a builder under the given id. It panics if the id is already taken.
// Language subpackages call this from their init() functions.
func Register(id string, builder Builder) {
	if _, exists := builders[id]; exists {
		panic(fmt.Sprintf("Builder '%s' is already registered", id))
	}

	builders[id] = builder
}

// GetBuildersIds returns the sorted IDs of all registered builders.
func GetBuildersIds() []string {
	res := make([]string, 0, len(builders))
	for name := range builders {
		res = append(res, name)
	}

	sort.Strings(res)

	return res
}

// GetVersions returns the available compiler versions for the given builder,
// sorted by release date (newest first).
func GetVersions(ctx context.Context, builderId string) ([]Version, error) {
	b, err := getBuilder(builderId)
	if err != nil {
		return nil, err
	}

	versions, err := b.GetVersions(ctx)
	if err != nil {
		return nil, err
	}

	sort.Slice(versions, func(i, j int) bool {
		if versions[i].ReleaseDate.Equal(versions[j].ReleaseDate) {
			return versions[j].Id < versions[i].Id
		}
		return versions[j].ReleaseDate.Before(versions[i].ReleaseDate)
	})

	return versions, nil
}

// Build compiles the source code in srcDir using the specified builder and version,
// writing the resulting binary to outDir. It creates an isolated sandbox with a 5 GB
// memory limit and network disabled. If version is empty, the builder's default is used.
func Build(ctx context.Context, builderId, version string, flags []string, srcDir, outDir string) error {
	if !validVersion.MatchString(version) {
		return fmt.Errorf("invalid version string: %q", version)
	}

	b, err := getBuilder(builderId)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "build")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	sb := sandbox.New(tmpDir).
		SetCGroup(fmt.Sprintf("build-%s-%d", builderId, rand.Uint())).
		SetMemLimit(5*1024*1024*1024).
		SetNoNewNet(true).
		MountDir(srcDir, SourceDir).
		MountDir(outDir, OutDir).
		ExecDir(SourceDir)

	if err := b.Build(ctx, sb, version, flags); err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			return errors.New(string(exitErr.Stderr))
		}
		return err
	}

	return nil
}

func getBuilder(id string) (Builder, error) {
	b := builders[id]
	if b == nil {
		return nil, fmt.Errorf("builder '%s' not found", id)
	}

	return b, nil
}
