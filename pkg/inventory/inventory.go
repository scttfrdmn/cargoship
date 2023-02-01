/*
Package inventory provides the needed pieces to correctly create an Inventory of a directory
*/
package inventory

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/karrick/godirwalk"
	"github.com/rs/zerolog/log"
	"golang.org/x/tools/godoc/util"
)

// Format is the format the inventory will use, such as yaml, json, etc
type Format int

const (
	// NullFormat is the unset value for this type
	NullFormat = iota
	// YAMLFormat is for yaml
	YAMLFormat
	// JSONFormat is for yaml
	JSONFormat
)

// DefaultSuitcaseFormat is just the default format we're going to use for a
// suitcase. Hopefully this fits for most use cases, but can always be
// overridden
const DefaultSuitcaseFormat string = "tar.gz"

var formatMap map[string]Format = map[string]Format{
	"yaml": YAMLFormat,
	"json": JSONFormat,
	"":     NullFormat,
}

// FormatCompletion returns shell completion
func FormatCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nonEmptyKeys(formatMap), cobra.ShellCompDirectiveNoFileComp
}

func (f Format) String() string {
	m := reverseMap(formatMap)
	if v, ok := m[f]; ok {
		return v
	}
	panic("invalid format")
}

// Type satisfies part of the pflags.Value interface
func (f Format) Type() string {
	return "Format"
}

// Set helps fulfill the pflag.Value interface
func (f *Format) Set(v string) error {
	if v, ok := formatMap[v]; ok {
		*f = v
		return nil
	}
	return fmt.Errorf("ProductionLevel should be one of: %v", nonEmptyKeys(formatMap))
}

// MarshalJSON ensures that json conversions use the string value here, not the int value
func (f *Format) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%v\"", f.String())), nil
}

// Inventoryer is an interface to define what an Inventory Operator does
type Inventoryer interface {
	Write(io.Writer, *DirectoryInventory) error
	Read([]byte) (*DirectoryInventory, error)
}

// DirectoryInventory is the inventory of a set of suitcases
type DirectoryInventory struct {
	Files            []*File                    `yaml:"files" json:"files"`
	Options          *DirectoryInventoryOptions `yaml:"options" json:"options"`
	TotalIndexes     int                        `yaml:"total_indexes" json:"total_indexes"`
	IndexSummaries   map[int]*IndexSummary      `yaml:"index_summaries" json:"index_summaries"`
	InternalMetadata map[string]string          `yaml:"internal_metadata" json:"internal_metadata"`
	ExternalMetadata map[string]string          `yaml:"external_metadata" json:"external_metadata"`
	CLIMeta          CLIMeta                    `yaml:"cli_meta" json:"cli_meta"`
}

// SummaryLog logs out a summary of the suitcase data
func (di DirectoryInventory) SummaryLog() {
	// Print some summary info about the index
	var totalC uint
	var totalS int64
	for k, item := range di.IndexSummaries {
		totalC += item.Count
		totalS += item.Size
		log.Info().
			Int("index", k).
			Uint("file-count", item.Count).
			Int64("file-size", item.Size).
			Str("file-size-human", humanize.Bytes(uint64(item.Size))).
			Msg("Individual Suitcase Data")
	}
	log.Info().
		Uint("file-count", totalC).
		Int64("file-size", totalS).
		Str("file-size-human", humanize.Bytes(uint64(totalS))).
		Msg("Total Suitcase Data")
}

// IndexSummary will give an overall summary to a set of suitcases
type IndexSummary struct {
	Count     uint   `yaml:"count"`
	Size      int64  `yaml:"size"`
	HumanSize string `yaml:"human_size"`
}

// CLIMeta is the meta information about the cli tool that generated an inventory
type CLIMeta struct {
	Date    *time.Time `yaml:"date" json:"date"`
	Version string     `yaml:"version" json:"version"`
}

