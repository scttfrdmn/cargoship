package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeS3 is an in-memory s3API with per-key ETags and a hook to inject a
// PreconditionFailed on the Nth PutObject to a given key (to exercise the CAS
// retry loop deterministically).
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	etags   map[string]string
	seq     int

	putCount map[string]int
	// fail412On[key] = attempt number (1-based) that should return 412.
	fail412On map[string]int
	// alwaysFail412[key] = true returns 412 on every PutObject to key.
	alwaysFail412 map[string]bool

	puts, gets, lists, deletes int
}

func newFakeS3() *fakeS3 {
	return &fakeS3{
		objects:       map[string][]byte{},
		etags:         map[string]string{},
		putCount:      map[string]int{},
		fail412On:     map[string]int{},
		alwaysFail412: map[string]bool{},
	}
}

func preconditionErr() error {
	return &smithy.GenericAPIError{Code: "PreconditionFailed", Message: "At least one of the pre-conditions you specified did not hold"}
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	key := aws.ToString(in.Key)
	body, ok := f.objects[key]
	if !ok {
		return nil, &s3types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(string(body))),
		ETag: aws.String(f.etags[key]),
	}, nil
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts++
	key := aws.ToString(in.Key)
	f.putCount[key]++

	if f.alwaysFail412[key] {
		return nil, preconditionErr()
	}
	if want, ok := f.fail412On[key]; ok && want == f.putCount[key] {
		return nil, preconditionErr()
	}

	cur, exists := f.objects[key]
	_ = cur
	// Enforce conditional semantics.
	if in.IfNoneMatch != nil && exists {
		return nil, preconditionErr() // create-only but object exists
	}
	if in.IfMatch != nil && aws.ToString(in.IfMatch) != f.etags[key] {
		return nil, preconditionErr() // update but ETag changed
	}

	body, _ := io.ReadAll(in.Body)
	f.objects[key] = body
	f.seq++
	etag := fmt.Sprintf("\"etag-%d\"", f.seq)
	f.etags[key] = etag
	return &s3.PutObjectOutput{ETag: aws.String(etag)}, nil
}

func (f *fakeS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	prefix := aws.ToString(in.Prefix)
	var contents []s3types.Object
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			contents = append(contents, s3types.Object{Key: aws.String(k)})
		}
	}
	return &s3.ListObjectsV2Output{Contents: contents}, nil
}

func (f *fakeS3) DeleteObject(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	key := aws.ToString(in.Key)
	delete(f.objects, key)
	delete(f.etags, key)
	return &s3.DeleteObjectOutput{}, nil
}

// newTestS3Store wires an s3Store to a fake client with deterministic, fast
// backoff (no real sleeps of consequence).
func newTestS3Store(f *fakeS3) *s3Store {
	s := newS3Store(context.Background(), f, "test-bucket", ".cargoship/", nil)
	s.baseWait = time.Microsecond
	s.randf = func() float64 { return 1.0 }
	return s
}

func TestParseStoreSpec(t *testing.T) {
	cases := []struct {
		spec       string
		wantS3     bool
		wantBucket string
		wantPrefix string
		wantErr    bool
	}{
		{"", false, "", "", false},
		{"/local/path/budgets.json", false, "", "", false},
		{"s3://mybucket", true, "mybucket", ".cargoship/", false},
		{"s3://mybucket/", true, "mybucket", ".cargoship/", false},
		{"s3://mybucket/team/budgets", true, "mybucket", "team/budgets/", false},
		{"s3://", false, "", "", true},
	}
	for _, c := range cases {
		isS3, bucket, prefix, err := parseStoreSpec(c.spec)
		if c.wantErr {
			assert.Error(t, err, "spec %q", c.spec)
			continue
		}
		require.NoError(t, err, "spec %q", c.spec)
		assert.Equal(t, c.wantS3, isS3, "spec %q isS3", c.spec)
		assert.Equal(t, c.wantBucket, bucket, "spec %q bucket", c.spec)
		assert.Equal(t, c.wantPrefix, prefix, "spec %q prefix", c.spec)
	}
}

func TestS3Token_RoundTripAndTolerant(t *testing.T) {
	tok := encodeToken(s3Token{Global: "g1", Projects: map[string]string{"p": "e1"}})
	got := decodeToken(tok)
	assert.Equal(t, "g1", got.Global)
	assert.Equal(t, "e1", got.Projects["p"])

	// Empty and garbage decode to a usable zero value (non-nil map).
	assert.NotNil(t, decodeToken("").Projects)
	assert.NotNil(t, decodeToken("not json").Projects)
}

