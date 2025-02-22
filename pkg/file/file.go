package file

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/LeoCommon/client/pkg/log"
	"go.uber.org/zap"
)

// CreateFileP Creates a file and all its directories
// Make sure you close the file when using this function!
func CreateFileP(filePath string, perm fs.FileMode) (*os.File, error) {
	absDirPath, err := filepath.Abs(filepath.Dir(filePath))
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(absDirPath, perm)
	if err != nil {
		return nil, err
	}

	return os.Create(filePath)
}

func WriteTo(filePath string, text string) error {
	f, err := CreateFileP(filePath, 0750)
	if err != nil {
		return err
	}

	// Close the file when done
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	// Write string to file
	_, err = f.WriteString(text)
	return err
}

// GetRelPathFromAbs Get the relative path from an absolute path with a specified base dir
func GetRelPathFromAbs(absPath string, base string) (string, error) {
	if !filepath.IsAbs(absPath) {
		return "", fmt.Errorf("path was not absolute %s", absPath)
	}

	// Check if the path is a prefix
	if !strings.HasPrefix(absPath, base) {
		return "", fmt.Errorf("path %s is not related to base path %s", absPath, base)
	}

	fileDirectory := filepath.Dir(absPath)
	relativeDir, err := filepath.Rel(base, fileDirectory)
	if err != nil {
		return "", err
	}

	// Create the relative path with the given base directory
	return filepath.Join(relativeDir, absPath[len(fileDirectory):]), nil
}

func addFileToZip(absFilePath string, writer *zip.Writer, baseDir string) error {
	// Open the source file for reading
	srcFile, err := os.Open(absFilePath)
	if err != nil {
		return err
	}

	defer func(srcFile *os.File) {
		_ = srcFile.Close()
	}(srcFile)

	fileName := filepath.Base(absFilePath)
	zipFileWriter, err := writer.Create(fileName)
	if err != nil {
		return err
	}

	// Copy the file contents to the zip
	_, err = io.Copy(zipFileWriter, srcFile)
	if err != nil {
		return err
	}

	return nil
}

func verifyZipArchive(archivePath string, addedFiles []string) error {
	zf, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	// go through each file that should be in the zip archive. check if it is there and if it has the right size
	counter := 0
	var validataErr = ""
	for _, fileIn := range addedFiles {
		fileInName := filepath.Base(fileIn)
		fileInSize, _ := GetFileSize(fileIn)
		for _, fileZip := range zf.File {
			if strings.Contains(fileZip.Name, fileInName) {
				counter++
				if int(fileZip.UncompressedSize64) != fileInSize {
					log.Error("File was not written properly!", zap.Int("rawSize", fileInSize), zap.Int("zip.UncompressedSize64", int(fileZip.UncompressedSize64)))
					validataErr = "File " + fileZip.Name + " was not written properly!"
				}
			}
		}
	}

	err = zf.Close()
	if err != nil {
		return err
	}

	if validataErr != "" {
		return errors.New(validataErr)
	}

	if counter != len(addedFiles) {
		return errors.New("Not all files added to archive")
	}

	return nil
}

func CreateArchive(archivePath string, filesToAdd []string, basePath string) error {
	if filepath.Ext(archivePath) != ".zip" {
		archivePath += ".zip"
	}

	// Create all files and directories
	archive, err := CreateFileP(archivePath, 0750)
	if err != nil {
		log.Error("Error creating job-archive", zap.String("file", archivePath))
		return err
	}

	// Close the file later
	defer func(archive *os.File) {
		_ = archive.Close()
	}(archive)

	// Create a new zip writer
	zipWriter := zip.NewWriter(archive)

	// Add all files to zip
	for _, file := range filesToAdd {
		if err := addFileToZip(file, zipWriter, basePath); err != nil {
			log.Error("error in addFileToZip", zap.Error(err))
			// don't return, since zipWriter is not yet closed. (can't use defer, otherwise verify would fail)
		}
	}

	err = zipWriter.Close()
	if err != nil {
		log.Error("error while closing zip file writer", zap.Error(err))
		return err
	}

	// Verify all files are written completely (via size)
	err = verifyZipArchive(archivePath, filesToAdd)
	if err != nil {
		return err
	}

	return nil
}

func MoveFile(sourcePath string, destPath string) error {
	return os.Rename(sourcePath, destPath)
}

var (
	ErrPathIsDir  = errors.New("supplied path is a directory")
	ErrPathIsFile = errors.New("supplied path is a file")
)

func Info(path string) (fs.FileInfo, error) {
	s, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func Exists(path string) error {
	s, err := Info(path)
	if err != nil {
		return err
	}

	if s.IsDir() {
		return ErrPathIsDir
	}

	return nil
}

func IsDir(path string) error {
	s, err := Info(path)
	if err != nil {
		return err
	}

	if !s.IsDir() {
		return ErrPathIsFile
	}

	return nil
}

func GetFileSize(filePath string) (int, error) {
	theFile, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	fileSize := int(theFile.Size())
	return fileSize, nil
}
