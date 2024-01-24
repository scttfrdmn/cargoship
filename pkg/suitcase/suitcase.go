/*
Package suitcase holds all the stuff necessary for a suitecase generation
*/
package suitcase

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase/tar"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase/tarbz2"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase/targpg"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase/targz"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase/targzgpg"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase/tarzstd"
)

// Format is the format the inventory will use, such as yaml, json, etc
type Format int

const (
	// NullFormat is the unset value for this type
	NullFormat = iota
	// TarFormat is for tar
	TarFormat
	// TarGzFormat is for tar.gz
	TarGzFormat
	// TarGzGpgFormat is for encrypted tar.gz (tar.gz.gpg)
	TarGzGpgFormat
	// TarGpgFormat is for encrypted tar.gz (tar.gpg)
	TarGpgFormat
	// TarZstFormat uses the zstd compression engine (tar.zst)
	TarZstFormat
	// TarZstGpgFormat uses the zstd compression engine with Gpg (tar.zst.gpg)
	TarZstGpgFormat
)

var formatMap = map[string]Format{
	"tar":         TarFormat,
	"tar.gpg":     TarGpgFormat,
	"tar.gz":      TarGzFormat,
	"tar.gz.gpg":  TarGzGpgFormat,
	"tar.zst":     TarZstFormat,
	"tar.zst.gpg": TarZstGpgFormat,
	"":            NullFormat,
}

