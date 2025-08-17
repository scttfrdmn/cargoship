/*
Package inventory provides the needed pieces to correctly create an Inventory of a directory
*/
package inventory

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/tools/godoc/util"
)

// GetMetadataWithGlob Given a file path with a glob, return metadata. The metadata is a map of filename to data
func GetMetadataWithGlob(fpg string) (map[string]string, error) {
	matches, err := filepath.Glob(fpg)
	if err != nil {
		return nil, err
	}
	return GetMetadataWithFiles(matches)
}

// GetMetadataWithFiles returns the metadata for a set of files
func GetMetadataWithFiles(files []string) (map[string]string, error) {
	ret := map[string]string{}
	var err error
	for _, f := range files {
		f, err = filepath.Abs(f)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(f) // nolint:gosec
		if err != nil {
			return nil, err
		}
		if !util.IsText(data) {
			return nil, fmt.Errorf("%s is not a text file", f)
		}
		ret[f] = string(data)
	}
	return ret, nil
}

// getInternalMeta retrieves internal metadata based on options configuration
func getInternalMeta(opts *Options) (map[string]string, error) {
	internalMeta := map[string]string{}
	for _, dir := range opts.Directories {
		data, err := GetMetadataWithGlob(fmt.Sprintf("%v/%v", dir, opts.InternalMetadataGlob))
		if err != nil {
			return nil, err
		}
		for k, v := range data {
			internalMeta[k] = v
		}
	}
	return internalMeta, nil
}
