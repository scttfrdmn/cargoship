package cost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// s3API is the subset of *s3.Client that s3Store uses. Extracted as an interface
// so the compare-and-swap loop can be exercised by a fake client in unit tests
// (mirrors the pricingAPIClient pattern in pricing_api.go). *s3.Client satisfies it.
type s3API interface {
	GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

const (
	globalObjectName   = "global.json"
	projectsPrefix     = "projects/"
	defaultStorePrefix = ".cargoship/"
	defaultMaxCASRetry = 5
	defaultCASBaseWait = 50 * time.Millisecond
	casMaxWait         = 2 * time.Second
)

// s3Store is an S3-backed BudgetStore. It persists one object per project plus a
// global object, and uses S3 conditional writes (If-Match / If-None-Match) for
// optimistic-concurrency compare-and-swap so concurrent writers from different
// machines converge without lost updates (#246 Phase B).
//
// The BudgetStore interface stays whole-document (Load returns the full
// LedgerState); the per-object layout and CAS live entirely inside this type.
// The opaque Token carries a serialized map of per-object ETags (s3Token).
type s3Store struct {
	client   s3API
	bucket   string
	prefix   string // normalized to end with "/"
	ctx      context.Context
	maxRetry int
	baseWait time.Duration
	now      func() time.Time
	randf    func() float64 // jitter source in [0,1); injectable for deterministic tests

	// mirror is an optional local write-through cache used to serve reads when S3
	// is unreachable. Nil disables mirroring.
	mirror *localStore

	// lastLoaded is the LedgerState returned by the most recent Load, used by
	// Save to compute which records are new (this process's appends) so a 412
	// re-GET can union-merge instead of clobbering a concurrent writer's records.
	lastLoaded *LedgerState

	// loadedGlobalBudget is the global budget observed at Load time, carried
	// through a Save so a records-only write doesn't wipe it.
	loadedGlobalBudget *config.GlobalBudget
}

// s3Token is the concrete payload serialized into the opaque cost.Token. It maps
// each persisted object to the ETag observed at Load time. An empty ETag for a
// key means the object did not exist (first write uses If-None-Match:*).
type s3Token struct {
	Global   string            `json:"global"`
	Projects map[string]string `json:"projects"`
}

func encodeToken(t s3Token) Token {
	b, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return Token(b)
}

// decodeToken is tolerant: an empty or malformed token decodes to the zero value,
// which drives every object down the "unknown ETag" path (re-GET before write)
// rather than risking an unsafe unconditional overwrite.
func decodeToken(tok Token) s3Token {
	t := s3Token{Projects: map[string]string{}}
	if tok == "" {
		return t
	}
	_ = json.Unmarshal([]byte(tok), &t)
	if t.Projects == nil {
		t.Projects = map[string]string{}
	}
	return t
}

// projectDoc is the per-project S3 object: one budget plus that project's ledger.
type projectDoc struct {
	Version int                  `json:"version"`
	Budget  config.ProjectBudget `json:"budget"`
	Records []CostRecord         `json:"records,omitempty"`
}

// globalDoc is the global.json S3 object: the global budget plus records that
// aren't attributable to a project with its own object.
type globalDoc struct {
	Version      int                  `json:"version"`
	Records      []CostRecord         `json:"records,omitempty"`
	GlobalBudget *config.GlobalBudget `json:"global_budget,omitempty"`
}

// parseStoreSpec classifies a budget-store location. An empty spec, or any value
// that isn't an s3:// URL, means "use the local store" (the caller falls back to
// localStore / the CARGOSHIP_BUDGET_STORE path). An s3://bucket/prefix spec
// selects the S3 store; the prefix is normalized to end with "/" and defaults to
// ".cargoship/" when only a bucket is given.
func parseStoreSpec(spec string) (isS3 bool, bucket, prefix string, err error) {
	if spec == "" || !strings.HasPrefix(spec, "s3://") {
		return false, "", "", nil
	}
	rest := strings.TrimPrefix(spec, "s3://")
	if rest == "" {
		return false, "", "", fmt.Errorf("invalid s3 store spec %q: missing bucket", spec)
	}
	bucket, prefix, _ = strings.Cut(rest, "/")
	if bucket == "" {
		return false, "", "", fmt.Errorf("invalid s3 store spec %q: missing bucket", spec)
	}
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		prefix = strings.TrimSuffix(defaultStorePrefix, "/")
	}
	return true, bucket, prefix + "/", nil
}

