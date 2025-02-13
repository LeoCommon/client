package file

import (
	"archive/zip"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	assert.NoError(t, CreateArchive(zipFilePath, files, tempPath))
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

func getFileSize(filePath string) (int, error) {
	theFile, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	fileSize := int(theFile.Size())
	return fileSize, nil
}

func TestZipArchiveCompression(t *testing.T) {
	log.Init(true)

	//use the file.go-file for check compression
	tempPath := t.TempDir()
	files := []string{
		filepath.Join(tempPath, "test.txt"),
		filepath.Join(tempPath, "test2.txt"),
	}
	totalSize := 0
	var fileSizes []int
	for _, v := range files {
		f, err := CreateFileP(v, 0750)
		defer func(f *os.File) {
			err := f.Close()
			assert.NoError(t, err)
		}(f)
		assert.NoError(t, err)
		_, err = f.WriteString(strings.Repeat("boring sample string", 1000+rand.Intn(500)))
		fileSize, _ := getFileSize(v)
		fileSizes = append(fileSizes, fileSize)
		totalSize += fileSize
	}

	zipFilePath := filepath.Join(tempPath, "myzip.zip")
	assert.NoError(t, CreateArchive(zipFilePath, files, tempPath))
	assert.FileExists(t, zipFilePath)

	zipFileSize, _ := getFileSize(zipFilePath)
	if zipFileSize >= totalSize {
		assert.Fail(t, "Zip archive did not compress: zipSize: "+strconv.Itoa(zipFileSize)+" vs filesInZipSize"+strconv.Itoa(totalSize))
	}

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
		assert.Contains(t, fileSizes, int(file.UncompressedSize64))
	}

	// Check if all files files we wanted to pack were packed
	assert.Equal(t, count, len(files))
	assert.NoError(t, zf.Close())
}
