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
)

// s3Store is an S3-backed BudgetStore. It persists one object per project plus a
// global object, and uses S3 conditional writes (If-Match / If-None-Match) for
// optimistic-concurrency compare-and-swap so concurrent writers from different
// machines converge without lost updates (#246 Phase B).
//
// The BudgetStore interface stays whole-document (Load returns the full
// LedgerState); the per-object layout and CAS live entirely inside this type.
// The opaque Token carries a serialized map of per-object ETags (s3Token). A
// conditional write that fails with 412 is reported as ErrBudgetStoreConflict;
// the caller re-loads and re-applies its own changes (saveMerged, #1031), so
// this type never merges on its own.
type s3Store struct {
	client s3API
	bucket string
	prefix string // normalized to end with "/"
	ctx    context.Context

	// mirror is an optional local write-through cache used to serve reads when S3
	// is unreachable. Nil disables mirroring.
	mirror *localStore

	// loaded is each object's ETag and body as of the most recent Load, so Save
	// can skip objects it would rewrite byte-for-byte. Not writing an unchanged
	// object keeps writers that touch different projects from conflicting.
	loaded map[string]loadedObject
}

// loadedObject is one store object as read by Load.
type loadedObject struct {
	etag string
	body []byte
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
	AlertConfig  *BudgetAlertConfig   `json:"alert_config,omitempty"` // #630
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
		client: client,
		bucket: bucket,
		prefix: prefix,
		ctx:    ctx,
		mirror: mirror,
	}
}

func (s *s3Store) globalKey() string { return s.prefix + globalObjectName }

func (s *s3Store) projectKey(projectID string) string {
	return s.prefix + projectsPrefix + url.PathEscape(projectID) + ".json"
}

// Load lists and fetches every store object, reassembling the whole LedgerState
// and a composite token of per-object ETags. A missing/empty store is not an
// error (empty state). When S3 is unreachable and a mirror is configured, it
// serves the cached state with an empty token (so a Save against it can only
// create objects, never overwrite them off stale cache). An object that exists
// but does not parse is an error wrapping ErrBudgetStoreCorrupt, and is never
// papered over with the mirror.
func (s *s3Store) Load() (LedgerState, Token, error) {
	state, tok, err := s.loadFromS3()
	if err != nil {
		if s.mirror != nil && !errors.Is(err, ErrBudgetStoreCorrupt) {
			if ms, _, mErr := s.mirror.Load(); mErr == nil {
				return ms, "", nil // empty token: reads OK, writes must re-Load from S3
			}
		}
		return LedgerState{}, "", err
	}
	return state, tok, nil
}

// corruptObjectError names an unparseable store object (#1031).
func (s *s3Store) corruptObjectError(key string, err error) error {
	return fmt.Errorf("%w: s3://%s/%s: %v (budgets cannot be enforced from it; cargoship will not overwrite it, so repair or remove the object)",
		ErrBudgetStoreCorrupt, s.bucket, key, err)
}

func (s *s3Store) loadFromS3() (LedgerState, Token, error) {
	keys, err := s.listKeys()
	if err != nil {
		return LedgerState{}, "", err
	}

	state := LedgerState{Version: StoreVersion, ProjectBudgets: map[string]config.ProjectBudget{}}
	tokenData := s3Token{Projects: map[string]string{}}
	loaded := map[string]loadedObject{}
	seen := map[string]bool{} // record identities contributed by earlier objects

	// Global object.
	if hasKey(keys, s.globalKey()) {
		body, etag, gErr := s.getObject(s.globalKey())
		if gErr != nil {
			return LedgerState{}, "", gErr
		}
		if body != nil {
			var gd globalDoc
			if uErr := json.Unmarshal(body, &gd); uErr != nil {
				return LedgerState{}, "", s.corruptObjectError(s.globalKey(), uErr)
			}
			state.Records = appendDistinct(state.Records, gd.Records, seen)
			tokenData.Global = etag
			state.GlobalBudget = gd.GlobalBudget
			state.AlertConfig = gd.AlertConfig
			loaded[s.globalKey()] = loadedObject{etag: etag, body: body}
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
			return LedgerState{}, "", s.corruptObjectError(key, uErr)
		}
		id := pd.Budget.ProjectID
		if id == "" {
			// Recover the ID from the key if the doc didn't carry it.
			id = projectIDFromKey(key, pfx)
		}
		if id != "" {
			state.ProjectBudgets[id] = pd.Budget
			tokenData.Projects[id] = etag
			loaded[key] = loadedObject{etag: etag, body: body}
		}
		state.Records = appendDistinct(state.Records, pd.Records, seen)
	}

	s.loaded = loaded
	return state, encodeToken(tokenData), nil
}