// DirectoryInventoryOptions are the options used to create a DirectoryInventory
type DirectoryInventoryOptions struct {
	User                  string   `yaml:"user" json:"user"`
	Prefix                string   `yaml:"prefix" json:"prefix"`
	TopLevelDirectories   []string `yaml:"top_level_directories" json:"top_level_directories"`
	SizeConsideredLarge   int64    `yaml:"size_considered_large" json:"size_considered_large"`
	MaxSuitcaseSize       int64    `yaml:"max_suitcase_size" json:"max_suitcase_size"`
	InternalMetadataGlob  string   `yaml:"internal_metadata_glob,omitempty" json:"internal_metadata_glob,omitempty"`
	IgnoreGlobs           []string `yaml:"ignore_globs,omitempty" json:"ignore_globs,omitempty"`
	ExternalMetadataFiles []string `yaml:"external_metadata_files,omitempty" json:"external_metadata_files,omitempty"`
	EncryptInner          bool     `yaml:"encrypt_inner" json:"encrypt_inner"`
	HashInner             bool     `yaml:"hash_inner" json:"hash_inner"`
	LimitFileCount        int      `yaml:"limit_file_count" json:"limit_file_count"`
	SuitcaseFormat        string   `yaml:"suitcase_format" json:"suitcase_format"`
	InventoryFormat       string   `yaml:"inventory_format" json:"inventory_format"`
	FollowSymlinks        bool     `yaml:"follow_symlinks" json:"follow_symlinks"`
}

// File is a file item inside an inventory
type File struct {
	Path          string `yaml:"path" json:"path"`
	Destination   string `yaml:"destination" json:"destination"`
	Name          string `yaml:"name" json:"name"`
	Size          int64  `yaml:"size" json:"size"`
	SuitcaseIndex int    `yaml:"suitcase_index,omitempty" json:"suitcase_index,omitempty"`
	SuitcaseName  string `yaml:"suitcase_name,omitempty" json:"suitcase_name,omitempty"`
}

// FileBucket describes what a filebucket state is
type FileBucket struct {
	Free int64
}

var errHalt = errors.New("halt")

// ExpandSuitcaseNames will fill in suitcase names for a given inventory
func ExpandSuitcaseNames(di *DirectoryInventory) error {
	var extension string
	if di.Options == nil || di.Options.SuitcaseFormat == "" {
		extension = "tar"
	} else {
		extension = di.Options.SuitcaseFormat
	}
	for _, f := range di.Files {
		if f.SuitcaseName == "" {
			n := FormatSuitcaseName(di.Options.Prefix, di.Options.User, f.SuitcaseIndex, di.TotalIndexes, extension)
			f.SuitcaseName = n
		}
	}
	return nil
}

// FormatSuitcaseName provides the proper formatting for a suitcase name
func FormatSuitcaseName(p, u string, i, t int, ext string) string {
	if u == "" {
		u = "unknown-user"
	}
	if p == "" {
		u = "unknown-prefix"
	}
	return fmt.Sprintf("%v-%v-%02d-of-%02d.%v", p, u, i, t, ext)
}

// ExtractSuitcaseNames returns a list of suitcase strings
func ExtractSuitcaseNames(di *DirectoryInventory) []string {
	ret := make([]string, len(di.Files))

	for idx, f := range di.Files {
		ret[idx] = f.SuitcaseName
	}
	return ret
}

// IndexInventory Loops through inventory and assign suitcase indexes
func IndexInventory(inventory *DirectoryInventory, maxSize int64) error {
	caseSet := NewCaseSet(maxSize)
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
			err := checkItemSize(item, maxSize)
			if err != nil {
				return err
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
				log.Debug().
					Str("path", item.Path).
					Int64("size", item.Size).
					Int("numCases", numCases).
					Msg("index is full, adding new index")
				numCases++
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
		s.Count++
		s.Size += item.Size
	}
	// Generate human readable total sizes
	for _, v := range inventory.IndexSummaries {
		v.HumanSize = humanize.Bytes(uint64(v.Size))
	}
	inventory.TotalIndexes = numCases
	return nil
}

