package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsupportedConfigFormat is returned for a config file that isn't YAML.
// CargoShip's config is YAML only (#890).
var ErrUnsupportedConfigFormat = errors.New("unsupported config file format")

// configBaseName is the config file name, without extension, searched for in
// each of SearchDirs.
const configBaseName = ".cargoship"

// yamlExts are the accepted extensions, in search order.
var yamlExts = []string{".yaml", ".yml"}

// otherExts are formats viper would also accept by name. CargoShip doesn't
// read them, so finding one is reported rather than silently half-used.
var otherExts = []string{".json", ".toml", ".hcl", ".tfvars", ".ini", ".env", ".dotenv", ".properties", ".props", ".prop"}

// SearchDirs returns the directories searched for .cargoship.yaml, in order:
// the home directory, ~/.config/cargoship, then the working directory.
func SearchDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, home, filepath.Join(home, ".config", "cargoship"))
	}
	return append(dirs, ".")
}

// ResolveConfigFile decides which config file to read.
//
// With an explicit path, the file must have a YAML extension (or none, which is
// read as YAML); any other extension returns ErrUnsupportedConfigFormat. With no
// path, it returns the first .cargoship.yaml or .cargoship.yml in SearchDirs,
// or "" when there is none. If a .cargoship file in another format is found
// instead, warning names it so the caller can say it is being ignored.
func ResolveConfigFile(explicit string) (path, warning string, err error) {
	if explicit != "" {
		ext := strings.ToLower(filepath.Ext(explicit))
		if ext != "" && !isYAMLExt(ext) {
			return "", "", fmt.Errorf("%w: %s (CargoShip reads YAML only: .yaml or .yml)", ErrUnsupportedConfigFormat, explicit)
		}
		return explicit, "", nil
	}

	dirs := SearchDirs()
	for _, dir := range dirs {
		for _, ext := range yamlExts {
			p := filepath.Join(dir, configBaseName+ext)
			if fileExists(p) {
				return p, "", nil
			}
		}
	}
	for _, dir := range dirs {
		for _, ext := range otherExts {
			p := filepath.Join(dir, configBaseName+ext)
			if fileExists(p) {
				return "", fmt.Sprintf("ignoring %s: CargoShip reads YAML config only; convert it to %s",
					p, filepath.Join(dir, configBaseName+".yaml")), nil
			}
		}
	}
	return "", "", nil
}

func isYAMLExt(ext string) bool {
	for _, e := range yamlExts {
		if ext == e {
			return true
		}
	}
	return false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
