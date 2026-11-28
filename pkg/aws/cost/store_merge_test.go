package cost

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1031: two processes that each hold a Manager on the same store must not
// revert or delete each other's budgets and spend records when they save.

const gib = int64(1024 * 1024 * 1024)

// TestManager_InterleavedSavesKeepBothWriters is the local-store reproduction:
// A and B both load an empty store, then save interleaved.
func TestManager_InterleavedSavesKeepBothWriters(t *testing.T) {
	isolateBudgetStore(t)
	ctx := context.Background()

	mgrA, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	mgrB, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)

	require.NoError(t, mgrA.SetProjectBudget("proj-a", 100, 0, 0.8, 0.75))
	require.NoError(t, mgrB.SetProjectBudget("proj-b", 200, 0, 0.8, 0.75))
	require.NoError(t, mgrA.RecordCompletedUpload(ctx, "fa", gib, config.StorageClassStandard, "us-east-1", "job-a", "proj-a", nil))
	require.NoError(t, mgrB.RecordCompletedUpload(ctx, "fb", gib, config.StorageClassStandard, "us-east-1", "job-b", "proj-b", nil))
	require.NoError(t, mgrB.SetGlobalBudget(5000, 0, 0.8, 0.75))
	require.NoError(t, mgrA.SetProjectBudget("proj-a", 150, 0, 0.8, 0.75)) // A saves last, after B's global budget

	got, _, err := localStore{}.Load()
	require.NoError(t, err)
	assertBothWriters(t, got)
	require.NotNil(t, got.GlobalBudget, "B's global budget must survive A's later save")
	assert.Equal(t, 5000.0, got.GlobalBudget.MaxBudget)
	assert.Equal(t, 150.0, got.ProjectBudgets["proj-a"].MaxBudget, "A's own update applies")
}

// TestManager_StaleWriterDoesNotRevertBudget: B loaded proj-a=100, A then raised
// it to 300; B's unrelated save (recording an upload) must not put 100 back.
func TestManager_StaleWriterDoesNotRevertBudget(t *testing.T) {
	isolateBudgetStore(t)
	ctx := context.Background()

	seed, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	require.NoError(t, seed.SetProjectBudget("proj-a", 100, 0, 0.8, 0.75))

	mgrA, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	mgrB, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)

	require.NoError(t, mgrA.SetProjectBudget("proj-a", 300, 0, 0.8, 0.75))
	require.NoError(t, mgrB.RecordCompletedUpload(ctx, "fb", gib, config.StorageClassStandard, "us-east-1", "job-b", "proj-b", nil))

	got, _, err := localStore{}.Load()
	require.NoError(t, err)
	assert.Equal(t, 300.0, got.ProjectBudgets["proj-a"].MaxBudget, "a stale writer must not revert another process's budget")
	assert.Len(t, got.Records, 1)
}

// TestManager_RemovedBudgetStaysRemoved: a budget removed by A must not be
// resurrected by B, which loaded it before the removal.
func TestManager_RemovedBudgetStaysRemoved(t *testing.T) {
	isolateBudgetStore(t)

	seed, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	require.NoError(t, seed.SetProjectBudget("gone", 100, 0, 0.8, 0.75))

	mgrA, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	mgrB, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)

	require.NoError(t, mgrA.RemoveProjectBudget("gone"))
	require.NoError(t, mgrB.SetProjectBudget("other", 5, 0, 0.8, 0.75))

	got, _, err := localStore{}.Load()
	require.NoError(t, err)
	_, resurrected := got.ProjectBudgets["gone"]
	assert.False(t, resurrected, "a budget another process removed must stay removed")
	_, hasOther := got.ProjectBudgets["other"]
	assert.True(t, hasOther)
}

