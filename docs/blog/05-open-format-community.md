# Open Format, Open Source: Building on CargoShip

**Published**: December 2025
**Author**: CargoShip Team
**Read time**: 12 minutes

---

Over the past four posts, we've shown you [why we built CargoShip](01-the-s3-upload-problem.md), [how it works](02-zero-disk-architecture.md), [why it's fast](03-multi-prefix-deep-dive.md), and [how it saves money](04-cost-optimization.md).

But there's one question we haven't addressed yet: **Why should you trust your data to CargoShip?**

This is the right question. Data is precious. Once uploaded, you need confidence that you can get it back—not just today, but 5, 10, or 20 years from now. Will CargoShip still exist? Will the format still be readable? What if you need to switch tools?

This post is about our answer to those questions: **open formats, open source, and open collaboration**.

## The Vendor Lock-In Problem

Most backup and archival tools use proprietary formats. This creates dependency:

### Example: Popular Backup Tool X

You use Tool X to backup 100TB to S3. The data is stored in Tool X's custom format:
```
s3://bucket/backups/
├── catalog.db (proprietary)
├── chunk_a8f3e21.dat (proprietary, encrypted)
├── chunk_b4c8912.dat (proprietary, encrypted)
└── ...
```

**What happens if:**
- Tool X shuts down or stops supporting your OS?
- The company is acquired and the product is discontinued?
- You want to switch to a competing tool?
- You need to access data 10 years from now and Tool X no longer exists?

Your options:
1. **Keep using Tool X forever** (hope it still works)
2. **Download everything and re-upload with a new tool** (expensive, time-consuming)
3. **Reverse-engineer the format** (difficult, error-prone)
4. **Hire the original developers** (if you can find them)

None of these are good.

## The CargoShip Philosophy: Open and Portable

CargoShip takes a different approach. **Every uploaded archive uses standard, open formats that are readable without CargoShip.**

### Storage Format

```
s3://bucket/prefix/uploads/<upload-id>/
├── shard-0/
│   ├── chunk-0.tar.zst
│   ├── chunk-1.tar.zst
│   └── ...
├── shard-1/
│   └── ...
└── manifest.json.gz
```

Let's break down each component:

#### 1. `.tar` Format

Tar (Tape Archive) is a **40-year-old Unix standard** (POSIX IEEE P1003.1). It's supported by every Unix-like system and has open-source implementations in every programming language.

The format is **trivial to parse**. The header spec is 512 bytes with fixed-width fields:

```
Offset  Size  Field
0       100   File name
100     8     File mode
108     8     Owner's numeric user ID
116     8     Group's numeric user ID
124     12    File size in bytes (octal)
136     12    Last modification time (octal)
148     8     Checksum for header record
156     1     Link indicator (file type)
...
```

You could implement a tar reader in 200 lines of C. Or use one of the thousands of existing implementations.

#### 2. `.zst` Compression

Zstandard is an **open-source compression algorithm** (BSD + MIT licenses) developed by Facebook/Meta. The spec is publicly available, and there are implementations in:

- **C**: `zstd` (reference implementation)
- **Go**: `github.com/klauspost/compress/zstd`
- **Python**: `python-zstandard`
- **Rust**: `zstd` crate
- **Java**: `zstd-jni`
- **JavaScript**: `@mongodb-js/zstd`

It's **used by major companies**: Facebook, Netflix, Apple, Dropbox, Android, Linux kernel.

Unlike proprietary compression formats, zstd will be readable for decades.

#### 3. `manifest.json` Format

The manifest is a **simple JSON file** (gzip compressed for efficiency):

```json
{
  "version": "1.0",
  "upload_id": "20251215-abc123",
  "timestamp": "2025-12-15T10:30:00Z",
  "source_path": "/data/genomics",
  "total_files": 847000,
  "total_bytes": 2147483648000,
  "compression_ratio": 0.62,
  "chunks": [
    {
      "chunk_id": 0,
      "shard_id": 0,
      "s3_key": "shard-0/chunk-0.tar.zst",
      "compressed_size": 104857600,
      "uncompressed_size": 167772160,
      "file_count": 523,
      "checksum": "sha256:a8f3e21b...",
      "files": [
        {
          "path": "experiment1/data.fastq",
          "size": 1048576,
          "checksum": "sha256:b4c8912d...",
          "offset": 0,
          "permissions": "0644"
        }
      ]
    }
  ]
}
```

