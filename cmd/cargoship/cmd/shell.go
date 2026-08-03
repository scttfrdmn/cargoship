package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/archivefs"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/repl"
)

// NewShellCmd creates the interactive shell command. When called with an S3 URL
// it opens an archive filesystem shell for navigating a CargoShip archive
// without extraction. When called without arguments it starts the generic
// CargoShip REPL.
func NewShellCmd() *cobra.Command {
	var (
		region  string
		cacheGB int64
	)

	cmd := &cobra.Command{
		Use:     "shell [S3_URL]",
		Aliases: []string{"repl", "interactive"},
		Short:   "Navigate a CargoShip archive or start an interactive shell",
		Long: `When called with an S3 URL, opens an interactive filesystem shell for
browsing and inspecting a CargoShip archive without downloading files.

  cargoship shell s3://my-bucket/uploads/20240101-abc123

When called without arguments, starts the generic CargoShip REPL.

Archive shell commands:
  ls [path]         List files and directories
  cd <dir>          Change current directory
  pwd               Print current directory
  cat <file>        Stream file content to stdout
  head <file> [n]   Print first n lines (default 10)
  stat <file>       Show file metadata (size, hash, chunk, DVC stage, git commit)
  find <pattern>    Find files by glob pattern (e.g. *.csv, data/*.parquet)
  stage list        List all DVC pipeline stages and their file counts
  stage <name>      List files belonging to a DVC stage
  get <file> [dst]  Extract file to a local path (default: current directory)
  help              Show this help
  exit / quit       Exit the shell`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return runArchiveShell(cmd.Context(), args[0], region, cacheGB)
			}
			// No S3 URL provided — fall back to the generic CargoShip REPL.
			logger := slog.Default()
			sh := repl.NewShell(cmd.Root(), logger)
			logger.Info("Starting CargoShip interactive shell")
			return sh.Start()
		},
	}

	cmd.Flags().StringVarP(&region, "region", "r", "us-east-1", "AWS region")
	cmd.Flags().Int64Var(&cacheGB, "cache-gb", 10, "LRU chunk cache size in GB")
	return cmd
}

// runArchiveShell loads the manifest from s3URL and starts the filesystem REPL.
func runArchiveShell(ctx context.Context, s3URL, region string, cacheGB int64) error {
	if ctx == nil {
		ctx = context.Background()
	}

	bucket, prefix, err := parseS3URL(s3URL)
	if err != nil {
		return fmt.Errorf("invalid S3 URL: %w", err)
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	kmsClient := kms.NewFromConfig(cfg)

	var actualPrefix, uploadID string
	if idx := strings.Index(prefix, "/uploads/"); idx != -1 {
		actualPrefix = prefix[:idx]
		uploadID = prefix[idx+9:]
	} else {
		uploadID = prefix
	}

	fmt.Printf("📥 Loading manifest: s3://%s/%s\n", bucket, prefix)
	m, err := manifest.DownloadFromS3WithDecryption(ctx, s3Client, kmsClient, bucket, actualPrefix, uploadID)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}
	fmt.Printf("✅ Manifest loaded: %d files, %d chunks\n\n", m.TotalFiles, m.TotalChunks)

	maxCacheBytes := cacheGB * 1024 * 1024 * 1024
	// SetBucket: fetch from where the manifest was read, not the bucket name
	// baked into it at upload time (#335).
	se := manifest.NewSelectiveExtractor(m, s3Client, maxCacheBytes).SetBucket(bucket)
	vfs := archivefs.New(m)

	sh := &archiveShell{
		vfs:   vfs,
		se:    se,
		s3URL: s3URL,
		cwd:   "",
		in:    bufio.NewScanner(os.Stdin),
		out:   os.Stdout,
	}

	sh.writef("CargoShip Archive Shell  —  type 'help' for commands, 'exit' to quit.\n\n")
	return sh.run(ctx)
}

// archiveShell holds the state for one interactive archive filesystem session.
type archiveShell struct {
	vfs   *archivefs.VirtualFS
	se    *manifest.SelectiveExtractor
	s3URL string
	cwd   string // current virtual directory ("" = root)
	in    *bufio.Scanner
	out   io.Writer
}

// writef writes a formatted string to the shell's output, discarding the error
// (output is always to an in-process writer or stdout where errors are fatal).
func (sh *archiveShell) writef(format string, a ...any) {
	_, _ = fmt.Fprintf(sh.out, format, a...)
}

// writeln writes a line to the shell's output.
func (sh *archiveShell) writeln(s string) {
	_, _ = fmt.Fprintln(sh.out, s)
}

func (sh *archiveShell) prompt() string {
	if sh.cwd == "" {
		return "archive:/> "
	}
	return fmt.Sprintf("archive:/%s> ", sh.cwd)
}

