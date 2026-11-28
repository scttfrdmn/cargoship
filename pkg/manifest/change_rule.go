package manifest

import "time"

// FileChanged reports whether a local file with the given size and modification
// time differs from its entry in a previous manifest, by the rule sync uses
// (#918): any size difference, or any modification-time difference beyond a
// coarser filesystem's precision. It lets other change detectors (upload
// --incremental, #1002) apply exactly the same rule.
func FileChanged(size int64, modTime time.Time, entry FileEntry) bool {
	return size != entry.Size || !sameModTime(modTime, entry.ModTime)
}
