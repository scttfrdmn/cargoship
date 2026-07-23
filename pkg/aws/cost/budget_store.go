package cost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
// filesystem at budgetStorePath, written atomically. It ignores the concurrency
// token (whole-document read/modify/write); the S3 store added in Phase B (#246)
// uses the token for ETag/If-Match conditional writes.
type localStore struct{}

// Load reads the persisted budget document. A missing file is not an error —
// it returns an empty, current-version state (the store simply hasn't been
// written yet). Legacy pre-versioning files (a bare
// map[string]config.ProjectBudget) are migrated in memory to the current shape;
// they're rewritten in the new shape on the next Save.
func (localStore) Load() (LedgerState, Token, error) {
	path, err := budgetStorePath()
	if err != nil {
		return LedgerState{}, "", err
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from home/XDG, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return LedgerState{Version: StoreVersion, ProjectBudgets: map[string]config.ProjectBudget{}}, "", nil
		}
		return LedgerState{}, "", fmt.Errorf("read budget store %q: %w", path, err)
	}
	state, err := decodeLedgerState(data)
	if err != nil {
		return LedgerState{}, "", fmt.Errorf("parse budget store %q: %w", path, err)
	}
	return state, "", nil
}

// Save writes the budget document atomically (temp file in the same dir, then
// rename) so a crash mid-write can't corrupt the store. The file is written
// 0600 since budgets may reference grant numbers. The token is ignored.
func (localStore) Save(state LedgerState, _ Token) error {
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
	return atomicWriteFile(path, data)
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