JSON is **the most widely-supported data format** on the planet. Every programming language can parse it.

## Manual Recovery: No Tool Required

Here's the key promise: **You can restore your data with nothing but standard Unix tools.**

### Complete Manual Restore Example

Let's say CargoShip no longer exists. You need to restore a 2TB genomics dataset from S3. Here's how:

#### Step 1: List Uploads

```bash
# List available uploads
aws s3 ls s3://genomics-archive/experiments/exp-42/uploads/
```

Output:
```
                           PRE 20251215-abc123/
                           PRE 20251210-def456/
```

#### Step 2: Download Manifest

```bash
aws s3 cp s3://genomics-archive/experiments/exp-42/uploads/20251215-abc123/manifest.json.gz .
gunzip manifest.json.gz
```

#### Step 3: Parse Manifest (Any Language)

Python example:
```python
import json

with open('manifest.json') as f:
    manifest = json.load(f)

print(f"Upload: {manifest['upload_id']}")
print(f"Files: {manifest['total_files']:,}")
print(f"Size: {manifest['total_bytes'] / (1024**3):.1f} GB")
print(f"Chunks: {len(manifest['chunks'])}")

# Show first 10 files
for chunk in manifest['chunks'][:1]:
    for file in chunk['files'][:10]:
        print(f"  {file['path']} ({file['size']:,} bytes)")
```

#### Step 4: Download and Extract Chunks

```bash
# Download all chunks
mkdir -p chunks
for shard in {0..7}; do
    aws s3 sync \
        s3://genomics-archive/experiments/exp-42/uploads/20251215-abc123/shard-$shard/ \
        chunks/shard-$shard/
done

# Extract all chunks
mkdir -p restored
for chunk in chunks/*/*.tar.zst; do
    tar --use-compress-program=unzstd -xf "$chunk" -C restored/
done
```

That's it. **No proprietary tools. No reverse-engineering. Just standard formats.**

### Selective File Recovery

What if you only need one file from a 2TB archive?

```python
# find_file.py
import json
import subprocess

def find_file(manifest_path, target_file):
    with open(manifest_path) as f:
        manifest = json.load(f)

    for chunk in manifest['chunks']:
        for file in chunk['files']:
            if file['path'] == target_file:
                return {
                    'chunk_id': chunk['chunk_id'],
                    'shard_id': chunk['shard_id'],
                    's3_key': chunk['s3_key'],
                    'offset': file['offset'],
                    'size': file['size']
                }
    return None

# Find the file
location = find_file('manifest.json', 'experiment1/critical-data.fastq')

if location:
    # Download only the chunk containing the file
    s3_path = f"s3://bucket/prefix/uploads/20251215-abc123/{location['s3_key']}"
    subprocess.run(['aws', 's3', 'cp', s3_path, './chunk.tar.zst'])

    # Extract only the target file
    subprocess.run([
        'tar', '--use-compress-program=unzstd',
        '-xf', './chunk.tar.zst',
        location['path']  # Extract specific file
    ])

    print(f"Restored: {location['path']}")
```

You just restored 1 file from 2TB without downloading the full dataset. **And you did it without CargoShip.**

## Open Source: Build Your Own Tools

CargoShip is **fully open source** (Apache 2.0). You can:
- Read the code
- Modify it for your needs
- Build custom integrations
- Create competing products

No licensing fees. No usage restrictions. No vendor approval required.

### Example: Custom Manifest Query Tool

Build a tool to search manifests without downloading data:

