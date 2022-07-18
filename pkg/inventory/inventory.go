package inventory

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/karrick/godirwalk"
	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/helpers"
	"golang.org/x/tools/godoc/util"
)

type Inventoryer interface {
	Write(io.Writer, *DirectoryInventory) error
	Read([]byte) (*DirectoryInventory, error)
}

type DirectoryInventory struct {
	Files            []*InventoryFile           `yaml:"files" json:"files"`
	Options          *DirectoryInventoryOptions `yaml:"options" json:"options"`
	TotalIndexes     int                        `yaml:"total_indexes" json:"total_indexes"`
	IndexSummaries   map[int]*IndexSummary      `yaml:"index_summaries" json:"index_summaries"`
	InternalMetadata map[string]string          `yaml:"internal_metadata" json:"internal_metadata"`
	ExternalMetadata map[string]string          `yaml:"external_metadata" json:"external_metadata"`
}

type IndexSummary struct {
	Count uint  `yaml:"count"`
	Size  int64 `yaml:"size"`
}

type DirectoryInventoryOptions struct {
	Name                  string   `yaml:"name" json:"name"`
	User                  string   `yaml:"user" json:"user"`
	Prefix                string   `yaml:"prefix" json:"prefix"`
	TopLevelDirectories   []string `yaml:"top_level_directories" json:"top_level_directories"`
	SizeConsideredLarge   int64    `yaml:"size_considered_large" json:"size_considered_large"`
	MaxSuitcaseSize       int64    `yaml:"max_suitcase_size" json:"max_suitcase_size"`
	InternalMetadataGlob  string   `yaml:"internal_metadata_glob,omitempty" json:"internal_metadata_glob,omitempty"`
	ExternalMetadataFiles []string `yaml:"external_metadata_files,omitempty" json:"external_metadata_files,omitempty"`
	EncryptInner          bool     `yaml:"encrypt_inner" json:"encrypt_inner"`
	HashInner             bool     `yaml:"hash_inner" json:"hash_inner"`
	LimitFileCount        int      `yaml:"limit_file_count" json:"limit_file_count"`
	Format                string   `yaml:"format" json:"format"`
}

type InventoryFile struct {
	Path        string `yaml:"path" json:"path"`
	Destination string `yaml:"destination" json:"destination"`
	Name        string `yaml:"name" json:"name"`
	Size        int64  `yaml:"size" json:"size"`
	/*
		Mode          os.FileMode `yaml:"mode,omitempty" json:"mode,omitempty"`
		ModTime       time.Time   `yaml:"mod_time,omitempty" json:"mod_time,omitempty"`
		IsDir         bool        `yaml:"is_dir" json:"is_dir"`
		SHA256        string      `yaml:"sha256,omitempty" json:"sha256,omitempty"`
		Encrypt       bool        `yaml:"encrypt,omitempty" json:"encrypt,omitempty"`
	*/
	SuitcaseIndex int    `yaml:"suitcase_index,omitempty" json:"suitcase_index,omitempty"`
	SuitcaseName  string `yaml:"suitcase_name,omitempty" json:"suitcase_name,omitempty"`
}

type FileBucket struct {
	Free int64
}

var errHalt = errors.New("halt")

func ExpandSuitcaseNames(di *DirectoryInventory, prefix string, user string, total int) error {
	var extension string
	if di.Options == nil || di.Options.Format == "" {
		extension = "tar"
	} else {
		extension = di.Options.Format
	}
	for _, f := range di.Files {
		if f.SuitcaseName == "" {
			n := FormatSuitcaseName(prefix, user, f.SuitcaseIndex, di.TotalIndexes, extension)
			f.SuitcaseName = n
			log.Info().Str("name", n).Msg("expanded suitcase names")
		}
	}
	return nil
}

func FormatSuitcaseName(p, u string, i, t int, ext string) string {
	if u == "" {
		u = "unknown-user"
	}
	if p == "" {
		u = "unknown-prefix"
	}
	return fmt.Sprintf("%v-%v-%02d-of-%02d.%v", p, u, i, t, ext)
}