// newS3Store builds an S3-backed store. client must be non-nil (*s3.Client in
// production, a fake in tests). mirror may be nil to disable the local cache.
func newS3Store(ctx context.Context, client s3API, bucket, prefix string, mirror *localStore) *s3Store {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &s3Store{
		client:   client,
		bucket:   bucket,
		prefix:   prefix,
		ctx:      ctx,
		maxRetry: defaultMaxCASRetry,
		baseWait: defaultCASBaseWait,
		now:      time.Now,
		randf:    pseudoRand(),
		mirror:   mirror,
	}
}

func (s *s3Store) globalKey() string { return s.prefix + globalObjectName }

func (s *s3Store) projectKey(projectID string) string {
	return s.prefix + projectsPrefix + url.PathEscape(projectID) + ".json"
}

// Load lists and fetches every store object, reassembling the whole LedgerState
// and a composite token of per-object ETags. A missing/empty store is not an
// error (empty state). When S3 is unreachable and a mirror is configured, it
// serves the cached state with an empty token (which forces a real S3 Load
// before any subsequent Save, so we never CAS-write off stale cache).
func (s *s3Store) Load() (LedgerState, Token, error) {
	state, tok, err := s.loadFromS3()
	if err != nil {
		if s.mirror != nil {
			if ms, _, mErr := s.mirror.Load(); mErr == nil {
				cached := ms
				s.lastLoaded = &cached
				return ms, "", nil // empty token: reads OK, writes must re-Load from S3
			}
		}
		return LedgerState{}, "", err
	}
	cached := state
	s.lastLoaded = &cached
	return state, tok, nil
}

func (s *s3Store) loadFromS3() (LedgerState, Token, error) {
	keys, err := s.listKeys()
	if err != nil {
		return LedgerState{}, "", err
	}

	state := LedgerState{Version: StoreVersion, ProjectBudgets: map[string]config.ProjectBudget{}}
	tokenData := s3Token{Projects: map[string]string{}}

	// Global object.
	if hasKey(keys, s.globalKey()) {
		body, etag, gErr := s.getObject(s.globalKey())
		if gErr != nil {
			return LedgerState{}, "", gErr
		}
		if body != nil {
			var gd globalDoc
			if uErr := json.Unmarshal(body, &gd); uErr != nil {
				return LedgerState{}, "", fmt.Errorf("parse %s: %w", s.globalKey(), uErr)
			}
			state.Records = append(state.Records, gd.Records...)
			tokenData.Global = etag
			s.loadedGlobalBudget = gd.GlobalBudget
			state.GlobalBudget = gd.GlobalBudget
		}
	}

	// Project objects.
	pfx := s.prefix + projectsPrefix
	for _, key := range keys {
		if !strings.HasPrefix(key, pfx) || !strings.HasSuffix(key, ".json") {
			continue
		}
		body, etag, gErr := s.getObject(key)
		if gErr != nil {
			return LedgerState{}, "", gErr
		}
		if body == nil {
			continue
		}
		var pd projectDoc
		if uErr := json.Unmarshal(body, &pd); uErr != nil {
			return LedgerState{}, "", fmt.Errorf("parse %s: %w", key, uErr)
		}
		id := pd.Budget.ProjectID
		if id == "" {
			// Recover the ID from the key if the doc didn't carry it.
			id = projectIDFromKey(key, pfx)
		}
		if id != "" {
			state.ProjectBudgets[id] = pd.Budget
			tokenData.Projects[id] = etag
		}
		state.Records = append(state.Records, pd.Records...)
	}

	return state, encodeToken(tokenData), nil
}

