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

	"disco.cs.uni-kl.de/apogee/pkg/log"
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
	defer func(zipWriter *zip.Writer) {
		err := zipWriter.Close()
		if err != nil {
			log.Error("error while closing zip file writer", zap.Error(err))
		}
	}(zipWriter)

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

//func SplitFileInChunks(inFilePath string, outFileName string, outFolderPath string, chunkSizeByte int) ([]string, []string, error) {
//	// src: https://gist.github.com/serverwentdown/03d4a2ff23896193c9856da04bf36a94
//	var chunkPaths []string
//	var chunkMD5sums []string
//	// Open file to be split
//	inFile, err := os.Open(inFilePath)
//	defer inFile.Close()
//	if err != nil {
//		log.Error("error while opening src file", zap.Error(err))
//		return nil, nil, err
//	}
//
//	// ensure the output-folder is available
//	err = os.MkdirAll(outFolderPath, os.ModePerm)
//	if err != nil {
//		log.Error("error while creating out-folder for chunk files", zap.Error(err))
//		return nil, nil, err
//	}
//
//	// create the chunks
//	fi, err := inFile.Stat()
//	if err != nil {
//		log.Error("error while obtaining src file size", zap.Error(err))
//		return nil, nil, err
//	}
//	fileSize := int(fi.Size())
//	chunkAmount := int(math.Ceil(float64(fileSize) / float64(chunkSizeByte)))
//	for i := 0; i < chunkAmount; i++ {
//		chunkPath := filepath.Join(outFolderPath, outFileName) + "_part" + fmt.Sprint(i)
//		// Check for existing chunk (and delete if exists)
//		if _, err := os.Stat(chunkPath); !os.IsNotExist(err) {
//			err := os.Remove(chunkPath)
//			if err != nil {
//				log.Error("error while removing existing chunk", zap.Error(err))
//				return nil, nil, err
//			}
//		}
//
//		// Create chunk file
//		outFile, err := os.Create(chunkPath)
//		if err != nil {
//			log.Error("error while creating chunk file", zap.Error(err))
//			return nil, nil, err
//		}
//
//		// Copy chunkSizeByte bytes to chunk file
//		_, err = io.CopyN(outFile, inFile, int64(chunkSizeByte))
//		if err == io.EOF {
//			//fmt.Printf("%d bytes written to last file\n", written)
//		} else if err != nil {
//			log.Error("error while writing chunk file", zap.Error(err))
//			return nil, nil, err
//		}
//		err = outFile.Close()
//		if err != nil {
//			log.Error("error while closing chunk file", zap.Error(err))
//			return nil, nil, err
//		}
//
//		// Get the MD5 hash
//		fileHash, err := os.Open(chunkPath)
//		if err != nil {
//			log.Error("error while opening chunk file", zap.Error(err))
//			return nil, nil, err
//		}
//		hash := md5.New()
//		if _, err := io.Copy(hash, fileHash); err != nil {
//			log.Error("error while calculating MD5-sum", zap.Error(err))
//			return nil, nil, err
//		}
//		hashString := hex.EncodeToString(hash.Sum(nil))
//		chunkPaths = append(chunkPaths, chunkPath)
//		chunkMD5sums = append(chunkMD5sums, hashString)
//		err = fileHash.Close()
//		if err != nil {
//			log.Error("error while closing chunk file", zap.Error(err))
//			return nil, nil, err
//		}
//	}
//	return chunkPaths, chunkMD5sums, nil
//}
//
//func DeleteChunks(chunkPaths []string) error {
//	// try to delete all files, if any has an error return it (after trying to delete the others)
//	var errFinal error
//	for _, path := range chunkPaths {
//		_, err := os.Stat(path)
//		if !os.IsNotExist(err) {
//			err := os.Remove(path)
//			if err != nil {
//				log.Error("error while deleting chunk file", zap.Error(err))
//				errFinal = err
//			}
//		}
//	}
//	return errFinal
//}
