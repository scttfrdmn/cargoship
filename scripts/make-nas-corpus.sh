#!/usr/bin/env bash
#
# make-nas-corpus.sh — build a small, deliberately hostile corpus for NAS hardware tests,
# and verify a restore of it BY RELATIVE PATH.
#
# The release lane's corpus is adversarial but synthetic: it has no idea what a real NAS
# share looks like. Synology scatters `@eaDir` thumbnail directories and a `#recycle` bin
# through every share, desktop clients leave `.DS_Store`/`Thumbs.db` everywhere, and real
# trees carry hardlinks, symlinks, sparse files and clock skew. Those shapes are where a
# backup tool actually breaks, so this generates them on purpose.
#
# Deliberately SMALL (~15 MB). The point is shape coverage, not volume: a multi-TB share
# costs real money and days, and finds nothing this does not.
#
# Usage:
#   scripts/make-nas-corpus.sh build  DIR    # create the corpus (refuses a non-empty DIR)
#   scripts/make-nas-corpus.sh verify SRC DST # compare restore against source by REL PATH
#
set -euo pipefail

die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# sha256 differs across macOS and Linux.
if command -v sha256sum >/dev/null 2>&1; then
  sha() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  die "need sha256sum or shasum"
fi

build() {
  local root="$1"
  [ -n "$root" ] || die "build needs a target directory"
  if [ -e "$root" ] && [ -n "$(ls -A "$root" 2>/dev/null || true)" ]; then
    die "$root exists and is not empty — refusing to scribble into it"
  fi
  mkdir -p "$root"

  # --- Synology/QNAP furniture -------------------------------------------------
  # @eaDir holds thumbnails and indexer metadata and appears INSIDE almost every
  # directory on a Synology share. #recycle is the share-level recycle bin. Both are
  # real content a naive backup happily uploads; both are the kind of thing an operator
  # later discovers they did not want. We back them up here on purpose: the test is that
  # they round-trip, not that we filter them.
  mkdir -p "$root/photos/@eaDir/holiday.jpg"
  printf 'SYNOPHOTO_THUMB_XL binary-ish \x01\x02\x03' > "$root/photos/@eaDir/holiday.jpg/SYNOPHOTO_THUMB_XL.jpg"
  printf 'SYNOPHOTO_THUMB_M  binary-ish \x04\x05\x06' > "$root/photos/@eaDir/holiday.jpg/SYNOPHOTO_THUMB_M.jpg"
  mkdir -p "$root/#recycle"
  printf 'deleted last tuesday\n' > "$root/#recycle/old-notes.txt"
  printf '\x00\x00desktop metadata\x00' > "$root/photos/.DS_Store"
  printf 'thumbs db\n' > "$root/photos/Thumbs.db"

  # --- names that break naive path handling -----------------------------------
  mkdir -p "$root/names"
  printf 'spaces\n'        > "$root/names/a file with spaces.txt"
  printf 'leading dash\n'  > "$root/names/-leading-dash.txt"
  printf 'single quote\n'  > "$root/names/it's-fine.txt"
  printf 'hash#and%%pct\n' > "$root/names/hash#and%pct.txt"
  printf 'unicode cjk\n'   > "$root/names/日本語のファイル.txt"
  printf 'unicode emoji\n' > "$root/names/emoji-🚢-ship.txt"
  # NFD vs NFC: macOS normalises, Linux does not. If both land, a tool keying on the
  # raw bytes sees two files; one keying on normalised form sees one and loses data.
  printf 'nfc cafe\n'      > "$root/names/café-nfc.txt"
  printf 'dotfile\n'       > "$root/names/.hidden-config"
  printf 'no extension\n'  > "$root/names/README"
  : > "$root/names/empty-file.txt"

  # --- depth ------------------------------------------------------------------
  mkdir -p "$root/deep/a/b/c/d/e/f/g/h"
  printf 'bottom of the well\n' > "$root/deep/a/b/c/d/e/f/g/h/leaf.txt"

  # --- link shapes ------------------------------------------------------------
  mkdir -p "$root/links"
  printf 'hardlink target content\n' > "$root/links/hardlink-a.txt"
  ln "$root/links/hardlink-a.txt" "$root/links/hardlink-b.txt" 2>/dev/null \
    || printf 'hardlink unsupported here\n' > "$root/links/hardlink-b.txt"
  ln -s hardlink-a.txt "$root/links/symlink-rel.txt"
  ln -s /nonexistent/target "$root/links/symlink-broken.txt"

  # --- compressibility extremes ------------------------------------------------
  mkdir -p "$root/content"
  # Incompressible: zstd should give up and store.
  dd if=/dev/urandom of="$root/content/random-4mb.bin" bs=1024 count=4096 status=none
  # Highly compressible: should shrink enormously.
  #
  # `head -c` closes the pipe once it has enough, which kills `yes` with SIGPIPE (141).
  # That is the intended end of the command, not a failure -- but `pipefail` cannot tell
  # the difference, so it must be off for exactly this line. Same trap as the fleet-image
  # CI guard, where `docker | grep -q` died the same way.
  set +o pipefail
  yes 'the same line over and over and over' | head -c 4194304 > "$root/content/repetitive-4mb.txt"
  set -o pipefail
  # Already-compressed: a gzip member, so content-aware selection should skip it.
  printf 'pretend archive payload %.0s' $(seq 1 2000) | gzip -c > "$root/content/already.gz"
  # Sparse: apparent size >> allocated blocks.
  dd if=/dev/zero of="$root/content/sparse-8mb.img" bs=1 count=1 seek=8388607 status=none

  # --- clock skew --------------------------------------------------------------
  # Change detection is size+mtime, so these are the inputs that make or break a delta.
  mkdir -p "$root/times"
  printf 'old file\n' > "$root/times/ancient.txt"
  touch -t 200001010000 "$root/times/ancient.txt"
  printf 'future file\n' > "$root/times/from-the-future.txt"
  # +1 day; a NAS with a bad RTC really does produce these.
  if date -v+1d >/dev/null 2>&1; then
    touch -t "$(date -v+1d +%Y%m%d%H%M)" "$root/times/from-the-future.txt"   # BSD/macOS
  else
    touch -d 'tomorrow' "$root/times/from-the-future.txt"                     # GNU
  fi

  # --- permission shapes -------------------------------------------------------
  mkdir -p "$root/perms"
  printf 'read only\n' > "$root/perms/read-only.txt";  chmod 0444 "$root/perms/read-only.txt"
  printf 'executable\n' > "$root/perms/script.sh";     chmod 0755 "$root/perms/script.sh"

  local files bytes
  files=$(find "$root" -type f | wc -l | tr -d ' ')
  bytes=$(find "$root" -type f -exec cat {} + 2>/dev/null | wc -c | tr -d ' ')
  printf 'built corpus at %s\n' "$root"
  printf '  regular files: %s\n' "$files"
  printf '  logical bytes: %s\n' "$bytes"
  printf '  symlinks: %s   hardlinked pairs: 1   sparse: 1\n' "$(find "$root" -type l | wc -l | tr -d ' ')"
}

