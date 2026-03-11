package zig_test

import (
	"embed"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Highload-fun/builders"
	"github.com/Highload-fun/builders/zig"
)

//go:embed test/*
var testSrcFs embed.FS

func TestZig(t *testing.T) {
	subFs, err := fs.Sub(testSrcFs, "test/src")
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	builders.Check(t, zig.BuilderId, subFs)
	builders.CheckBuilding(t, "with master branch", zig.BuilderId, "master", nil, subFs)

	md5Fs, err := fs.Sub(testSrcFs, "test/md5")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	builders.CheckBuilding(t, "md5 with std.crypto", zig.BuilderId, "", nil, md5Fs)
}
