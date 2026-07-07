// Package manifest — DVC pipeline metadata extraction (Issue #185).
package manifest

import (
	"crypto/md5" //nolint:gosec // MD5 used only as a content fingerprint, not for security
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	sum := md5.Sum(lockBytes) //nolint:gosec // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-md5 -- lock-file fingerprint, not security
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

// BuildFileStageIndex parses dvc.yaml at repoPath and returns a map of every
// stage output path → stage name.
// Directory outputs (no file extension) are stored with a trailing "/" for
// prefix-based matching.
// Returns an empty map (not an error) when dvc.yaml is absent — graceful fallback.
func BuildFileStageIndex(repoPath string) (map[string]string, error) {
	absPath, err := filepath.Abs(filepath.Clean(repoPath))
	if err != nil {
		return map[string]string{}, nil
	}

	dvcYAMLPath := filepath.Join(absPath, "dvc.yaml")
	yamlBytes, err := os.ReadFile(dvcYAMLPath) //nolint:gosec // path sanitised above
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return map[string]string{}, fmt.Errorf("read dvc.yaml: %w", err)
	}

	var dvcYAML dvcYAMLFile
	if err := yaml.Unmarshal(yamlBytes, &dvcYAML); err != nil {
		return map[string]string{}, fmt.Errorf("parse dvc.yaml: %w", err)
	}

	index := make(map[string]string)

	// Iterate stages in deterministic order for consistent results.
	stageNames := make([]string, 0, len(dvcYAML.Stages))
	for name := range dvcYAML.Stages {
		stageNames = append(stageNames, name)
	}
	sort.Strings(stageNames)

	for _, stageName := range stageNames {
		stage := dvcYAML.Stages[stageName]
		for i := range stage.Outs {
			outPath := extractYAMLNodePath(&stage.Outs[i])
			if outPath == "" {
				continue
			}
			// Paths with no file extension are treated as directory outputs.
			if filepath.Ext(outPath) == "" {
				index[outPath+"/"] = stageName
			} else {
				index[outPath] = stageName
			}
		}
	}

	return index, nil
}

// AnnotateFilesWithDVCStages sets DVCMetadata.Stage on each FileEntry whose
// path falls under a stage output discovered in dvc.yaml at repoPath.
// Files not matching any stage output are left unchanged.
// Index-build failures are silently ignored (graceful degradation).
func AnnotateFilesWithDVCStages(files []FileEntry, repoPath string) {
	index, err := BuildFileStageIndex(repoPath)
	if err != nil || len(index) == 0 {
		return
	}

	// Collect sorted keys so deterministic first-match wins when multiple
	// stages could match the same path (alphabetical stage name order).
	keys := make([]string, 0, len(index))
	for k := range index {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := range files {
		f := &files[i]
		for _, key := range keys {
			stageName := index[key]
			var matched bool
			if strings.HasSuffix(key, "/") {
				// Directory output: prefix match.
				dirPath := key[:len(key)-1]
				matched = f.Path == dirPath || strings.HasPrefix(f.Path, dirPath+"/")
			} else {
				// File output: exact match.
				matched = f.Path == key
			}
			if matched {
				if f.DVCMetadata == nil {
					f.DVCMetadata = &DVCMetadata{}
				}
				f.DVCMetadata.Stage = stageName
				break // first match wins
			}
		}
	}
}