// TestManager_InterleavedSavesKeepBothWriters_S3 is the shared s3:// store
// reproduction (the multi-machine case #246 exists for).
func TestManager_InterleavedSavesKeepBothWriters_S3(t *testing.T) {
	isolateBudgetStore(t)
	ctx := context.Background()
	f := newFakeS3()

	mgrA, err := newManagerWithStore(testCostConfig(), aws.Config{}, nil, newTestS3Store(f))
	require.NoError(t, err)
	mgrB, err := newManagerWithStore(testCostConfig(), aws.Config{}, nil, newTestS3Store(f))
	require.NoError(t, err)

	require.NoError(t, mgrA.SetProjectBudget("proj-a", 100, 0, 0.8, 0.75))
	require.NoError(t, mgrB.SetProjectBudget("proj-b", 200, 0, 0.8, 0.75))
	require.NoError(t, mgrA.RecordCompletedUpload(ctx, "fa", gib, config.StorageClassStandard, "us-east-1", "job-a", "proj-a", nil))
	require.NoError(t, mgrB.RecordCompletedUpload(ctx, "fb", gib, config.StorageClassStandard, "us-east-1", "job-b", "proj-b", nil))
	require.NoError(t, mgrA.SetProjectBudget("proj-a", 150, 0, 0.8, 0.75))

	got, _, err := newTestS3Store(f).Load()
	require.NoError(t, err)
	assertBothWriters(t, got)
	assert.Equal(t, 150.0, got.ProjectBudgets["proj-a"].MaxBudget)
}

func assertBothWriters(t *testing.T, got LedgerState) {
	t.Helper()
	assert.Contains(t, got.ProjectBudgets, "proj-a", "A's budget must survive B's save")
	assert.Contains(t, got.ProjectBudgets, "proj-b", "B's budget must survive A's save")
	jobs := map[string]bool{}
	for _, r := range got.Records {
		jobs[r.JobID] = true
	}
	assert.True(t, jobs["job-a"] && jobs["job-b"], "both processes' spend records must survive; got %v", jobs)
}

// --- an unparseable store must be refused loudly and never overwritten ---

func writeCorruptStore(t *testing.T) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budgets.json")
	t.Setenv(budgetStoreEnvOverride, path)
	corrupt := []byte(`{"version": 1, "project_budgets": {"p": {"max_budget": 10}` + "\n") // truncated
	require.NoError(t, os.WriteFile(path, corrupt, 0600))
	return path, corrupt
}

// TestManager_CorruptStoreRefused: constructing a manager over an unparseable
// budgets.json must fail with an error naming the file (enforcement cannot be
// silently off), and nothing may overwrite the file.
func TestManager_CorruptStoreRefused(t *testing.T) {
	path, corrupt := writeCorruptStore(t)

	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	if err == nil {
		// Pre-#1031 the manager came up with an empty state; its next save wiped
		// every budget. Drive that save so the test shows the overwrite.
		_ = mgr.SetProjectBudget("new", 1, 0, 0.8, 0.75)
	}
	after, rErr := os.ReadFile(path)
	require.NoError(t, rErr)
	assert.Equal(t, string(corrupt), string(after), "an unparseable store must be preserved byte-for-byte")
	if assert.Error(t, err, "an unparseable store must be refused, not treated as empty") {
		assert.Contains(t, err.Error(), path, "the error must name the file")
	}
}

// TestManager_StoreCorruptedMidRunNotOverwritten: the store became unparseable
// after the manager loaded it; the next save must fail and leave it alone.
func TestManager_StoreCorruptedMidRunNotOverwritten(t *testing.T) {
	isolateBudgetStore(t)
	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)

	path, corrupt := writeCorruptStore(t)
	err = mgr.SetProjectBudget("p", 1, 0, 0.8, 0.75)

	after, rErr := os.ReadFile(path)
	require.NoError(t, rErr)
	assert.Equal(t, string(corrupt), string(after), "a save must never overwrite an unparseable store")
	if assert.Error(t, err, "a save over an unparseable store must fail") {
		assert.Contains(t, err.Error(), path)
	}
}

// TestLocalStore_SaveRefusesToOverwriteCorrupt: the store-level invariant, for
// any caller (including the s3 store's local mirror).
func TestLocalStore_SaveRefusesToOverwriteCorrupt(t *testing.T) {
	path, corrupt := writeCorruptStore(t)

	err := localStore{}.Save(LedgerState{Version: StoreVersion}, "")

	after, rErr := os.ReadFile(path)
	require.NoError(t, rErr)
	assert.Equal(t, string(corrupt), string(after))
	assert.Error(t, err)
}

// --- local store compare-and-swap and lock ---

