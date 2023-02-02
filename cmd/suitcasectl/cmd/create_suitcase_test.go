package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

func TestNewSuitcaseWithDir(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", testD})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestNewSuitcaseWithViper(t *testing.T) {
	// testD := t.TempDir()
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "-o", testD, "../../../pkg/testdata/viper-enabled-target"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.FileExists(t, path.Join(testD, "snakey-thing-joebob-01-of-01.tar.gz"))
}

// Ensure that if we set a value on the CLI that it gets preference over whatever is in the user overrides
func TestNewSuitcaseWithViperFlag(t *testing.T) {
	// testD, err := os.MkdirTemp("", "")
	// require.NoError(t, err)
	// defer os.RemoveAll(testD)
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", testD, "--user", "darcy", "../../../pkg/testdata/viper-enabled-target"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.FileExists(t, path.Join(testD, "snakey-thing-darcy-01-of-01.tar.gz"))
}

func TestNewSuitcaseWithInventory(t *testing.T) {
	// outDir, err := os.MkdirTemp("", "")
	// require.NoError(t, err)
	// defer os.RemoveAll(outDir)
	outDir := t.TempDir()
	i, err := inventory.NewDirectoryInventory(&inventory.DirectoryInventoryOptions{
		TopLevelDirectories: []string{"../../../pkg/testdata/fake-dir"},
		SuitcaseFormat:      "tar",
		InventoryFormat:     "yaml",
	})
	require.NoError(t, err)
	outF, err := os.Create(path.Join(outDir, "inventory.yaml"))
	require.NoError(t, err)
	ir, err := inventory.NewInventoryerWithFilename(outF.Name())
	require.NoError(t, err)

	err = ir.Write(outF, i)
	require.NoError(t, err)

	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", outDir, "--inventory-file", outF.Name()})
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestNewSuitcaseWithInventoryAndDir(t *testing.T) {
	cmd := NewRootCmd(io.Discard)
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", t.TempDir(), "--inventory-file", "doesnt-matter", t.TempDir()})
	err := cmd.Execute()
	require.Error(t, err, "Did NOT get an error when executing command")
	require.EqualError(t, err, "error: You can't specify an inventory file and target dir arguments at the same time", "Got an unexpected error")
}

func TestInventoryFormatComplete(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(io.Discard)
	cmd.SetOut(b)
	cmd.SetArgs([]string{"__complete", "create", "suitcase", "--inventory-format", ""})
	err := cmd.Execute()
	require.NoError(t, err)
	// require.Equal(t, "json\tJSON inventory is not very readable, but could allow for faster machine parsing under certain conditions\nyaml\tYAML is the preferred format. It allows for easy human readable inventories that can also be easily parsed by machines\n:4\n", b.String())
	require.Contains(t, b.String(), "yaml\t")
	require.Contains(t, b.String(), "json\t")
}

func TestSuitcaseFormatComplete(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(io.Discard)
	cmd.SetOut(b)
	cmd.SetArgs([]string{"__complete", "create", "suitcase", "--suitcase-format", ""})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Equal(t, "tar\ntar.gpg\ntar.gz\ntar.gz.gpg\ntar.zst\ntar.zst.gpg\n:4\n", b.String())
}

func BenchmarkSuitcaseCreate(b *testing.B) {
	benchmarks := map[string]struct {
		format  string
		tarargs string
	}{
		"tar": {
			format:  "tar",
			tarargs: "c",
		},
		"targz": {
			format:  "tar.gz",
			tarargs: "cz",
		},
	}
	cmd := NewRootCmd(io.Discard)
	// formats := []string{"tar", "tar.gz"}
	datasets := map[string]struct {
		path string
	}{
		"672M-american-gut": {
			path: "American-Gut",
		},
		"3.3G-Synthetic-cell-images": {
			path: "BBBC005_v1_images",
		},
	}
	bdd := os.Getenv("BENCHMARK_DATA_DIR")
	if bdd == "" {
		bdd = "../../../benchmark_data/"
	}
	for desc, opts := range benchmarks {
		opts := opts
		for dataDesc, dataSet := range datasets {
			location := path.Join(bdd, dataSet.path)
			if _, err := os.Stat(location); err == nil {
				b.Run(fmt.Sprintf("suitcase_format_golang_%v_%v", dataDesc, desc), func(b *testing.B) {
					out := b.TempDir()
					cmd.SetArgs([]string{"create", "suitcase", location, "--destination", out, "--suitcase-format", opts.format})
					cmd.Execute()
				})
				//				b.Run(fmt.Sprintf("suitcase_format_gtar_%v_%v", dataDesc, desc), func(b *testing.B) {
				//				out := b.TempDir()
				//			exec.Command("tar", fmt.Sprintf("%vvf", opts.tarargs), path.Join(out, fmt.Sprintf("gnutar.%v", opts.format)), location).Output()
				//	})
			}
		}
	}
}
