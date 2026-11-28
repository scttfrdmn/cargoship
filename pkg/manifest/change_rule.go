package manifest

import "time"

// ntfsTimePrecision is the unit NTFS stores times in. A nanosecond time recorded
// on Linux or macOS comes back truncated to it once the file is on NTFS.
const ntfsTimePrecision = 100 * time.Nanosecond

// FileChanged reports whether a local file with the given size and modification
// time differs from its entry in a previous manifest, by the rule sync uses
// (#918): any size difference, or any modification-time difference beyond a
// coarser filesystem's precision. It lets other change detectors (upload
// --incremental, #1002) apply the same rule.
//
// It also accepts the recorded time truncated to NTFS's 100 ns precision, which
// sameModTime's list (1µs, 1s, 2s) lacks: without it, a tree recorded with
// nanosecond times and then restored onto NTFS looked changed in every file.
// sameModTime has the same gap for sync; once it covers 100 ns, this can be
// just sync's rule again.
func FileChanged(size int64, modTime time.Time, entry FileEntry) bool {
	if size != entry.Size {
		return true
	}
	return !sameModTime(modTime, entry.ModTime) && !modTime.Equal(entry.ModTime.Truncate(ntfsTimePrecision))
}
