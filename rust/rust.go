// Package rust provides a Rust builder that downloads rustc toolchains from static.rust-lang.org.
//
// Import this package for its side effects to register the builder:
//
//	import _ "github.com/Highload-fun/builders/rust"
package rust

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
const BuilderId = "rust"

// Rust implements [builders.Builder] for the Rust programming language.
type Rust struct {
	versions   []builders.Version
	lastUpdate time.Time
	mtx        sync.Mutex
}

var (
	versionRe  = regexp.MustCompile(`Version\s+(\d+\.\d+\.\d+)\s+\((\d+-\d+-\d+)\)`)
	prepareMtx = sync.Mutex{}
)

func init() {
	builders.Register(BuilderId, &Rust{})
}

// GetVersions parses the Rust RELEASES.md on GitHub for available versions.
// Results are cached for 1 hour.
func (r *Rust) GetVersions(ctx context.Context) ([]builders.Version, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if r.lastUpdate.Add(1 * time.Hour).After(time.Now()) {
		return r.versions, nil
	}

	resp, err := builders.HttpGet(ctx, "https://raw.githubusercontent.com/rust-lang/rust/master/RELEASES.md")
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
		date, err := time.Parse("2006-01-02", string(submatch[2]))
		if err != nil {
			return nil, fmt.Errorf("cannot parse release date: %w", err)
		}

		versionsDate[string(submatch[1])] = date
	}

	res := make([]builders.Version, 0, len(versionsDate))
	for v, date := range versionsDate {
		res = append(res, builders.Version{
			Id:          "rust-" + v,
			ReleaseDate: date,
		})
	}

	r.versions = res
	r.lastUpdate = time.Now()

	return res, nil
}

// Build compiles Rust source code (main.rs) inside the sandbox using rustc.
// If version is empty, "rust-1.47.0" is used. The toolchain is downloaded on
// demand and cached in [builders.CompilersHostDir].
func (r *Rust) Build(ctx context.Context, sb *sandbox.Sandbox, version string, flags []string) error {
	if version == "" {
		version = "rust-1.47.0"
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
		{"/usr/bin/gcc-13", "/usr/bin/cc"},
		{"/usr/bin/x86_64-linux-gnu-ld.bfd", "/usr/bin/ld"},
	} {
		sb.AddFile(f.Source, f.Target, true)
	}

	sb.AddEnv("LANG=C")
	sb.AddEnv("PATH=/usr/bin")
	sb.AddEnv("RUSTC_BOOTSTRAP=1")

	args := append(flags, "-L", "/compiler/rustc/lib",
		"-L", "/compiler/rust-std-x86_64-unknown-linux-gnu/lib/rustlib/x86_64-unknown-linux-gnu/lib",
		"-o", filepath.Join(builders.OutDir, "main"), "main.rs")

	if _, err := sb.CommandContext(ctx, "/compiler/rustc/bin/rustc", args...).Output(); err != nil {
		return err
	}

	return nil
}

func prepareCompiler(ctx context.Context, version string) (string, error) {
	prepareMtx.Lock()
	defer prepareMtx.Unlock()

	finalDir := filepath.Join(builders.CompilersHostDir, version)
	dir := filepath.Join(finalDir, version+"-x86_64-unknown-linux-gnu")
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}

	tmpDir, err := os.MkdirTemp(builders.CompilersHostDir, ".download-")
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://static.rust-lang.org/dist/%s-x86_64-unknown-linux-gnu.tar.gz", version)
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
