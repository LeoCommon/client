package test

import (
	"go/build"
	"path"
	"path/filepath"
)

var (
	TEST_SCRIPT_DIR = filepath.Join(importPathToDir("disco.cs.uni-kl.de/apogee/"), "/scripts/tests/")
)

func GetScriptPath(subfolder string) string {
	return path.Join(TEST_SCRIPT_DIR, subfolder) + string(filepath.Separator)
}

func importPathToDir(importPath string) string {
	p, err := build.Import(importPath, "", build.FindOnly)
	if err != nil {
		panic("could not find directory for import path")
	}
	return p.Dir
}
