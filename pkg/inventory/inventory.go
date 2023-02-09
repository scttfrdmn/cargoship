/*
Package inventory provides the needed pieces to correctly create an Inventory of a directory
*/
package inventory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"reflect"
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

// DefaultSuitcaseFormat is just the default format we're going to use for a
// suitcase. Hopefully this fits for most use cases, but can always be
// overridden
const DefaultSuitcaseFormat string = "tar.zst"

type contextKey int

const (
	// InventoryKey is where the inventory lives. Hoping to get rid of most (if not all) other keys
	InventoryKey contextKey = iota
	// SuitcaseOptionsKey is the location for suitcase options
	SuitcaseOptionsKey
	// DestinationKey is the key for the target diretory of a suitcase operation
	DestinationKey
	// LogFileKey is the detination of the log file
	LogFileKey
	// HashesKey is the location of the hashes
	HashesKey
	// HashTypeKey is the location for a given hash type (sha1, md5, etc)
	HashTypeKey
	// UserOverrideKey is where the user overrides live
	UserOverrideKey
	// CLIMetaKey is where the CLI metadata lives
	CLIMetaKey
	// LogWriterKey is where the log writer goes
	LogWriterKey
)

var errHalt = errors.New("halt")

// HashAlgorithm is the hashing algorithm used for calculating file signatures
type HashAlgorithm int

const (
	// NullHash represents no hashing
	NullHash HashAlgorithm = iota
	// MD5Hash uses and md5 checksum
	MD5Hash
	// SHA1Hash is the sha-1 version of a signature
	SHA1Hash
	// SHA256Hash is the more secure sha-256 version of a signature
	SHA256Hash
	// SHA512Hash is most secure, but super slow, probably not useful here
	SHA512Hash
)

var hashMap map[string]HashAlgorithm = map[string]HashAlgorithm{
	"md5":    MD5Hash,
	"sha1":   SHA1Hash,
	"sha256": SHA256Hash,
	"sha512": SHA512Hash,
	"":       NullHash,
}

var hashHelp map[string]string = map[string]string{
	"md5":    "Fast but older hashing method, but usually fine for signatures",
	"sha1":   "Less intensive on CPUs than sha256, and more secure than md5",
	"sha256": "CPU intensive but very secure signature hashing",
	"sha512": "CPU intensive but very VERY secure signature hashing",
}

// HashCompletion returns shell completion
func HashCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	help := []string{}
	for _, format := range nonEmptyKeys(hashMap) {
		if strings.Contains(format, toComplete) {
			help = append(help, fmt.Sprintf("%v\t%v", format, hashHelp[format]))
		}
	}
	return help, cobra.ShellCompDirectiveNoFileComp
}

// String satisfies the pflags interface
func (h HashAlgorithm) String() string {
	m := reverseMap(hashMap)
	if v, ok := m[h]; ok {
		return v
	}
	panic("invalid hash algorithm")
}

// Type satisfies part of the pflags.Value interface
func (h HashAlgorithm) Type() string {
	return "HashAlgorithm"
}

// Set helps fulfill the pflag.Value interface
func (h *HashAlgorithm) Set(v string) error {
	if v, ok := hashMap[v]; ok {
		*h = v
		return nil
	}
	return fmt.Errorf("HashAlgorithm should be one of: %v", nonEmptyKeys(hashMap))
}

// MarshalJSON ensures that json conversions use the string value here, not the int value
func (h *HashAlgorithm) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%v\"", h.String())), nil
}

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

var formatMap map[string]Format = map[string]Format{
	"yaml": YAMLFormat,
	"json": JSONFormat,
	"":     NullFormat,
}

var formatHelp map[string]string = map[string]string{
	"yaml": "YAML is the preferred format. It allows for easy human readable inventories that can also be easily parsed by machines",
	"json": "JSON inventory is not very readable, but could allow for faster machine parsing under certain conditions",
}

