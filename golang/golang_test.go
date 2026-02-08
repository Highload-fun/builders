package golang_test

import (
	"embed"
	_ "embed"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Highload-fun/builders"
	"github.com/Highload-fun/builders/golang"
)

//go:embed test/*
var testSrcFs embed.FS

func TestGolang(t *testing.T) {
	subFs, err := fs.Sub(testSrcFs, "test/src")
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	builders.Check(t, golang.BuilderId, subFs)
	builders.CheckBuilding(t, "with go1.25.1", golang.BuilderId, "go1.25.1", []string{}, subFs)
}
