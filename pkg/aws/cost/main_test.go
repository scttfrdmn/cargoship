package cost

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates the on-disk project-budget store (#241) so tests never read
// or write the developer's real ~/.cargoship/budgets.json. Each test that needs
// a clean slate can still point CARGOSHIP_BUDGET_STORE at its own t.TempDir().
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cargoship-cost-budget-store")
	if err != nil {
		panic("create temp budget store dir: " + err.Error())
	}
	_ = os.Setenv(budgetStoreEnvOverride, filepath.Join(dir, "budgets.json"))

	code := m.Run()

	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// isolateBudgetStore points the persistent budget store (#241) at a fresh file
// unique to this test, so budget mutations in one test can't leak into another.
func isolateBudgetStore(t *testing.T) {
	t.Helper()
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))
}