func (sh *archiveShell) run(ctx context.Context) error {
	for {
		_, _ = fmt.Fprint(sh.out, sh.prompt())
		if !sh.in.Scan() {
			sh.writeln("") // newline after Ctrl+D
			return nil
		}
		line := strings.TrimSpace(sh.in.Text())
		if line == "" {
			continue
		}
		if done := sh.dispatch(ctx, line); done {
			return nil
		}
	}
}

func (sh *archiveShell) dispatch(ctx context.Context, line string) (exit bool) {
	fields := strings.Fields(line)
	verb := fields[0]
	args := fields[1:]

	switch verb {
	case "exit", "quit":
		sh.writeln("Goodbye.")
		return true
	case "help", "?":
		sh.cmdHelp()
	case "pwd":
		sh.cmdPwd()
	case "ls":
		sh.cmdLs(args)
	case "cd":
		sh.cmdCd(args)
	case "stat":
		sh.cmdStat(args)
	case "cat":
		sh.cmdCat(ctx, args)
	case "head":
		sh.cmdHead(ctx, args)
	case "find":
		sh.cmdFind(args)
	case "stage":
		sh.cmdStage(args)
	case "get":
		sh.cmdGet(ctx, args)
	default:
		sh.writef("unknown command %q — type 'help' for available commands\n", verb)
	}
	return false
}

// ---------------------------------------------------------------------------
// Command implementations
// ---------------------------------------------------------------------------

func (sh *archiveShell) cmdHelp() {
	sh.writeln(`Archive filesystem commands:

  ls [path]         List files and directories
  cd <dir>          Change current directory (supports ..)
  pwd               Print current directory
  cat <file>        Stream file content to stdout
  head <file> [n]   Print first n lines of a file (default: 10)
  stat <file>       Show file metadata
  find <pattern>    Find files by glob (e.g. *.csv, data/*.parquet)
  stage list        List all DVC pipeline stages
  stage <name>      List files in a DVC stage
  get <file> [dst]  Extract file to local path (default: .)
  help              Show this help
  exit / quit       Exit the shell`)
}

func (sh *archiveShell) cmdPwd() {
	if sh.cwd == "" {
		sh.writeln("/")
	} else {
		sh.writef("/%s\n", sh.cwd)
	}
}

func (sh *archiveShell) cmdLs(args []string) {
	target := sh.cwd
	if len(args) > 0 {
		target = sh.vfs.Resolve(sh.cwd, args[0])
	}

	// Allow ls on a file path — just stat it.
	if fe := sh.vfs.Stat(target); fe != nil {
		sh.printFileEntry(fe.Path, fe)
		return
	}

	entries := sh.vfs.List(target)
	if entries == nil {
		sh.writef("ls: %s: no such file or directory\n", args[0])
		return
	}
	if len(entries) == 0 {
		return
	}
	for _, e := range entries {
		if e.IsDir {
			sh.writef("  %s/\n", e.Name)
		} else {
			sh.printFileEntry(e.Name, e.File)
		}
	}
}

func (sh *archiveShell) cmdCd(args []string) {
	if len(args) == 0 {
		sh.cwd = ""
		return
	}
	target := sh.vfs.Resolve(sh.cwd, args[0])
	if !sh.vfs.IsDir(target) {
		if sh.vfs.Stat(target) != nil {
			sh.writef("cd: %s: not a directory\n", args[0])
		} else {
			sh.writef("cd: %s: no such file or directory\n", args[0])
		}
		return
	}
	sh.cwd = target
}

func (sh *archiveShell) cmdStat(args []string) {
	if len(args) == 0 {
		sh.writeln("usage: stat <file>")
		return
	}
	p := sh.vfs.Resolve(sh.cwd, args[0])
	fe := sh.vfs.Stat(p)
	if fe == nil {
		if sh.vfs.IsDir(p) {
			sh.writef("stat: %s: is a directory\n", args[0])
		} else {
			sh.writef("stat: %s: no such file\n", args[0])
		}
		return
	}
	sh.writef("  Path:      %s\n", fe.Path)
	sh.writef("  Size:      %s\n", humanize.Bytes(uint64(fe.Size)))
	if !fe.ModTime.IsZero() {
		sh.writef("  Modified:  %s\n", fe.ModTime.Format(time.DateTime))
	}
	if fe.ContentHash != "" {
		sh.writef("  Hash:      %s\n", fe.ContentHash)
	}
	sh.writef("  Chunk:     %s\n", fe.S3Key)
	if fe.DVCMetadata != nil && fe.DVCMetadata.Stage != "" {
		sh.writef("  DVC stage: %s\n", fe.DVCMetadata.Stage)
	}
	m := sh.vfs.Manifest()
	if m.GitMetadata != nil && m.GitMetadata.Commit != "" {
		sh.writef("  Commit:    %s\n", m.GitMetadata.Commit)
	}
}

