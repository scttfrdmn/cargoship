package inventory

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/helpers"
	"golang.org/x/tools/godoc/util"
)

type DirectoryInventory struct {
	Files            []*InventoryFile           `yaml:"files"`
	Options          *DirectoryInventoryOptions `yaml:"options"`
	TotalIndexes     int                        `yaml:"total_indexes"`
	IndexSummaries   map[int]*IndexSummary      `yaml:"index_summaries"`
	InternalMetadata map[string]string          `yaml:"internal_metadata"`
	ExternalMetadata map[string]string          `yaml:"external_metadata"`
}

type IndexSummary struct {
	Count uint   `yaml:"count"`
	Size  uint64 `yaml:"size"`
}

type DirectoryInventoryOptions struct {
	Name                  string   `yaml:"name"`
	TopLevelDirectories   []string `yaml:"top_level_directories"`
	SizeConsideredLarge   int64    `yaml:"size_considered_large"`
	MaxSuitcaseSize       uint64   `yaml:"max_suitcase_size"`
	InternalMetadataGlob  string   `yaml:"internal_metadata_glob,omitempty"`
	ExternalMetadataFiles []string `yaml:"external_metadata_files,omitempty"`
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
				log.Info().
					Str("path", item.Path).
					Uint64("size", item.Size).
					Int("numCases", numCases).
					Msg("index is full, adding new index")
				numCases += 1
				caseSet[numCases] = maxSize - item.Size
				item.SuitcaseIndex = numCases
			}
		}
		// Write up summary
		if inventory.IndexSummaries == nil {
			inventory.IndexSummaries = map[int]*IndexSummary{}
		}

		if _, ok := inventory.IndexSummaries[item.SuitcaseIndex]; !ok {
			inventory.IndexSummaries[item.SuitcaseIndex] = &IndexSummary{}
		}
		s := inventory.IndexSummaries[item.SuitcaseIndex]
		s.Count += 1
		s.Size += item.Size
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
	if opts.InternalMetadataGlob == "" {
		opts.InternalMetadataGlob = "suitcase-meta*"
	}
	// Need at least 1 directory
	if len(opts.TopLevelDirectories) == 0 {
		return nil, fmt.Errorf("must specify at least one top level directory")
	}
	// First up, slurp in that yummy metadata
	internalMeta := map[string]string{}
	for _, dir := range opts.TopLevelDirectories {
		data, err := GetMetadataWithGlob(fmt.Sprintf("%v/%v", dir, opts.InternalMetadataGlob))
		if err != nil {
			return nil, err
		}
		for k, v := range data {
			internalMeta[k] = v
		}
	}
	ret.InternalMetadata = internalMeta

	// Mmm...internal metadata is tasty, but I'm still hungry for some of that external metadata
	externalMeta := map[string]string{}
	if len(opts.ExternalMetadataFiles) > 0 {
		ret.ExternalMetadata = externalMeta
	}

	if len(ret.InternalMetadata) == 0 && len(ret.ExternalMetadata) == 0 {
		log.Warn().
			Str("internal-glob", opts.InternalMetadataGlob).
			Strs("external-files", opts.ExternalMetadataFiles).
			Strs("topLevelDirectories", opts.TopLevelDirectories).
			Msg("no metadata found")
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

				// No symlink dirs
				if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return nil
				}
				// log.Info().Msgf("adding file to inventory: %+v", info)
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

// Given a file path with a glob, return metadata. The metadata is a map of filename to data
func GetMetadataWithGlob(fpg string) (map[string]string, error) {
	matches, err := filepath.Glob(fpg)
	if err != nil {
		return nil, err
	}
	return GetMetadataWithFiles(matches)
}

func GetMetadataWithFiles(files []string) (map[string]string, error) {
	ret := map[string]string{}
	var err error
	for _, f := range files {
		f, err = filepath.Abs(f)
		if err != nil {
			return nil, err
		}
		data, err := ioutil.ReadFile(f)
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
