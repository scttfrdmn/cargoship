/*
Package inventory provides the needed pieces to correctly create an Inventory of a directory
*/
package inventory

import (
	"context" // nolint
	json "encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/mholt/archiver/v4"
	"github.com/scttfrdmn/cargoship/pkg/plugins/transporters"
	"github.com/scttfrdmn/cargoship/pkg/plugins/transporters/cloud"
	"github.com/scttfrdmn/cargoship/pkg/plugins/transporters/shell"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/karrick/godirwalk"
)

// DefaultSuitcaseFormat is just the default format we're going to use for a
// suitcase. Hopefully this fits for most use cases, but can always be
// overridden
const DefaultSuitcaseFormat string = "tar.zst"

var errHalt = errors.New("halt")

// HashAlgorithm is the hashing algorithm used for calculating file signatures

// Inventoryer is an interface to define what an Inventory Operator does
type Inventoryer interface {
	Write(io.Writer, *Inventory) error
	Read([]byte) (*Inventory, error)
}

// Inventory is the inventory of a set of suitcases
type Inventory struct {
	Files            []*File               `yaml:"files" json:"files"`
	Options          *Options              `yaml:"options" json:"options"`
	TotalIndexes     int                   `yaml:"total_indexes" json:"total_indexes"`
	IndexSummaries   map[int]*IndexSummary `yaml:"index_summaries" json:"index_summaries"`
	InternalMetadata map[string]string     `yaml:"internal_metadata" json:"internal_metadata"`
	ExternalMetadata map[string]string     `yaml:"external_metadata" json:"external_metadata"`
	// CLIMeta          CLIMeta               `yaml:"cli_meta" json:"cli_meta"`
}

// MustJSONString returns the json representation as a string or panic
func (di Inventory) MustJSONString() string {
	j, err := di.JSONString()
	if err != nil {
		panic(err)
	}
	return j
}

// ValidateAccess ensures that we have access to all files in a given inventory
func (di Inventory) ValidateAccess() error {
	invalidFiles := []string{}
	for _, item := range di.Files {
		if !isFileReadable(item.Path) {
			invalidFiles = append(invalidFiles, item.Path)
		}
	}
	if len(invalidFiles) > 0 {
		return fmt.Errorf("the following files are not readable: %v", strings.Join(invalidFiles, ","))
	}
	return nil
}

// JSONString returns the inventory in JSON and an optional error
func (di Inventory) JSONString() (string, error) {
	j, err := json.Marshal(di)
	if err != nil {
		return "", err
	}
	return string(j), nil
}

// Analysis is some useful information about a given inventory
type Analysis struct {
	LargestFileSize   int64
	LargestFileSizeHR string
	FileCount         uint
	AverageFileSize   int64
	AverageFileSizeHR string
	TotalFileSize     int64
	TotalFileSizeHR   string
}

// Analyze examines an inventory and returns an Analysis object
func (di Inventory) Analyze() Analysis {
	if len(di.Files) == 0 {
		return Analysis{}
	}
	// Find biggest
	var largest File
	var fc uint  // file count
	var ts int64 // total size
	// allSizes := make([]int64, len(di.Files))
	for _, f := range di.Files {
		// allSizes[idx] = f.Size
		fc++
		ts += f.Size
		if f.Size > largest.Size {
			largest = *f
		}
	}

	avg := ts / int64(len(di.Files))
	return Analysis{
		LargestFileSize:   largest.Size,
		LargestFileSizeHR: humanize.Bytes(int64ToUint64(largest.Size)),
		FileCount:         fc,
		AverageFileSize:   avg,
		AverageFileSizeHR: humanize.Bytes(int64ToUint64(avg)),
		TotalFileSize:     ts,
		TotalFileSizeHR:   humanize.Bytes(int64ToUint64(ts)),
	}
}

func int64ToUint64(i int64) uint64 {
	if i < 0 {
		// Return 0 for negative values instead of panicking
		// Callers should validate values before calling
		return 0
	}
	return uint64(i)
}

// SummaryLog logs out a summary of the suitcase data
func (di Inventory) SummaryLog() {
	// Print some summary info about the index
	var totalC uint
	var totalS int64
	for k, item := range di.IndexSummaries {
		totalC += item.Count
		totalS += item.Size

		slog.Info("suitcase archive created",
			"index", k,
			"file-count", item.Count,
			"file-size", item.Size,
			"file-size-human", humanize.Bytes(int64ToUint64(item.Size)),
		)
	}
	slog.Info("total suitcase archives",
		"file-count", totalC,
		"file-size", totalS,
		"file-size-human", humanize.Bytes(int64ToUint64(totalS)),
	)
}

