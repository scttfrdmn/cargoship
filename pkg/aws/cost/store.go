package cost

import "github.com/scttfrdmn/cargoship/pkg/aws/config"

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
}

// Token is an opaque optimistic-concurrency token returned by Load and passed
// back to Save. The local file store does not use it (returns ""); the S3 store
// added in Phase B (#246) carries the object ETag here for If-Match writes.
type Token string

// BudgetStore abstracts durable persistence of budget limits and the cost
// ledger. Load returns the current state plus a concurrency token; Save writes
// state, passing back the token from the prior Load so a compare-and-swap
// implementation can reject a stale write.
//
// The local implementation (localStore) is whole-document read/modify/write and
// ignores the token. Phase B adds an S3-backed implementation that uses it for
// ETag/If-Match conditional writes.
type BudgetStore interface {
	Load() (LedgerState, Token, error)
	Save(state LedgerState, token Token) error
}
