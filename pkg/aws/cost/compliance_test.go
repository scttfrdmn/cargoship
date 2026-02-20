package cost

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeProjectRecord creates a cost record with a specific project and optional DVC provenance.
func makeProjectRecord(projectID, dvcStage, gitCommit, pipeline string, cost float64, sizeGB float64, ts time.Time) CostRecord {
	return CostRecord{
		Timestamp:    ts,
		Operation:    "upload",
		Service:      "s3",
		Region:       "us-east-1",
		StorageClass: "GLACIER_IR",
		SizeGB:       sizeGB,
		Cost:         cost,
		Currency:     "USD",
		FileName:     "chunk.tar.zst",
		ProjectID:    projectID,
		DVCStage:     dvcStage,
		DVCPipeline:  pipeline,
		GitCommit:    gitCommit,
	}
}

// ---------------------------------------------------------------------------
// GenerateNSFComplianceReport
// ---------------------------------------------------------------------------

func TestGenerateNSFComplianceReport_Basic(t *testing.T) {
	reporter := newTestReporter()
	now := time.Now()

	reporter.RecordCost(makeProjectRecord("proj-abc", "preprocess", "aaa111", "dvc.yaml", 0.01, 0.5, now.Add(-2*time.Hour)))
	reporter.RecordCost(makeProjectRecord("proj-abc", "train", "bbb222", "dvc.yaml", 0.05, 2.0, now.Add(-1*time.Hour)))
	reporter.RecordCost(makeProjectRecord("proj-abc", "train", "aaa111", "dvc.yaml", 0.04, 1.5, now))
	// Different project — must not appear
	reporter.RecordCost(makeProjectRecord("other-proj", "evaluate", "ccc333", "dvc.yaml", 0.10, 5.0, now))

	report, err := reporter.GenerateNSFComplianceReport("proj-abc", "NSF-2024-12345")
	require.NoError(t, err)

	assert.Equal(t, "NSF", report.ReportType)
	assert.Equal(t, "NSF-2024-12345", report.GrantNumber)
	assert.Equal(t, "proj-abc", report.BudgetID)
	assert.Equal(t, int64(3), report.TotalFiles)
	assert.InDelta(t, 4.0, report.TotalSizeGB, 1e-9)
	assert.InDelta(t, 0.10, report.TotalCost, 1e-9)
	assert.Equal(t, "USD", report.Currency)

	// Reproducibility fields
	assert.ElementsMatch(t, []string{"aaa111", "bbb222"}, report.GitCommits)
	assert.ElementsMatch(t, []string{"preprocess", "train"}, report.DVCStages)
	assert.Equal(t, []string{"dvc.yaml"}, report.PipelineIDs)

	// Cost by stage
	require.NotNil(t, report.CostByDVCStage)
	assert.InDelta(t, 0.01, report.CostByDVCStage["preprocess"], 1e-9)
	assert.InDelta(t, 0.09, report.CostByDVCStage["train"], 1e-9)

	// DMP fields
	dmp := report.DataManagementPlan
	assert.Contains(t, dmp.RetentionPolicy, "NSF")
	assert.NotEmpty(t, dmp.StorageProvider)
	assert.NotEmpty(t, dmp.ReproducibilityNote)
	assert.Contains(t, dmp.ReproducibilityNote, "2 pipeline stage")
	assert.Contains(t, dmp.ReproducibilityNote, "2 git commit")
}

