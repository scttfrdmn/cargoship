package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	statusObjectName = "status.json"
	// maxStatusBytes bounds an untrusted heartbeat read.
	maxStatusBytes = 1 << 20 // 1 MiB
)

// SourceStatus is the last-known state of one backed-up source directory (#615).
type SourceStatus struct {
	Path        string    `json:"path"`
	OK          bool      `json:"ok"`
	LastError   string    `json:"last_error,omitempty"`
	LastSuccess time.Time `json:"last_success,omitempty"`
	UploadID    string    `json:"upload_id,omitempty"`
	Files       int64     `json:"files"`
	Bytes       int64     `json:"bytes"`
	SyncType    string    `json:"sync_type,omitempty"`
	NoChanges   bool      `json:"no_changes"`
}

// WriterStatus is a ghostship writer's heartbeat, PUT to writers/<id>/status.json each
// cycle (#615). The agent writes it (write-only); the control side reads it
// (`cargoship fleet status`, and the stale-writer monitor). InstanceID is per-boot: two
// distinct instance ids observed under one writer id indicate a cloned-VM collision.
type WriterStatus struct {
	WriterID         string `json:"writer_id"`
	Hostname         string `json:"hostname"`
	InstanceID       string `json:"instance_id"`
	CargoshipVersion string `json:"cargoship_version"`
	// ConfigVersion is the fleet config version this writer is running (#614), from the
	// config's `version` field. Zero (omitted) means unversioned or single-source/flag
	// mode. Surfacing it here makes a bad config rollout visible across the fleet.
	ConfigVersion int            `json:"config_version,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Sources       []SourceStatus `json:"sources"`
}

// Healthy reports whether the writer's most recent cycle succeeded for every source.
func (w WriterStatus) Healthy() bool {
	if len(w.Sources) == 0 {
		return false
	}
	for _, s := range w.Sources {
		if !s.OK {
			return false
		}
	}
	return true
}

// s3PutAPI / s3ReadAPI are the minimal S3 surfaces used here (satisfied by *s3.Client),
// narrow so tests can fake them.
type s3PutAPI interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}
type s3ReadAPI interface {
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// StatusKey returns the S3 key of a writer's heartbeat given its (writer-folded) prefix.
func StatusKey(writerPrefix string) string {
	return path.Join(writerPrefix, statusObjectName)
}

// WriteStatus PUTs a writer's heartbeat to <writerPrefix>/status.json. writerPrefix is
// the folded prefix (writers/<id>); one small PutObject, no read (write-only-friendly).
func WriteStatus(ctx context.Context, api s3PutAPI, bucket, writerPrefix string, st WriterStatus) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal writer status: %w", err)
	}
	if _, err := api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(StatusKey(writerPrefix)),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	}); err != nil {
		return fmt.Errorf("put writer status: %w", err)
	}
	return nil
}

// ListWriterStatuses enumerates writers under <basePrefix>/writers/ and returns each
// one's heartbeat. basePrefix is the fleet's shared base prefix (NOT writer-folded).
// Writers whose status.json is missing or unparseable are skipped.
func ListWriterStatuses(ctx context.Context, api s3ReadAPI, bucket, basePrefix string) ([]WriterStatus, error) {
	listPrefix := path.Join(basePrefix, "writers") + "/"
	pager := s3.NewListObjectsV2Paginator(api, &s3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Prefix:    aws.String(listPrefix),
		Delimiter: aws.String("/"),
	})
	var out []WriterStatus
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list writers under %s: %w", listPrefix, err)
		}
		for _, cp := range page.CommonPrefixes {
			writerPrefix := strings.TrimSuffix(aws.ToString(cp.Prefix), "/") // <basePrefix>/writers/<id>
			st, err := getWriterStatus(ctx, api, bucket, writerPrefix)
			if err != nil {
				continue // skip writers without a readable heartbeat
			}
			out = append(out, st)
		}
	}
	return out, nil
}

func getWriterStatus(ctx context.Context, api s3ReadAPI, bucket, writerPrefix string) (WriterStatus, error) {
	obj, err := api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(StatusKey(writerPrefix)),
	})
	if err != nil {
		return WriterStatus{}, err
	}
	defer func() { _ = obj.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(obj.Body, maxStatusBytes))
	if err != nil {
		return WriterStatus{}, err
	}
	var st WriterStatus
	if err := json.Unmarshal(data, &st); err != nil {
		return WriterStatus{}, err
	}
	return st, nil
}