// WriteOutDirectoryInventoryAndFileAndInventoyerWithViper uses viper to write out an inventory file
func WriteOutDirectoryInventoryAndFileAndInventoyerWithViper(v *viper.Viper, args []string, outDir, version string) (*DirectoryInventory, *os.File, error) {
	i, f, ir, err := NewDirectoryInventoryAndFileAndInventoyerWithViper(v, args, outDir)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	i.CLIMeta.Date = &now
	i.CLIMeta.Version = version
	err = ir.Write(f, i)
	if err != nil {
		return nil, nil, err
	}
	return i, f, nil
}

// NewDirectoryInventoryAndFileAndInventoyerWithViper does the interface with viper
func NewDirectoryInventoryAndFileAndInventoyerWithViper(v *viper.Viper, args []string, outDir string) (*DirectoryInventory, *os.File, Inventoryer, error) {
	i, f, err := NewDirectoryInventoryAndFileWithViper(v, args, outDir)
	if err != nil {
		return nil, nil, nil, err
	}
	ir, err := NewInventoryerWithFilename(f.Name())
	if err != nil {
		return nil, nil, nil, err
	}
	return i, f, ir, nil
}

// NewDirectoryInventoryAndFileWithViper creates a new inventory with viper
func NewDirectoryInventoryAndFileWithViper(v *viper.Viper, args []string, outDir string) (*DirectoryInventory, *os.File, error) {
	i, err := NewDirectoryInventoryWithViper(v, args)
	if err != nil {
		return nil, nil, err
	}
	outF, err := os.Create(path.Join(outDir, fmt.Sprintf("inventory.%v", i.Options.InventoryFormat))) // nolint:gosec
	if err != nil {
		return nil, nil, err
	}
	return i, outF, nil
}

// NewDirectoryInventoryWithViper new DirectoryInventory with Viper
func NewDirectoryInventoryWithViper(v *viper.Viper, args []string) (*DirectoryInventory, error) {
	inventoryOpts, err := NewDirectoryInventoryOptionsWithViper(v, args)
	if err != nil {
		return nil, err
	}
	return NewDirectoryInventory(inventoryOpts)
}

// NewDirectoryInventory creates a new DirectoryInventory using options
func NewDirectoryInventory(opts *DirectoryInventoryOptions) (*DirectoryInventory, error) {
	ret := &DirectoryInventory{
		Options: opts,
	}
	if opts.Prefix == "" {
		opts.Prefix = "suitcase"
	}
	if opts.InternalMetadataGlob == "" {
		opts.InternalMetadataGlob = "suitcase-meta*"
	}
	// Need at least 1 directory
	if len(opts.TopLevelDirectories) == 0 {
		return nil, fmt.Errorf("must specify at least one top level directory")
	}
	// First up, slurp in that yummy metadata
	var imerr error
	ret.InternalMetadata, imerr = getInternalMeta(opts)
	if imerr != nil {
		return nil, imerr
	}

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
		if !isDirectory(dir) {
			log.Warn().
				Str("path", dir).
				Msg("top level directory does not exist")
			return nil, errors.New("not a directory")
		}
		log.Info().
			Str("dir", dir).
			Msg("walking directory")
		err := walkDir(dir, opts, ret)
		if err != nil {
			return nil, err
		}
		log.Info().Str("path", dir).Msg("Ignoring file as it matches ignore globs")
	}
	if ierr := IndexInventory(ret, opts.MaxSuitcaseSize); ierr != nil {
		return nil, ierr
	}

	if eserr := ExpandSuitcaseNames(ret); eserr != nil {
		return nil, eserr
	}
	return ret, nil
}

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

func printMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Debug().
		Uint64("allocated", m.Alloc).
		Uint64("total-allocated", m.TotalAlloc).
		Float64("allocated-percent", (float64(m.Alloc)/float64(m.TotalAlloc))*float64(100)).
		Uint64("system", m.Sys).
		Uint64("gc-count", uint64(m.NumGC)).
		Msg("Memory Usage in MB")
}

// NewInventoryWithFilename returns a new DirectoryInventory from an inventory File
func NewInventoryWithFilename(s string) (*DirectoryInventory, error) {
	ib, err := os.ReadFile(s) // nolint:gosec
	if err != nil {
		return nil, err
	}
	ir, err := NewInventoryerWithFilename(s)
	if err != nil {
		return nil, err
	}

	inventoryD, err := ir.Read(ib)
	if err != nil {
		return nil, err
	}
	return inventoryD, nil
}

