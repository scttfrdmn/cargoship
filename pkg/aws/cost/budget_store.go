package cost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// budgetStoreEnvOverride lets callers (notably tests) redirect the budget store
// to a scratch location instead of the user's home directory.
const budgetStoreEnvOverride = "CARGOSHIP_BUDGET_STORE"

// budgetStorePath returns the path to the persisted budget store file:
//   - $CARGOSHIP_BUDGET_STORE if set (an explicit file path), else
//   - $XDG_DATA_HOME/cargoship/budgets.json, else
//   - ~/.cargoship/budgets.json
//
// This mirrors the state-dir convention used by the restore and resume packages.
func budgetStorePath() (string, error) {
	if override := os.Getenv(budgetStoreEnvOverride); override != "" {
		return override, nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		base = filepath.Join(home, ".cargoship")
	} else {
		base = filepath.Join(base, "cargoship")
	}
	return filepath.Join(base, "budgets.json"), nil
}

// localStore is the default BudgetStore: a single JSON document on the local
// filesystem at budgetStorePath, written atomically. Its Token is a content hash
// of the file a Load read; Save takes a lock file, re-reads, and refuses with
// ErrBudgetStoreConflict if the file changed since (#1031).
type localStore struct{}

const (
	// localTokenAbsent is the Token for a store file that did not exist at Load.
	localTokenAbsent Token = "absent"

	// storeLockStaleAfter is how old a lock file must be before a writer treats it
	// as left behind by a crashed process and removes it. A save holds the lock
	// for milliseconds.
	storeLockStaleAfter = 30 * time.Second

	// storeLockWait bounds how long a writer waits for a live lock.
	storeLockWait = 10 * time.Second

	storeLockPoll = 5 * time.Millisecond
)

// localToken returns the concurrency token for store bytes read from disk.
func localToken(data []byte) Token {
	sum := sha256.Sum256(data)
	return Token("sha256:" + hex.EncodeToString(sum[:]))
}

// corruptStoreError wraps a parse failure of the store at path so it names the
// file, says what to do, and matches ErrBudgetStoreCorrupt.
func corruptStoreError(path string, err error) error {
	return fmt.Errorf("%w: \"%s\": %v (budgets cannot be enforced from it; cargoship will not overwrite it, so repair the file or move it aside)",
		ErrBudgetStoreCorrupt, path, err)
}

// readLocalStore reads and parses the store file. A missing file is an empty,
// current-version state with the absent token.
func readLocalStore(path string) (LedgerState, Token, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from home/XDG, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return LedgerState{Version: StoreVersion, ProjectBudgets: map[string]config.ProjectBudget{}}, localTokenAbsent, nil
		}
		return LedgerState{}, "", fmt.Errorf("read budget store %q: %w", path, err)
	}
	state, err := decodeLedgerState(data)
	if err != nil {
		return LedgerState{}, "", corruptStoreError(path, err)
	}
	return state, localToken(data), nil
}

// Load reads the persisted budget document. A missing file is not an error —
// it returns an empty, current-version state (the store simply hasn't been
// written yet). Legacy pre-versioning files (a bare
// map[string]config.ProjectBudget) are migrated in memory to the current shape;
// they're rewritten in the new shape on the next Save. An unparseable file is an
// error wrapping ErrBudgetStoreCorrupt.
func (localStore) Load() (LedgerState, Token, error) {
	path, err := budgetStorePath()
	if err != nil {
		return LedgerState{}, "", err
	}
	return readLocalStore(path)
}