// Save splits the whole LedgerState into per-object docs and writes each one
// that changed with a conditional PUT (CAS) against the ETag in token. state is
// the complete document: a nil GlobalBudget or AlertConfig is written as unset.
// A 412 on any object returns ErrBudgetStoreConflict; objects already written
// in this call stay written, which is safe because the caller re-loads and
// re-applies the same change set (saveMerged).
func (s *s3Store) Save(state LedgerState, token Token) error {
	prev := decodeToken(token)

	// Route records to their owning object: a record whose ProjectID has a budget
	// goes in that project's object; everything else goes to global.json.
	projRecords, globalRecords := routeRecords(state.Records, state.ProjectBudgets)

	// Write each project object.
	for id, budget := range state.ProjectBudgets {
		doc := projectDoc{Version: StoreVersion, Budget: budget, Records: projRecords[id]}
		if err := s.saveObject(s.projectKey(id), prev.Projects[id], doc); err != nil {
			return err
		}
	}

	gdoc := globalDoc{Version: StoreVersion, Records: globalRecords, GlobalBudget: state.GlobalBudget, AlertConfig: state.AlertConfig}
	if err := s.saveObject(s.globalKey(), prev.Global, gdoc); err != nil {
		return err
	}

	// Delete project objects that existed before but are gone now (budget
	// removed). Their records were just routed into global.json above, so the
	// delete comes last; a failure is returned, or the next Load would resurrect
	// the budget.
	for id := range prev.Projects {
		if _, still := state.ProjectBudgets[id]; still {
			continue
		}
		key := s.projectKey(id)
		if _, err := s.client.DeleteObject(s.ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		}); err != nil && !isNotFound(err) {
			return fmt.Errorf("delete %s: %w", key, err)
		}
	}

	// Best-effort write-through to the local mirror so reads work offline. The
	// authoritative S3 write already succeeded, so a mirror failure is not one.
	if s.mirror != nil {
		_ = s.mirror.Save(state, "")
	}
	return nil
}

// saveObject marshals doc and writes it with a conditional PUT, unless the
// object is byte-for-byte what the last Load read at the same ETag.
func (s *s3Store) saveObject(key, knownETag string, doc any) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}
	if lo, ok := s.loaded[key]; ok && knownETag != "" && lo.etag == knownETag && bytes.Equal(lo.body, data) {
		return nil
	}
	return s.putConditional(key, data, knownETag)
}

// putConditional writes data to key only if the object is still at knownETag
// (If-Match), or still absent when knownETag is empty (If-None-Match: *). A 412
// PreconditionFailed is returned wrapping ErrBudgetStoreConflict.
func (s *s3Store) putConditional(key string, data []byte, knownETag string) error {
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
	if _, err := s.client.PutObject(s.ctx, in); err != nil {
		if isPreconditionFailed(err) {
			return fmt.Errorf("conditional put %s: %w (%w)", key, ErrBudgetStoreConflict, err)
		}
		return fmt.Errorf("conditional put %s: %w", key, err)
	}
	return nil
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

// appendDistinct appends one object's records to dst, skipping any whose
// identity an earlier object already contributed. A budget set or remove moves a
// project's records between global.json and the project's object in two writes;
// if the second never lands, a record is briefly in both, and it must count once.
// Records repeated within one object are kept as they are.
func appendDistinct(dst, recs []CostRecord, seen map[string]bool) []CostRecord {
	mine := make([]string, 0, len(recs))
	for _, r := range recs {
		k := recordKey(r)
		if seen[k] {
			continue
		}
		dst = append(dst, r)
		mine = append(mine, k)
	}
	for _, k := range mine {
		seen[k] = true
	}
	return dst
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
