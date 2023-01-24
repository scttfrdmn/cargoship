/*
Package helpers is just some helper utils. Arguably this should be moved out of
it's own package in to the appropriate main packages
*/
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

// ConvertDirsToAboluteDirs turns directories in to absolute path directories
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

// GetSha256 Get sha256 hash from a file
func GetSha256(file string) (string, error) {
	f, err := os.Open(file) // nolint:gosec
	if err != nil {
		return "", err
	}
	defer func() {
		cerr := f.Close()
		if err != nil {
			panic(cerr)
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// MustGetSha256 panics if a Sha256 cannot be generated
func MustGetSha256(file string) string {
	hash, err := GetSha256(file)
	if err != nil {
		panic(err)
	}
	return hash
}

// SuitcaseFormatWithFilename detects the format of a suitcase from the given filename
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
	return "", errors.New("unknown archive format")
}

// IsDirectory returns a bool if a file is a directory
func IsDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

// HashSet is a combination Filename and Hash
type HashSet struct {
	Filename string
	Hash     string
}

// WriteHashFile  writes out the hashset array to an io.Writer
func WriteHashFile(hs []HashSet, o io.Writer) error {
	w := bufio.NewWriter(o)
	for _, hs := range hs {
		_, err := w.WriteString(fmt.Sprintf("%s\t%s\n", hs.Filename, hs.Hash))
		if err != nil {
			return err
		}
	}
	err := w.Flush()
	if err != nil {
		return err
	}
	return nil
}

// FilenameMatchesGlobs Check if a filename matches a set of globs
func FilenameMatchesGlobs(filename string, globs []string) bool {
	for _, glob := range globs {
		if ok, _ := filepath.Match(glob, filename); ok {
			return true
		}
	}
	return false
}
