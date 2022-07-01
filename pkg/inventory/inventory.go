package inventory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/apex/log"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/helpers"
)

type DirectoryInventory struct {
	SmallFiles []*InventoryFile           `json:"small_files"`
	LargeFiles []*InventoryFile           `json:"large_files"`
	Options    *DirectoryInventoryOptions `json:"options"`
}

type DirectoryInventoryOptions struct {
	TopLevelDirectories []string
	SizeConsideredLarge int64 // in bytes
	MaxSuitcaseSize     int64 // in bytes
}

type InventoryFile struct {
	Path          string
	Destination   string
	Name          string
	Size          int64
	Mode          os.FileMode
	ModTime       time.Time
	IsDir         bool
	SHA256        string
	Encrypt       bool
	SuitcaseIndex int
}

type FileBucket struct {
	Free int64
}

func IndexInventory(inventory *DirectoryInventory, maxSize int64) (int, error) {
	all := append(inventory.SmallFiles, inventory.LargeFiles...)
	// Case set represents the index (suitecase-1) and the remaining space in the suitcase
	caseSet := map[int]int64{
		1: maxSize,
	}
	numCases := 1
	// Sort by descending size
	sort.Slice(all, func(i, j int) bool {
		return all[i].Size > all[j].Size
	})
	for _, item := range all {
		// Implementation requires that maxSize is greater than or equal to the size of the largest file
		// If maxSize == 0, everything goes in the same suitcase
		if maxSize == 0 {
			item.SuitcaseIndex = 1
		} else {
			if item.Size > maxSize {
				log.WithFields(log.Fields{
					"path":    item.Path,
					"size":    item.Size,
					"maxSize": maxSize,
				}).Warn("file is too large for suitcase")
				return 0, errors.New("index containes at least one file that is too large")
			}
			// for loop := true; loop; {
			var sorted bool
			for index, sizeLeft := range caseSet {
				if item.Size <= sizeLeft {
					item.SuitcaseIndex = index
					caseSet[index] -= item.Size
					sorted = true
					break
				}
			}
			if !sorted {
				log.WithFields(log.Fields{
					"path":     item.Path,
					"size":     item.Size,
					"numCases": numCases,
				}).Warn("index is full, adding new index")
				caseSet[numCases] = maxSize - item.Size
				item.SuitcaseIndex = numCases
				numCases += 1
			}
		}
	}
	return numCases, nil
}

func NewDirectoryInventory(opts *DirectoryInventoryOptions) (*DirectoryInventory, error) {
	ret := &DirectoryInventory{
		Options: opts,
	}
	// Need at least 1 directory
	if len(opts.TopLevelDirectories) == 0 {
		return nil, fmt.Errorf("must specify at least one top level directory")
	}
	// We still may need to set this occasionally
	if opts.SizeConsideredLarge == 0 {
		defaultSize := int64(1024) * int64(1024)
		log.WithFields(log.Fields{
			"large_file_size": defaultSize,
		}).Debug("setting large file size to default value since none was specified")
		opts.SizeConsideredLarge = defaultSize
	}

	// Ok, lets walk!
	// smallFilesC := make(chan string)
	// largeFilesC := make(chan string)

	for _, dir := range opts.TopLevelDirectories {
		log.WithFields(log.Fields{
			"dir": dir,
		}).Info("walking directory")
		err := filepath.Walk(dir,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				// Skip top level directories from inventory
				if path == dir {
					return nil
				}
				if info.IsDir() {
					return nil
				}
				sha256hash, err := helpers.GetSha256(path)
				if err != nil {
					log.WithError(err).WithField("path", path).Warn("error getting sha256 hash")
				}
				fItem := InventoryFile{
					Path:        path,
					Destination: strings.TrimPrefix(path, dir),
					Name:        info.Name(),
					Size:        info.Size(),
					Mode:        info.Mode(),
					ModTime:     info.ModTime(),
					SHA256:      sha256hash,
					// SuitcaseIndex: 1,
				}
				if info.Size() > opts.SizeConsideredLarge {
					ret.LargeFiles = append(ret.LargeFiles, &fItem)
				} else {
					ret.SmallFiles = append(ret.SmallFiles, &fItem)
				}
				// fmt.Println(path, info.Size())
				return nil
			})
		if err != nil {
			log.WithError(err).Error("error walking directory")
		}
	}
	_, err := IndexInventory(ret, opts.MaxSuitcaseSize)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func Ptr[T any](v T) *T {
	return &v
}