func TestLocalStore_StaleTokenConflicts(t *testing.T) {
	isolateBudgetStore(t)
	store := localStore{}

	_, tokA, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, localTokenAbsent, tokA)
	_, tokB, err := store.Load()
	require.NoError(t, err)

	require.NoError(t, store.Save(LedgerState{ProjectBudgets: map[string]config.ProjectBudget{"a": {ProjectID: "a"}}}, tokA))
	err = store.Save(LedgerState{ProjectBudgets: map[string]config.ProjectBudget{"b": {ProjectID: "b"}}}, tokB)
	require.ErrorIs(t, err, ErrBudgetStoreConflict, "B's token predates A's write")

	got, _, err := store.Load()
	require.NoError(t, err)
	assert.Contains(t, got.ProjectBudgets, "a", "the rejected write changed nothing")
	assert.NotContains(t, got.ProjectBudgets, "b")
}

func TestLocalStore_CorruptLoadMatchesSentinel(t *testing.T) {
	path, _ := writeCorruptStore(t)
	_, _, err := localStore{}.Load()
	require.ErrorIs(t, err, ErrBudgetStoreCorrupt)
	assert.Contains(t, err.Error(), path)
}

func TestLocalStore_WaitsForLiveLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budgets.json")
	t.Setenv(budgetStoreEnvOverride, path)
	lock := path + ".lock"
	require.NoError(t, os.WriteFile(lock, []byte("123\n"), 0600))

	released := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.Remove(lock)
		close(released)
	}()
	require.NoError(t, localStore{}.Save(LedgerState{}, ""), "a released lock lets the writer through")
	select {
	case <-released:
	default:
		t.Fatal("Save went through while another writer held the lock")
	}
	_, err := os.Stat(lock)
	assert.True(t, os.IsNotExist(err), "the writer releases its own lock")
}

func TestLocalStore_StaleLockIsBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budgets.json")
	t.Setenv(budgetStoreEnvOverride, path)
	lock := path + ".lock"
	require.NoError(t, os.WriteFile(lock, []byte("123\n"), 0600))
	old := time.Now().Add(-2 * storeLockStaleAfter)
	require.NoError(t, os.Chtimes(lock, old, old))

	require.NoError(t, localStore{}.Save(LedgerState{}, ""), "a crashed writer's lock must not block forever")
}

// --- delta / merge ---

func rec(job string, sec int64) CostRecord {
	return CostRecord{Timestamp: time.Unix(sec, 0).UTC(), JobID: job, FileName: job, ProjectID: "p", Cost: 1}
}

func TestLedgerDelta_ApplyTo(t *testing.T) {
	baseState := LedgerState{
		ProjectBudgets: map[string]config.ProjectBudget{
			"keep":   {ProjectID: "keep", MaxBudget: 1},
			"change": {ProjectID: "change", MaxBudget: 1},
			"drop":   {ProjectID: "drop", MaxBudget: 1},
		},
		Records: []CostRecord{rec("old", 1), rec("purged", 2)},
	}
	base := fingerprintLedger(baseState)
	gb := &config.GlobalBudget{MaxBudget: 9}
	cur := LedgerState{
		ProjectBudgets: map[string]config.ProjectBudget{
			"keep":   {ProjectID: "keep", MaxBudget: 1},
			"change": {ProjectID: "change", MaxBudget: 2},
			"new":    {ProjectID: "new", MaxBudget: 3},
		},
		Records:      []CostRecord{rec("old", 1), rec("mine", 3)},
		GlobalBudget: gb,
	}
	d := diffLedger(base, cur)
	require.False(t, d.empty())

	// Another process meanwhile: changed "keep", set "theirs", added a record,
	// and configured alerts.
	fresh := LedgerState{
		ProjectBudgets: map[string]config.ProjectBudget{
			"keep":   {ProjectID: "keep", MaxBudget: 7},
			"change": {ProjectID: "change", MaxBudget: 1},
			"drop":   {ProjectID: "drop", MaxBudget: 1},
			"theirs": {ProjectID: "theirs", MaxBudget: 4},
		},
		Records:     []CostRecord{rec("old", 1), rec("purged", 2), rec("their", 4)},
		AlertConfig: &BudgetAlertConfig{Enabled: true},
	}
	out := d.applyTo(fresh)

	assert.Equal(t, 7.0, out.ProjectBudgets["keep"].MaxBudget, "untouched by us: theirs wins")
	assert.Equal(t, 2.0, out.ProjectBudgets["change"].MaxBudget, "our change applies")
	assert.Equal(t, 3.0, out.ProjectBudgets["new"].MaxBudget)
	assert.Equal(t, 4.0, out.ProjectBudgets["theirs"].MaxBudget)
	assert.NotContains(t, out.ProjectBudgets, "drop")
	var jobs []string
	for _, r := range out.Records {
		jobs = append(jobs, r.JobID)
	}
	assert.ElementsMatch(t, []string{"old", "their", "mine"}, jobs)
	assert.Equal(t, gb, out.GlobalBudget)
	require.NotNil(t, out.AlertConfig, "an alert config we did not touch is kept")
	assert.True(t, out.AlertConfig.Enabled)

	// Applying again (a retry after a partial write) does not duplicate ours.
	again := d.applyTo(out)
	assert.Len(t, again.Records, 3)
}

