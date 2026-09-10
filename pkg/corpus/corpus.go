// Package corpus generates reproducible synthetic file corpora for the
// benchmark harness. The generators are byte-for-byte deterministic given a
// fixed seed, so a published benchmark number can be re-produced and verified by
// anyone. They mirror the adversarial corpora the pipeline torture/round-trip
// tests use (hostile paths, tiny files, incompressible vs compressible, mixed
// chunk kinds), lifted out of the test files into an importable package.
package corpus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
)

// File records one planted file: its path relative to the corpus root, its
// basename (unique across a corpus, so a basename-flattened restore round-trips),
// its SHA-256 (hex) content hash, and its size in bytes.
type File struct {
	RelPath string
	Base    string
	Sum     string
	Size    int
}

// sha256Hex returns the hex SHA-256 of b.
func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// write plants one file with the given content, creating parent dirs, and
// returns its File record.
func write(root, rel string, content []byte) (File, error) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return File{}, fmt.Errorf("mkdir for %s: %w", rel, err)
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		return File{}, fmt.Errorf("write %s: %w", rel, err)
	}
	return File{RelPath: rel, Base: filepath.Base(filepath.FromSlash(rel)), Sum: sha256Hex(content), Size: len(content)}, nil
}

// PlantHostile writes a deliberately awkward set of files (empty, one-byte,
// unicode/space/dotfile names, deep nesting, compressible + incompressible) with
// unique basenames. baseSize pads every file up to that floor so the average
// clears the direct-upload threshold and steers toward the chunked path; 0 keeps
// the tiny edge sizes (direct upload). Random content uses rng for determinism.
func PlantHostile(root string, rng *rand.Rand, baseSize int) ([]File, error) {
	specs := []struct {
		rel  string
		size int
		mode string // "zero", "random", "compressible"
	}{
		{"empty.dat", 0, "zero"},
		{"one-byte.dat", 1, "random"},
		{"small.txt", 4096, "compressible"},
		{"incompressible.bin", 512 * 1024, "random"},
		{"large-compressible.log", 512 * 1024, "compressible"},
		{"nested/a/b/c/deep.txt", 2048, "compressible"},
		{"unicode/日本語/файл.dat", 1024, "random"},
		{"has spaces/my file (1).txt", 512, "compressible"},
		{"dir/.hidden", 256, "random"},
		{"dir2/UPPER.DAT", 3000, "random"},
	}
	seen := map[string]bool{}
	out := make([]File, 0, len(specs))
	for _, s := range specs {
		base := filepath.Base(filepath.FromSlash(s.rel))
		if seen[base] {
			return nil, fmt.Errorf("duplicate basename %q: corpus basenames must be unique", base)
		}
		seen[base] = true

		size := s.size
		if baseSize > size {
			size = baseSize
		}
		content := make([]byte, size)
		switch s.mode {
		case "random":
			_, _ = rng.Read(content)
		case "compressible":
			fillPattern(content, "cargoship-roundtrip-")
		}
		f, err := write(root, s.rel, content)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// PlantManyFiles writes n small random files (1–4096 bytes) with unique
// basenames spread across subdirectories so no single directory is enormous.
func PlantManyFiles(root string, rng *rand.Rand, n int) ([]File, error) {
	out := make([]File, 0, n)
	for i := 0; i < n; i++ {
		rel := filepath.Join(fmt.Sprintf("d%03d", i/500), fmt.Sprintf("f%06d.dat", i))
		content := make([]byte, 1+rng.Intn(4096))
		_, _ = rng.Read(content)
		f, err := write(root, rel, content)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// PlantSized writes one incompressible (random) file per requested size, with
// unique basenames. Size 0 yields an empty file.
func PlantSized(root string, rng *rand.Rand, sizes []int) ([]File, error) {
	out := make([]File, 0, len(sizes))
	for i, sz := range sizes {
		content := make([]byte, sz)
		_, _ = rng.Read(content)
		f, err := write(root, fmt.Sprintf("sized%02d.bin", i), content)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// PlantCompressible writes highly-compressible patterned .log files (steers the
// zstd/frames path). Content is a deterministic repeating pattern per index, so
// no rng is needed — fully reproducible.
func PlantCompressible(root string, sizes []int) ([]File, error) {
	out := make([]File, 0, len(sizes))
	for i, sz := range sizes {
		content := make([]byte, sz)
		fillPattern(content, fmt.Sprintf("cargoship benchmark line %02d — the quick brown fox\n", i))
		f, err := write(root, fmt.Sprintf("text%02d.log", i), content)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// PlantAlreadyCompressed writes random-content .zip files, which the compression
// detector treats as already-compressed → the archiver writes them as plain
// .tar chunks. Combined with PlantCompressible this yields both chunk kinds.
func PlantAlreadyCompressed(root string, rng *rand.Rand, sizes []int) ([]File, error) {
	out := make([]File, 0, len(sizes))
	for i, sz := range sizes {
		content := make([]byte, sz)
		_, _ = rng.Read(content)
		f, err := write(root, fmt.Sprintf("blob%02d.zip", i), content)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// fillPattern fills dst with a repeating ASCII pattern (deterministic).
func fillPattern(dst []byte, pattern string) {
	p := []byte(pattern)
	for i := range dst {
		dst[i] = p[i%len(p)]
	}
}

// IndexByBase walks dir and maps each regular file's basename to its full path
// (basenames are unique within a corpus).
func IndexByBase(dir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			out[filepath.Base(path)] = path
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
