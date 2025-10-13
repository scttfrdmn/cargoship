/*
Package inventory provides the needed pieces to correctly create an Inventory of a directory
*/
package inventory

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// HashAlgorithm represents different hashing algorithms available for file verification
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

var hashMap = map[string]HashAlgorithm{
	"md5":    MD5Hash,
	"sha1":   SHA1Hash,
	"sha256": SHA256Hash,
	"sha512": SHA512Hash,
	"":       NullHash,
}

var hashHelp = map[string]string{
	"md5":    "Fast but older hashing method, but usually fine for signatures",
	"sha1":   "Less intensive on CPUs than sha256, and more secure than md5",
	"sha256": "CPU intensive but very secure signature hashing",
	"sha512": "CPU intensive but very VERY secure signature hashing",
}

// HashCompletion returns shell completion
func HashCompletion(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	help := []string{}
	for _, format := range nonEmptyKeys(hashMap) {
		if strings.Contains(format, toComplete) {
			help = append(help, fmt.Sprintf("%v\t%v", format, hashHelp[format]))
		}
	}
	return help, cobra.ShellCompDirectiveNoFileComp
}

// String satisfies the pflags interface
// Returns empty string for invalid hash algorithm values
func (h HashAlgorithm) String() string {
	m := reverseMap(hashMap)
	if v, ok := m[h]; ok {
		return v
	}
	// Return empty string for invalid values instead of panicking
	// Callers should validate hash algorithm values before using
	return ""
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

var formatMap = map[string]Format{
	"yaml": YAMLFormat,
	"json": JSONFormat,
	"":     NullFormat,
}

var formatHelp = map[string]string{
	"yaml": "YAML is the preferred format. It allows for easy human readable inventories that can also be easily parsed by machines",
	"json": "JSON inventory is not very readable, but could allow for faster machine parsing under certain conditions",
}

// FormatCompletion returns shell completion
func FormatCompletion(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	help := []string{}
	for _, format := range nonEmptyKeys(formatMap) {
		if strings.Contains(format, toComplete) {
			help = append(help, fmt.Sprintf("%v\t%v", format, formatHelp[format]))
		}
	}
	return help, cobra.ShellCompDirectiveNoFileComp
}

// String returns the string representation of Format
// Returns empty string for invalid format values
func (f Format) String() string {
	m := reverseMap(formatMap)
	if v, ok := m[f]; ok {
		return v
	}
	// Return empty string for invalid values instead of panicking
	// Callers should validate format values before using
	return ""
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

// Utility functions

// nonEmptyKeys returns all non-empty string keys from a map, sorted alphabetically
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