func ExtractSuitcaseNames(di *DirectoryInventory) []string {
	ret := []string{}
	for _, f := range di.Files {
		ret = append(ret, f.SuitcaseName)
	}
	return ret
}

// Loop through inventory and assign suitcase indexes
func IndexInventory(inventory *DirectoryInventory, maxSize int64) error {
	caseSet := map[int]int64{
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
					Int64("size", item.Size).
					Int64("maxSize", maxSize).
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
					Int64("size", item.Size).
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
		// err := filepath.Walk(dir,
		// func(path string, info os.FileInfo, err error) error {
		var addedCount int
		err := godirwalk.Walk(dir, &godirwalk.Options{
			Callback: func(path string, de *godirwalk.Dirent) error {
				// Skip top level directories from inventory
				var err error
				/*
					if path == dir {
						return nil
					}
				*/
				if de.IsDir() {
					return nil
				}

				// No symlink dirs
				if de.IsSymlink() {
					// return godirwalk.SkipThis
					return nil
				}

				// Finally look at the size
				st, err := os.Stat(path)
				if err != nil {
					return err
				}
				size := st.Size()

				fItem := InventoryFile{
					Path:        path,
					Destination: strings.TrimPrefix(path, dir),
					Name:        de.Name(),
					Size:        size,
				}
				if opts.HashInner {
					/*
						fItem.SHA256, err = helpers.GetSha256(path)
						if err != nil {
							log.Warn().Err(err).Str("path", path).Msg("error getting sha256 hash")
						}
					*/
				}
				ret.Files = append(ret.Files, &fItem)
				addedCount++

				if addedCount%1000 == 0 {
					log.Debug().
						Int("count", addedCount).
						Msg("Added files to inventory")
					printMemUsage()
				}

				if opts.LimitFileCount > 0 && addedCount >= opts.LimitFileCount {
					log.Warn().Msg("Reached file count limit, stopping walk")
					return errHalt

				}
				return nil
			},
			Unsorted: true,
			ErrorCallback: func(osPathname string, err error) godirwalk.ErrorAction {
				// Desired way, but currently wrong (not halting) due to different error types.
				if err == errHalt {
					return godirwalk.Halt
				}

				// Currently correct way.
				// if err.Error() == errHalt.Error() {
				// 	return godirwalk.Halt
				// }

				return godirwalk.SkipNode
			},
		})
		if err != nil {
			log.Warn().Err(err).Int("files", addedCount).Msg("error walking directory")
		} else {
			log.Info().Int("files", addedCount).Msg("Finished walking directory")
		}
	}
	log.Info().Msg("FINISHED WLAKING")
	err := IndexInventory(ret, opts.MaxSuitcaseSize)
	if err != nil {
		return nil, err
	}
	/*
		err = ExpandSuitcaseNames(ret, ret.Options.Prefix, ret.Options.User, ret.TotalIndexes)
		log.Warn().Msgf("GOT INDEXES: %v", ret.TotalIndexes)
		if err != nil {
			return nil, err
		}
	*/
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

func printMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Info().
		Uint64("allocated", m.Alloc).
		Uint64("total-allocated", m.TotalAlloc).
		Float64("allocated-percent", (float64(m.Alloc)/float64(m.TotalAlloc))*float64(100)).
		Uint64("system", m.Sys).
		Uint64("gc-count", uint64(m.NumGC)).
		Msg("Memory Usage in MB")
}

func NewInventoryerWithFilename(filename string) (Inventoryer, error) {
	ext := filepath.Ext(filename)
	var ir Inventoryer
	switch ext {
	case ".json":
		ir = &EJSONer{}
	case ".yaml", ".yml":
		ir = &VAMLer{}
	default:
		return nil, fmt.Errorf("unsupported file extension %s", ext)
	}
	return ir, nil
}
