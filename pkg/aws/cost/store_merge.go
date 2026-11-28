package cost

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// Merge-on-save for the budget store (#1031).
//
// Several processes can hold a Manager on the same store at once: an upload, a
// `budget set`, a long-running `ghostship run` or `fleet monitor`, on one machine
// or (with an s3:// store) many. Writing a process's whole in-memory state would
// revert or delete what the others saved since it loaded. Instead a Manager
// remembers what it last read or wrote (a ledgerFingerprint), turns its current
// state into a delta against that, and saveMerged applies the delta to a fresh
// read of the store under a compare-and-swap, retrying on conflict.

const (
	// maxMergeSaveAttempts bounds the re-read/re-apply/CAS loop.
	maxMergeSaveAttempts = 8

	mergeSaveBaseWait = 10 * time.Millisecond
	mergeSaveMaxWait  = 500 * time.Millisecond
)

// ledgerFingerprint is the comparable shape of a LedgerState: enough to tell
// what changed, without aliasing the Manager's live maps and pointers.
type ledgerFingerprint struct {
	budgets map[string]string // project ID -> canonical JSON of its budget
	records map[string]int    // recordKey -> occurrences
	global  string            // canonical JSON of the global budget ("null" when unset)
	alert   string            // canonical JSON of the alert config ("null" when unset)
}

// ledgerDelta is one process's change set against its fingerprint.
type ledgerDelta struct {
	setBudgets     map[string]config.ProjectBudget
	removedBudgets map[string]bool

	// addedRecords are records this process appended; baseRecords holds each
	// one's occurrence count at the fingerprint, so a retry after a partial write
	// (the s3 store writes several objects) does not append them twice.
	addedRecords []CostRecord
	baseRecords  map[string]int
	// removedRecords counts, per recordKey, occurrences this process dropped
	// (e.g. PurgeCosts).
	removedRecords map[string]int

	globalChanged bool
	globalBudget  *config.GlobalBudget
	alertChanged  bool
	alertConfig   *BudgetAlertConfig
}

// canonicalJSON renders v for equality checks. Marshal errors cannot occur for
// these plain data types; one would make the value compare as changed, which
// only costs a redundant (idempotent) write.
func canonicalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "\x00unmarshalable"
	}
	return string(b)
}

func countRecords(records []CostRecord) map[string]int {
	n := make(map[string]int, len(records))
	for _, r := range records {
		n[recordKey(r)]++
	}
	return n
}

// fingerprintLedger captures s for a later diffLedger.
func fingerprintLedger(s LedgerState) ledgerFingerprint {
	fp := ledgerFingerprint{
		budgets: make(map[string]string, len(s.ProjectBudgets)),
		records: countRecords(s.Records),
		global:  canonicalJSON(s.GlobalBudget),
		alert:   canonicalJSON(s.AlertConfig),
	}
	for id, b := range s.ProjectBudgets {
		fp.budgets[id] = canonicalJSON(b)
	}
	return fp
}

// diffLedger returns what cur changed relative to base.
func diffLedger(base ledgerFingerprint, cur LedgerState) ledgerDelta {
	d := ledgerDelta{
		setBudgets:     map[string]config.ProjectBudget{},
		removedBudgets: map[string]bool{},
		baseRecords:    map[string]int{},
		removedRecords: map[string]int{},
	}
	for id, b := range cur.ProjectBudgets {
		if was, ok := base.budgets[id]; !ok || was != canonicalJSON(b) {
			d.setBudgets[id] = b
		}
	}
	for id := range base.budgets {
		if _, ok := cur.ProjectBudgets[id]; !ok {
			d.removedBudgets[id] = true
		}
	}

	// Occurrences beyond the fingerprint's count were appended by this process;
	// the latest ones in ledger order are the new ones.
	seen := map[string]int{}
	for _, r := range cur.Records {
		k := recordKey(r)
		seen[k]++
		if seen[k] > base.records[k] {
			d.addedRecords = append(d.addedRecords, r)
			d.baseRecords[k] = base.records[k]
		}
	}
	for k, was := range base.records {
		if now := seen[k]; now < was {
			d.removedRecords[k] = was - now
		}
	}

	if canonicalJSON(cur.GlobalBudget) != base.global {
		d.globalChanged, d.globalBudget = true, cur.GlobalBudget
	}
	if canonicalJSON(cur.AlertConfig) != base.alert {
		d.alertChanged, d.alertConfig = true, cur.AlertConfig
	}
	return d
}