// Save splits the whole LedgerState into per-object docs and writes each changed
// object with a conditional PUT (CAS). Records new since the last Load are
// union-merged into concurrently-updated objects on a 412, so no writer's
// records are lost.
func (s *s3Store) Save(state LedgerState, token Token) error {
	prev := decodeToken(token)

	// Records this process added since Load (used for loss-free merge on 412).
	newRecords := s.recordsAddedSince(state.Records)

	// Route records to their owning object: a record whose ProjectID has a budget
	// goes in that project's object; everything else goes to global.json.
	projRecords, globalRecords := routeRecords(state.Records, state.ProjectBudgets)

	// Write each project object.
	for id, budget := range state.ProjectBudgets {
		doc := projectDoc{Version: StoreVersion, Budget: budget, Records: projRecords[id]}
		mine := recordsForProject(newRecords, id, state.ProjectBudgets)
		if err := s.saveObject(s.projectKey(id), prev.Projects[id], doc, mine); err != nil {
			return err
		}
	}

	// Write global object. Use the caller's GlobalBudget when set; otherwise
	// carry through whatever we loaded so a records-only save doesn't wipe it.
	gb := state.GlobalBudget
	if gb == nil {
		gb = s.loadedGlobalBudget
	}
	gdoc := globalDoc{Version: StoreVersion, Records: globalRecords, GlobalBudget: gb}
	mineGlobal := recordsForProject(newRecords, "", state.ProjectBudgets)
	if err := s.saveObject(s.globalKey(), prev.Global, gdoc, mineGlobal); err != nil {
		return err
	}

	// Delete project objects that existed before but are gone now (budget removed).
	for id := range prev.Projects {
		if _, still := state.ProjectBudgets[id]; !still {
			_, _ = s.client.DeleteObject(s.ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(s.bucket),
				Key:    aws.String(s.projectKey(id)),
			})
		}
	}

	// Best-effort write-through to the local mirror so reads work offline.
	if s.mirror != nil {
		if err := s.mirror.Save(state, ""); err != nil {
			// non-fatal: the authoritative S3 write already succeeded
			_ = err
		}
	}
	return nil
}

// saveObject marshals doc and writes it under a per-object CAS loop. mineRecords
// are the records this process contributed to this object, re-merged on conflict.
func (s *s3Store) saveObject(key, knownETag string, doc any, mineRecords []CostRecord) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}
	_, err = s.putConditional(key, data, knownETag, doc, mineRecords)
	return err
}

// putConditional writes data to key with a conditional PUT, retrying on a 412
// PreconditionFailed by re-reading the object, union-merging this process's own
// records onto the fresh state, and backing off. Returns the new ETag.
func (s *s3Store) putConditional(key string, data []byte, knownETag string, doc any, mineRecords []CostRecord) (string, error) {
	for attempt := 0; ; attempt++ {
		in := &s3.PutObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
			Body:   bytes.NewReader(data),
		}
		if knownETag == "" {
			in.IfNoneMatch = aws.String("*") // create-only: fail if it already exists
		} else {
			in.IfMatch = aws.String(knownETag) // update: fail if changed under us
		}
		out, err := s.client.PutObject(s.ctx, in)
		if err == nil {
			return aws.ToString(out.ETag), nil
		}
		if !isPreconditionFailed(err) || attempt >= s.maxRetry {
			return "", fmt.Errorf("conditional put %s: %w", key, err)
		}
		// Conflict: re-read the object, merge our records onto the fresh state,
		// recompute the bytes + ETag, back off, and retry.
		freshBody, freshETag, gErr := s.getObject(key)
		if gErr != nil {
			return "", gErr
		}
		merged, mErr := mergeDoc(doc, freshBody, mineRecords)
		if mErr != nil {
			return "", mErr
		}
		data = merged
		knownETag = freshETag
		s.backoff(attempt)
	}
}

// mergeDoc re-applies this process's records onto the freshly-read object body.
// For a project or global doc, the merged record set is (fresh records ∪ ours)
// deduped by identity, so concurrent appends are loss-free; non-record fields
// (the budget) keep the value from doc (last-writer-wins, fine for human edits).
func mergeDoc(doc any, freshBody []byte, mineRecords []CostRecord) ([]byte, error) {
	switch d := doc.(type) {
	case projectDoc:
		var fresh projectDoc
		if freshBody != nil {
			if err := json.Unmarshal(freshBody, &fresh); err != nil {
				return nil, fmt.Errorf("merge project doc: %w", err)
			}
		}
		d.Records = unionRecords(fresh.Records, mineRecords)
		return json.MarshalIndent(d, "", "  ")
	case globalDoc:
		var fresh globalDoc
		if freshBody != nil {
			if err := json.Unmarshal(freshBody, &fresh); err != nil {
				return nil, fmt.Errorf("merge global doc: %w", err)
			}
		}
		d.Records = unionRecords(fresh.Records, mineRecords)
		if d.GlobalBudget == nil {
			d.GlobalBudget = fresh.GlobalBudget
		}
		return json.MarshalIndent(d, "", "  ")
	default:
		return nil, fmt.Errorf("mergeDoc: unsupported doc type %T", doc)
	}
}

// recordsAddedSince returns records present in current but not in the LedgerState
// returned by the last Load (this process's appends), keyed by record identity.
func (s *s3Store) recordsAddedSince(current []CostRecord) []CostRecord {
	seen := map[string]bool{}
	if s.lastLoaded != nil {
		for _, r := range s.lastLoaded.Records {
			seen[recordKey(r)] = true
		}
	}
	var added []CostRecord
	for _, r := range current {
		if !seen[recordKey(r)] {
			added = append(added, r)
		}
	}
	return added
}

