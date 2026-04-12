// Package zig provides a Zig builder that downloads toolchains from ziglang.org.
//
// Import this package for its side effects to register the builder:
//
//	import _ "github.com/Highload-fun/builders/zig"
package zig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	sandbox "github.com/Highload-fun/libsandbox"

	"github.com/Highload-fun/builders"
)

// BuilderId is the identifier used to register and look up this builder.
const BuilderId = "zig"

// Zig implements [builders.Builder] for the Zig programming language.
type Zig struct {
	sync.Mutex `json:"-"`
	branches   map[string]*Branch
	nextUpdate time.Time
}

type (
	// Branch represents a Zig release branch from the ziglang.org JSON index.
	Branch struct {
		sync.Mutex `json:"-"`
		Version    string    `json:"version"`
		Date       string    `json:"date"`
		AMD64      *Download `json:"x86_64-linux"`
	}

	// Download holds the tarball URL and metadata for a platform-specific Zig release.
	Download struct {
		Tarball string `json:"tarball"`
		SHASum  string `json:"shasum"`
		Size    string `json:"size"`
	}
)

func init() {
	builders.Register(BuilderId, &Zig{})
}

// GetVersions fetches available Zig versions from the ziglang.org JSON index.
// The "master" branch is excluded. Results are cached for 1 hour.
func (zig *Zig) GetVersions(ctx context.Context) (versions []builders.Version, err error) {
	zig.Lock()
	defer zig.Unlock()

	now := time.Now()

	if zig.branches == nil || now.After(zig.nextUpdate) {
		zig.branches, err = getBranches(ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot get versions: %w", err)
		}

		zig.nextUpdate = now.Add(time.Hour)
	}

	versions = make([]builders.Version, 0, len(zig.branches))

	for branchName, branch := range zig.branches {
		if branch == nil || branch.AMD64 == nil || branchName == "master" {
			continue
		}

		date, err := time.Parse("2006-01-02", branch.Date)
		if err != nil {
			return nil, fmt.Errorf("cannot parse release date: %w", err)
		}

		versions = append(versions, builders.Version{
			Id:          branchName,
			ReleaseDate: date,
		})
	}

	return versions, nil
}

func getBranches(ctx context.Context) (map[string]*Branch, error) {
	resp, err := builders.HttpGet(ctx, "https://ziglang.org/download/index.json")
	if err != nil {
		return nil, fmt.Errorf("failed to get branches: %w", err)
	}
	defer resp.Body.Close()

	branches := make(map[string]*Branch)

	err = json.NewDecoder(resp.Body).Decode(&branches)
	if err != nil {
		return nil, fmt.Errorf("json decoding error: %w", err)
	}

	return branches, nil
}

// Build compiles Zig source code inside the sandbox using "zig build-exe".
// If version is empty, "0.13.0" is used. The toolchain is downloaded on demand
// and cached in [builders.CompilersHostDir].
func (zig *Zig) Build(ctx context.Context, sb *sandbox.Sandbox, version string, flags []string) error {
	if version == "" {
		version = "0.13.0"
	}

	if _, err := zig.GetVersions(ctx); err != nil {
		return err
	}

	branch, err := zig.peek(version)
	if err != nil {
		return fmt.Errorf("cannot peek branch: %w", err)
	}

	compilerPath, err := prepareCompiler(ctx, version, branch)
	if err != nil {
		return fmt.Errorf("cannot prepare compiler: %w", err)
	}

	// Zig archives use "zig-x86_64-linux-" prefix since 0.14+, and "zig-linux-x86_64-" for older versions.
	binary := ""
	if version == "master" {
		binary = filepath.Join("/compiler", "zig-x86_64-linux-"+branch.Version, "zig")
	} else {
		newStyleDir := filepath.Join(compilerPath, "zig-x86_64-linux-"+version)
		if _, err := os.Stat(newStyleDir); err == nil {
			binary = filepath.Join("/compiler", "zig-x86_64-linux-"+version, "zig")
		} else {
			binary = filepath.Join("/compiler", "zig-linux-x86_64-"+version, "zig")
		}
	}

	sb.MountDir(compilerPath, "/compiler")
	sb.AddFile("/usr/bin/chmod", "/usr/bin/chmod", true)
	sb.AddFile("/usr/bin/cp", "/usr/bin/cp", true)

	sb.AddEnv("ZIG_GLOBAL_CACHE_DIR=" + filepath.Join(builders.OutDir, ".zig-cache-global"))

	args := append([]string{"build-exe"}, flags...)
	args = append(args, "-femit-bin="+filepath.Join(builders.OutDir, "main"), "main.zig")

	if _, err := sb.CommandContext(ctx, binary, args...).Output(); err != nil {
		return err
	}

	// To enable cleanup from host without sudo
	if _, err := sb.CommandContext(ctx, "/usr/bin/chmod", "-R", "a+rwx",
		filepath.Join(builders.OutDir, ".zig-cache-global"),
	).Output(); err != nil {
		return err
	}

	return nil
}

func (zig *Zig) peek(branchName string) (*Branch, error) {
	zig.Lock()
	defer zig.Unlock()

	branch, found := zig.branches[branchName]
	if !found {
		return nil, errors.New("branch not found")
	}

	return branch, nil
}

func prepareCompiler(ctx context.Context, branchName string, branch *Branch) (string, error) {
	branch.Lock()
	defer branch.Unlock()

	dir := filepath.Join(compilersHostDir, "zig", branchName)
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}

	// Download into a staging directory first, then rename atomically.
	// Without this, a mid-download failure leaves a partial/corrupt "dir"
	// which passes the os.Stat check on the next call and wedges the builder.
	tmpDir := dir + ".tmp"
	_ = os.RemoveAll(tmpDir)
	if err := downloadAndExtract(ctx, branch.AMD64.Tarball, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", err
	}
	if err := os.Rename(tmpDir, dir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", err
	}

	return dir, nil
}

// downloadAndExtract is an indirection over builders.DownloadAndExtractArchive
// so tests can substitute a deterministic implementation.
var downloadAndExtract = builders.DownloadAndExtractArchive

// compilersHostDir points at the cache directory for downloaded toolchains.
// Defaults to the package-wide constant; tests override it to an isolated
// directory.
var compilersHostDir = builders.CompilersHostDir