# verify compares by RELATIVE PATH, never by basename. A restore that puts the right bytes
# at the wrong path is a failure, and basename comparison cannot see it -- that is the
# class of bug the 2026-09 data-integrity hunt turned up (#486).
verify() {
  local src="$1" dst="$2"
  [ -d "$src" ] || die "source $src is not a directory"
  [ -d "$dst" ] || die "restored $dst is not a directory"

  local missing=0 differing=0 checked=0 extra=0
  while IFS= read -r rel; do
    checked=$((checked+1))
    if [ ! -f "$dst/$rel" ]; then
      printf 'MISSING   %s\n' "$rel"; missing=$((missing+1)); continue
    fi
    if [ "$(sha "$src/$rel")" != "$(sha "$dst/$rel")" ]; then
      printf 'DIFFERS   %s\n' "$rel"; differing=$((differing+1))
    fi
  done < <(cd "$src" && find . -type f | sed 's|^\./||' | sort)

  while IFS= read -r rel; do
    [ -f "$src/$rel" ] || { printf 'UNEXPECTED %s\n' "$rel"; extra=$((extra+1)); }
  done < <(cd "$dst" && find . -type f | sed 's|^\./||' | sort)

  printf '\nchecked %d files by relative path: %d missing, %d differing, %d unexpected\n' \
    "$checked" "$missing" "$differing" "$extra"
  if [ "$missing" -ne 0 ] || [ "$differing" -ne 0 ] || [ "$extra" -ne 0 ]; then
    printf 'FAIL\n'; return 1
  fi
  printf 'PASS — every file byte-identical at its original relative path\n'
}

case "${1:-}" in
  build)  shift; build "${1:-}" ;;
  verify) shift; verify "${1:-}" "${2:-}" ;;
  *) die "usage: $0 build DIR | $0 verify SRC DST" ;;
esac