// NewInventoryerWithFilename creates a new inventoryer with a filename
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

func suitcaseFormatWithViper(v *viper.Viper) string {
	f := strings.TrimPrefix(v.GetString("suitcase-format"), ".")
	if f != "" {
		return f
	}
	return DefaultSuitcaseFormat
}

// NewDirectoryInventoryOptionsWithViper creates new inventory options with viper
func NewDirectoryInventoryOptionsWithViper(v *viper.Viper, args []string) (*DirectoryInventoryOptions, error) {
	var err error

	opt := &DirectoryInventoryOptions{
		TopLevelDirectories:   args,
		InternalMetadataGlob:  v.GetString("internal-metadata-glob"),
		ExternalMetadataFiles: v.GetStringSlice("external-metadata-file"),
		IgnoreGlobs:           v.GetStringSlice("ignore-glob"),
		LimitFileCount:        v.GetInt("limit-file-count"),
		SuitcaseFormat:        suitcaseFormatWithViper(v),
		Prefix:                v.GetString("prefix"),
		EncryptInner:          v.GetBool("encrypt-inner"),
		FollowSymlinks:        v.GetBool("follow-symlinks"),
	}
	opt.TopLevelDirectories, err = convertDirsToAboluteDirs(args)
	if err != nil {
		return nil, err
	}

	// User can specify a human readable string here. We will convert it to bytes for them
	mssF := v.GetString("max-suitcase-size")
	if mssF == "" {
		mssF = "0"
	}
	mssU, err := humanize.ParseBytes(mssF)
	if err != nil {
		return nil, err
	}
	opt.MaxSuitcaseSize = int64(mssU)

	// Inventory file format (yaml or json)
	opt.InventoryFormat = strings.TrimPrefix(v.GetString("inventory-format"), ".")
	if opt.InventoryFormat == "" {
		opt.InventoryFormat = "yaml"
	}

	// We want a username so we can shove it in the suitcase name
	opt.User = v.GetString("user")
	if opt.User == "" {
		log.Info().Msg("No user specified, using current user")
		currentUser, err := user.Current()
		if err != nil {
			return nil, err
		}
		opt.User = currentUser.Username
	}

	// Do we want to skip hashes?
	opt.HashInner = v.GetBool("hash-inner")
	if opt.HashInner {
		log.Warn().
			Msg("Generating file hashes. This will will likely increase the inventory generation time.")
	} else {
		log.Warn().
			Msg("Skipping file hashes. This will increase the speed of the inventory, but will not be able to verify the integrity of the files.")
	}

	return opt, nil
}

// CaseSet is just a holder for case sizes
type CaseSet map[int]int64

// NewCaseSet returns a new set of suitcase params
// This is used to keep track of sizing
func NewCaseSet(maxSize int64) CaseSet {
	return map[int]int64{
		1: maxSize,
	}
}

// CreateOrReadInventory will either create a new inventory (if given an empty string), or read an existing one
func CreateOrReadInventory(inventoryFile string, v *viper.Viper, args []string, outDir string, version string) (*DirectoryInventory, error) {
	// Create an inventory file if one isn't specified
	var inventoryD *DirectoryInventory
	if inventoryFile == "" {
		log.Info().Msg("No inventory file specified, we're going to go ahead and create one")
		var outF *os.File
		var err error
		inventoryD, outF, err = WriteOutDirectoryInventoryAndFileAndInventoyerWithViper(v, args, outDir, version)
		if err != nil {
			return nil, err
		}
		log.Info().Str("file", outF.Name()).Msg("Created inventory file")
	} else {
		var err error
		inventoryD, err = NewInventoryWithFilename(inventoryFile)
		if err != nil {
			return nil, err
		}
	}
	inventoryD.SummaryLog()
	return inventoryD, nil
}

func checkItemSize(item *File, maxSize int64) error {
	if item.Size > maxSize {
		log.Warn().
			Str("path", item.Path).
			Int64("size", item.Size).
			Int64("maxSize", maxSize).
			Msg("file is too large for suitcase")
		return errors.New("index contains at least one file that is too large")
	}
	return nil
}

