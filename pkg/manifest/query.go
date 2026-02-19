package manifest

// Hash-based and DVC-aware query methods for ManifestQuery (Issue #188).
// These extend the basic path-lookup API in types.go with O(1) hash lookup
// and DVC pipeline provenance queries.

// FindFileByHash returns the FileEntry whose ContentHash matches hash.
// The second return value is false when no file has that hash.
// Lookup is O(1) via the pre-built hash index.
func (mq *ManifestQuery) FindFileByHash(hash string) (*FileEntry, bool) {
	if hash == "" {
		return nil, false
	}
	f, ok := mq.hashIndex[hash]
	return f, ok
}

// FindFilesByCommit returns all FileEntry pointers whose associated git commit
// matches commit. For a single-manifest query this is all files when the
// manifest's GitMetadata.Commit equals commit, and nil otherwise.
// The returned slice must not be modified.
func (mq *ManifestQuery) FindFilesByCommit(commit string) []*FileEntry {
	if commit == "" {
		return nil
	}
	return mq.commitIndex[commit]
}

// FindFilesByDVCStage returns all FileEntry pointers whose DVCMetadata.Stage
// field matches stage. Returns nil when no files carry that stage tag.
// The returned slice must not be modified.
func (mq *ManifestQuery) FindFilesByDVCStage(stage string) []*FileEntry {
	if stage == "" {
		return nil
	}
	return mq.stageIndex[stage]
}

// RebuildIndex discards and rebuilds all internal lookup indices from the
// current manifest file list. Call after adding files to the manifest via
// the Builder if you need queries to reflect the updated state.
func (mq *ManifestQuery) RebuildIndex() {
	mq.buildIndices()
}
