package test

import (
	"go/build"
	"path"
	"path/filepath"
)

var (
	ScriptDir = filepath.Join(importPathToDir("disco.cs.uni-kl.de/apogee/"), "/scripts/tests/")
	ConfigDir = filepath.Join(importPathToDir("disco.cs.uni-kl.de/apogee/"), "/config/")
)

func GetScriptPath(subfolder string) string {
	return path.Join(ScriptDir, subfolder) + string(filepath.Separator)
}

func importPathToDir(importPath string) string {
	p, err := build.Import(importPath, "", build.FindOnly)
	if err != nil {
		panic("could not find directory for import path")
	}
	return p.Dir
}