// Save writes the budget document atomically (temp file in the same dir, then
// rename) so a crash mid-write can't corrupt the store. The file is written
// 0600 since budgets may reference grant numbers.
//
// Under a lock file it re-reads the store first: an unparseable file is never
// overwritten (ErrBudgetStoreCorrupt), and a non-empty token that no longer
// matches the file is a conflict (ErrBudgetStoreConflict). An empty token skips
// only the conflict check.
func (localStore) Save(state LedgerState, token Token) error {
	if state.Version == 0 {
		state.Version = StoreVersion
	}
	if state.ProjectBudgets == nil {
		state.ProjectBudgets = map[string]config.ProjectBudget{}
	}
	path, err := budgetStorePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal budget store: %w", err)
	}
	unlock, err := lockStoreFile(path)
	if err != nil {
		return err
	}
	defer unlock()

	_, current, err := readLocalStore(path)
	if err != nil {
		return err
	}
	if token != "" && token != current {
		return fmt.Errorf("save budget store \"%s\": %w", path, ErrBudgetStoreConflict)
	}
	return atomicWriteFile(path, data)
}

// lockStoreFile takes an exclusive lock on the store at path by creating
// path+".lock" with O_EXCL, waiting up to storeLockWait for another writer. A
// lock older than storeLockStaleAfter was left by a crashed process and is
// removed. The returned func releases the lock.
func lockStoreFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create budget store dir: %w", err)
	}
	lockPath := path + ".lock"
	deadline := time.Now().Add(storeLockWait)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) //nolint:gosec // derived from the store path
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("lock budget store \"%s\": %w", path, err)
		}
		if fi, sErr := os.Stat(lockPath); sErr == nil && time.Since(fi.ModTime()) > storeLockStaleAfter {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("budget store \"%s\" is locked by another cargoship process; if none is running, remove \"%s\"", path, lockPath)
		}
		time.Sleep(storeLockPoll)
	}
}

// decodeLedgerState parses budget-store bytes, migrating the legacy format
// (a bare map[string]config.ProjectBudget with no "version" field) to the
// current LedgerState shape. Detection: a versioned document has a top-level
// "version" key; a legacy document's top-level keys are all project IDs.
func decodeLedgerState(data []byte) (LedgerState, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return LedgerState{}, err
	}
	// New format: has "version" (and/or "project_budgets").
	if _, ok := probe["version"]; ok {
		var state LedgerState
		if err := json.Unmarshal(data, &state); err != nil {
			return LedgerState{}, err
		}
		if state.ProjectBudgets == nil {
			state.ProjectBudgets = map[string]config.ProjectBudget{}
		}
		return state, nil
	}
	// Legacy format: a bare map of project ID -> ProjectBudget.
	var legacy map[string]config.ProjectBudget
	if err := json.Unmarshal(data, &legacy); err != nil {
		return LedgerState{}, err
	}
	if legacy == nil {
		legacy = map[string]config.ProjectBudget{}
	}
	return LedgerState{Version: StoreVersion, ProjectBudgets: legacy}, nil
}

// atomicWriteFile writes data to path via a temp file in the same directory
// followed by a rename, creating the parent dir 0700 and the file 0600.
func atomicWriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create budget store dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".budgets-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp budget file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp budget file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp budget file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp budget file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename budget store into place: %w", err)
	}
	return nil
}

// loadProjectBudgets reads persisted project budgets (limits only). A missing
// file is not an error (returns an empty map). Retained as a thin adapter over
// localStore for callers that only need the limits view.
func loadProjectBudgets() (map[string]config.ProjectBudget, error) {
	state, _, err := localStore{}.Load()
	if err != nil {
		return nil, err
	}
	if state.ProjectBudgets == nil {
		return map[string]config.ProjectBudget{}, nil
	}
	return state.ProjectBudgets, nil
}

// saveProjectBudgets writes project budgets (limits only), preserving any
// existing recorded ledger in the store so a limits update can't clobber spend
// history (#246). Retained as a thin adapter over localStore.
func saveProjectBudgets(budgets map[string]config.ProjectBudget) error {
	store := localStore{}
	state, tok, err := store.Load()
	if err != nil {
		return err
	}
	state.ProjectBudgets = budgets
	return store.Save(state, tok)
}