func TestS3Store_LoadEmptyBucket(t *testing.T) {
	s := newTestS3Store(newFakeS3())
	state, tok, err := s.Load()
	require.NoError(t, err)
	assert.Equal(t, StoreVersion, state.Version)
	assert.Empty(t, state.ProjectBudgets)
	assert.Empty(t, state.Records)
	// Empty store → token with no per-object etags.
	assert.Empty(t, decodeToken(tok).Global)
}

func TestS3Store_SaveThenLoadRoundTrip(t *testing.T) {
	f := newFakeS3()
	s := newTestS3Store(f)

	// Seed a fresh load so lastLoaded is set.
	_, tok, err := s.Load()
	require.NoError(t, err)

	state := LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{"proj-a": {ProjectID: "proj-a", MaxBudget: 100}},
		Records: []CostRecord{
			{Timestamp: time.Unix(1, 0).UTC(), ProjectID: "proj-a", JobID: "j1", FileName: "f1", Cost: 1.5, SizeGB: 3},
			{Timestamp: time.Unix(2, 0).UTC(), ProjectID: "", JobID: "j2", FileName: "f2", Cost: 0.5, SizeGB: 1}, // global
		},
	}
	require.NoError(t, s.Save(state, tok))

	// Objects landed at the expected keys.
	_, hasProj := f.objects[".cargoship/projects/proj-a.json"]
	_, hasGlobal := f.objects[".cargoship/global.json"]
	assert.True(t, hasProj, "project object should exist")
	assert.True(t, hasGlobal, "global object should exist")

	// A fresh store reloads the whole state.
	s2 := newTestS3Store(f)
	got, _, err := s2.Load()
	require.NoError(t, err)
	assert.Equal(t, 100.0, got.ProjectBudgets["proj-a"].MaxBudget)
	require.Len(t, got.Records, 2)
	// Records reassembled from both objects (order: global first, then project).
	var jobIDs []string
	for _, r := range got.Records {
		jobIDs = append(jobIDs, r.JobID)
	}
	assert.ElementsMatch(t, []string{"j1", "j2"}, jobIDs)
}

func TestS3Store_FirstWriteUsesIfNoneMatch(t *testing.T) {
	f := newFakeS3()
	s := newTestS3Store(f)
	_, tok, _ := s.Load()

	// Track conditional headers via a wrapper.
	var sawIfNoneMatch, sawIfMatch bool
	wrapped := &headerSpyS3{fakeS3: f, onPut: func(in *s3.PutObjectInput) {
		if in.IfNoneMatch != nil {
			sawIfNoneMatch = true
		}
		if in.IfMatch != nil {
			sawIfMatch = true
		}
	}}
	s.client = wrapped

	require.NoError(t, s.Save(LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{"p": {ProjectID: "p", MaxBudget: 10}},
	}, tok))
	assert.True(t, sawIfNoneMatch, "first write must use If-None-Match:*")
	assert.False(t, sawIfMatch, "first write must not use If-Match")
}

func TestS3Store_UpdateUsesIfMatch(t *testing.T) {
	f := newFakeS3()
	// First write via one store.
	s1 := newTestS3Store(f)
	_, tok1, _ := s1.Load()
	require.NoError(t, s1.Save(LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{"p": {ProjectID: "p", MaxBudget: 10}},
	}, tok1))

	// Second store loads (gets real etags), updates → must use If-Match.
	s2 := newTestS3Store(f)
	_, tok2, _ := s2.Load()
	var sawIfMatch bool
	s2.client = &headerSpyS3{fakeS3: f, onPut: func(in *s3.PutObjectInput) {
		if in.IfMatch != nil {
			sawIfMatch = true
		}
	}}
	require.NoError(t, s2.Save(LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{"p": {ProjectID: "p", MaxBudget: 20}},
	}, tok2))
	assert.True(t, sawIfMatch, "update of an existing object must use If-Match")
}

func TestS3Store_CASRetryOn412(t *testing.T) {
	f := newFakeS3()
	// Fail the first PutObject to the project key with a 412, succeed on retry.
	f.fail412On[".cargoship/projects/p.json"] = 1

	s := newTestS3Store(f)
	_, tok, _ := s.Load()
	err := s.Save(LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{"p": {ProjectID: "p", MaxBudget: 10}},
	}, tok)
	require.NoError(t, err, "CAS should recover after a single 412")
	assert.GreaterOrEqual(t, f.putCount[".cargoship/projects/p.json"], 2, "should have retried the PUT")
}