// FormatCompletion returns shell completion
func FormatCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	help := []string{}
	for _, format := range nonEmptyKeys(formatMap) {
		if strings.Contains(format, toComplete) {
			help = append(help, fmt.Sprintf("%v\t%v", format, formatHelp[format]))
		}
	}
	return help, cobra.ShellCompDirectiveNoFileComp
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
	Files            []*File               `yaml:"files" json:"files"`
	Options          *Options              `yaml:"options" json:"options"`
	TotalIndexes     int                   `yaml:"total_indexes" json:"total_indexes"`
	IndexSummaries   map[int]*IndexSummary `yaml:"index_summaries" json:"index_summaries"`
	InternalMetadata map[string]string     `yaml:"internal_metadata" json:"internal_metadata"`
	ExternalMetadata map[string]string     `yaml:"external_metadata" json:"external_metadata"`
	CLIMeta          CLIMeta               `yaml:"cli_meta" json:"cli_meta"`
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
			Msg("🧳 suitcase archive created")
	}
	log.Info().
		Uint("file-count", totalC).
		Int64("file-size", totalS).
		Str("file-size-human", humanize.Bytes(uint64(totalS))).
		Msg("🧳 total suitcase archives")
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

// Options are the options used to create a DirectoryInventory
type Options struct {
	User                  string        `yaml:"user" json:"user"`
	Prefix                string        `yaml:"prefix" json:"prefix"`
	Directories           []string      `yaml:"top_level_directories" json:"top_level_directories"`
	SizeConsideredLarge   int64         `yaml:"size_considered_large" json:"size_considered_large"`
	MaxSuitcaseSize       int64         `yaml:"max_suitcase_size" json:"max_suitcase_size"`
	InternalMetadataGlob  string        `yaml:"internal_metadata_glob,omitempty" json:"internal_metadata_glob,omitempty"`
	IgnoreGlobs           []string      `yaml:"ignore_globs,omitempty" json:"ignore_globs,omitempty"`
	ExternalMetadataFiles []string      `yaml:"external_metadata_files,omitempty" json:"external_metadata_files,omitempty"`
	EncryptInner          bool          `yaml:"encrypt_inner" json:"encrypt_inner"`
	HashInner             bool          `yaml:"hash_inner" json:"hash_inner"`
	LimitFileCount        int           `yaml:"limit_file_count" json:"limit_file_count"`
	SuitcaseFormat        string        `yaml:"suitcase_format" json:"suitcase_format"`
	InventoryFormat       string        `yaml:"inventory_format" json:"inventory_format"`
	FollowSymlinks        bool          `yaml:"follow_symlinks" json:"follow_symlinks"`
	HashAlgorithm         HashAlgorithm `yaml:"hash_algorithm" json:"hash_algorithm"`
}

// AbsoluteDirectories converts the Directories entries to absolute paths
func (o *Options) AbsoluteDirectories() error {
	ad, err := convertDirsToAboluteDirs(o.Directories)
	if err != nil {
		return err
	}
	o.Directories = ad
	return nil
}

// WithIgnoreGlobs sets the IgnoreGlobs strings
func WithIgnoreGlobs(g []string) func(*Options) {
	return func(o *Options) {
		o.IgnoreGlobs = g
	}
}

// WithFollowSymlinks sets the FollowSymlinks option to true
func WithFollowSymlinks() func(*Options) {
	return func(o *Options) {
		o.FollowSymlinks = true
	}
}

// WithDirectories sets the top level directories to be suitcased up
func WithDirectories(d []string) func(*Options) {
	return func(o *Options) {
		o.Directories = d
	}
}

// WithInventoryFormat sets the format for the suitcases that will be generated
func WithInventoryFormat(f string) func(*Options) {
	return func(o *Options) {
		o.InventoryFormat = f
	}
}

// WithSuitcaseFormat sets the format for the suitcases that will be generated
func WithSuitcaseFormat(f string) func(*Options) {
	format := strings.TrimPrefix(f, ".")
	return func(o *Options) {
		if f != "" {
			o.SuitcaseFormat = format
		}
	}
}

// WithLimitFileCount sets the number of files to process before stopping. 0 means process them all
func WithLimitFileCount(c int) func(*Options) {
	return func(o *Options) {
		o.LimitFileCount = c
	}
}

// WithMaxSuitcaseSize sets the maximum size for any of the generated suitcases
func WithMaxSuitcaseSize(s int64) func(*Options) {
	return func(o *Options) {
		o.MaxSuitcaseSize = s
	}
}

// WithUser sets the user for an inventory option
func WithUser(u string) func(*Options) {
	return func(o *Options) {
		if u != "" {
			o.User = u
		}
	}
}

// WithPrefix sets the prefix for an inventory
func WithPrefix(p string) func(*Options) {
	return func(o *Options) {
		o.Prefix = p
	}
}

