package helpers

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ConvertDirsToAboluteDirs(orig []string) ([]string, error) {
	ret := []string{}
	for _, item := range orig {
		abs, err := filepath.Abs(item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, fmt.Sprintf("%s/", abs))
	}
	return ret, nil
}

// Get sha256 hash from a file
func GetSha256(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func MustGetSha256(file string) string {
	hash, err := GetSha256(file)
	if err != nil {
		panic(err)
	}
	return hash
}

func SuitcaseFormatWithFilename(filename string) (string, error) {
	switch {
	case strings.HasSuffix(filename, ".tar"):
		return "tar", nil
	case strings.HasSuffix(filename, ".tar.gpg"):
		return "tar.gpg", nil
	case strings.HasSuffix(filename, ".tar.gz"):
		return "tar.gz", nil
	case strings.HasSuffix(filename, ".tar.gz.gpg"):
		return "tar.gz.gpg", nil
	}
	return "", errors.New("Unknown archive format")
}

func IsDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

type HashSet struct {
	Filename string
	Hash     string
}

func WriteHashFile(hs []HashSet, o io.Writer) error {
	w := bufio.NewWriter(o)
	for _, hs := range hs {
		_, err := w.WriteString(fmt.Sprintf("%s\t%s\n", hs.Filename, hs.Hash))
		if err != nil {
			return err
		}
	}
	w.Flush()
	return nil
}
