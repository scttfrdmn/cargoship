package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// #993: lifecycle templates must never move a manifest to a colder class. The
// manifest sits next to its chunks (<prefix>/uploads/<id>/manifest.json.gz), so
// a prefix-only rule froze it in GLACIER/DEEP_ARCHIVE and broke sync, list,
// info, verify and restore.

// chunkTags is what a CargoShip archive chunk carries; manifests carry nothing.
var chunkTags = map[string]string{"cargoship-object": "chunk"}

// ruleMatches evaluates an S3 lifecycle rule filter against an object the way S3
// does: Prefix, Tag, or an And of prefix and tags (size bounds unused here).
func ruleMatches(r types.LifecycleRule, key string, tags map[string]string) bool {
	if r.Status != types.ExpirationStatusEnabled {
		return false
	}
	f := r.Filter
	if f == nil {
		return true // no filter: the whole bucket
	}
	if f.And != nil {
		if !strings.HasPrefix(key, aws.ToString(f.And.Prefix)) {
			return false
		}
		for _, tg := range f.And.Tags {
			if tags[aws.ToString(tg.Key)] != aws.ToString(tg.Value) {
				return false
			}
		}
		return true
	}
	if f.Tag != nil {
		return tags[aws.ToString(f.Tag.Key)] == aws.ToString(f.Tag.Value)
	}
	return strings.HasPrefix(key, aws.ToString(f.Prefix))
}

// rulePrefix returns the key prefix a rule is scoped to.
func rulePrefix(r types.LifecycleRule) string {
	if r.Filter == nil {
		return ""
	}
	if r.Filter.And != nil {
		return aws.ToString(r.Filter.And.Prefix)
	}
	return aws.ToString(r.Filter.Prefix)
}

func appliedRules(t *testing.T, tmpl PolicyTemplate) []types.LifecycleRule {
	t.Helper()
	f := &fakeLifecycleS3{}
	if err := NewManager(f, "b").ApplyPolicy(context.Background(), tmpl); err != nil {
		t.Fatalf("apply %s: %v", tmpl.ID, err)
	}
	return f.rules
}

func TestTemplates_NeverTransitionManifests(t *testing.T) {
	manifests := []string{
		"manifest.json", "manifest.json.gz",
		"manifest.encrypted.json", "manifest.encrypted.json.gz",
		"manifest.partial.json.gz", "manifest.partial.encrypted.json.gz",
	}
	for _, tmpl := range GetPredefinedTemplates() {
		t.Run(tmpl.ID, func(t *testing.T) {
			rules := appliedRules(t, tmpl)
			var transitioning []types.LifecycleRule
			for _, r := range rules {
				if len(r.Transitions) > 0 {
					transitioning = append(transitioning, r)
				}
			}
			if len(transitioning) == 0 {
				t.Fatal("template has no transition rule")
			}
			for _, r := range transitioning {
				p := rulePrefix(r)
				for _, name := range manifests {
					key := p + "uploads/20261127-abc/" + name
					if ruleMatches(r, key, nil) {
						t.Errorf("rule %s transitions manifest %s", aws.ToString(r.ID), key)
					}
				}
				chunk := p + "uploads/20261127-abc/shard-0/chunk-0.tar.zst"
				if !ruleMatches(r, chunk, chunkTags) {
					t.Errorf("rule %s does not transition chunk %s", aws.ToString(r.ID), chunk)
				}
			}
		})
	}
}