// WithHashAlgorithms sets the hashing algorithms to use for signatures
func WithHashAlgorithms(a HashAlgorithm) func(*Options) {
	return func(o *Options) {
		o.HashAlgorithm = a
	}
}

// NewOptions uses functional options to generatea DirectoryInventoryOptions object
func NewOptions(options ...func(*Options)) *Options {
	currentUser, err := user.Current()
	if err != nil {
		panic(err)
	}
	dio := &Options{
		SuitcaseFormat:  DefaultSuitcaseFormat,
		InventoryFormat: "yaml",
		User:            currentUser.Username,
		HashAlgorithm:   SHA1Hash,
	}
	for _, opt := range options {
		opt(dio)
	}
	err = dio.AbsoluteDirectories()
	if err != nil {
		panic(err)
	}
	return dio
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

// expandSuitcaseNames will fill in suitcase names for a given inventory
func (di *DirectoryInventory) expandSuitcaseNames() {
	for _, f := range di.Files {
		if f.SuitcaseName == "" {
			f.SuitcaseName = di.SuitcaseNameWithIndex(f.SuitcaseIndex)
		}
	}
}

// SuitcaseNames returns a list of suitcase names as strings
func (di DirectoryInventory) SuitcaseNames() []string {
	ret := make([]string, len(di.Files))

	for idx, f := range di.Files {
		ret[idx] = f.SuitcaseName
	}
	return ret
}

// UniqueSuitcaseNames returns a list of suitcase names as strings
func (di DirectoryInventory) UniqueSuitcaseNames() []string {
	ret := make([]string, di.TotalIndexes)

	for i := 0; i < di.TotalIndexes; i++ {
		ret[i] = di.SuitcaseNameWithIndex(i + 1)
	}
	return ret
}

// IndexWithSize Loops through inventory and assign suitcase indexes based on a
// given max size
func (di *DirectoryInventory) IndexWithSize(maxSize int64) error {
	caseSet := NewCaseSet(maxSize)
	numCases := 1
	// Sort by descending size
	sort.Slice(di.Files, func(i, j int) bool {
		return di.Files[i].Size > di.Files[j].Size
	})
	for _, item := range di.Files {
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
		if di.IndexSummaries == nil {
			di.IndexSummaries = map[int]*IndexSummary{}
		}

		if _, ok := di.IndexSummaries[item.SuitcaseIndex]; !ok {
			di.IndexSummaries[item.SuitcaseIndex] = &IndexSummary{}
		}
		s := di.IndexSummaries[item.SuitcaseIndex]
		s.Count++
		s.Size += item.Size
	}
	// Generate human readable total sizes
	for _, v := range di.IndexSummaries {
		v.HumanSize = humanize.Bytes(uint64(v.Size))
	}
	di.TotalIndexes = numCases
	di.expandSuitcaseNames()
	return nil
}

// WriteInventoryAndFileWithViper uses viper to write out an inventory file
func WriteInventoryAndFileWithViper(
	v *viper.Viper, cmd *cobra.Command, args []string, version string,
) (*DirectoryInventory, *os.File, error) {
	outDir := destinationWithCobra(cmd)
	i, f, ir, err := NewDirectoryInventoryAndFileAndInventoyerWithViper(v, cmd, args, outDir)
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
func NewDirectoryInventoryAndFileAndInventoyerWithViper(v *viper.Viper, cmd *cobra.Command, args []string, outDir string) (*DirectoryInventory, *os.File, Inventoryer, error) {
	if v == nil {
		panic("must pass viper to NewDirectoryInventoryAndFileAndInventoyerWithViper")
	}
	i, f, err := NewDirectoryInventoryAndFileWithViper(v, cmd, args, outDir)
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
func NewDirectoryInventoryAndFileWithViper(v *viper.Viper, cmd *cobra.Command, args []string, outDir string) (*DirectoryInventory, *os.File, error) {
	i, err := NewDirectoryInventoryWithViper(v, cmd, args)
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
func NewDirectoryInventoryWithViper(v *viper.Viper, cmd *cobra.Command, args []string) (*DirectoryInventory, error) {
	inventoryOpts := NewOptions(
		WithViper(v),
		WithCobra(cmd, args),
	)
	return NewDirectoryInventory(inventoryOpts)
}

// NewDirectoryInventory creates a new DirectoryInventory using options
func NewDirectoryInventory(opts *Options) (*DirectoryInventory, error) {
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
	if len(opts.Directories) == 0 {
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
			Strs("topLevelDirectories", opts.Directories).
			Msg("no metadata found")
	}

	for _, dir := range opts.Directories {
		if !isDirectory(dir) {
			log.Warn().Str("path", dir).Msg("top level directory does not exist")
			return nil, errors.New("not a directory")
		}
		log.Debug().Str("dir", dir).Msg("walking directory")
		err := walkDir(dir, opts, ret)
		if err != nil {
			if err.Error() != "halt" {
				return nil, err
			}
		}
	}
	if ierr := ret.IndexWithSize(opts.MaxSuitcaseSize); ierr != nil {
		return nil, ierr
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

// WithViper applies options from a Viper instance for options
func WithViper(v *viper.Viper) func(*Options) {
	if v == nil {
		return func(*Options) {}
	}
	return func(o *Options) {
		// Helper commands to work with both viper and cmd args
		setLimitFileCount(*v, o)
		setInternalMetadataGlob(*v, o)
		setPrefix(*v, o)
		setIgnoreGlobs(*v, o)
		setExternalMetadataFiles(*v, o)
		setEncryptInner(*v, o)
		setHashInner(*v, o)
		setFollowSymlinks(*v, o)
		setMaxSuitcaseSize(*v, o)
		setUser(*v, o)

		// Formats are a little funky...should we set them special?
		// Strip out leading dots
		format := strings.TrimPrefix(v.GetString("suitcase-format"), ".")
		if format != "" {
			o.SuitcaseFormat = format
		}
		iformat := strings.TrimPrefix(v.GetString("inventory-format"), ".")
		if iformat != "" {
			o.InventoryFormat = iformat
		}
	}
}

func setInternalMetadataGlob[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "internal-metadata-glob"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.InternalMetadataGlob = vi.GetString(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.InternalMetadataGlob = mustGetCmd[string](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setLimitFileCount[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "limit-file-count"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.LimitFileCount = vi.GetInt(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.LimitFileCount = mustGetCmd[int](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setPrefix[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "prefix"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.Prefix = vi.GetString(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.Prefix = mustGetCmd[string](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setIgnoreGlobs[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "ignore-glob"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.IgnoreGlobs = vi.GetStringSlice(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.IgnoreGlobs = mustGetCmd[[]string](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setExternalMetadataFiles[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "external-metadata-file"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.ExternalMetadataFiles = vi.GetStringSlice(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.ExternalMetadataFiles = mustGetCmd[[]string](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setEncryptInner[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "encrypt-inner"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.EncryptInner = vi.GetBool(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.EncryptInner = mustGetCmd[bool](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setHashInner[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "hash-inner"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.HashInner = vi.GetBool(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.HashInner = mustGetCmd[bool](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setFollowSymlinks[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "follow-symlinks"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.FollowSymlinks = vi.GetBool(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.FollowSymlinks = mustGetCmd[bool](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setMaxSuitcaseSize[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "max-suitcase-size"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.MaxSuitcaseSize = mustBytesFromHuman(vi.GetString(k))
		}
	case *cobra.Command:
		o.MaxSuitcaseSize = mustBytesFromHuman(mustGetCmd[string](mustGetCommand(v), k))
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setUser[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "user"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.User = vi.GetString(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.User = mustGetCmd[string](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func mustBytesFromHuman(h string) int64 {
	b, err := humanize.ParseBytes(h)
	if err != nil {
		panic(err)
	}
	return int64(b)
}

// WithCobra applies options using a cobra Command and args
func WithCobra(cmd *cobra.Command, args []string) func(*Options) {
	return func(o *Options) {
		setMaxSuitcaseSize(*cmd, o)
		setUser(*cmd, o)
		setFollowSymlinks(*cmd, o)
		setHashInner(*cmd, o)
		setEncryptInner(*cmd, o)
		setExternalMetadataFiles(*cmd, o)
		setIgnoreGlobs(*cmd, o)
		setPrefix(*cmd, o)
		setInternalMetadataGlob(*cmd, o)
		setLimitFileCount(*cmd, o)

		if len(args) > 0 {
			o.Directories = args
		}
	}
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

// userOverrideWithCobra returns a user override viper object from a cmd
func userOverrideWithCobra(cmd *cobra.Command) *viper.Viper {
	if cmd.Context() == nil {
		return &viper.Viper{}
	}
	v := cmd.Context().Value(UserOverrideKey)
	if v == nil {
		return &viper.Viper{}
	}
	vi, ok := v.(*viper.Viper)
	if !ok {
		return &viper.Viper{}
	}
	return vi
}

// destinationWithCobra returns a destination string from a cmd
func destinationWithCobra(cmd *cobra.Command) string {
	if cmd.Context() == nil {
		return mustTempDir()
	}
	v := cmd.Context().Value(DestinationKey)
	if v == nil {
		return mustTempDir()
	}
	d, ok := v.(string)
	if !ok {
		return ""
	}
	return d
}

// CreateOrReadInventory will either create a new inventory (if given an empty string), or read an existing one
func CreateOrReadInventory(inventoryFile string, cmd *cobra.Command, args []string, version string) (*DirectoryInventory, error) {
	// Create an inventory file if one isn't specified
	var inventoryD *DirectoryInventory
	if inventoryFile == "" {
		log.Debug().Msg("No inventory file specified, we're going to go ahead and create one")
		var outF *os.File
		var err error
		v := userOverrideWithCobra(cmd)
		outDir := destinationWithCobra(cmd)
		if outDir == "" {
			outDir = mustTempDir()
			var ctx context.Context
			if cmd.Context() == nil {
				ctx = context.Background()
			} else {
				ctx = cmd.Context()
			}

			cmd.SetContext(context.WithValue(ctx, DestinationKey, outDir))
		}
		// inventoryD, outF, err = WriteInventoryAndFileWithViper(v, cmd, args, outDir, version)
		inventoryD, outF, err = WriteInventoryAndFileWithViper(v, cmd, args, version)
		if err != nil {
			return nil, err
		}
		log.Debug().Str("file", outF.Name()).Msg("Created inventory file")
	} else {
		var err error
		inventoryD, err = NewInventoryWithFilename(inventoryFile)
		if err != nil {
			return nil, err
		}
	}
	// Store the inventory in context, so we can access it in the other run stages
	cmd.SetContext(context.WithValue(cmd.Context(), InventoryKey, inventoryD))
	inventoryD.SummaryLog()
	return inventoryD, nil
}

func mustTempDir() string {
	o, err := os.MkdirTemp("", "suitcasectl")
	if err != nil {
		panic(err)
	}
	return o
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

func printMemUsageIncr(addedCount, div int) {
	if addedCount%div == 0 {
		log.Debug().
			Int("count", addedCount).
			Msg("Added files to inventory")
		printMemUsage()
	}
}

func walkDir(dir string, opts *Options, ret *DirectoryInventory) error {
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

func haltIfLimit(opts *Options, addedCount int) error {
	log.Warn().Int("limit", opts.LimitFileCount).Int("added", addedCount)
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
func reverseMap[K string, V string | Format | HashAlgorithm](m map[K]V) map[V]K {
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

// SuitcaseNameWithIndex gives what the name of a suitcase file will be, given
// the index number
func (di *DirectoryInventory) SuitcaseNameWithIndex(i int) string {
	return fmt.Sprintf("%v-%v-%02d-of-%02d.%v", di.Options.Prefix, di.Options.User, i, di.TotalIndexes, di.Options.SuitcaseFormat)
}

func newInventoryCmd() *cobra.Command {
	cmd := &cobra.Command{}
	BindCobra(cmd)
	return cmd
}

// BindCobra binds the needed inventory bits to a cobra.Command
func BindCobra(cmd *cobra.Command) {
	cmd.PersistentFlags().Int("concurrency", 10, "Number of concurrent files to create")
	cmd.PersistentFlags().String("inventory-file", "", "Use the given inventory file to create the suitcase")
	cmd.PersistentFlags().String("max-suitcase-size", "0", "Maximum size for the set of suitcases generated. If no unit is specified, 'bytes' is assumed. 0 means no limit.")
	cmd.PersistentFlags().String("internal-metadata-glob", "suitcase-meta*", "Glob pattern for internal metadata files. This should be directly under the top level directories of the targets that are being packaged up. Multiple matches will be included if found.")
	cmd.PersistentFlags().StringArray("external-metadata-file", []string{}, "Additional files to include as metadata in the inventory. This should NOT be part of the suitcase target directories...use internal-metadata-glob for those")
	cmd.PersistentFlags().StringArray("ignore-glob", []string{}, "Ignore files matching this glob pattern. Can be specified multiple times")
	cmd.PersistentFlags().Bool("hash-inner", false, "Create SHA256 hashes for the inner contents of the suitcase")
	cmd.PersistentFlags().Bool("hash-outer", false, "Create SHA256 hashes for the container and metadata files")
	cmd.PersistentFlags().Bool("encrypt-inner", false, "Encrypt files within the suitcase")
	cmd.PersistentFlags().Bool("follow-symlinks", false, "Follow symlinks when traversing the target directories and files")
	cmd.PersistentFlags().Int("buffer-size", 1024, "Buffer size if using a YAML inventory.")
	cmd.PersistentFlags().Int("limit-file-count", 0, "Limit the number of files to include in the inventory. If 0, no limit is applied. Should only be used for debugging")
	cmd.PersistentFlags().String("user", "", "Username to insert into the suitcase filename. If omitted, we'll try and detect from the current user")
	cmd.PersistentFlags().String("prefix", "suitcase", "Prefix to insert into the suitcase filename")
	cmd.PersistentFlags().StringArrayP("public-key", "p", []string{}, "Public keys to use for encryption")
	cmd.PersistentFlags().Bool("exclude-systems-pubkeys", false, "By default, we will include the systems teams pubkeys, unless this option is specified")
	cmd.PersistentFlags().Bool("only-inventory", false, "Only generate the inventory file, skip the actual suitcase archive creation")

	/*
		// Could we get these in here??
		cmd.PersistentFlags().Var(&suitcaseFormat, "suitcase-format", "Format for the suitcase. Should be 'tar', 'tar.gpg', 'tar.gz' or 'tar.gz.gpg'")
		if err := cmd.RegisterFlagCompletionFunc("suitcase-format", suitcase.FormatCompletion); err != nil {
			panic(err)
		}
		cmd.PersistentFlags().Lookup("suitcase-format").DefValue = inventory.DefaultSuitcaseFormat

		// Inventory Format needs some extra love for auto complete
		cmd.PersistentFlags().Var(&inventoryFormat, "inventory-format", "Format for the inventory. Should be 'yaml' or 'json'")
		if err := cmd.RegisterFlagCompletionFunc("inventory-format", inventory.FormatCompletion); err != nil {
			panic(err)
		}
		cmd.PersistentFlags().Lookup("inventory-format").DefValue = "yaml"
	*/
}

// mustGetCmd uses generics to get a given flag with the appropriate Type from a cobra.Command
func mustGetCmd[T []int | int | []string | string | bool | time.Duration](cmd cobra.Command, s string) T {
	switch any(new(T)).(type) {
	case *int:
		item, err := cmd.Flags().GetInt(s)
		panicIfErr(err)
		return any(item).(T)
	case *string:
		item, err := cmd.Flags().GetString(s)
		panicIfErr(err)
		return any(item).(T)
	case *[]string:
		item, err := cmd.Flags().GetStringSlice(s)
		panicIfErr(err)
		return any(item).(T)
	case *bool:
		item, err := cmd.Flags().GetBool(s)
		panicIfErr(err)
		return any(item).(T)
	case *[]int:
		item, err := cmd.Flags().GetIntSlice(s)
		panicIfErr(err)
		return any(item).(T)
	case *time.Time:
		item, err := cmd.Flags().GetDuration(s)
		panicIfErr(err)
		return any(item).(T)
	default:
		panic(fmt.Sprintf("unexpected use of mustGetCmd: %v", reflect.TypeOf(s)))
	}
}

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

func mustGetViper(v any) viper.Viper {
	vi, ok := v.(viper.Viper)
	if !ok {
		panic("error getting viper.Viper from generic")
	}
	return vi
}

func mustGetCommand(v any) cobra.Command {
	ci, ok := v.(cobra.Command)
	if !ok {
		panic("error getting cobra.Command from generic")
	}
	return ci
}

// WithCmd returns the inventory object from a cobra command context
func WithCmd(cmd *cobra.Command) *DirectoryInventory {
	inv, ok := cmd.Context().Value(InventoryKey).(*DirectoryInventory)
	if !ok {
		panic("could not get inventory")
	}
	return inv
}
