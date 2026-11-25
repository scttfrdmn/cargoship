package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// fakeLifecycleS3 holds one bucket's lifecycle configuration.
type fakeLifecycleS3 struct {
	rules   []types.LifecycleRule
	has     bool
	getErr  error
	deleted bool
}

func (f *fakeLifecycleS3) PutBucketLifecycleConfiguration(_ context.Context, in *s3.PutBucketLifecycleConfigurationInput, _ ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
	f.rules, f.has = in.LifecycleConfiguration.Rules, true
	return &s3.PutBucketLifecycleConfigurationOutput{}, nil
}

func (f *fakeLifecycleS3) GetBucketLifecycleConfiguration(context.Context, *s3.GetBucketLifecycleConfigurationInput, ...func(*s3.Options)) (*s3.GetBucketLifecycleConfigurationOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if !f.has {
		return nil, &smithy.GenericAPIError{Code: "NoSuchLifecycleConfiguration"}
	}
	return &s3.GetBucketLifecycleConfigurationOutput{Rules: f.rules}, nil
}

func (f *fakeLifecycleS3) DeleteBucketLifecycle(context.Context, *s3.DeleteBucketLifecycleInput, ...func(*s3.Options)) (*s3.DeleteBucketLifecycleOutput, error) {
	f.rules, f.has, f.deleted = nil, false, true
	return &s3.DeleteBucketLifecycleOutput{}, nil
}

func ruleIDs(rules []types.LifecycleRule) map[string]bool {
	ids := map[string]bool{}
	for _, r := range rules {
		ids[aws.ToString(r.ID)] = true
	}
	return ids
}

func otherTeamsRule() types.LifecycleRule {
	return types.LifecycleRule{ID: aws.String("finance-expire-logs"), Status: types.ExpirationStatusEnabled,
		Filter: &types.LifecycleRuleFilter{Prefix: aws.String("logs/")}, Expiration: &types.LifecycleExpiration{Days: aws.Int32(30)}}
}

// TestApplyPolicy_KeepsOtherRules pins #920: PutBucketLifecycleConfiguration
// replaces the whole configuration, so applying a template deleted every other
// team's rules on a shared bucket.
func TestApplyPolicy_KeepsOtherRules(t *testing.T) {
	templates := GetPredefinedTemplates()
	f := &fakeLifecycleS3{rules: []types.LifecycleRule{otherTeamsRule()}, has: true}
	m := NewManager(f, "shared")

	if err := m.ApplyPolicy(context.Background(), templates[0]); err != nil {
		t.Fatal(err)
	}
	ids := ruleIDs(f.rules)
	if !ids["finance-expire-logs"] {
		t.Fatalf("another team's rule was dropped: %v", ids)
	}
	if !ids[templates[0].Rules[0].ID] {
		t.Fatalf("the template's rule is missing: %v", ids)
	}

	// Switching templates replaces CargoShip's rule rather than stacking both.
	if err := m.ApplyPolicy(context.Background(), templates[1]); err != nil {
		t.Fatal(err)
	}
	ids = ruleIDs(f.rules)
	if ids[templates[0].Rules[0].ID] || !ids[templates[1].Rules[0].ID] || !ids["finance-expire-logs"] {
		t.Errorf("after switching templates: %v", ids)
	}
}

func TestApplyPolicy_EmptyBucketAndReadErrors(t *testing.T) {
	f := &fakeLifecycleS3{}
	if err := NewManager(f, "b").ApplyPolicy(context.Background(), GetPredefinedTemplates()[0]); err != nil {
		t.Fatalf("a bucket with no configuration: %v", err)
	}
	if len(f.rules) == 0 {
		t.Error("the template must be applied")
	}

	broken := &fakeLifecycleS3{getErr: errors.New("AccessDenied")}
	if err := NewManager(broken, "b").ApplyPolicy(context.Background(), GetPredefinedTemplates()[0]); err == nil {
		t.Error("without the current rules, nothing may be written")
	}
	if broken.has {
		t.Error("a configuration was written blind")
	}
}

// TestRemovePolicy_RemovesOnlyCargoShipRules pins #920: --remove deleted the
// bucket's entire lifecycle configuration.
func TestRemovePolicy_RemovesOnlyCargoShipRules(t *testing.T) {
	ours := GetPredefinedTemplates()[0]
	f := &fakeLifecycleS3{}
	m := NewManager(f, "shared")
	f.rules, f.has = []types.LifecycleRule{otherTeamsRule()}, true
	if err := m.ApplyPolicy(context.Background(), ours); err != nil {
		t.Fatal(err)
	}

	if err := m.RemovePolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	ids := ruleIDs(f.rules)
	if f.deleted || !ids["finance-expire-logs"] || ids[ours.Rules[0].ID] {
		t.Errorf("remove must drop only CargoShip's rules: deleted=%v rules=%v", f.deleted, ids)
	}

	// With only CargoShip's rules, the configuration is deleted.
	solo := &fakeLifecycleS3{}
	ms := NewManager(solo, "ours")
	if err := ms.ApplyPolicy(context.Background(), ours); err != nil {
		t.Fatal(err)
	}
	if err := ms.RemovePolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !solo.deleted {
		t.Error("the configuration should be deleted when nothing else remains")
	}

	// Nothing of ours: a no-op.
	only := &fakeLifecycleS3{rules: []types.LifecycleRule{otherTeamsRule()}, has: true}
	if err := NewManager(only, "x").RemovePolicy(context.Background()); err != nil || only.deleted || len(only.rules) != 1 {
		t.Errorf("a bucket with no CargoShip rules must be left alone: err=%v deleted=%v", err, only.deleted)
	}
}
