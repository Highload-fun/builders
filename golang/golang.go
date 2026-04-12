// Package golang provides a Go builder that downloads toolchains from go.dev.
//
// Import this package for its side effects to register the builder:
//
//	import _ "github.com/Highload-fun/builders/golang"
package golang

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	sandbox "github.com/Highload-fun/libsandbox"

	"github.com/Highload-fun/builders"
)

// BuilderId is the identifier used to register and look up this builder.
const BuilderId = "go"

// Golang implements [builders.Builder] for the Go programming language.
type Golang struct {
	versions   []builders.Version
	lastUpdate time.Time
	mtx        sync.Mutex
}

var (
	versionRe  = regexp.MustCompile(`(go\d+.\d+(:?.\d+)?)\s*\(\s*released\s*(\d+-\d+-\d+)\)`)
	prepareMtx = sync.Mutex{}
)

func init() {
	builders.Register(BuilderId, &Golang{})
}

// GetVersions scrapes the Go release history page for available versions.
// Results are cached for 1 hour.
func (g *Golang) GetVersions(ctx context.Context) ([]builders.Version, error) {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	if g.lastUpdate.Add(1 * time.Hour).After(time.Now()) {
		return g.versions, nil
	}

	resp, err := builders.HttpGet(ctx, "https://go.dev/doc/devel/release")
	if err != nil {
		return nil, fmt.Errorf("cannot get versions: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read versions: %w", err)
	}

	submatches := versionRe.FindAllSubmatch(body, -1)

	versionsDate := map[string]time.Time{}

	for _, submatch := range submatches {
		date, err := time.Parse("2006-01-02", string(submatch[3]))
		if err != nil {
			return nil, fmt.Errorf("cannot parse release date: %w", err)
		}

		versionsDate[string(submatch[1])] = date
	}

	res := make([]builders.Version, 0, len(versionsDate))
	for v, date := range versionsDate {
		res = append(res, builders.Version{
			Id:          v,
			ReleaseDate: date,
		})
	}

	g.versions = res
	g.lastUpdate = time.Now()

	return res, nil
}

// Build compiles Go source code inside the sandbox. If version is empty, "go1.13.8" is used.
// The toolchain is downloaded on demand and cached in [builders.CompilersHostDir].
func (g *Golang) Build(ctx context.Context, sb *sandbox.Sandbox, version string, flags []string) error {
	if version == "" {
		version = "go1.13.8"
	}

	compilerPath, err := prepareCompiler(ctx, version)
	if err != nil {
		return err
	}

	sb.MountDir(compilerPath, "/compiler")
	for _, d := range []string{
		"/lib",
		"/usr/lib",
		"/usr/libexec",
	} {
		sb.MountDir(d, d)
	}

	for _, f := range []struct{ Source, Target string }{
		{"/usr/bin/gcc-13", "/usr/bin/gcc"},
		{"/usr/bin/x86_64-linux-gnu-ld.bfd", "/usr/bin/ld"},
		{"/usr/bin/stat", "/usr/bin/stat"},
	} {
		sb.AddFile(f.Source, f.Target, true)
	}

	sb.AddEnv("LANG=C")
	sb.AddEnv("PATH=/usr/bin")
	sb.AddEnv("GOPATH=/tmp/.gopath")
	sb.AddEnv("GOCACHE=/tmp/.gocache")

	// Create go.mod if missed
	if _, err := sb.CommandContext(ctx, "/usr/bin/stat", "go.mod").Output(); err != nil {
		cmd := sb.CommandContext(ctx, "/compiler/bin/go", "mod", "init", "main")
		if _, err = cmd.Output(); err != nil {
			return err
		}
	}

	args := buildArgs(flags)

	cmd := sb.CommandContext(ctx, "/compiler/bin/go", args...)
	if _, err = cmd.Output(); err != nil {
		return err
	}

	return nil
}

// buildArgs assembles the argument list for `go build`. The caller-supplied
// flags come first, followed by the fixed output and package path. Note that
// `-linkshared` is not passed: it was previously handed to the linker as an
// ldflag value, which the Go linker does not recognise.
func buildArgs(flags []string) []string {
	args := make([]string, 0, len(flags)+4)
	args = append(args, "build")
	args = append(args, flags...)
	args = append(args, "-o", filepath.Join(builders.OutDir, "main"), "./")
	return args
}

func prepareCompiler(ctx context.Context, version string) (string, error) {
	prepareMtx.Lock()
	defer prepareMtx.Unlock()

	finalDir := filepath.Join(builders.CompilersHostDir, version)
	dir := filepath.Join(finalDir, "go")
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}

	tmpDir, err := os.MkdirTemp(builders.CompilersHostDir, ".download-")
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://golang.org/dl/%s.linux-amd64.tar.gz", version)
	if err := builders.DownloadAndExtractArchive(ctx, url, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}

	if err := os.Rename(tmpDir, finalDir); err != nil {
		os.RemoveAll(tmpDir)
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
		return "", fmt.Errorf("cannot install compiler: %w", err)
	}

	return dir, nil
}