// IndexSummary will give an overall summary to a set of suitcases
type IndexSummary struct {
	Count     uint   `yaml:"count"`
	Size      int64  `yaml:"size"`
	HumanSize string `yaml:"human_size"`
}

// Options are the options used to create a DirectoryInventory

// File is a file item inside an inventory
type File struct {
	Path          string   `yaml:"path" json:"path"`
	Destination   string   `yaml:"destination" json:"destination"`
	Name          string   `yaml:"name" json:"name"`
	Size          int64    `yaml:"size" json:"size"`
	ArchiveTOC    []string `yaml:"archive_toc,omitempty" json:"archive_toc,omitempty"`
	SuitcaseIndex int      `yaml:"suitcase_index,omitempty" json:"suitcase_index,omitempty"`
	SuitcaseName  string   `yaml:"suitcase_name,omitempty" json:"suitcase_name,omitempty"`
}

// FileBucket describes what a filebucket state is
type FileBucket struct {
	Free int64
}

// expandSuitcaseNames will fill in suitcase names for a given inventory
func (di *Inventory) expandSuitcaseNames() {
	for _, f := range di.Files {
		if f.SuitcaseName == "" {
			f.SuitcaseName = di.SuitcaseNameWithIndex(f.SuitcaseIndex)
		}
	}
}

// SuitcaseNames returns a list of suitcase names as strings
func (di Inventory) SuitcaseNames() []string {
	ret := make([]string, len(di.Files))

	for idx, f := range di.Files {
		ret[idx] = f.SuitcaseName
	}
	return ret
}

// UniqueSuitcaseNames returns a list of suitcase names as strings
func (di Inventory) UniqueSuitcaseNames() []string {
	ret := make([]string, di.TotalIndexes)

	for i := 0; i < di.TotalIndexes; i++ {
		ret[i] = di.SuitcaseNameWithIndex(i + 1)
	}
	return ret
}

// IndexWithSize Loops through inventory and assign suitcase indexes based on a
// given max size
func (di *Inventory) IndexWithSize(maxSize int64) error {
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
				slog.Debug("index is full, adding new index",
					"path", item.Path,
					"size", item.Size,
					"numCases", numCases,
				)
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
		v.HumanSize = humanize.Bytes(int64ToUint64(v.Size))
	}
	di.TotalIndexes = numCases
	di.expandSuitcaseNames()
	return nil
}

// NewDirectoryInventory creates a new DirectoryInventory using options
func NewDirectoryInventory(opts *Options) (*Inventory, error) {
	ret := &Inventory{
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
		log.Debug("no metadata found",
			"internal-glob", opts.InternalMetadataGlob,
			"external-files", opts.ExternalMetadataFiles,
			"topLevelDirectories", opts.Directories,
		)
	}

	for _, dir := range opts.Directories {
		if !isDirectory(dir) {
			log.Warn("top level directory does not exist", "directory", dir)
			return nil, errors.New("not a directory")
		}
		log.Debug("walking directory", "directory", dir)
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

func printMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	slog.Debug("memory usage in MB",
		"allocated", m.Alloc,
		"total-allocated", m.TotalAlloc,
		"allocated-percent", (float64(m.Alloc)/float64(m.TotalAlloc))*float64(100),
		"system", m.Sys,
		"gc-count", uint64(m.NumGC),
	)
}

// NewInventoryWithFilename returns a new DirectoryInventory from an inventory File
func NewInventoryWithFilename(s string) (*Inventory, error) {
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
	case ".yaml", ".yml":
		ir = &VAMLer{}
	default:
		return nil, fmt.Errorf("unsupported file extension %s", ext)
	}
	return ir, nil
}

// WizardForm are the little fields from our nice lil wizard 🧙
type WizardForm struct {
	Destination      string
	Source           string
	TravelAgentToken string
	MaxSize          string
}