// getObject fetches an object body and ETag. A NoSuchKey / 404 is reported as a
// nil body with an empty ETag (absent), not an error.
func (s *s3Store) getObject(key string) ([]byte, string, error) {
	out, err := s.client.GetObject(s.ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("get %s: %w", key, err)
	}
	defer func() { _ = out.Body.Close() }()
	body, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", key, err)
	}
	return body, aws.ToString(out.ETag), nil
}

func (s *s3Store) listKeys() ([]string, error) {
	var keys []string
	var token *string
	for {
		out, err := s.client.ListObjectsV2(s.ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(s.prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", s.prefix, err)
		}
		for _, obj := range out.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			break
		}
		token = out.NextContinuationToken
	}
	return keys, nil
}

func (s *s3Store) backoff(attempt int) {
	wait := s.baseWait << attempt
	if wait > casMaxWait || wait <= 0 {
		wait = casMaxWait
	}
	jitter := s.randf()
	time.Sleep(time.Duration(float64(wait) * jitter))
}

// --- helpers ---

// routeRecords partitions records: those whose ProjectID has a budget object go
// under that project; everything else (including empty ProjectID) goes global.
func routeRecords(records []CostRecord, budgets map[string]config.ProjectBudget) (perProject map[string][]CostRecord, global []CostRecord) {
	perProject = map[string][]CostRecord{}
	for _, r := range records {
		if _, ok := budgets[r.ProjectID]; ok && r.ProjectID != "" {
			perProject[r.ProjectID] = append(perProject[r.ProjectID], r)
		} else {
			global = append(global, r)
		}
	}
	return perProject, global
}

// recordsForProject filters a record slice to those routed to project id (or to
// global when id == "").
func recordsForProject(records []CostRecord, id string, budgets map[string]config.ProjectBudget) []CostRecord {
	var out []CostRecord
	for _, r := range records {
		_, hasBudget := budgets[r.ProjectID]
		routed := ""
		if hasBudget && r.ProjectID != "" {
			routed = r.ProjectID
		}
		if routed == id {
			out = append(out, r)
		}
	}
	return out
}

// unionRecords returns the set union of two record slices, deduped by identity,
// preserving a stable order (existing first, then new).
func unionRecords(existing, added []CostRecord) []CostRecord {
	seen := map[string]bool{}
	out := make([]CostRecord, 0, len(existing)+len(added))
	for _, r := range existing {
		k := recordKey(r)
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	for _, r := range added {
		k := recordKey(r)
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	return out
}

// recordKey is the identity used to dedupe records across concurrent writers.
func recordKey(r CostRecord) string {
	return r.JobID + "\x00" + r.FileName + "\x00" + r.Timestamp.UTC().Format(time.RFC3339Nano)
}

// validateProjectID rejects project IDs that would be unsafe as an S3 object key
// component: path separators and control characters. Normal manifest upload IDs
// (e.g. "20251206-abc123") and typical user labels pass.
func validateProjectID(id string) error {
	if strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("project ID %q must not contain path separators", id)
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("project ID %q must not contain control characters", id)
		}
	}
	return nil
}

func projectIDFromKey(key, pfx string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(key, pfx), ".json")
	if id, err := url.PathUnescape(name); err == nil {
		return id
	}
	return name
}

func hasKey(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// isPreconditionFailed reports whether err is an S3 412 PreconditionFailed. This
// status is NOT a modeled SDK error type, so detect it via the generic smithy
// APIError code.
func isPreconditionFailed(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed"
}

// isNotFound reports whether err is a NoSuchKey (modeled) or a generic 404.
func isNotFound(err error) bool {
	var nsk *s3types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound")
}

// pseudoRand returns a deterministic-enough jitter source without pulling in
// math/rand global state; it walks a simple LCG seeded from a fixed value. Tests
// override s.randf for determinism, and jitter needn't be cryptographic.
func pseudoRand() func() float64 {
	state := uint64(0x2545F4914F6CDD1D)
	return func() float64 {
		state = state*6364136223846793005 + 1442695040888963407
		// use the top 53 bits for a [0,1) float; keep it in (0.5,1] so backoff
		// never collapses to zero wait.
		v := float64(state>>11) / float64(1<<53)
		return 0.5 + v/2
	}
}
