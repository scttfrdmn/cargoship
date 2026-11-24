package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ReadConfigFile returns the defaults overlaid with the settings in the YAML file
// at path. Unlike Manager.LoadConfig it reads only the file: environment
// overrides are not folded in, so a caller that writes the result back (setup)
// can't persist a value that came from the environment.
func ReadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// WriteConfigSections writes the named top-level sections of cfg into the YAML
// file at path, leaving everything else in the file as it was (#909).
//
// A section is merged key by key into the existing one, so keys the Config
// struct doesn't know, other top-level sections (cost_control budgets, alert
// settings) and the comments on them survive. Rewriting the whole file from the
// struct, as setup used to through SaveConfig, dropped all of those. The same
// class of loss was #891.
//
// When the file exists its previous contents are kept at path+".bak", which is
// returned. The new contents are written to a temporary file and renamed into
// place, keeping the file's mode; a new file is created 0600.
func WriteConfigSections(path string, cfg *Config, sections []string) (backup string, err error) {
	var full yaml.Node
	if err := full.Encode(cfg); err != nil {
		return "", fmt.Errorf("encode config: %w", err)
	}
	want := make(map[string]bool, len(sections))
	for _, s := range sections {
		want[s] = true
	}

	mode := fs.FileMode(0o600)
	old, err := os.ReadFile(path)
	switch {
	case err == nil:
		if fi, serr := os.Stat(path); serr == nil {
			mode = fi.Mode().Perm()
		}
	case errors.Is(err, fs.ErrNotExist):
		old = nil
	default:
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	var doc yaml.Node
	if len(bytes.TrimSpace(old)) > 0 {
		if err := yaml.Unmarshal(old, &doc); err != nil {
			return "", fmt.Errorf("parse %s (left unchanged): %w", path, err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return "", fmt.Errorf("%s is not a YAML mapping (left unchanged)", path)
	}
	top := doc.Content[0]
	for i := 0; i+1 < len(full.Content); i += 2 {
		if want[full.Content[i].Value] {
			mergeKey(top, full.Content[i], full.Content[i+1])
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("encode %s: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("encode %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	if old != nil {
		backup = path + ".bak"
		if err := writeFileAtomic(backup, old, mode); err != nil {
			return "", fmt.Errorf("back up %s (left unchanged): %w", path, err)
		}
	}
	if err := writeFileAtomic(path, buf.Bytes(), mode); err != nil {
		return backup, err
	}
	return backup, nil
}

// mergeKey sets key to val in the mapping node m. A mapping merges into an
// existing mapping key by key; anything else replaces the old value, keeping its
// comments.
func mergeKey(m, key, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key.Value {
			continue
		}
		old := m.Content[i+1]
		if old.Kind == yaml.MappingNode && val.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(val.Content); j += 2 {
				mergeKey(old, val.Content[j], val.Content[j+1])
			}
			return
		}
		if val.HeadComment == "" {
			val.HeadComment = old.HeadComment
		}
		if val.LineComment == "" {
			val.LineComment = old.LineComment
		}
		if val.FootComment == "" {
			val.FootComment = old.FootComment
		}
		m.Content[i+1] = val
		return
	}
	m.Content = append(m.Content, key, val)
}

// writeFileAtomic writes data to a temporary file beside path and renames it into
// place, so a failure never leaves a half-written file.
func writeFileAtomic(path string, data []byte, mode fs.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			err = fmt.Errorf("write %s: %w", path, err)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
