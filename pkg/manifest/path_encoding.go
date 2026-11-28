package manifest

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// StorablePathName reports whether a manifest can record the path name p. The
// delta scan and the pipeline scanner both apply it, so they refuse the same files
// (#1003).
func StorablePathName(p string) bool {
	return utf8.ValidString(p)
}

// NonUTF8PathsError reports source files whose names are not valid UTF-8, which
// neither the delta scan nor the upload accepts (#1003).
//
// A manifest is JSON, and encoding/json replaces each invalid byte with U+FFFD,
// while the tar entry keeps the raw name. Such a file uploaded "successfully" but
// could not be restored, two names differing only in an invalid byte collapsed to
// one manifest path, and every later sync saw the file as new. Names like this are
// common on ext4 and SMB/NAS shares written by Latin-1 clients. Until the format
// can store them losslessly (#1049), they are refused loudly rather than stored
// corrupted.
type NonUTF8PathsError struct {
	// Root is the scan root the paths are relative to.
	Root string
	// Paths are the offending paths, relative to Root. It is empty when Root
	// itself is not valid UTF-8.
	Paths []string
}

const nonUTF8Remedy = "A manifest can't store these names yet: they would be restored under a " +
	"different, corrupted name, or not at all. Rename them to UTF-8 (for example with convmv " +
	"from Latin-1), or exclude them deliberately via exclude_patterns. Storing such names " +
	"losslessly is tracked in https://github.com/scttfrdmn/cargoship/issues/1049"

func (e *NonUTF8PathsError) Error() string {
	if len(e.Paths) == 0 {
		return fmt.Sprintf("the source path %q is not valid UTF-8, so nothing under it was archived. %s",
			e.Root, nonUTF8Remedy)
	}
	const show = 5
	shown := e.Paths
	suffix := ""
	if len(shown) > show {
		shown = shown[:show]
		suffix = fmt.Sprintf(" (and %d more)", len(e.Paths)-show)
	}
	quoted := make([]string, len(shown))
	for i, p := range shown {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return fmt.Sprintf("%d path(s) under %q are not valid UTF-8 and were not archived: %s%s. %s",
		len(e.Paths), e.Root, strings.Join(quoted, ", "), suffix, nonUTF8Remedy)
}
