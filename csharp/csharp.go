// Package csharp provides a C# builder that downloads .NET SDK toolchains from Microsoft.
//
// Only .NET 8.x and above are supported. The builder uses native AOT compilation
// (dotnet publish) to produce a self-contained binary.
//
// Import this package for its side effects to register the builder:
//
//	import _ "github.com/Highload-fun/builders/csharp"
package csharp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sandbox "github.com/Highload-fun/libsandbox"

	"github.com/Highload-fun/builders"
)

// BuilderId is the identifier used to register and look up this builder.
const BuilderId = "csharp"

// CSharp implements [builders.Builder] for the C# programming language via the .NET SDK.
type CSharp struct {
	versions   []builders.Version
	lastUpdate time.Time
	mtx        sync.Mutex
}

var (
	prepareMtx = sync.Mutex{}
)

func init() {
	builders.Register(BuilderId, &CSharp{})
}

// GetVersions fetches available .NET SDK versions from the dotnet releases-index.json API.
// Only channels with major version >= 8 are included. Results are cached for 1 hour.
func (c *CSharp) GetVersions(ctx context.Context) ([]builders.Version, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.lastUpdate.Add(1 * time.Hour).After(time.Now()) {
		return c.versions, nil
	}

	resp, err := builders.HttpGet(ctx, "https://dotnetcli.blob.core.windows.net/dotnet/release-metadata/releases-index.json")
	if err != nil {
		return nil, fmt.Errorf("cannot get versions: %w", err)
	}
	defer resp.Body.Close()

	versionsDate := map[string]time.Time{}

	var versions struct {
		ReleasesIndex []struct {
			ChannelVersion string `json:"channel-version"`
			ReleasesJson   string `json:"releases.json"`
		} `json:"releases-index"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, err
	}

	for _, version := range versions.ReleasesIndex {
		major, _ := strconv.ParseInt(strings.SplitN(version.ChannelVersion, ".", 2)[0], 10, 16)
		if major < 8 {
			continue
		}

		resp, err := builders.HttpGet(ctx, version.ReleasesJson)
		if err != nil {
			return nil, fmt.Errorf("cannot get versions: %w", err)
		}
		defer resp.Body.Close()

		var versionData struct {
			Releases []struct {
				ReleaseDate    string `json:"release-date"`
				ReleaseVersion string `json:"release-version"`
			}
		}
		if err := json.NewDecoder(resp.Body).Decode(&versionData); err != nil {
			return nil, err
		}

		for _, release := range versionData.Releases {
			date, err := time.Parse("2006-01-02", release.ReleaseDate)
			if err != nil {
				return nil, fmt.Errorf("cannot parse release date: %w", err)
			}

			versionsDate[release.ReleaseVersion] = date
		}
	}

	res := make([]builders.Version, 0, len(versionsDate))
	for v, date := range versionsDate {
		res = append(res, builders.Version{
			Id:          v,
			ReleaseDate: date,
		})
	}

	c.versions = res
	c.lastUpdate = time.Now()

	return res, nil
}

// Build compiles C# source code inside the sandbox using "dotnet publish" with native AOT.
// If version is empty, "8.0.0" is used. The .NET SDK is downloaded on demand and cached
// in [builders.CompilersHostDir].
func (c *CSharp) Build(ctx context.Context, sb *sandbox.Sandbox, version string, flags []string) error {
	if version == "" {
		version = "8.0.0"
	}

	compilerDir, err := prepareCompiler(ctx, version)
	if err != nil {
		return err
	}

	sb.MountDir(compilerDir, "/compiler")
	for _, d := range []string{
		"/etc/ssl/certs",
		"/lib",
		"/usr/lib",
		"/usr/libexec",
	} {
		sb.MountDir(d, d)
	}
	sb.AddFile("/bin/sh", "/bin/sh", true)
	sb.AddFile("/etc/resolv.conf", "/etc/resolv.conf", false)
	sb.AddFile("/usr/bin/chmod", "/usr/bin/chmod", true)
	sb.AddFile("/usr/bin/cp", "/usr/bin/cp", true)
	sb.AddFile("/usr/bin/ln", "/usr/bin/ln", true)
	sb.AddFile("/usr/bin/gcc-13", "/usr/bin/gcc", true)
	sb.AddFile("/usr/bin/x86_64-linux-gnu-ld.bfd", "/usr/bin/ld.bfd", true)
	sb.AddFile("/usr/bin/objcopy", "/usr/bin/objcopy", true)

	f, err := os.CreateTemp("", "")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(06444); err != nil {
		return err
	}
	if _, err := f.WriteString("builder:x:65534:65534:Builder:/home/builder:/bin/nologin\n"); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	sb.AddFile(f.Name(), "/etc/passwd", false)

	sb.AddEnv("LANG=C")
	sb.AddEnv("PATH=/bin:/usr/bin")
	sb.AddEnv("HOME=/tmp/dotnet" + version)
	sb.AddEnv("DOTNET_CLI_HOME=/tmp/dotnet" + version)

	sb.ExecDir(builders.OutDir)

	// Generate a project
	if _, err := sb.CommandContext(ctx, "/compiler/dotnet", "new", "console", "-n", "project", "--aot").Output(); err != nil {
		return err
	}

	// Copy sources into the generated project
	if _, err := sb.CommandContext(ctx, "/usr/bin/cp", "-r", "/src/.", "/out/project/").Output(); err != nil {
		return err
	}

	// Build the project
	args := []string{"publish"}
	args = append(args, flags...)
	args = append(args, "--self-contained", "-o", "output")

	sb.ExecDir(filepath.Join(builders.OutDir, "project"))

	if _, err := sb.CommandContext(ctx, "/compiler/dotnet", args...).Output(); err != nil {
		return err
	}

	if _, err := sb.CommandContext(ctx, "/usr/bin/ln", filepath.Join(builders.OutDir, "project/output/project"), filepath.Join(builders.OutDir, "main")).Output(); err != nil {
		return err
	}

	// To enable cleanup from host without sudo
	if _, err := sb.CommandContext(ctx, "/usr/bin/chmod", "-R", "a+rwx", "/out/project").Output(); err != nil {
		return err
	}

	return nil
}

func prepareCompiler(ctx context.Context, version string) (string, error) {
	prepareMtx.Lock()
	defer prepareMtx.Unlock()

	dir := filepath.Join(builders.CompilersHostDir, "dotnet", version)
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}

	runtimeLink, err := getReleaseLinks(ctx, version)
	if err != nil {
		return "", err
	}

	if err := builders.DownloadAndExtractArchive(ctx, runtimeLink, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func getReleasesJsonUrl(ctx context.Context, version string) (string, error) {
	resp, err := builders.HttpGet(ctx, "https://dotnetcli.blob.core.windows.net/dotnet/release-metadata/releases-index.json")
	if err != nil {
		return "", fmt.Errorf("cannot get versions: %w", err)
	}
	defer resp.Body.Close()

	var versions struct {
		ReleasesIndex []struct {
			ReleasesJson   string `json:"releases.json"`
			ChannelVersion string `json:"channel-version"`
		} `json:"releases-index"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return "", err
	}

	parts := strings.Split(version, ".")
	channelVersion := strings.Join(parts[:2], ".")

	for _, release := range versions.ReleasesIndex {
		if channelVersion == release.ChannelVersion {
			return release.ReleasesJson, nil
		}
	}

	return "", fmt.Errorf("invalid version")
}

