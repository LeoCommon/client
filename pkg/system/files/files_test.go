package files

import (
	"archive/zip"
	"path/filepath"
	"testing"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestZipArchiveBasic(t *testing.T) {
	log.Init(true)

	tempPath := t.TempDir()

	files := []string{
		filepath.Join(tempPath, "test.txt"),
		filepath.Join(tempPath, "subdir/test2.txt"),
	}

	for _, v := range files {
		f, err := CreateFileP(v, 0750)
		if err != nil {
			log.Error("Could not create file", zap.Error(err))
			return
		}

		_ = f.Close()
	}

	zipFilePath := filepath.Join(tempPath, "myzip.zip")
	assert.NoError(t, WriteFilesInArchive(zipFilePath, files, tempPath))
	assert.FileExists(t, zipFilePath)

	// Make the paths relative
	for k, v := range files {
		relPath, err := GetRelPathFromAbs(v, tempPath)
		assert.NoError(t, err)
		files[k] = relPath
	}

	// Read the zip file contents
	zf, err := zip.OpenReader(zipFilePath)
	assert.NoError(t, err)

	// Check if all the files in the zip exist
	count := 0
	for _, file := range zf.File {
		count++
		assert.Contains(t, files, file.Name)
	}

	// Check if all files files we wanted to pack were packed
	assert.Equal(t, count, len(files))
	assert.NoError(t, zf.Close())
}
