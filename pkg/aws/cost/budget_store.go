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

// budgetStorePath returns the path to the persisted project-budgets file:
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

// loadProjectBudgets reads persisted project budgets from disk. A missing file
// is not an error (returns an empty map) — the store simply hasn't been written
// yet.
func loadProjectBudgets() (map[string]config.ProjectBudget, error) {
	path, err := budgetStorePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from home/XDG, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]config.ProjectBudget{}, nil
		}
		return nil, fmt.Errorf("read budget store %q: %w", path, err)
	}
	var budgets map[string]config.ProjectBudget
	if err := json.Unmarshal(data, &budgets); err != nil {
		return nil, fmt.Errorf("parse budget store %q: %w", path, err)
	}
	if budgets == nil {
		budgets = map[string]config.ProjectBudget{}
	}
	return budgets, nil
}

// saveProjectBudgets writes project budgets to disk atomically (write to a temp
// file in the same directory, then rename) so a crash mid-write can't corrupt
// the store. The file is written 0600 since budgets may reference grant numbers.
func saveProjectBudgets(budgets map[string]config.ProjectBudget) error {
	path, err := budgetStorePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create budget store dir: %w", err)
	}
	data, err := json.MarshalIndent(budgets, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal budgets: %w", err)
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
