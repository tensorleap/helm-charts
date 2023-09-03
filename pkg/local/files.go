package local

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

func ExtractTarGzFile(file io.Reader, outputDir string) ([]string, error) {
	fmt.Println("Extract files...")
	gzipReader, err := gzip.NewReader(file)
	files := []string{}

	if err != nil {
		return files, err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	// Iterate through the tar archive and extract the files
	for {
		fileName, readEnd, err := readNextTarFile(tarReader, outputDir)
		if err != nil {
			return files, err
		}
		if readEnd {
			return files, nil
		}
		files = append(files, fileName)
	}
}

func readNextTarFile(tarReader *tar.Reader, outputDir string) (fileName string, readEnd bool, err error) {
	header, err := tarReader.Next()
	if err == io.EOF {
		readEnd = true
		err = nil
		return
	}
	if err != nil {
		return
	}

	// Clean the path to prevent path traversal attacks
	fileName = filepath.Clean(header.Name)

	extractedFilePath := filepath.Join(outputDir, header.Name)
	if header.FileInfo().IsDir() {
		err = os.MkdirAll(extractedFilePath, header.FileInfo().Mode())
		return
	}

	// Create the directory for the file if it doesn't exist
	err = os.MkdirAll(filepath.Dir(extractedFilePath), 0755)
	if err != nil {
		return
	}

	file, err := os.Create(extractedFilePath)
	if err != nil {
		return
	}
	defer file.Close()

	_, err = io.Copy(file, tarReader)
	if err == io.EOF {
		err = nil
	}
	return
}

func CreateTarGzFile(filesDir string, filePaths []string, file io.Writer) error {
	fmt.Println("Packing files...")
	gzip := gzip.NewWriter(file)
	defer gzip.Close()
	tarWriter := tar.NewWriter(gzip)
	defer tarWriter.Close()

	for _, relativePath := range filePaths {
		filePath := path.Join(filesDir, relativePath)
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return err
		}
		fmt.Println(filePath)
		header, err := tar.FileInfoHeader(fileInfo, fileInfo.Name())
		if err != nil {
			return err
		}
		header.Name = filePath
		err = tarWriter.WriteHeader(header)
		if err != nil {
			return err
		}
		fileContents, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		_, err = tarWriter.Write(fileContents)
		if err != nil {
			return err
		}
	}

	return nil
}

func CleanupTempFile(file *os.File) {
	file.Close()
	os.Remove(file.Name())
}

func GetFileChecksum(file io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
