/*
Package inventory provides the needed pieces to correctly create an Inventory of a directory
*/
package inventory

import (
	"fmt"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/scttfrdmn/cargoship/pkg/plugins/transporters"
)

// Options represents all configuration options for inventory creation
type Options struct {
	User                  string                   `yaml:"user" json:"user"`
	Prefix                string                   `yaml:"prefix" json:"prefix"`
	Directories           []string                 `yaml:"top_level_directories" json:"top_level_directories"`
	SizeConsideredLarge   int64                    `yaml:"size_considered_large" json:"size_considered_large"`
	MaxSuitcaseSize       int64                    `yaml:"max_suitcase_size" json:"max_suitcase_size"`
	InternalMetadataGlob  string                   `yaml:"internal_metadata_glob,omitempty" json:"internal_metadata_glob,omitempty"`
	IgnoreGlobs           []string                 `yaml:"ignore_globs,omitempty" json:"ignore_globs,omitempty"`
	ExternalMetadataFiles []string                 `yaml:"external_metadata_files,omitempty" json:"external_metadata_files,omitempty"`
	EncryptInner          bool                     `yaml:"encrypt_inner" json:"encrypt_inner"`
	HashInner             bool                     `yaml:"hash_inner" json:"hash_inner"`
	LimitFileCount        int                      `yaml:"limit_file_count" json:"limit_file_count"`
	SuitcaseFormat        string                   `yaml:"suitcase_format" json:"suitcase_format"`
	InventoryFormat       string                   `yaml:"inventory_format" json:"inventory_format"`
	FollowSymlinks        bool                     `yaml:"follow_symlinks" json:"follow_symlinks"`
	HashAlgorithm         HashAlgorithm            `yaml:"hash_algorithm" json:"hash_algorithm"`
	IncludeArchiveTOC     bool                     `yaml:"include_archive_toc" json:"include_archive_toc"`
	IncludeArchiveTOCDeep bool                     `yaml:"include_archive_toc_deep" json:"include_archive_toc_deep"`
	TransportPlugin       transporters.Transporter `yaml:"transport_plugin" json:"transport_plugin"`
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

// convertDirsToAboluteDirs converts relative directory paths to absolute paths
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

// Functional Options Pattern

// WithArchiveTOC enables table of contents in the archive file inventory. This only checks files with a known archive extension.
func WithArchiveTOC() func(*Options) {
	return func(o *Options) {
		o.IncludeArchiveTOC = true
	}
}

// WithArchiveTOCDeep enables table of contents in the archive file inventory. This checks every file, regardless of extension.
func WithArchiveTOCDeep() func(*Options) {
	return func(o *Options) {
		o.IncludeArchiveTOCDeep = true
	}
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

// NewOptions uses functional options to generate a DirectoryInventoryOptions object
// Returns error if unable to get current user or convert directories to absolute paths
func NewOptions(options ...func(*Options)) (*Options, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	dio := &Options{
		SuitcaseFormat:  DefaultSuitcaseFormat,
		InventoryFormat: "yaml",
		User:            currentUser.Username,
		HashAlgorithm:   MD5Hash,
	}
	for _, opt := range options {
		opt(dio)
	}
	if err := dio.AbsoluteDirectories(); err != nil {
		return nil, fmt.Errorf("failed to convert directories to absolute paths: %w", err)
	}
	return dio, nil
}
