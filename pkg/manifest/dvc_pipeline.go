// Package manifest — DVC pipeline metadata extraction (Issue #185).
package manifest

import (
	"crypto/md5" //nolint:gosec // MD5 used only as a content fingerprint, not for security
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ExtractDVCPipeline parses dvc.yaml and dvc.lock located at repoPath and
// returns pipeline provenance for the named stage.
//
// Graceful fallbacks:
//   - Returns a zero-value DVCPipeline (StageName set, all other fields empty)
//     when dvc.yaml or dvc.lock is absent — this is not an error because many
//     repos do not use DVC pipelines.
//   - Returns a partially-populated DVCPipeline when dvc.lock is absent or does
//     not contain the requested stage (command, deps, and outs are still extracted
//     from dvc.yaml).
//   - Returns an error only for unexpected I/O failures (e.g. a file exists but
//     cannot be read due to permissions).
//
// The repoPath argument is sanitized with filepath.Clean + filepath.Abs before
// use to prevent path-traversal sequences.
func ExtractDVCPipeline(repoPath, stageName string) (*DVCPipeline, error) {
	result := &DVCPipeline{StageName: stageName}

	absPath, err := filepath.Abs(filepath.Clean(repoPath))
	if err != nil {
		return result, nil
	}
	if _, err := os.Stat(absPath); err != nil {
		return result, nil
	}

	dvcYAMLPath := filepath.Join(absPath, "dvc.yaml")
	dvcLockPath := filepath.Join(absPath, "dvc.lock")

	result.PipelineFile = "dvc.yaml"

	// -------------------------------------------------------------------
	// Parse dvc.yaml
	// -------------------------------------------------------------------
	yamlBytes, err := os.ReadFile(dvcYAMLPath) //nolint:gosec // path sanitised above
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil // not a DVC pipeline project — graceful fallback
		}
		return result, fmt.Errorf("read dvc.yaml: %w", err)
	}

	var dvcYAML dvcYAMLFile
	if err := yaml.Unmarshal(yamlBytes, &dvcYAML); err != nil {
		return result, fmt.Errorf("parse dvc.yaml: %w", err)
	}

	stage, ok := dvcYAML.Stages[stageName]
	if !ok {
		return result, nil // stage not found — graceful fallback
	}

	result.Command = stage.Cmd
	result.Deps = parseDVCYAMLDeps(stage.Deps)
	result.Outputs = parseDVCYAMLOuts(stage.Outs)

	// -------------------------------------------------------------------
	// Parse dvc.lock (optional enrichment)
	// -------------------------------------------------------------------
	lockBytes, err := os.ReadFile(dvcLockPath) //nolint:gosec // path sanitised above
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil // no lock file yet — partial result is fine
		}
		return result, fmt.Errorf("read dvc.lock: %w", err)
	}

	// LockHash: stable fingerprint of the entire lock file.
	sum := md5.Sum(lockBytes) //nolint:gosec
	result.LockHash = fmt.Sprintf("%x", sum)

	// ExecutedAt: use the lock file's modification time as a proxy for when
	// the pipeline was last run.
	if fi, err := os.Stat(dvcLockPath); err == nil {
		result.ExecutedAt = fi.ModTime().UTC().Truncate(time.Second)
	}

	var dvcLock dvcLockFile
	if err := yaml.Unmarshal(lockBytes, &dvcLock); err != nil {
		return result, fmt.Errorf("parse dvc.lock: %w", err)
	}

	lockedStage, ok := dvcLock.Stages[stageName]
	if !ok {
		return result, nil // stage in dvc.yaml but not yet in dvc.lock
	}

	// Enrich deps and outs with locked md5/size values.
	result.Deps = mergeLockDeps(result.Deps, lockedStage.Deps)
	result.Outputs = mergeLockOuts(result.Outputs, lockedStage.Outs)

	// Flatten params: merge all param file entries into a single map.
	if len(lockedStage.Params) > 0 {
		result.Params = flattenParams(lockedStage.Params)
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Internal YAML document models
// ---------------------------------------------------------------------------

// dvcYAMLFile is the top-level structure of dvc.yaml.
type dvcYAMLFile struct {
	Stages map[string]dvcYAMLStage `yaml:"stages"`
}

// dvcYAMLStage represents a single stage entry in dvc.yaml.
type dvcYAMLStage struct {
	Cmd  string      `yaml:"cmd"`
	Deps []yaml.Node `yaml:"deps"`
	Outs []yaml.Node `yaml:"outs"`
	// Params field is intentionally not parsed here; we use dvc.lock params.
}

// dvcLockFile is the top-level structure of dvc.lock.
type dvcLockFile struct {
	Schema string                  `yaml:"schema"`
	Stages map[string]dvcLockStage `yaml:"stages"`
}

// dvcLockStage is a single stage entry in dvc.lock.
type dvcLockStage struct {
	Cmd    string                    `yaml:"cmd"`
	Deps   []dvcLockEntry            `yaml:"deps"`
	Outs   []dvcLockEntry            `yaml:"outs"`
	Params map[string]map[string]any `yaml:"params"`
}

// dvcLockEntry is a dependency or output entry in dvc.lock.
type dvcLockEntry struct {
	Path string `yaml:"path"`
	MD5  string `yaml:"md5"`
	Size int64  `yaml:"size"`
}

// ---------------------------------------------------------------------------
// dvc.yaml parsing helpers
// ---------------------------------------------------------------------------

// parseDVCYAMLDeps converts the deps YAML nodes (which can be plain strings or
// dicts with additional options) into a []DVCDep slice.
func parseDVCYAMLDeps(nodes []yaml.Node) []DVCDep {
	deps := make([]DVCDep, 0, len(nodes))
	for i := range nodes {
		path := extractYAMLNodePath(&nodes[i])
		if path != "" {
			deps = append(deps, DVCDep{Path: path})
		}
	}
	return deps
}

// parseDVCYAMLOuts converts outs YAML nodes into a []DVCOut slice.
// DVC allows outs to be plain strings or dicts like:
//
//   - path/to/file:
//     cache: false
func parseDVCYAMLOuts(nodes []yaml.Node) []DVCOut {
	outs := make([]DVCOut, 0, len(nodes))
	for i := range nodes {
		path := extractYAMLNodePath(&nodes[i])
		if path != "" {
			outs = append(outs, DVCOut{Path: path})
		}
	}
	return outs
}

// extractYAMLNodePath returns the path string from a YAML node that is either
// a plain scalar (the path itself) or a mapping with a single string key.
func extractYAMLNodePath(n *yaml.Node) string {
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Value
	case yaml.MappingNode:
		// First key of the mapping is the path.
		if len(n.Content) >= 1 {
			return n.Content[0].Value
		}
	case yaml.DocumentNode:
		if len(n.Content) > 0 {
			return extractYAMLNodePath(n.Content[0])
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// dvc.lock merge helpers
// ---------------------------------------------------------------------------

// mergeLockDeps enriches deps extracted from dvc.yaml with MD5/Size from dvc.lock.
// Matching is done by path. Lock entries that have no corresponding dvc.yaml dep
// are appended (handles cases where dvc.yaml was edited after the last run).
func mergeLockDeps(yamlDeps []DVCDep, lockDeps []dvcLockEntry) []DVCDep {
	index := make(map[string]int, len(yamlDeps))
	for i, d := range yamlDeps {
		index[d.Path] = i
	}
	for _, le := range lockDeps {
		if i, ok := index[le.Path]; ok {
			yamlDeps[i].MD5 = le.MD5
			yamlDeps[i].Size = le.Size
		} else {
			yamlDeps = append(yamlDeps, DVCDep(le))
		}
	}
	return yamlDeps
}

// mergeLockOuts enriches outs from dvc.yaml with MD5/Size from dvc.lock.
func mergeLockOuts(yamlOuts []DVCOut, lockOuts []dvcLockEntry) []DVCOut {
	index := make(map[string]int, len(yamlOuts))
	for i, o := range yamlOuts {
		index[o.Path] = i
	}
	for _, le := range lockOuts {
		if i, ok := index[le.Path]; ok {
			yamlOuts[i].MD5 = le.MD5
			yamlOuts[i].Size = le.Size
		} else {
			yamlOuts = append(yamlOuts, DVCOut(le))
		}
	}
	return yamlOuts
}

// flattenParams merges param entries from all param files into a single
// key→value map.  When the same key appears in multiple files the last one wins.
func flattenParams(paramFiles map[string]map[string]any) map[string]any {
	flat := make(map[string]any)
	for _, kvs := range paramFiles {
		for k, v := range kvs {
			flat[k] = v
		}
	}
	if len(flat) == 0 {
		return nil
	}
	return flat
}
