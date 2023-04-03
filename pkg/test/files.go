package test

import (
	"path"
	"path/filepath"
	"runtime"
)

var (
	// Hacky way to get the base path of the module
	_, b, _, _ = runtime.Caller(0)
	basepath   = filepath.Join(filepath.Dir(b), "../..")

	ScriptDir = filepath.Join(basepath, "/scripts/tests/")
	ConfigDir = filepath.Join(basepath, "/config/")
)

func GetScriptPath(subfolder string) string {
	return path.Join(ScriptDir, subfolder) + string(filepath.Separator)
}