func TestLedgerDelta_DuplicateKeysAreCounted(t *testing.T) {
	r := rec("same", 1)
	base := fingerprintLedger(LedgerState{Records: []CostRecord{r}})
	d := diffLedger(base, LedgerState{Records: []CostRecord{r, r, r}})
	assert.Len(t, d.addedRecords, 2, "occurrences beyond the baseline are new")
	out := d.applyTo(LedgerState{Records: []CostRecord{r}})
	assert.Len(t, out.Records, 3)
}

func TestLedgerDelta_EmptyWhenUnchanged(t *testing.T) {
	s := LedgerState{
		ProjectBudgets: map[string]config.ProjectBudget{"p": {ProjectID: "p", MaxBudget: 1}},
		Records:        []CostRecord{rec("a", 1)},
		GlobalBudget:   &config.GlobalBudget{MaxBudget: 2},
	}
	assert.True(t, diffLedger(fingerprintLedger(s), s).empty())
}

func TestTrimLedger_KeepsNewest(t *testing.T) {
	records := make([]CostRecord, 0, maxLedgerRecords+2)
	records = append(records, rec("newest", int64(maxLedgerRecords+10)))
	for i := 0; i < maxLedgerRecords+1; i++ {
		records = append(records, rec(fmt.Sprintf("r%d", i), int64(i)))
	}
	out := trimLedger(records)
	require.Len(t, out, maxLedgerRecords)
	assert.Equal(t, "newest", out[len(out)-1].JobID)
	assert.Equal(t, "r2", out[0].JobID, "the two oldest are dropped")
}

func TestSaveMerged_LocalStore(t *testing.T) {
	isolateBudgetStore(t)
	d := diffLedger(fingerprintLedger(LedgerState{}), LedgerState{
		ProjectBudgets: map[string]config.ProjectBudget{"p": {ProjectID: "p", MaxBudget: 1}},
	})
	require.NoError(t, saveMerged(localStore{}, d, noWait))
	got, _, err := localStore{}.Load()
	require.NoError(t, err)
	assert.Contains(t, got.ProjectBudgets, "p")
}

// TestManager_SaveSkipsWhenNothingChanged: a save with no changes writes nothing.
func TestManager_SaveSkipsWhenNothingChanged(t *testing.T) {
	isolateBudgetStore(t)
	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	require.NoError(t, mgr.saveState())
	path, err := budgetStorePath()
	require.NoError(t, err)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "no change, no write")
}

// TestManager_ConcurrentGoroutineWriters hammers one local store from several
// managers at once; every budget and record must land.
func TestManager_ConcurrentGoroutineWriters(t *testing.T) {
	isolateBudgetStore(t)
	ctx := context.Background()
	const writers = 6

	mgrs := make([]*Manager, writers)
	for i := range mgrs {
		m, err := NewManager(testCostConfig(), aws.Config{}, nil)
		require.NoError(t, err)
		mgrs[i] = m
	}
	var wg sync.WaitGroup
	errs := make(chan error, writers*2)
	for i, m := range mgrs {
		wg.Add(1)
		go func(i int, m *Manager) {
			defer wg.Done()
			id := fmt.Sprintf("proj-%d", i)
			errs <- m.SetProjectBudget(id, float64(i+1), 0, 0.8, 0.75)
			errs <- m.RecordCompletedUpload(ctx, id, gib, config.StorageClassStandard, "us-east-1", "job-"+id, id, nil)
		}(i, m)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	got, _, err := localStore{}.Load()
	require.NoError(t, err)
	assert.Len(t, got.ProjectBudgets, writers)
	assert.Len(t, got.Records, writers)
}