func TestS3Store_CASGivesUpAfterMaxRetry(t *testing.T) {
	f := newFakeS3()
	s := newTestS3Store(f)
	s.maxRetry = 2
	_, tok, _ := s.Load()

	// Always 412 on this key.
	f.alwaysFail412[".cargoship/projects/p.json"] = true

	err := s.Save(LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{"p": {ProjectID: "p", MaxBudget: 10}},
	}, tok)
	require.Error(t, err, "persistent 412 should surface an error after maxRetry")
	assert.Contains(t, err.Error(), "conditional put")
}

// TestS3Store_RecordUnionMergeOnConflict proves the loss-free-append property:
// when a concurrent writer changes the object between our Load and Save, the
// 412 re-GET union-merges records so the other writer's record survives.
func TestS3Store_RecordUnionMergeOnConflict(t *testing.T) {
	f := newFakeS3()
	key := ".cargoship/projects/p.json"

	// Pre-existing object with the budget and one record from "writer B".
	writerB := projectDoc{
		Version: StoreVersion,
		Budget:  config.ProjectBudget{ProjectID: "p", MaxBudget: 10},
		Records: []CostRecord{{Timestamp: time.Unix(1, 0).UTC(), ProjectID: "p", JobID: "B", FileName: "b", Cost: 1}},
	}
	seedObject(f, key, writerB)

	// Our store loaded an EARLIER (empty) view, then appends its own record and
	// saves against a stale/absent etag → 412 → re-GET merges.
	s := newTestS3Store(f)
	// Simulate: we loaded when the object didn't exist (empty token), and our
	// in-memory ledger has only our record.
	s.lastLoaded = &LedgerState{Version: StoreVersion, ProjectBudgets: map[string]config.ProjectBudget{}}
	ourRecord := CostRecord{Timestamp: time.Unix(2, 0).UTC(), ProjectID: "p", JobID: "A", FileName: "a", Cost: 2}
	err := s.Save(LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{"p": {ProjectID: "p", MaxBudget: 10}},
		Records:        []CostRecord{ourRecord},
	}, encodeToken(s3Token{Projects: map[string]string{}})) // empty etag → If-None-Match → 412 (object exists)
	require.NoError(t, err)

	// Reload: BOTH records must be present.
	s2 := newTestS3Store(f)
	got, _, err := s2.Load()
	require.NoError(t, err)
	var jobs []string
	for _, r := range got.Records {
		jobs = append(jobs, r.JobID)
	}
	assert.ElementsMatch(t, []string{"A", "B"}, jobs, "both writers' records must survive the conflict")
}

func TestS3Store_NoSuchKeyIsAbsent(t *testing.T) {
	f := newFakeS3()
	s := newTestS3Store(f)
	body, etag, err := s.getObject(".cargoship/missing.json")
	require.NoError(t, err)
	assert.Nil(t, body)
	assert.Empty(t, etag)
}

func TestUnionRecords_Dedup(t *testing.T) {
	a := CostRecord{Timestamp: time.Unix(1, 0).UTC(), JobID: "j", FileName: "f"}
	b := CostRecord{Timestamp: time.Unix(2, 0).UTC(), JobID: "k", FileName: "g"}
	// a appears in both slices → deduped to one.
	out := unionRecords([]CostRecord{a}, []CostRecord{a, b})
	assert.Len(t, out, 2)
}

func TestValidateProjectID(t *testing.T) {
	require.NoError(t, validateProjectID("20251206-abc123"))
	require.NoError(t, validateProjectID("team_project.1"))
	assert.Error(t, validateProjectID("a/b"))
	assert.Error(t, validateProjectID("a\x00b"))
}

func TestNewStoreForSpec_DefaultsLocal(t *testing.T) {
	store, err := newStoreForSpec(context.Background(), "", aws.Config{})
	require.NoError(t, err)
	_, isLocal := store.(localStore)
	assert.True(t, isLocal, "empty spec must select the local store")
}

// --- test helpers ---

// headerSpyS3 wraps a fakeS3 to observe PutObject conditional headers.
type headerSpyS3 struct {
	*fakeS3
	onPut func(*s3.PutObjectInput)
}

func (h *headerSpyS3) PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if h.onPut != nil {
		h.onPut(in)
	}
	return h.fakeS3.PutObject(ctx, in, optFns...)
}

func seedObject(f *fakeS3, key string, doc any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		panic(err)
	}
	f.objects[key] = b
	f.seq++
	f.etags[key] = fmt.Sprintf("\"seed-%d\"", f.seq)
}
