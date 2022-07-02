package inventory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/helpers"
)

type DirectoryInventory struct {
	Files        []*InventoryFile           `json:"files"`
	Options      *DirectoryInventoryOptions `json:"options"`
	TotalIndexes int                        `json:"total_indexes"`
}

type DirectoryInventoryOptions struct {
	Name                string   `json:"name"`
	TopLevelDirectories []string `json:"top_level_directories"`
	SizeConsideredLarge int64    `json:"size_considered_large"`
	MaxSuitcaseSize     uint64   `json:"max_suitcase_size"`
}

type InventoryFile struct {
	Path          string
	Destination   string
	Name          string
	Size          uint64
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

// Loop through inventory and assign suitcase indexes
func IndexInventory(inventory *DirectoryInventory, maxSize uint64) error {
	caseSet := map[int]uint64{
		1: maxSize,
	}
	numCases := 1
	// Sort by descending size
	sort.Slice(inventory.Files, func(i, j int) bool {
		return inventory.Files[i].Size > inventory.Files[j].Size
	})
	for _, item := range inventory.Files {
		// Implementation requires that maxSize is greater than or equal to the size of the largest file
		// If maxSize == 0, everything goes in the same suitcase
		if maxSize == 0 {
			item.SuitcaseIndex = 1
		} else {
			if item.Size > maxSize {
				log.Warn().
					Str("path", item.Path).
					Uint64("size", item.Size).
					Uint64("maxSize", maxSize).
					Msg("file is too large for suitcase")
				return errors.New("index containes at least one file that is too large")
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
				log.Warn().
					Str("path", item.Path).
					Uint64("size", item.Size).
					Int("numCases", numCases).
					Msg("index is full, adding new index")
				numCases += 1
				caseSet[numCases] = maxSize - item.Size
				item.SuitcaseIndex = numCases
			}
		}
	}
	inventory.TotalIndexes = numCases
	return nil
}

func NewDirectoryInventory(opts *DirectoryInventoryOptions) (*DirectoryInventory, error) {
	ret := &DirectoryInventory{
		Options: opts,
	}
	if opts.Name == "" {
		opts.Name = "suitcase"
	}
	// Need at least 1 directory
	if len(opts.TopLevelDirectories) == 0 {
		return nil, fmt.Errorf("must specify at least one top level directory")
	}

	for _, dir := range opts.TopLevelDirectories {
		if !helpers.IsDirectory(dir) {
			log.Warn().
				Str("path", dir).
				Msg("top level directory does not exist")
			return nil, errors.New("not a directory")
		}
		log.Info().
			Str("dir", dir).
			Msg("walking directory")
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
					log.Warn().Err(err).Str("path", path).Msg("error getting sha256 hash")
				}
				fItem := InventoryFile{
					Path:        path,
					Destination: strings.TrimPrefix(path, dir),
					Name:        info.Name(),
					Size:        uint64(info.Size()),
					Mode:        info.Mode(),
					ModTime:     info.ModTime(),
					SHA256:      sha256hash,
					// SuitcaseIndex: 1,
				}
				ret.Files = append(ret.Files, &fItem)

				return nil
			})
		if err != nil {
			log.Warn().Err(err).Msg("error walking directory")
		}
	}
	err := IndexInventory(ret, opts.MaxSuitcaseSize)
	if err != nil {
		return nil, err
	}
	return ret, nil
}