func TestGenerateNSFComplianceReport_NotFound(t *testing.T) {
	reporter := newTestReporter()
	_, err := reporter.GenerateNSFComplianceReport("nonexistent", "NSF-000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

// ---------------------------------------------------------------------------
// GenerateNIHComplianceReport
// ---------------------------------------------------------------------------

func TestGenerateNIHComplianceReport_Basic(t *testing.T) {
	reporter := newTestReporter()
	now := time.Now()

	reporter.RecordCost(makeProjectRecord("nih-proj", "align", "abc", "dvc.yaml", 0.02, 1.0, now))
	reporter.RecordCost(makeProjectRecord("nih-proj", "variant_call", "abc", "dvc.yaml", 0.08, 4.0, now))

	report, err := reporter.GenerateNIHComplianceReport("nih-proj", "R01-GM123456")
	require.NoError(t, err)

	assert.Equal(t, "NIH", report.ReportType)
	assert.Equal(t, "R01-GM123456", report.GrantNumber)

	dmp := report.DataManagementPlan
	assert.Contains(t, dmp.RetentionPolicy, "NIH")
	assert.Contains(t, dmp.ComplianceNotes, "NIH")
}

func TestGenerateNIHComplianceReport_NotFound(t *testing.T) {
	reporter := newTestReporter()
	_, err := reporter.GenerateNIHComplianceReport("missing", "R01-XY000")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// No DVC provenance
// ---------------------------------------------------------------------------

func TestGenerateNSFComplianceReport_NoDVCProvenance(t *testing.T) {
	reporter := newTestReporter()
	// Records with no DVC or git fields
	r := makeProjectRecord("plain-proj", "", "", "", 0.03, 1.5, time.Now())
	reporter.RecordCost(r)

	report, err := reporter.GenerateNSFComplianceReport("plain-proj", "NSF-9999")
	require.NoError(t, err)

	assert.Nil(t, report.GitCommits)
	assert.Nil(t, report.DVCStages)
	assert.Nil(t, report.CostByDVCStage)
	assert.Contains(t, report.DataManagementPlan.ReproducibilityNote, "0 pipeline stage")
	assert.Contains(t, report.DataManagementPlan.ReproducibilityNote, "0 git commit")
}

// ---------------------------------------------------------------------------
// Period tracking
// ---------------------------------------------------------------------------

func TestComplianceReport_PeriodTracking(t *testing.T) {
	reporter := newTestReporter()
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)

	reporter.RecordCost(makeProjectRecord("p", "", "", "", 0.01, 1, t2))
	reporter.RecordCost(makeProjectRecord("p", "", "", "", 0.02, 2, t3))
	reporter.RecordCost(makeProjectRecord("p", "", "", "", 0.03, 3, t1))

	report, err := reporter.GenerateNSFComplianceReport("p", "NSF-1")
	require.NoError(t, err)

	assert.Equal(t, t1, report.PeriodStart)
	assert.Equal(t, t3, report.PeriodEnd)
}

// ---------------------------------------------------------------------------
// FormatComplianceReportText
// ---------------------------------------------------------------------------

func TestFormatComplianceReportText_ContainsKeyFields(t *testing.T) {
	reporter := newTestReporter()
	reporter.RecordCost(makeProjectRecord("grant-proj", "preprocess", "abc123", "dvc.yaml", 0.05, 2.0, time.Now()))

	report, err := reporter.GenerateNSFComplianceReport("grant-proj", "NSF-2025-99")
	require.NoError(t, err)

	text := FormatComplianceReportText(report)
	assert.Contains(t, text, "NSF")
	assert.Contains(t, text, "NSF-2025-99")
	assert.Contains(t, text, "grant-proj")
	assert.Contains(t, text, "preprocess")
	assert.Contains(t, text, "abc123")
	assert.Contains(t, text, "Data Management Plan")
	assert.Contains(t, text, "Retention")

	// Should not be empty
	assert.Greater(t, len(strings.TrimSpace(text)), 100)
}

func TestFormatComplianceReportText_WithPIName(t *testing.T) {
	report := &ComplianceReport{
		ReportType:  "NSF",
		GeneratedAt: time.Now(),
		GrantNumber: "NSF-TEST",
		PIName:      "Dr. Jane Smith",
		BudgetID:    "budget-xyz",
		Currency:    "USD",
		PeriodStart: time.Now(),
		PeriodEnd:   time.Now(),
		DataManagementPlan: DataManagementPlan{
			StorageProvider:     "Amazon S3",
			RetentionPolicy:     "3 years",
			AccessControls:      "IAM",
			BackupStrategy:      "S3 durability",
			DataFormat:          "zstd",
			ReproducibilityNote: "test note",
		},
	}

	text := FormatComplianceReportText(report)
	assert.Contains(t, text, "Dr. Jane Smith")
}

// ---------------------------------------------------------------------------
// Sorted output
// ---------------------------------------------------------------------------

func TestComplianceReport_SortedSlices(t *testing.T) {
	reporter := newTestReporter()
	now := time.Now()

	reporter.RecordCost(makeProjectRecord("s", "zzz", "ccc", "b.yaml", 0.01, 1, now))
	reporter.RecordCost(makeProjectRecord("s", "aaa", "aaa", "a.yaml", 0.02, 2, now))
	reporter.RecordCost(makeProjectRecord("s", "mmm", "bbb", "a.yaml", 0.03, 3, now))

	report, err := reporter.GenerateNSFComplianceReport("s", "NSF-SORT")
	require.NoError(t, err)

	assert.Equal(t, []string{"aaa", "bbb", "ccc"}, report.GitCommits)
	assert.Equal(t, []string{"aaa", "mmm", "zzz"}, report.DVCStages)
	assert.Equal(t, []string{"a.yaml", "b.yaml"}, report.PipelineIDs)
}