func errCallback(osPathname string, err error) godirwalk.ErrorAction {
	// Desired way, but currently wrong (not halting) due to different error types.
	if err == errHalt {
		return godirwalk.Halt
	}
	return godirwalk.SkipNode
}

func getInternalMeta(opts *DirectoryInventoryOptions) (map[string]string, error) {
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
	return internalMeta, nil
}

func printMemUsageIncr(addedCount, div int) {
	if addedCount%div == 0 {
		log.Debug().
			Int("count", addedCount).
			Msg("Added files to inventory")
		printMemUsage()
	}
}

func walkDir(dir string, opts *DirectoryInventoryOptions, ret *DirectoryInventory) error {
	var addedCount int
	err := godirwalk.Walk(dir, &godirwalk.Options{
		FollowSymbolicLinks: opts.FollowSymlinks,
		Callback: func(path string, de *godirwalk.Dirent) error {
			// Skip top level directories from inventory
			// var err error
			if de.IsDir() {
				return nil
			}

			// We may need the original path again for a symlink later on
			ogPath := path
			// No symlink...dirs?
			if de.IsSymlink() {
				target, skip := shouldSkipSymlink(path)
				if skip {
					return nil
				}
				// Finally...
				if !opts.FollowSymlinks {
					return nil
				}
				path = target
			}

			// Finally look at the size
			st, err := os.Stat(path)
			if err != nil {
				return err
			}

			// Ignore certain items?
			name := de.Name()
			if filenameMatchesGlobs(name, opts.IgnoreGlobs) {
				return nil
			}
			ret.Files = append(ret.Files, &File{
				Path:        path,
				Destination: strings.TrimPrefix(ogPath, dir),
				Name:        name,
				Size:        st.Size(),
			})
			addedCount++

			// Print memory usage every X files
			printMemUsageIncr(addedCount, 1000)

			if herr := haltIfLimit(opts, addedCount); herr != nil {
				return herr
			}

			return nil
		},
		Unsorted:      true,
		ErrorCallback: errCallback,
	})
	if err != nil {
		return err
	}
	return nil
}

// shouldSkipSymlink returns the target of the symlink and a boolean on if it should be skipped
func shouldSkipSymlink(path string) (string, bool) {
	target, eerr := filepath.EvalSymlinks(path)
	if eerr != nil {
		log.Debug().Err(eerr).Msg("error evaluating symlink")
		return target, true
	}
	s, serr := os.Stat(target)
	if serr != nil {
		log.Warn().Err(serr).Msg("Error stating file")
		return target, true
	}
	// Finally, if a link to a dir...skip it always
	if s.IsDir() {
		return target, true
	}
	return target, false
}

func haltIfLimit(opts *DirectoryInventoryOptions, addedCount int) error {
	if opts.LimitFileCount > 0 && addedCount >= opts.LimitFileCount {
		log.Warn().Msg("Reached file count limit, stopping walk")
		return errHalt
	}
	return nil
}

// nonEmptyKeys returns the non-empty keys of a map in an array
func nonEmptyKeys[V any](m map[string]V) []string {
	var ret []string
	for k := range m {
		if k != "" {
			ret = append(ret, k)
		}
	}
	sort.Strings(ret)
	return ret
}

// reverseMap takes a map[k]v and returns a map[v]k
func reverseMap[K string, V string | Format](m map[K]V) map[V]K {
	ret := make(map[V]K, len(m))
	for k, v := range m {
		ret[v] = k
	}
	return ret
}

// convertDirsToAboluteDirs turns directories in to absolute path directories
func convertDirsToAboluteDirs(orig []string) ([]string, error) {
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

// isDirectory returns a bool if a file is a directory
func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

// filenameMatchesGlobs Check if a filename matches a set of globs
func filenameMatchesGlobs(filename string, globs []string) bool {
	for _, glob := range globs {
		if ok, _ := filepath.Match(glob, filename); ok {
			log.Debug().Str("path", filename).Msg("matched on file globbing")
			return true
		}
	}
	return false
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