```go
// manifest-search.go
package main

import (
    "compress/gzip"
    "encoding/json"
    "fmt"
    "os"
    "strings"
)

type Manifest struct {
    UploadID   string `json:"upload_id"`
    TotalFiles int    `json:"total_files"`
    TotalBytes int64  `json:"total_bytes"`
    Chunks     []Chunk `json:"chunks"`
}

type Chunk struct {
    ChunkID int    `json:"chunk_id"`
    Files   []File `json:"files"`
}

type File struct {
    Path     string `json:"path"`
    Size     int64  `json:"size"`
    Checksum string `json:"checksum"`
}

func main() {
    query := os.Args[1]

    f, _ := os.Open("manifest.json.gz")
    gz, _ := gzip.NewReader(f)

    var manifest Manifest
    json.NewDecoder(gz).Decode(&manifest)

    fmt.Printf("Searching %d files...\n", manifest.TotalFiles)

    matches := 0
    for _, chunk := range manifest.Chunks {
        for _, file := range chunk.Files {
            if strings.Contains(file.Path, query) {
                fmt.Printf("  %s (%d bytes)\n", file.Path, file.Size)
                matches++
            }
        }
    }

    fmt.Printf("\nFound %d matches\n", matches)
}
```

Usage:
```bash
go run manifest-search.go "experiment"
```

This searches 847,000 files **instantly** without touching S3. You just built a custom tool in 50 lines of Go.

## Integration Opportunities

CargoShip's open format enables integrations:

### 1. Data Catalogs

Build metadata search engines that index manifests:

```python
# catalog.py
import json
import sqlite3

def index_manifest(manifest_path):
    conn = sqlite3.connect('catalog.db')
    c = conn.cursor()

    c.execute('''CREATE TABLE IF NOT EXISTS files
                 (path TEXT, size INTEGER, checksum TEXT,
                  upload_id TEXT, chunk_id INTEGER)''')

    with open(manifest_path) as f:
        manifest = json.load(f)

    for chunk in manifest['chunks']:
        for file in chunk['files']:
            c.execute('INSERT INTO files VALUES (?, ?, ?, ?, ?)',
                     (file['path'], file['size'], file['checksum'],
                      manifest['upload_id'], chunk['chunk_id']))

    conn.commit()

# Index all manifests
for manifest in discover_manifests():
    index_manifest(manifest)

# Query across all uploads
conn = sqlite3.connect('catalog.db')
results = conn.execute(
    "SELECT path, upload_id FROM files WHERE path LIKE '%.fastq' LIMIT 100"
).fetchall()
```

Now you can search **across all uploads** without CargoShip.

### 2. Cost Analysis Tools

Parse manifests to analyze storage costs:

```python
def analyze_costs(manifest):
    by_extension = {}

    for chunk in manifest['chunks']:
        for file in chunk['files']:
            ext = os.path.splitext(file['path'])[1]
            by_extension[ext] = by_extension.get(ext, 0) + file['size']

    print("Storage by file type:")
    for ext, size in sorted(by_extension.items(), key=lambda x: -x[1]):
        gb = size / (1024**3)
        cost = gb * 0.023  # STANDARD pricing
        print(f"  {ext}: {gb:.1f} GB (${cost:.2f}/month)")
```

### 3. Compliance Reporting

Generate audit reports from manifests:

```python
def compliance_report(manifest):
    report = {
        'upload_date': manifest['timestamp'],
        'file_count': manifest['total_files'],
        'total_size': manifest['total_bytes'],
        'checksums': [
            (f['path'], f['checksum'])
            for chunk in manifest['chunks']
            for f in chunk['files']
        ]
    }

    # Sign report
    report['signature'] = sign_report(json.dumps(report))

    return report
```

### 4. Custom Restore Tools

Build domain-specific restore tools:

```bash
# genomics-restore.sh
# Restore only FASTQ files from a genomics archive

MANIFEST="manifest.json"
OUTPUT_DIR="./restored-fastq"

# Parse manifest and find FASTQ files
jq -r '.chunks[].files[] | select(.path | endswith(".fastq")) |
       .path' $MANIFEST > fastq_files.txt

# Download only chunks containing FASTQ files
jq -r '.chunks[] | select(.files[].path | endswith(".fastq")) |
       .s3_key' $MANIFEST | sort -u > chunks_to_download.txt

# Download and extract
while read chunk; do
    aws s3 cp "s3://bucket/prefix/uploads/20251215-abc123/$chunk" .
    tar --use-compress-program=unzstd -xf "$(basename $chunk)" \
        --files-from=fastq_files.txt -C "$OUTPUT_DIR"
done < chunks_to_download.txt
```

## Community Contributions

The open source nature enables community improvements:

### Proposed Community Extensions

