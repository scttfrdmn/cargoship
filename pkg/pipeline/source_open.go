package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// openContainedSourceFile opens fullPath for reading through an os.Root anchored
// at sourceDir, closing the source-side TOCTOU/symlink-substitution hole
// (CSH-SEC-001, CWE-367 / CWE-59): the scanner validates a path at walk time, but
// consumers reopen it later, so a path swapped to a symlink in between would
// otherwise be followed and its target streamed to S3. Opening through the root
// confines the read to the source tree — a symlink pointing outside it (e.g. an
// SSH key or /etc material) is refused — and a symlink at the leaf is rejected
// outright, enforcing the scanner's FollowSymlinks==false policy at *use* time
// rather than only at check time. The opened object must be a regular file.
//
// A fresh root is opened per call: the extra dir open is negligible against the
// per-file S3 PUT and avoids any shared-root lifecycle. When sourceDir is empty
// (not a normal upload path) it falls back to a direct open with the regular-file
// check only. Callers must Close the returned file.
func openContainedSourceFile(sourceDir, fullPath string) (*os.File, error) {
	if sourceDir == "" {
		f, err := os.Open(fullPath) //nolint:gosec // path is from the pipeline's own scan
		if err != nil {
			return nil, err
		}
		return f, ensureRegularFile(f, fullPath)
	}

	rel, err := filepath.Rel(sourceDir, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil, fmt.Errorf("source file %q is outside source root %q", fullPath, sourceDir)
	}

	root, err := os.OpenRoot(sourceDir)
	if err != nil {
		// Can't anchor a root (e.g. sourceDir vanished): fail closed rather than
		// fall back to an unconfined open.
		return nil, fmt.Errorf("open source root %q: %w", sourceDir, err)
	}
	defer func() { _ = root.Close() }()

	// Reject a symlink at the path (checked through the root, not followed). The
	// scanner skips symlinks at scan time; this catches one introduced afterward.
	// os.Root.Open additionally refuses any symlink that escapes the root, so a
	// swap targeting a file outside the source tree fails.
	if fi, lerr := root.Lstat(rel); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to read %q: path is now a symlink (source-file TOCTOU)", fullPath)
	}

	f, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	return f, ensureRegularFile(f, fullPath)
}

// ensureRegularFile closes f and errors unless it is a regular file.
func ensureRegularFile(f *os.File, name string) error {
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	if !st.Mode().IsRegular() {
		_ = f.Close()
		return fmt.Errorf("refusing to read %q: not a regular file (%s)", name, st.Mode().Type())
	}
	return nil
}
