package cpp_test

import (
	"embed"
	_ "embed"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Highload-fun/builders"
	"github.com/Highload-fun/builders/cpp"
)

//go:embed test/*
var testSrcFs embed.FS

func TestCpp(t *testing.T) {
	subFs, err := fs.Sub(testSrcFs, "test/src")
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	builders.Check(t, cpp.BuilderId, subFs)
	builders.CheckBuilding(t, "with clang++18.1.3", cpp.BuilderId, "clang++18.1.3", []string{}, subFs)
}