// empty reports whether the delta changes nothing.
func (d ledgerDelta) empty() bool {
	return len(d.setBudgets) == 0 && len(d.removedBudgets) == 0 &&
		len(d.addedRecords) == 0 && len(d.removedRecords) == 0 &&
		!d.globalChanged && !d.alertChanged
}

// applyTo returns fresh with d applied: budgets set or removed, records
// appended or removed, and the global budget and alert config replaced if this
// process changed them. Everything else is kept from fresh, so another
// process's concurrent changes survive. The ledger is then bounded FIFO.
func (d ledgerDelta) applyTo(fresh LedgerState) LedgerState {
	out := LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: make(map[string]config.ProjectBudget, len(fresh.ProjectBudgets)+len(d.setBudgets)),
		GlobalBudget:   fresh.GlobalBudget,
		AlertConfig:    fresh.AlertConfig,
	}
	for id, b := range fresh.ProjectBudgets {
		if !d.removedBudgets[id] {
			out.ProjectBudgets[id] = b
		}
	}
	for id, b := range d.setBudgets {
		out.ProjectBudgets[id] = b
	}

	toRemove := make(map[string]int, len(d.removedRecords))
	for k, n := range d.removedRecords {
		toRemove[k] = n
	}
	records := make([]CostRecord, 0, len(fresh.Records)+len(d.addedRecords))
	for _, r := range fresh.Records {
		if k := recordKey(r); toRemove[k] > 0 {
			toRemove[k]--
			continue
		}
		records = append(records, r)
	}
	// An added record already in fresh beyond the fingerprint's count is one this
	// process wrote on an earlier, partially applied attempt: skip that many.
	inFresh := countRecords(records)
	already := map[string]int{}
	for k, base := range d.baseRecords {
		if extra := inFresh[k] - base; extra > 0 {
			already[k] = extra
		}
	}
	for _, r := range d.addedRecords {
		if k := recordKey(r); already[k] > 0 {
			already[k]--
			continue
		}
		records = append(records, r)
	}
	out.Records = trimLedger(records)

	if d.globalChanged {
		out.GlobalBudget = d.globalBudget
	}
	if d.alertChanged {
		out.AlertConfig = d.alertConfig
	}
	return out
}

// trimLedger bounds the ledger at maxLedgerRecords, dropping the oldest records
// (#246). Records merged from several writers are not in time order, so order
// by timestamp before cutting.
func trimLedger(records []CostRecord) []CostRecord {
	if len(records) <= maxLedgerRecords {
		return records
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Timestamp.Before(records[j].Timestamp) })
	return records[len(records)-maxLedgerRecords:]
}

// saveMerged applies d to a fresh read of store and writes the result with the
// read's token, retrying when another writer got in first. It never writes when
// the read fails, so an unreadable store is never overwritten. wait(attempt)
// backs off between attempts; nil uses jittered exponential backoff.
func saveMerged(store BudgetStore, d ledgerDelta, wait func(attempt int)) error {
	if wait == nil {
		wait = mergeSaveBackoff
	}
	var lastErr error
	for attempt := 0; attempt < maxMergeSaveAttempts; attempt++ {
		if attempt > 0 {
			wait(attempt)
		}
		fresh, tok, err := store.Load()
		if err != nil {
			return fmt.Errorf("read budget store before save: %w", err)
		}
		err = store.Save(d.applyTo(fresh), tok)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrBudgetStoreConflict) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("budget store kept changing across %d save attempts: %w", maxMergeSaveAttempts, lastErr)
}

// mergeSaveBackoff sleeps a jittered, exponentially growing interval so
// contending writers spread out instead of colliding in lockstep.
func mergeSaveBackoff(attempt int) {
	wait := mergeSaveBaseWait << attempt
	if wait <= 0 || wait > mergeSaveMaxWait {
		wait = mergeSaveMaxWait
	}
	time.Sleep(wait/2 + jitter(wait/2))
}

// jitter returns a random duration in [0, limit]. It reads crypto/rand, since
// the repo's SAST policy rejects the non-crypto generator. The value only
// spreads retries out, so if the read fails it falls back to half the limit.
func jitter(limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return limit / 2
	}
	return time.Duration(binary.LittleEndian.Uint64(b[:]) % (uint64(limit) + 1))
}
