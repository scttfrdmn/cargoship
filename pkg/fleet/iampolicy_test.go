package fleet

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWriterIAMPolicy(t *testing.T) {
	const bucket = "my-bucket"

	t.Run("writer with kms", func(t *testing.T) {
		out, err := WriterIAMPolicy(bucket, "backups/writers/lab-nas-1", "arn:aws:kms:us-west-2:111122223333:key/abcd")
		if err != nil {
			t.Fatalf("WriterIAMPolicy: %v", err)
		}
		assertNoDestructive(t, out)

		var doc struct {
			Version   string
			Statement []struct {
				Sid       string
				Effect    string
				Action    []string
				Resource  string
				Condition map[string]map[string][]string
			}
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("emitted policy is not valid JSON: %v\n%s", err, out)
		}
		if doc.Version != "2012-10-17" {
			t.Errorf("Version=%q, want 2012-10-17", doc.Version)
		}
		if len(doc.Statement) != 3 {
			t.Fatalf("want 3 statements (objects, list, kms), got %d", len(doc.Statement))
		}
		obj := doc.Statement[0]
		if obj.Effect != "Allow" || obj.Resource != "arn:aws:s3:::my-bucket/backups/writers/lab-nas-1/*" {
			t.Errorf("object statement = %+v", obj)
		}
		if !hasAll(obj.Action, "s3:PutObject", "s3:GetObject", "s3:AbortMultipartUpload") {
			t.Errorf("object actions = %v", obj.Action)
		}
		list := doc.Statement[1]
		if list.Resource != "arn:aws:s3:::my-bucket" {
			t.Errorf("list resource = %q, want bucket ARN", list.Resource)
		}
		if got := list.Condition["StringLike"]["s3:prefix"]; len(got) != 1 || got[0] != "backups/writers/lab-nas-1/*" {
			t.Errorf("list s3:prefix condition = %v", got)
		}
		kms := doc.Statement[2]
		if kms.Resource != "arn:aws:kms:us-west-2:111122223333:key/abcd" {
			t.Errorf("kms resource = %q", kms.Resource)
		}
		if !hasAll(kms.Action, "kms:GenerateDataKey", "kms:Decrypt") {
			t.Errorf("kms actions = %v", kms.Action)
		}
	})

	t.Run("writer without kms omits the kms statement", func(t *testing.T) {
		out, err := WriterIAMPolicy(bucket, "backups/writers/w1", "")
		if err != nil {
			t.Fatal(err)
		}
		assertNoDestructive(t, out)
		if strings.Contains(out, "kms:") {
			t.Errorf("no-kms policy should not mention kms:\n%s", out)
		}
		if n := strings.Count(out, `"Sid"`); n != 2 {
			t.Errorf("want 2 statements without kms, got %d", n)
		}
	})

	t.Run("empty prefix scopes to the whole bucket with no prefix condition", func(t *testing.T) {
		out, err := WriterIAMPolicy(bucket, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, `"arn:aws:s3:::my-bucket/*"`) {
			t.Errorf("empty prefix should scope objects to bucket/*:\n%s", out)
		}
		if strings.Contains(out, "s3:prefix") {
			t.Errorf("empty prefix should emit no s3:prefix condition:\n%s", out)
		}
	})

	t.Run("empty bucket errors", func(t *testing.T) {
		if _, err := WriterIAMPolicy("", "p", ""); err == nil {
			t.Error("empty bucket should error")
		}
	})
}

// assertNoDestructive is the core safety invariant: an emitted writer policy must
// never grant a delete, a wildcard, or KMS encrypt/beyond the two named actions.
func assertNoDestructive(t *testing.T, policy string) {
	t.Helper()
	for _, forbidden := range []string{
		"s3:DeleteObject", "s3:DeleteObjects", "s3:DeleteBucket",
		"kms:Encrypt", "s3:*", "kms:*",
	} {
		if strings.Contains(policy, forbidden) {
			t.Errorf("policy must not grant %q; got:\n%s", forbidden, policy)
		}
	}
}

func hasAll(have []string, want ...string) bool {
	set := make(map[string]bool, len(have))
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}