// WithWizardForm sets up data from a wizard form
func WithWizardForm(f WizardForm) func(*Options) {
	return func(o *Options) {
		o.Directories = []string{f.Source}
	}
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
		setArchiveTOC(*v, o)
		setArchiveTOCDeep(*v, o)
		setFollowSymlinks(*v, o)
		setMaxSuitcaseSize(*v, o)
		setUser(*v, o)
		// setTransportPlugin(*v, o)
		setCloudDestination(*v, o)
		setShellDestination(*v, o)

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

func setArchiveTOCDeep[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "archive-toc-deep"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.IncludeArchiveTOCDeep = vi.GetBool(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.IncludeArchiveTOCDeep = mustGetCmd[bool](ci, k)
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setArchiveTOC[T viper.Viper | cobra.Command](v T, o *Options) {
	k := "archive-toc"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.IncludeArchiveTOC = vi.GetBool(k)
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.IncludeArchiveTOC = mustGetCmd[bool](ci, k)
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

func setCloudDestination[T viper.Viper | cobra.Command](v T, o *Options) { //nolint:dupl
	k := "cloud-destination"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.TransportPlugin = &cloud.Transporter{
				Config: transporters.Config{
					Destination: vi.GetString(k),
				},
			}
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.TransportPlugin = &cloud.Transporter{
				Config: transporters.Config{
					Destination: mustGetCmd[string](ci, k),
				},
			}
		}
	default:
		panic(fmt.Sprintf("unexpected use of set %v", k))
	}
}

func setShellDestination[T viper.Viper | cobra.Command](v T, o *Options) { // nolint:dupl
	k := "shell-destination"
	switch any(new(T)).(type) {
	case *viper.Viper:
		vi := mustGetViper(v)
		if vi.IsSet(k) {
			o.TransportPlugin = &shell.Transporter{Config: transporters.Config{Destination: vi.GetString(k)}}
		}
	case *cobra.Command:
		ci := mustGetCommand(v)
		if ci.Flags().Changed(k) {
			o.TransportPlugin = &shell.Transporter{Config: transporters.Config{Destination: mustGetCmd[string](ci, k)}}
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
			size, err := bytesFromHuman(vi.GetString(k))
			if err != nil {
				panic(fmt.Errorf("invalid max-suitcase-size from viper: %w", err))
			}
			o.MaxSuitcaseSize = size
		}
	case *cobra.Command:
		cmdStr := mustGetCmd[string](mustGetCommand(v), k)
		size, err := bytesFromHuman(cmdStr)
		if err != nil {
			panic(fmt.Errorf("invalid max-suitcase-size from command: %w", err))
		}
		o.MaxSuitcaseSize = size
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

func bytesFromHuman(h string) (int64, error) {
	b, err := humanize.ParseBytes(h)
	if err != nil {
		return 0, err
	}
	return uint64ToInt64(b)
}

func uint64ToInt64(u uint64) (int64, error) {
	if u > math.MaxInt64 {
		return 0, errors.New("value out of range for int64")
	}
	return int64(u), nil
}

// WithCobra applies options using a cobra Command and args
func WithCobra(cmd *cobra.Command, args []string) func(*Options) {
	return func(o *Options) {
		setMaxSuitcaseSize(*cmd, o)
		setUser(*cmd, o)
		setFollowSymlinks(*cmd, o)
		setHashInner(*cmd, o)
		setArchiveTOC(*cmd, o)
		setArchiveTOCDeep(*cmd, o)
		setEncryptInner(*cmd, o)
		setExternalMetadataFiles(*cmd, o)
		setIgnoreGlobs(*cmd, o)
		setPrefix(*cmd, o)
		setInternalMetadataGlob(*cmd, o)
		setLimitFileCount(*cmd, o)
		// setTransportPlugin(*cmd, o)
		setCloudDestination(*cmd, o)
		setShellDestination(*cmd, o)

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

/*
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

func mustTempDir() string {
	o, err := os.MkdirTemp("", "cargoship")
	if err != nil {
		panic(err)
	}
	return o
}
*/

func checkItemSize(item *File, maxSize int64) error {
	if item.Size > maxSize {
		log.Warn("file is too large for suitcase",
			"path", item.Path,
			"size", item.Size,
			"maxSize", maxSize,
		)
		return errors.New("index contains at least one file that is too large")
	}
	return nil
}

func errCallback(_ string, err error) godirwalk.ErrorAction {
	// Desired way, but currently wrong (not halting) due to different error types.
	if err == errHalt {
		return godirwalk.Halt
	}
	return godirwalk.SkipNode
}

/*
func printMemUsageIncr(addedCount, div int) {
	if addedCount%div == 0 {
		log.Debug().
			Int("count", addedCount).
			Msg("Added files to inventory")
		printMemUsage()
	}
}
*/

func isFileReadable(filePath string) bool {
	// Open the file in read-only mode
	file, err := os.Open(path.Clean(filePath))
	if err != nil {
		// Error opening the file, it may not be readable
		return false
	}
	defer dclose(file)

	// Check if the file can be read
	_, err = file.Read([]byte{0})
	if err == io.EOF {
		return true
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
	}
	return err == nil
}

func walkDir(dir string, opts *Options, ret *Inventory) error {
	var addedCount int
	if err := godirwalk.Walk(dir, &godirwalk.Options{
		FollowSymbolicLinks: opts.FollowSymlinks,
		Callback: func(path string, de *godirwalk.Dirent) error {
			if de.IsDir() { // Skip top level directories from inventory
				return nil
			}

			ogPath := path
			if de.IsSymlink() {
				target, skip := shouldSkipSymlink(path)
				if skip || !opts.FollowSymlinks {
					return nil
				}
				path = target
			}

			st := mustStat(path)

			name := de.Name()
			if filenameMatchesGlobs(name, opts.IgnoreGlobs) {
				return nil
			}

			invf := &File{
				Path:        path,
				Destination: strings.TrimPrefix(ogPath, dir),
				Name:        name,
				Size:        st.Size(),
			}
			// if opts.IncludeArchiveTOC || opts.IncludeArchiveTOCDeep {
			if opts.IncludeArchiveTOCDeep || (opts.IncludeArchiveTOC && isTOCAble(path)) {
				var aerr error
				if invf.ArchiveTOC, aerr = ArchiveTOC(path); aerr != nil {
					slog.Debug("error attemping to look at table of contents in file", "file", path)
				}
			}

			ret.Files = append(ret.Files, invf)
			addedCount++

			if herr := haltIfLimit(opts, addedCount); herr != nil {
				return herr
			}

			return nil
		},
		Unsorted:      true,
		ErrorCallback: errCallback,
	}); err != nil {
		return err
	}
	return nil
}

// shouldSkipSymlink returns the target of the symlink and a boolean on if it should be skipped
func shouldSkipSymlink(path string) (string, bool) {
	target, eerr := filepath.EvalSymlinks(path)
	if eerr != nil {
		slog.Debug("error evaluating symlink", "error", eerr)
		return target, true
	}
	s, serr := os.Stat(target)
	if serr != nil {
		slog.Debug("error stating file", "error", serr)
		return target, true
	}
	// Finally, if a link to a dir...skip it always
	if s.IsDir() {
		return target, true
	}
	return target, false
}

func haltIfLimit(opts *Options, addedCount int) error {
	if opts.LimitFileCount > 0 && addedCount >= opts.LimitFileCount {
		slog.Warn("Reached file count limit, stopping walk", "limit", opts.LimitFileCount, "added", addedCount)
		return errHalt
	}
	return nil
}

// nonEmptyKeys returns the non-empty keys of a map in an array

// convertDirsToAboluteDirs turns directories in to absolute path directories

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
			slog.Debug("matched on file globbing", "path", filename)
			return true
		}
	}
	return false
}

// SuitcaseNameWithIndex gives what the name of a suitcase file will be, given
// the index number
func (di *Inventory) SuitcaseNameWithIndex(i int) string {
	return fmt.Sprintf("%v-%v-%02d-of-%02d.%v", di.Options.Prefix, di.Options.User, i, di.TotalIndexes, di.Options.SuitcaseFormat)
}

// NewInventoryCmd is a shortcut for an inventory command
func NewInventoryCmd() *cobra.Command {
	cmd := &cobra.Command{}
	BindCobra(cmd)
	return cmd
}

// BindCobra binds the needed inventory bits to a cobra.Command
func BindCobra(cmd *cobra.Command) {
	cmd.PersistentFlags().Int("concurrency", 10, "Number of concurrent files to create")
	cmd.PersistentFlags().String("inventory-file", "", "Use the given inventory file to create the suitcase")
	cmd.PersistentFlags().String("max-suitcase-size", "500GiB", "Maximum size for the set of suitcases generated. If no unit is specified, 'bytes' is assumed. 0 means no limit.")
	cmd.PersistentFlags().String("internal-metadata-glob", "suitcase-meta*", "Glob pattern for internal metadata files. This should be directly under the top level directories of the targets that are being packaged up. Multiple matches will be included if found.")
	cmd.PersistentFlags().StringArray("external-metadata-file", []string{}, "Additional files to include as metadata in the inventory. This should NOT be part of the suitcase target directories...use internal-metadata-glob for those")
	cmd.PersistentFlags().StringArray("ignore-glob", []string{}, "Ignore files matching this glob pattern. Can be specified multiple times")
	cmd.PersistentFlags().Bool("hash-inner", false, "Create hashes for the inner contents of the suitcase")
	cmd.PersistentFlags().Bool("hash-outer", true, "Create hashes for the container and metadata files. Disable with --hash-outer=false")
	cmd.PersistentFlags().Bool("encrypt-inner", false, "Encrypt files within the suitcase")
	cmd.PersistentFlags().Bool("follow-symlinks", false, "Follow symlinks when traversing the target directories and files")
	cmd.PersistentFlags().Int("buffer-size", 1024, "Buffer size if using a YAML inventory.")
	cmd.PersistentFlags().Int("limit-file-count", 0, "Limit the number of files to include in the inventory. If 0, no limit is applied. Should only be used for debugging")
	cmd.PersistentFlags().String("user", "", "Username to insert into the suitcase filename. If omitted, we'll try and detect from the current user")
	cmd.PersistentFlags().String("prefix", "suitcase", "Prefix to insert into the suitcase filename")
	cmd.PersistentFlags().StringArrayP("public-key", "p", []string{}, "Public keys to use for encryption")
	cmd.PersistentFlags().Bool("exclude-systems-pubkeys", false, "By default, we will include the systems teams pubkeys, unless this option is specified")
	cmd.PersistentFlags().Bool("only-inventory", false, "Only generate the inventory file, skip the actual suitcase archive creation")
	cmd.PersistentFlags().Bool("archive-toc", false, "Also include the Table-of-Contents for supported archives, such as zip, tar, etc in the inventory")
	cmd.PersistentFlags().Bool("archive-toc-deep", false, "Also include the Table-of-Contents for supported archives. This will look at any file, regardless of extension")
	// cmd.PersistentFlags().String("transport-plugin", "", "Transport plugin to use (if any). Options: shell, rclone...")
	cmd.PersistentFlags().String("cloud-destination", "", "Send files to this cloud destination after creation. Destination must be a valid rclone location.")
	cmd.PersistentFlags().String("shell-destination", "", "Send files through this shell destination after creation.")
	cmd.PersistentFlags().Int("retry-count", 5, "Number of times to retry a failed operation.")
	cmd.PersistentFlags().Duration("retry-interval", 1*time.Second, "How long to wait between retries.")
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

// SearchFileMatches is a listing of files that match a given search
type SearchFileMatches []File

type (
	// SearchDirMatches is a recursive listing of what is contained in a directory
	SearchDirMatches []SearchDirMatch
	// SearchDirMatch is a single directory match
	SearchDirMatch struct {
		Directory   string
		TotalSize   uint64
		TotalSizeHR string
		Suitcases   []string
	}
)

// SearchResults is all the stuff that a given search gets back
type SearchResults struct {
	Files       SearchFileMatches
	Directories SearchDirMatches
}

// Search iterates through files inside of an inventory and returns a set of
// results
func (di Inventory) Search(p string) SearchResults {
	r := SearchResults{}
	// First check files
	fm := SearchFileMatches{}
	var possibleDirs []File
	for _, f := range di.Files {
		dirName, fileName := filepath.Split(f.Destination)
		if strings.Contains(strings.ToLower(fileName), strings.ToLower(p)) {
			fm = append(fm, *f)
		}

		// How about table of contents files?
		for _, toc := range f.ArchiveTOC {
			if strings.Contains(strings.ToLower(toc), strings.ToLower(p)) {
				fm = append(fm, *f)
				break
			}
		}

		// Now look at dirs
		if strings.Contains(strings.ToLower(dirName), strings.ToLower(p)) {
			possibleDirs = append(possibleDirs, *f)
		}
	}
	r.Files = fm

	md := uniqDirsWithFiles(possibleDirs, p)
	for _, pos := range md {
		size, suitcases := dirSummary(possibleDirs, pos)
		r.Directories = append(r.Directories, SearchDirMatch{
			// Directory:   strings.TrimSuffix(pos[0:strings.Index(pos, p)+len(p)], "/"),
			Directory:   pos,
			TotalSize:   size,
			TotalSizeHR: humanize.Bytes(size),
			Suitcases:   suitcases,
		})
		humanize.Bytes(size)
	}

	return r
}

// Return total size and suitcases for a given directory
func dirSummary(all []File, p string) (uint64, []string) {
	var s uint64
	var scs []string
	for _, f := range all {
		if strings.HasPrefix(f.Destination, p) {
			s += int64ToUint64(f.Size)
			if !containsString(scs, f.SuitcaseName) {
				scs = append(scs, f.SuitcaseName)
			}
		}
	}
	return s, scs
}

func uniqDirsWithFiles(files []File, p string) []string {
	all := []string{}
	for _, f := range files {
		all = append(all, f.Destination)
	}
	return uniqDirs(all, p)
}

// Given a set of directories, return the unique prefixes
func uniqDirs(dirs []string, p string) []string {
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) < len(dirs[j])
	})

	res := []string{}
	for _, cd := range dirs {
		m := strings.TrimSuffix(cd[0:strings.Index(cd, p)+len(p)], "/")
		if !containsString(res, m) {
			res = append(res, m)
		}
	}
	return res
}

func containsString(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// ArchiveTOC is a v4 TOC generator for archiver
func ArchiveTOC(fn string) ([]string, error) {
	fsys, err := archiver.FileSystem(context.Background(), fn)
	if err != nil {
		return nil, err
	}
	ret := []string{}
	log := slog.With("archive", fn)

	// Track visited paths to prevent infinite recursion in self-referential archives
	visitedPaths := make(map[string]bool)
	maxDepth := 1000   // Conservative depth limit
	maxFiles := 100000 // Maximum files to prevent runaway processing
	fileCount := 0

	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Check file count limit
		fileCount++
		if fileCount > maxFiles {
			log.Warn("maximum file count exceeded, stopping walk", "path", path, "count", fileCount)
			return fmt.Errorf("archive contains too many files (>%d)", maxFiles)
		}

		// Count directory depth
		depth := strings.Count(path, "/")
		if depth > maxDepth {
			log.Warn("maximum depth exceeded, stopping walk", "path", path, "depth", depth)
			return fs.SkipDir
		}

		// Track visited paths to detect cycles
		if visitedPaths[path] {
			log.Warn("cycle detected, skipping path", "path", path)
			return fs.SkipDir
		}
		visitedPaths[path] = true

		// Handle: https://github.com/mholt/archiver/issues/383
		// Detect self-referential archives that cause infinite recursion
		if (path == ".") && d.Name() == "." && strings.Contains(fn, ".tar") {
			log.Debug("detected potentially problematic self-referential archive", "archive", fn)
			// Continue with extra safety checks
		}

		log.Debug("examining path", "path", path, "depth", depth)
		if !d.IsDir() {
			ret = append(ret, path)
			return nil
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Junky way to check this...
	if (len(ret) == 1) && (ret[0] == ".") {
		return nil, errors.New("could not scan a non archive file")
	}

	// I think we want to do this...
	sort.Strings(ret)
	return ret, nil
}

func mustStat(path string) fs.FileInfo {
	st, err := os.Stat(path)
	panicIfErr(err)
	return st
}

// Collection is map of inventory paths to Inventory objects
type Collection map[string]Inventory

// CollectionWithDirs returns a Collection of inventories using a list of directories
func CollectionWithDirs(d []string) (*Collection, error) {
	ret := Collection{}
	for _, di := range d {
		err := filepath.WalkDir(di, func(path string, _ fs.DirEntry, _ error) error {
			if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
				i, err := NewInventoryWithFilename(path)
				if err != nil {
					log.Debug("ignoring file as it did not load as an inventory", "file", path)
				}
				ret[path] = *i
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return &ret, nil
}

var tocExts = []string{
	".tar", ".br", ".bz2", ".zip", ".gz", ".lz4",
	".sz", ".xz", ".zz", ".zst", ".rar", ".7z",
}

func isTOCAble(s string) bool {
	for _, ext := range tocExts {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

func dclose(c io.Closer) {
	if err := c.Close(); err != nil {
		fmt.Fprint(os.Stderr, "could not close file")
	}
}