**Already in discussion**:
1. **Multi-cloud support** - Azure Blob, Google Cloud Storage
2. **Alternative compression** - brotli, lz4 for different workloads
3. **Encryption layer** - client-side encryption before upload
4. **Deduplication** - cross-upload deduplication
5. **Manifest database** - Centralized manifest query service

**How to contribute**:
```bash
git clone https://github.com/scttfrdmn/cargoship
cd cargoship

# Make your changes
git checkout -b feat/your-feature

# Run tests
go test ./...

# Submit PR
git push origin feat/your-feature
gh pr create
```

No corporate approval. No licensing negotiations. Just code.

## Roadmap: What's Next

CargoShip's roadmap is public and community-driven: [GitHub Issues](https://github.com/scttfrdmn/cargoship/issues)

**Near-term (Q1 2026)**:
- ✅ Streaming pipeline (done, v0.5.1)
- ✅ Multi-prefix sharding (done, v0.5.1)
- ✅ Incremental sync (done, v0.6.0)
- ✅ Budget management (done, v0.6.0)
- ✅ Distributed tracing (done, v0.6.2)
- 🚧 Web UI for manifest browsing
- 🚧 Restore command with selective file extraction
- 🚧 Manifest query API (REST + GraphQL)

**Medium-term (Q2-Q3 2026)**:
- Multi-cloud backends (Azure, GCS)
- Client-side encryption (age, GPG)
- Parallel restore with progress tracking
- Deduplicated uploads across versions

**Long-term (2026+)**:
- Distributed upload agents
- Global metadata index
- AI-powered cost optimization
- Real-time replication

## Why Open Matters

Data outlives software. The tools you use today might not exist in 10 years. By using open formats and open source software, you're protecting your data's future.

**With CargoShip**:
- Your data is readable with standard Unix tools
- No proprietary lock-in
- Community-driven development
- Freedom to fork and modify
- Transparent security and reliability

**Without open formats**:
- Dependency on a single vendor
- Risk of format obsolescence
- Limited integration options
- Potential data loss if tool disappears

## A Real Story: The University Archive

A university faced this exact problem. They had 500TB of research data archived with a proprietary tool from 2010. The company was acquired, the product discontinued, and the tool no longer ran on modern operating systems.

They had three options:
1. Keep an old Windows XP VM running (security nightmare)
2. Pay $50K to the original developers for a one-time extraction
3. Reverse-engineer the format (months of work)

They chose option 2. Painfully expensive, but necessary.

**With CargoShip, this wouldn't happen.** Standard tar.zst files readable by any system, today or 20 years from now.

## Join Us

CargoShip is more than a tool—it's a philosophy. Open formats. Open source. Open collaboration.

If you believe data should outlive the tools that created it, join us:

- **Use CargoShip**: [github.com/scttfrdmn/cargoship](https://github.com/scttfrdmn/cargoship)
- **Contribute**: [CONTRIBUTING.md](https://github.com/scttfrdmn/cargoship/blob/main/CONTRIBUTING.md)
- **Discuss**: [GitHub Discussions](https://github.com/scttfrdmn/cargoship/discussions)
- **Report Issues**: [GitHub Issues](https://github.com/scttfrdmn/cargoship/issues)

We're building the future of data archival—together.

---

## The Complete Series

1. [The S3 Upload Problem: Why We Built CargoShip](01-the-s3-upload-problem.md)
2. [Zero-Disk Architecture: Streaming 100GB to S3](02-zero-disk-architecture.md)
3. [8x Faster: The Multi-Prefix Parallel Upload Deep Dive](03-multi-prefix-deep-dive.md)
4. [Cost Optimization: Saving 90% with Intelligent Tiering](04-cost-optimization.md)
5. **Open Format, Open Source: Building on CargoShip** *(you are here)*

---

**Resources**:
- [CargoShip Repository](https://github.com/scttfrdmn/cargoship)
- [Storage Format Documentation](../STORAGE_FORMAT.md)
- [Contributing Guide](https://github.com/scttfrdmn/cargoship/blob/main/CONTRIBUTING.md)
- [Tar Format Spec (POSIX)](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/pax.html)
- [Zstandard Spec](https://github.com/facebook/zstd/blob/dev/doc/zstd_compression_format.md)

**Thank you for reading!** We'd love to hear your feedback, questions, and ideas. Join the discussion on GitHub.