func TestNormalizePrefix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"/", ""},
		{"backup", "backup/"},
		{"backup/", "backup/"},
		{"/backup/daily//", "backup/daily/"},
	}
	for _, tt := range tests {
		if got := NormalizePrefix(tt.in); got != tt.want {
			t.Errorf("NormalizePrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestTemplateFor_ScopesToUploadPrefix: templates follow the user's prefix
// instead of a fixed archives/ one, and leave other prefixes alone.
func TestTemplateFor_ScopesToUploadPrefix(t *testing.T) {
	for _, tmpl := range PredefinedTemplates("") {
		t.Run(tmpl.ID, func(t *testing.T) {
			scoped, err := TemplateFor(tmpl.ID, "backup")
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range appliedRules(t, scoped) {
				if got := rulePrefix(r); got != "backup/" {
					t.Errorf("rule %s prefix = %q, want backup/", aws.ToString(r.ID), got)
				}
				if ruleMatches(r, "backup2/uploads/x/shard-0/chunk-0.tar.zst", chunkTags) {
					t.Errorf("rule %s leaks into backup2/", aws.ToString(r.ID))
				}
			}
		})
	}
}

func TestTemplateFor_Errors(t *testing.T) {
	if _, err := TemplateFor("no-such-template", "backup"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown template: err = %v", err)
	}
	_, err := TemplateFor(templateCompliance, "")
	if err == nil || !strings.Contains(err.Error(), "prefix") {
		t.Errorf("compliance-retention over the whole bucket must be refused, err = %v", err)
	}
	if _, err := TemplateFor("archive-optimization", ""); err != nil {
		t.Errorf("a non-deleting template may cover the whole bucket: %v", err)
	}
}

// TestComplianceTemplate_ExpiresWholeUploads: the 7-year expiration removes an
// upload's manifests with its chunks, so neither is stranded, and nothing
// outside the prefix.
func TestComplianceTemplate_ExpiresWholeUploads(t *testing.T) {
	tmpl, err := TemplateFor(templateCompliance, "records")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tmpl.Description, "DELETES") || !strings.Contains(tmpl.Description, "2555") {
		t.Errorf("description must say it deletes data and when: %q", tmpl.Description)
	}
	var expiring []types.LifecycleRule
	for _, r := range appliedRules(t, tmpl) {
		if r.Expiration != nil {
			expiring = append(expiring, r)
			if got := aws.ToInt32(r.Expiration.Days); got != ComplianceRetentionDays {
				t.Errorf("expiration = %d days, want %d", got, ComplianceRetentionDays)
			}
		}
	}
	if len(expiring) != 1 {
		t.Fatalf("want one expiring rule, got %d", len(expiring))
	}
	r := expiring[0]
	if !ruleMatches(r, "records/uploads/u/manifest.json.gz", nil) || !ruleMatches(r, "records/uploads/u/shard-0/chunk-0.tar.zst", chunkTags) {
		t.Error("expiration must cover both manifests and chunks under the prefix")
	}
	if ruleMatches(r, "other/uploads/u/manifest.json.gz", nil) {
		t.Error("expiration must not reach outside the prefix")
	}
}

func TestConvertToAWSRule_WholeBucketFilter(t *testing.T) {
	m := NewManager(&fakeLifecycleS3{}, "b")
	r, err := m.convertToAWSRule(LifecycleRule{ID: "cargoship-x", Status: "Enabled", AbortIncompleteUploads: aws.Int(1)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Filter == nil || r.Filter.Prefix == nil || *r.Filter.Prefix != "" {
		t.Fatalf("a rule with no prefix or tags needs an explicit empty-prefix filter, got %+v", r.Filter)
	}

	tagOnly, err := m.convertToAWSRule(LifecycleRule{ID: "cargoship-y", Status: "Enabled",
		Filter: RuleFilter{Tags: map[string]string{ChunkTagKey: ChunkTagValue}}})
	if err != nil {
		t.Fatal(err)
	}
	if tagOnly.Filter == nil || tagOnly.Filter.Tag == nil || aws.ToString(tagOnly.Filter.Tag.Key) != ChunkTagKey {
		t.Fatalf("whole-bucket chunk rule should be a tag filter, got %+v", tagOnly.Filter)
	}
}

func TestValidateRule_TagFilterWithAbortRejected(t *testing.T) {
	m := NewManager(&fakeLifecycleS3{}, "b")
	err := m.ValidatePolicy(PolicyTemplate{ID: "p", Rules: []LifecycleRule{{
		ID: "r", Status: "Enabled",
		Filter:                 RuleFilter{Prefix: "x/", Tags: map[string]string{ChunkTagKey: ChunkTagValue}},
		AbortIncompleteUploads: aws.Int(1),
	}}})
	if err == nil || !strings.Contains(err.Error(), "tag filter") {
		t.Fatalf("err = %v, want the tag+abort rejection", err)
	}
	for _, tmpl := range PredefinedTemplates("x") {
		if err := m.ValidatePolicy(tmpl); err != nil {
			t.Errorf("built-in %s fails validation: %v", tmpl.ID, err)
		}
	}
}

// TestTemplates_TagRulesDoNotAbortUploads: S3 rejects a tag-filtered rule with
// AbortIncompleteMultipartUpload, so multipart cleanup must live in a rule
// scoped by prefix only.
func TestTemplates_TagRulesDoNotAbortUploads(t *testing.T) {
	for _, tmpl := range GetPredefinedTemplates() {
		hasAbort := false
		for _, r := range appliedRules(t, tmpl) {
			tagged := r.Filter != nil && (r.Filter.Tag != nil || (r.Filter.And != nil && len(r.Filter.And.Tags) > 0))
			if r.AbortIncompleteMultipartUpload != nil {
				hasAbort = true
				if tagged {
					t.Errorf("%s: rule %s combines a tag filter with AbortIncompleteMultipartUpload", tmpl.ID, aws.ToString(r.ID))
				}
			}
		}
		if !hasAbort {
			t.Errorf("%s: incomplete multipart uploads are no longer cleaned up", tmpl.ID)
		}
	}
}