func getReleaseLinks(ctx context.Context, version string) (string, error) {
	url, err := getReleasesJsonUrl(ctx, version)
	if err != nil {
		return "", err
	}

	resp, err := builders.HttpGet(ctx, url)
	if err != nil {
		return "", fmt.Errorf("cannot get versions: %w", err)
	}
	defer resp.Body.Close()

	var metadata struct {
		Releases []struct {
			ReleaseVersion string `json:"release-version"`
			Runtime        struct {
				Files []struct {
					Name string `json:"name"`
					Rid  string `json:"rid"`
					Url  string `json:"url"`
				} `json:"files"`
			} `json:"runtime"`
			Sdk struct {
				Files []struct {
					Name string `json:"name"`
					Rid  string `json:"rid"`
					Url  string `json:"url"`
				} `json:"files"`
			} `json:"sdk"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return "", err
	}

	for _, release := range metadata.Releases {
		if release.ReleaseVersion == version {
			for _, file := range release.Sdk.Files {
				if file.Rid == "linux-x64" {
					return file.Url, nil
				}
			}
		}
	}

	return "", fmt.Errorf("invalid version")
}

func writeFile(filename string, mode int64, source io.Reader) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := f.Chmod(os.FileMode(mode)); err != nil {
		return err
	}

	if _, err := io.Copy(f, source); err != nil {
		return err
	}

	return nil
}