// FormatCompletion returns shell completion
func FormatCompletion(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ret := []string{}
	for _, item := range nonEmptyKeys(formatMap) {
		if strings.Contains(item, toComplete) {
			ret = append(ret, item)
		}
	}
	return ret, cobra.ShellCompDirectiveNoFileComp
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

// Suitcase is the interface that describes what a Suitcase does
type Suitcase interface {
	Close() error
	Add(inventory.File) (*config.HashSet, error)
	AddEncrypt(f inventory.File) error
	Config() *config.SuitCaseOpts
}

// FillState is the current state of a suitcase file
type FillState struct {
	Current        uint
	Total          uint
	Completed      bool
	CurrentPercent float64
	Index          int
}

// New Create a new suitcase
func New(w io.Writer, opts *config.SuitCaseOpts) (Suitcase, error) {
	// Decide if we are encrypting the whole shebang or not
	if strings.HasSuffix(opts.Format, ".gpg") {
		opts.EncryptOuter = true
	}
	// We may want to allow this later...but not yet
	if opts.EncryptInner && opts.EncryptOuter {
		return nil, fmt.Errorf("cannot encrypt inner and outer")
	}
	// If we are encrypting something, be sure encryptTo is set
	if (opts.EncryptInner || opts.EncryptOuter) && opts.EncryptTo == nil {
		return nil, fmt.Errorf("cannot encrypt without EncryptTo")
	}
	switch opts.Format {
	case "tar":
		return tar.New(w, opts), nil
	case "tar.gpg":
		return targpg.New(w, opts), nil
	case "tar.gz":
		return targz.New(w, opts), nil
	case "tar.gz.gpg":
		return targzgpg.New(w, opts), nil
	case "tar.zst":
		return tarzstd.New(w, opts), nil
	case "tar.bz2":
		return tarbz2.New(w, opts), nil
	}
	return nil, fmt.Errorf("invalid archive format: %s", opts.Format)
}

// FillWithInventoryIndex fills up a suitcase using the given inventory
func FillWithInventoryIndex(s Suitcase, i *inventory.Inventory, index int, stateC chan FillState) ([]config.HashSet, error) {
	if i == nil {
		return nil, errors.New("inventory is nil")
	}
	var err error

	var total uint
	if index > 0 {
		if _, ok := i.IndexSummaries[index]; ok {
			total = i.IndexSummaries[index].Count
		}
	} else {
		total = uint(len(i.Files))
	}
	cur := uint(0)
	var suitcaseHashes []config.HashSet

	for _, f := range i.Files {
		l := slog.With(
			"path", f.Path,
			"index", index)
		if f.SuitcaseIndex != index {
			continue
		}

		l.Debug("Adding file to suitcase",
			"cur", cur,
			"total", total,
		)

		if s.Config().EncryptInner {
			err = s.AddEncrypt(*f)
			if err != nil {
				return nil, fmt.Errorf("encountered error adding file to suitcase: %v", err)
			}
		} else {
			hs, err := s.Add(*f)
			if err != nil {
				return nil, fmt.Errorf("encountered error adding file to suitcase: %v", err)
			}
			if s.Config().HashInner {
				suitcaseHashes = append(suitcaseHashes, *hs)
			}
		}

		cur++
		if stateC != nil {
			stateC <- FillState{
				Current:        cur,
				Total:          total,
				Index:          index,
				CurrentPercent: float64(cur) / float64(total) * 100,
			}
		}
	}
	return suitcaseHashes, nil
}

// validateSuitcase checks a suitcase file against an inventory, and ensures it is up to date
func validateSuitcase(s string, i inventory.Inventory, idx int) bool {
	log := slog.With("suitcase", s)
	reqFiles := map[string]bool{}
	for _, item := range i.Files {
		if item.SuitcaseIndex == idx {
			reqFiles[item.Destination] = false
		}
	}
	log.Debug("about to get TOC")
	toc, err := inventory.ArchiveTOC(s)
	if err != nil {
		log.Debug("file appears to be corrupted, we'll recreate it if needed", "suitcase", s)
		return false
	}
	for _, item := range toc {
		reqFiles[item] = true
	}

	for k, v := range reqFiles {
		if !v {
			log.Debug("found suitcase but appears to be incomplete, we'll recreate it if needed", "suitcase", s, "file", k)
			return false
		}
	}
	return true
}

func inProcessName(s string) string {
	return path.Join(path.Dir(s), fmt.Sprintf(".__creating-%v", path.Base(s)))
}

/*
// WriteSuitcaseFile will write out the suitcase
//
// Deprecated: Use WriteSuitcaseFile from the porter object instead
func WriteSuitcaseFile(so *config.SuitCaseOpts, i *inventory.Inventory, index int, stateC chan FillState) (string, error) {
	if i == nil {
		return "", errors.New("inventory must not be nil in WriteSuitcaseFile")
	}
	if so == nil {
		return "", errors.New("suitcaseopts must not be nil in WriteSuitcaseFile")
	}
	targetFn := path.Join(so.Destination, i.SuitcaseNameWithIndex(index))
	log := slog.With("suitcase", targetFn)
	if fileExists(targetFn) {
		return targetFn, nil
	}

	tmpTargetFn := inProcessName(targetFn)
	target, err := os.Create(tmpTargetFn) // nolint:gosec
	if err != nil {
		return "", err
	}
	defer func() {
		if terr := target.Close(); terr != nil {
			panic(terr)
		}
	}()

	s, err := New(target, so)
	if err != nil {
		return "", err
	}
	defer dclose(s)

	log.Debug("Filling suitcase", "destination", targetFn, "format", so.Format, "encrypt-inner", so.EncryptInner)
	hashes, err := FillWithInventoryIndex(s, i, index, stateC)
	if err != nil {
		return "", err
	}

	if stateC != nil {
		// This is hanging... maybe?
		stateC <- FillState{
			Completed: true,
			Index:     index,
		}
	}

	if so.HashInner {
		if err := hashInner(targetFn, i.Options.HashAlgorithm, hashes); err != nil {
			return "", err
		}
	}

	if err := os.Rename(tmpTargetFn, targetFn); err != nil {
		return "", err
	}

	return targetFn, nil
}
*/

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

func mustHexToBin(s string) string {
	got, err := hexToBin(s)
	if err != nil {
		panic(err)
	}
	return got
}

func hexToBin(s string) (string, error) {
	data, err := hex.DecodeString(s)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// WriteHashFileBin  writes out the hashset array to an io.Writer
func WriteHashFileBin(hs []config.HashSet, o io.Writer) error {
	w := bufio.NewWriter(o)
	for _, hs := range hs {
		hx, err := hexToBin(hs.Hash)
		if err != nil {
			return err
		}
		if _, err := w.WriteString(fmt.Sprintf("%s\t%s\n", hs.Filename, hx)); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return nil
}

// WriteHashFile  writes out the hashset array to an io.Writer
func WriteHashFile(hs []config.HashSet, o io.Writer) error {
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