func (sh *archiveShell) cmdCat(ctx context.Context, args []string) {
	if len(args) == 0 {
		sh.writeln("usage: cat <file>")
		return
	}
	p := sh.vfs.Resolve(sh.cwd, args[0])
	if sh.vfs.Stat(p) == nil {
		sh.writef("cat: %s: no such file\n", args[0])
		return
	}
	sh.streamFile(ctx, "cat", p, func(f *os.File) {
		_, _ = io.Copy(sh.out, f)
	})
}

func (sh *archiveShell) cmdHead(ctx context.Context, args []string) {
	if len(args) == 0 {
		sh.writeln("usage: head <file> [n]")
		return
	}
	n := 10
	if len(args) > 1 {
		if v, err := strconv.Atoi(args[1]); err == nil && v > 0 {
			n = v
		}
	}
	p := sh.vfs.Resolve(sh.cwd, args[0])
	if sh.vfs.Stat(p) == nil {
		sh.writef("head: %s: no such file\n", args[0])
		return
	}
	sh.streamFile(ctx, "head", p, func(f *os.File) {
		sc := bufio.NewScanner(f)
		for i := 0; i < n && sc.Scan(); i++ {
			sh.writeln(sc.Text())
		}
	})
}

func (sh *archiveShell) cmdFind(args []string) {
	if len(args) == 0 {
		sh.writeln("usage: find <pattern>")
		return
	}
	results := sh.vfs.FindGlob(args[0])
	if len(results) == 0 {
		sh.writef("find: no files matching %q\n", args[0])
		return
	}
	for _, fe := range results {
		sh.printFileEntry(fe.Path, fe)
	}
}

func (sh *archiveShell) cmdStage(args []string) {
	if len(args) == 0 || args[0] == "list" {
		stages := sh.vfs.Stages()
		if len(stages) == 0 {
			sh.writeln("  (no DVC stage metadata in this archive)")
			return
		}
		// Sort stage names for consistent output.
		names := make([]string, 0, len(stages))
		for n := range stages {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			sh.writef("  %-24s  %d file(s)\n", n, stages[n])
		}
		return
	}
	// List files for named stage.
	files := sh.vfs.FilesForStage(args[0])
	if len(files) == 0 {
		sh.writef("stage: %q: no files found\n", args[0])
		return
	}
	for _, fe := range files {
		sh.printFileEntry(fe.Path, fe)
	}
}

func (sh *archiveShell) cmdGet(ctx context.Context, args []string) {
	if len(args) == 0 {
		sh.writeln("usage: get <file> [dest]")
		return
	}
	p := sh.vfs.Resolve(sh.cwd, args[0])
	if sh.vfs.Stat(p) == nil {
		sh.writef("get: %s: no such file\n", args[0])
		return
	}
	dest := "."
	if len(args) > 1 {
		dest = args[1]
	}
	stats, err := sh.se.BatchRestore(ctx, []string{p}, dest)
	if err != nil {
		sh.writef("get: restore failed: %v\n", err)
		return
	}
	if stats.Restored > 0 {
		local := filepath.Join(dest, filepath.FromSlash(p))
		sh.writef("✅ Restored → %s\n", local)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// streamFile extracts p to a temp directory, opens it, calls fn, then cleans up.
func (sh *archiveShell) streamFile(ctx context.Context, cmdName, p string, fn func(*os.File)) {
	tmpDir, err := os.MkdirTemp("", "cargoship-"+cmdName+"-*")
	if err != nil {
		sh.writef("%s: failed to create temp dir: %v\n", cmdName, err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if _, err := sh.se.BatchRestore(ctx, []string{p}, tmpDir); err != nil {
		sh.writef("%s: restore failed: %v\n", cmdName, err)
		return
	}
	f, err := os.Open(filepath.Join(tmpDir, filepath.FromSlash(p)))
	if err != nil {
		sh.writef("%s: open failed: %v\n", cmdName, err)
		return
	}
	defer func() { _ = f.Close() }()
	fn(f)
}

// printFileEntry prints a formatted line for a file entry.
func (sh *archiveShell) printFileEntry(displayName string, fe *manifest.FileEntry) {
	size := humanize.Bytes(uint64(fe.Size))
	var meta []string
	if fe.DVCMetadata != nil && fe.DVCMetadata.Stage != "" {
		meta = append(meta, "stage:"+fe.DVCMetadata.Stage)
	}
	if fe.ContentHash != "" {
		meta = append(meta, "hash:"+fe.ContentHash[:8]+"…")
	}
	if len(meta) > 0 {
		sh.writef("  %-40s  %10s  [%s]\n", displayName, size, strings.Join(meta, "  "))
	} else {
		sh.writef("  %-40s  %10s\n", displayName, size)
	}
}
