package files

import (
	"archive/zip"
	"bufio"
	"io"
	"os"
	"strings"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
)

func WriteInFile(filePath string, text string) (os.File, error) {
	dirPathLength := strings.LastIndex(filePath, "/")
	dirPath := filePath[:dirPathLength]
	_, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			return os.File{}, err
		}
	}
	f, err := os.Create(filePath)
	if err != nil {
		return os.File{}, err
	}
	w := bufio.NewWriter(f)
	_, err = w.WriteString(text)
	if err != nil {
		return os.File{}, err
	}
	err = w.Flush()
	if err != nil {
		return os.File{}, err
	}
	return *f, nil
}

func WriteFilesInArchive(archivePath string, filesToAdd []string) (os.File, error) {
	if !strings.Contains(archivePath, ".zip") {
		archivePath = archivePath + ".zip"
	}
	dirPathLength := strings.LastIndex(archivePath, "/")
	dirPath := archivePath[:dirPathLength]
	_, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			return os.File{}, err
		}
	}
	archive, err := os.Create(archivePath)
	if err != nil {
		apglog.Error("Error creating job-archive: " + err.Error())
		return os.File{}, err
	}
	defer func(archive *os.File) {
		err := archive.Close()
		if err != nil {
			apglog.Error("Error closing job-archive: " + err.Error())
		}
	}(archive)
	zipWriter := zip.NewWriter(archive)
	//adding files to archive
	for i := 0; i < len(filesToAdd); i++ {
		tempFile := filesToAdd[i]
		fileName := tempFile[strings.LastIndex(tempFile, "/")+1:]
		f1, err := os.Open(tempFile)
		if err != nil {
			apglog.Error("Error opening file to add to archive: " + err.Error())
			return os.File{}, err
		}
		defer func(f1 *os.File) {
			err := f1.Close()
			if err != nil {
				apglog.Error("Error closing file to add to archive: " + err.Error())
			}
		}(f1)
		w1, err := zipWriter.Create(fileName)
		if err != nil {
			apglog.Error("Error adding file to archive: " + err.Error())
			return os.File{}, err
		}
		if _, err := io.Copy(w1, f1); err != nil {
			apglog.Error("Error adding file-content to archive: " + err.Error())
			return os.File{}, err
		}
	}
	err = zipWriter.Close()
	if err != nil {
		apglog.Error("Error closing archive-writer: " + err.Error())
		return os.File{}, err
	}
	return *archive, nil
}

func MoveFile(sourcePath string, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		apglog.Error("Couldn't open source file: " + err.Error())
		return err
	}
	outputFile, err := os.Create(destPath)
	if err != nil {
		err2 := inputFile.Close()
		if err2 != nil {
			apglog.Error("Error during Error: Couldn't open dest file: " + err.Error() +
				"\n and couldn't close source file: " + err2.Error())
			return err
		}
		apglog.Error("Couldn't open dest file: " + err.Error())
		return err
	}
	defer func(outputFile *os.File) {
		err := outputFile.Close()
		if err != nil {
			apglog.Error("Couldn't close destination file after copy: " + err.Error())
		}
	}(outputFile)
	_, err = io.Copy(outputFile, inputFile)
	err = inputFile.Close()
	if err != nil {
		apglog.Error("Couldn't close source file after copy: " + err.Error())
		return err
	}
	if err != nil {
		apglog.Error("Writing to output file failed: " + err.Error())
		return err
	}
	err = os.Remove(sourcePath)
	if err != nil {
		apglog.Error("Failed removing original file: " + err.Error())
		return err
	}
	return nil
}
