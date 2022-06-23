package suitcase

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
