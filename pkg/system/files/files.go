package files

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"go.uber.org/zap"
)

// Creates a file and all its directories
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

func WriteInFile(filePath string, text string) error {
	f, err := CreateFileP(filePath, 0750)
	if err != nil {
		return err
	}

	// Close the file when done
	defer f.Close()

	// Write string to file
	_, err = f.WriteString(text)
	return err
}

// Get the relative path from an absolute path with a specified base dir
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

	defer srcFile.Close()

	// Get some file infos
	fileInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// If the user specified a base directory, build the structure from this
	zipPath := absFilePath
	if baseDir != "" {
		// Create the relative path with the given base directory
		zipPath, err = GetRelPathFromAbs(absFilePath, baseDir)
		if err != nil {
			return err
		}
	}

	// Create a new zip FileHeader
	zipFileHeader := zip.FileHeader{
		Name:               filepath.Clean(zipPath),
		UncompressedSize64: uint64(fileInfo.Size()),
		Modified:           fileInfo.ModTime(),
	}

	// Mirror the filesystem permissions
	zipFileHeader.SetMode(fileInfo.Mode())

	// Get the writer for the file entry
	zipFileWriter, err := writer.CreateHeader(&zipFileHeader)
	if err != nil {
		return err
	}

	// If this was a directory, stop here!
	if fileInfo.IsDir() {
		return nil
	}

	// Copy the file contents to the zip
	bytesWritten, err := io.Copy(zipFileWriter, srcFile)
	if err != nil {
		return err
	}

	// Sanity check filesizes
	sourceFileSize := fileInfo.Size()
	if bytesWritten != sourceFileSize {
		return fmt.Errorf(
			"%s file size differs written: %d != size: %d",
			filepath.Base(zipPath), bytesWritten, sourceFileSize,
		)
	}

	return nil
}

func WriteFilesInArchive(archivePath string, filesToAdd []string, basePath string) error {
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
	defer archive.Close()

	// Create a new zip writer
	zipWriter := zip.NewWriter(archive)
	defer zipWriter.Close()

	// Add all files to zip
	for _, file := range filesToAdd {
		if err := addFileToZip(file, zipWriter, basePath); err != nil {
			// We dont allow corrupt zip files
			return err
		}
	}

	return nil
}

func MoveFile(sourcePath string, destPath string) error {
	return os.Rename(sourcePath, destPath)
}
