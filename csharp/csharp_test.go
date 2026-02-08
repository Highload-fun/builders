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
}
