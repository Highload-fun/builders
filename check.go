package builders

import (
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	sandbox "github.com/Highload-fun/libsandbox"
	"github.com/stretchr/testify/assert"
)

// Check is a test helper that validates a builder's version listing and default-version
// build. It first asserts that GetVersions returns a non-empty list of valid versions,
// then delegates to [CheckBuilding] with an empty version to exercise the default.
func Check(t *testing.T, builderId string, sources fs.FS) {
	t.Run("GetVersions", func(t *testing.T) {
		//t.SkipNow()
		versions, err := GetVersions(t.Context(), builderId)
		assert.NoError(t, err)
		assert.NotZero(t, len(versions))
		t.Logf("Found %d versions", len(versions))

		for _, version := range versions {
			assert.NotEmpty(t, version.Id)
			assert.NotZero(t, version.ReleaseDate)
		}
	})

	CheckBuilding(t, "with default version", builderId, "", nil, sources)
}

// CheckBuilding is a test helper that compiles sources with the given builder and version,
// then verifies the resulting binary can execute inside a sandbox.
func CheckBuilding(t *testing.T, description, builderId, version string, flags []string, sources fs.FS) {
	t.Run("Building "+description, func(t *testing.T) {
		tmpDir := t.TempDir()

		srcDir := filepath.Join(tmpDir, "src")
		if !assert.NoError(t, os.MkdirAll(srcDir, 0777)) {
			t.FailNow()
		}
		if !assert.NoError(t, os.CopyFS(srcDir, sources)) {
			t.FailNow()
		}

		outDir := filepath.Join(tmpDir, "out")
		if !assert.NoError(t, os.MkdirAll(outDir, 0777)) {
			t.FailNow()
		}

		if !assert.NoError(t, Build(t.Context(), builderId, version, flags, srcDir, outDir)) {
			t.FailNow()
		}

		if !assert.FileExists(t, filepath.Join(outDir, "main")) {
			t.FailNow()
		}

		sb := sandbox.New(t.TempDir())
		_, err := sb.AddFile(filepath.Join(outDir, "main"), "/test/main", true).
			SetCGroup(fmt.Sprintf("test-%d", rand.Uint())).
			SetCpuSet("1").
			SetMemLimit(256*1024*1024).
			CommandContext(t.Context(), "/test/main").
			Output()
		if !assert.NoError(t, err) {
			t.FailNow()
		}
	})
}
