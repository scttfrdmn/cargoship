package cost

import (
	"errors"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// ErrBudgetStoreConflict reports that the budget store changed between the Load
// that produced a Token and the Save that presented it: another process wrote
// first. The write was not applied; the caller re-loads, re-applies its own
// changes, and retries (#1031). Test with errors.Is.
var ErrBudgetStoreConflict = errors.New("budget store changed concurrently")

// ErrBudgetStoreCorrupt reports that the persisted budget store exists but
// cannot be parsed. Budgets cannot be enforced from it, and no store
// implementation overwrites it: the file (or object) is left byte-for-byte as
// found for the user to repair or move aside (#1031). Test with errors.Is.
var ErrBudgetStoreCorrupt = errors.New("budget store is unreadable")

// StoreVersion is the current on-disk schema version for the budget store.
// Version 1 introduced the LedgerState wrapper (project budgets + recorded cost
// ledger); legacy files (a bare map[string]config.ProjectBudget with no version
// field) are migrated to this shape on load. See localStore.Load.
const StoreVersion = 1

// LedgerState is the full persisted budget document: the project budget limits
// and the recorded cost ledger, versioned so future schema changes can migrate.
type LedgerState struct {
	// Version is the schema version (StoreVersion). Zero means a legacy,
	// pre-versioning document that Load migrates in memory.
	Version int `json:"version"`

	// ProjectBudgets maps project ID (manifest upload ID) to its budget limits.
	ProjectBudgets map[string]config.ProjectBudget `json:"project_budgets"`

	// Records is the recorded cost ledger — the running history of upload costs
	// that backs `budget status` spend/volume totals. Bounded by the caller
	// (FIFO rotation) so the file can't grow without limit.
	Records []CostRecord `json:"records,omitempty"`

	// GlobalBudget is the persisted org/team-wide budget ceiling (#246 Phase B
	// PR2). Pointer + omitempty so pre-PR2 documents (and every local file
	// written before this) round-trip unchanged: absent → nil.
	GlobalBudget *config.GlobalBudget `json:"global_budget,omitempty"`

	// AlertConfig is the persisted alert-channel configuration (#630). Pointer +
	// omitempty so older documents round-trip unchanged. Its secret fields are
	// tagged json:"-", so persisting it never writes plaintext credentials; secrets
	// are sourced from the environment at notifier-build time (see withSecretsFromEnv).
	AlertConfig *BudgetAlertConfig `json:"alert_config,omitempty"`
}

// Token is an opaque optimistic-concurrency token returned by Load and passed
// back to Save. The local file store carries a content hash of the file it read;
// the S3 store (#246 Phase B) carries the per-object ETags for If-Match writes.
type Token string

// BudgetStore abstracts durable persistence of budget limits and the cost
// ledger. Load returns the current state plus a concurrency token; Save writes
// state, passing back the token from the prior Load.
//
// Save is a compare-and-swap (#1031): if the store changed since that Load, it
// writes nothing and returns an error wrapping ErrBudgetStoreConflict. Callers
// that must not lose another process's update (Manager.saveState) re-load,
// re-apply only their own changes, and retry. An empty token means "no prior
// Load" and is accepted unconditionally by the local store. No implementation
// overwrites a store it cannot parse: Load and Save both return an error
// wrapping ErrBudgetStoreCorrupt instead.
type BudgetStore interface {
	Load() (LedgerState, Token, error)
	Save(state LedgerState, token Token) error
}
