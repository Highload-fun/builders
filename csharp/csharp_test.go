package csharp_test

import (
	"embed"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Highload-fun/builders"
	"github.com/Highload-fun/builders/csharp"
)

//go:embed test/*
var testSrcFs embed.FS

func TestCSharp(t *testing.T) {
	subFs, err := fs.Sub(testSrcFs, "test/src")
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	builders.Check(t, csharp.BuilderId, subFs)

	md5Fs, err := fs.Sub(testSrcFs, "test/md5")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	builders.CheckBuilding(t, "md5 with System.Security.Cryptography", csharp.BuilderId, "", nil, md5Fs)
}
