package cost

import (
	"fmt"
	"sort"
	"time"
)

// ComplianceReport is a structured report for federal grant agencies (NSF, NIH)
// containing data provenance, reproducibility, and cost information required
// for data management plan compliance (Issue #187).
type ComplianceReport struct {
	// Report metadata
	ReportType  string    `json:"report_type"`  // "NSF" or "NIH"
	GeneratedAt time.Time `json:"generated_at"` // Report generation timestamp
	GrantNumber string    `json:"grant_number"` // Agency grant/award number
	PIName      string    `json:"pi_name"`      // Principal investigator name
	BudgetID    string    `json:"budget_id"`    // CargoShip project/budget identifier

	// Data statistics
	TotalFiles  int64   `json:"total_files"`
	TotalSizeGB float64 `json:"total_size_gb"`
	TotalCost   float64 `json:"total_cost"`
	Currency    string  `json:"currency"`

	// Period covered by the records in this report
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`

	// Reproducibility provenance — sourced from DVC/git fields in CostRecord
	GitCommits  []string `json:"git_commits,omitempty"`  // Unique git commit SHAs
	DVCStages   []string `json:"dvc_stages,omitempty"`   // Unique DVC stage names
	PipelineIDs []string `json:"pipeline_ids,omitempty"` // Unique dvc.yaml identifiers

	// Cost breakdown by DVC pipeline stage
	CostByDVCStage map[string]float64 `json:"cost_by_dvc_stage,omitempty"`

	// Data management plan required by NSF/NIH
	DataManagementPlan DataManagementPlan `json:"data_management_plan"`
}

// DataManagementPlan captures the structured data management information required
// by federal grant agencies (NSF / NIH).
type DataManagementPlan struct {
	// StorageProvider describes where data is stored (e.g., "Amazon S3")
	StorageProvider string `json:"storage_provider"`

	// RetentionPolicy is the declared retention duration
	RetentionPolicy string `json:"retention_policy"`

	// AccessControls describes how data is protected
	AccessControls string `json:"access_controls"`

	// BackupStrategy describes data durability / replication
	BackupStrategy string `json:"backup_strategy"`

	// DataFormat describes the file format and compression used
	DataFormat string `json:"data_format"`

	// ReproducibilityNote describes how data can be reproduced
	ReproducibilityNote string `json:"reproducibility_note"`

	// ComplianceNotes holds agency-specific notes
	ComplianceNotes string `json:"compliance_notes,omitempty"`
}

// GenerateNSFComplianceReport creates an NSF data-management compliance report
// for all cost records matching budgetID. grantNumber is the NSF award identifier
// (e.g., "NSF-2024-12345"). Returns an error when no matching records exist.
func (cr *CostReporter) GenerateNSFComplianceReport(budgetID, grantNumber string) (*ComplianceReport, error) {
	report, err := cr.buildComplianceReport(budgetID, grantNumber, "NSF")
	if err != nil {
		return nil, err
	}

	report.DataManagementPlan = DataManagementPlan{
		StorageProvider: "Amazon S3 (via CargoShip)",
		RetentionPolicy: "Minimum 3 years post-award, per NSF Data Management Plan requirements",
		AccessControls:  "AWS IAM roles and bucket policies; access limited to project personnel",
		BackupStrategy:  "S3 eleven-nines (99.999999999%) durability; cross-region replication recommended for critical datasets",
		DataFormat:      "Compressed archives (zstd) with per-file checksums; DVC content-addressed storage compatible",
		ReproducibilityNote: fmt.Sprintf(
			"Data provenance tracked via DVC pipeline hashes and git commit SHAs. "+
				"%d pipeline stage(s) and %d git commit(s) recorded in cost metadata.",
			len(report.DVCStages), len(report.GitCommits)),
		ComplianceNotes: "NSF Public Access Policy compliance: data uploaded with CargoShip preserves " +
			"full provenance chain from source code (git) through processing pipeline (DVC) to archived output.",
	}

	return report, nil
}

// GenerateNIHComplianceReport creates an NIH data-management compliance report
// for all cost records matching budgetID. grantNumber is the NIH award number
// (e.g., "R01-GM123456"). Returns an error when no matching records exist.
func (cr *CostReporter) GenerateNIHComplianceReport(budgetID, grantNumber string) (*ComplianceReport, error) {
	report, err := cr.buildComplianceReport(budgetID, grantNumber, "NIH")
	if err != nil {
		return nil, err
	}

	report.DataManagementPlan = DataManagementPlan{
		StorageProvider: "Amazon S3 (via CargoShip)",
		RetentionPolicy: "Minimum 7 years post-award, per NIH Data Sharing Policy requirements",
		AccessControls:  "AWS IAM roles and bucket policies; dbGaP-compatible access control available for human subjects data",
		BackupStrategy:  "S3 eleven-nines (99.999999999%) durability; Glacier Deep Archive for long-term retention",
		DataFormat:      "Compressed archives (zstd) with per-file checksums; FAIR data principles supported via DVC metadata",
		ReproducibilityNote: fmt.Sprintf(
			"Computational reproducibility supported via DVC pipeline hashes and git commit SHAs. "+
				"%d pipeline stage(s) and %d git commit(s) recorded in cost metadata.",
			len(report.DVCStages), len(report.GitCommits)),
		ComplianceNotes: "NIH Data Management and Sharing Policy compliance: all processed data archived " +
			"with full computational provenance. DVC pipeline stage costs tracked for budget reporting.",
	}

	return report, nil
}

// buildComplianceReport aggregates cost records for budgetID into a ComplianceReport.
func (cr *CostReporter) buildComplianceReport(budgetID, grantNumber, reportType string) (*ComplianceReport, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	var records []CostRecord
	for _, c := range cr.costs {
		if c.ProjectID == budgetID {
			records = append(records, c)
		}
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no cost records found for budget ID: %s", budgetID)
	}

	report := &ComplianceReport{
		ReportType:     reportType,
		GeneratedAt:    time.Now().UTC(),
		GrantNumber:    grantNumber,
		BudgetID:       budgetID,
		Currency:       records[0].Currency,
		PeriodStart:    records[0].Timestamp,
		PeriodEnd:      records[0].Timestamp,
		CostByDVCStage: make(map[string]float64),
	}

	commitSet := make(map[string]bool)
	stageSet := make(map[string]bool)
	pipelineSet := make(map[string]bool)

	for _, c := range records {
		report.TotalFiles++
		report.TotalSizeGB += c.SizeGB
		report.TotalCost += c.Cost

		if c.Timestamp.Before(report.PeriodStart) {
			report.PeriodStart = c.Timestamp
		}
		if c.Timestamp.After(report.PeriodEnd) {
			report.PeriodEnd = c.Timestamp
		}

		if c.GitCommit != "" {
			commitSet[c.GitCommit] = true
		}
		if c.DVCStage != "" {
			stageSet[c.DVCStage] = true
			report.CostByDVCStage[c.DVCStage] += c.Cost
		}
		if c.DVCPipeline != "" {
			pipelineSet[c.DVCPipeline] = true
		}
	}

	// Convert sets to sorted slices for deterministic output
	for k := range commitSet {
		report.GitCommits = append(report.GitCommits, k)
	}
	sort.Strings(report.GitCommits)

	for k := range stageSet {
		report.DVCStages = append(report.DVCStages, k)
	}
	sort.Strings(report.DVCStages)

	for k := range pipelineSet {
		report.PipelineIDs = append(report.PipelineIDs, k)
	}
	sort.Strings(report.PipelineIDs)

	if len(report.CostByDVCStage) == 0 {
		report.CostByDVCStage = nil
	}

	return report, nil
}

// FormatComplianceReportText returns a human-readable text rendering of a ComplianceReport.
func FormatComplianceReportText(r *ComplianceReport) string {
	var b []byte
	line := func(s string) {
		b = append(b, s...)
		b = append(b, '\n')
	}
	sep := "─────────────────────────────────────────────"

	line(fmt.Sprintf("%s Data Management Compliance Report", r.ReportType))
	line(sep)
	line(fmt.Sprintf("Grant Number : %s", r.GrantNumber))
	if r.PIName != "" {
		line(fmt.Sprintf("PI Name      : %s", r.PIName))
	}
	line(fmt.Sprintf("Budget ID    : %s", r.BudgetID))
	line(fmt.Sprintf("Generated    : %s", r.GeneratedAt.Format(time.RFC3339)))
	line(fmt.Sprintf("Period       : %s → %s",
		r.PeriodStart.Format("2006-01-02"),
		r.PeriodEnd.Format("2006-01-02")))
	line("")
	line("Data Statistics")
	line(sep)
	line(fmt.Sprintf("  Total files  : %d", r.TotalFiles))
	line(fmt.Sprintf("  Total size   : %.3f GB", r.TotalSizeGB))
	line(fmt.Sprintf("  Total cost   : $%.4f %s", r.TotalCost, r.Currency))
	line("")
	line("Reproducibility")
	line(sep)
	if len(r.GitCommits) > 0 {
		line(fmt.Sprintf("  Git commits  : %d unique", len(r.GitCommits)))
		for _, c := range r.GitCommits {
			line(fmt.Sprintf("    %s", c))
		}
	} else {
		line("  Git commits  : none recorded")
	}
	if len(r.DVCStages) > 0 {
		line(fmt.Sprintf("  DVC stages   : %s", joinStrings(r.DVCStages)))
	}
	if len(r.PipelineIDs) > 0 {
		line(fmt.Sprintf("  Pipelines    : %s", joinStrings(r.PipelineIDs)))
	}
	if len(r.CostByDVCStage) > 0 {
		line("")
		line("Cost by DVC Stage")
		line(sep)
		stages := make([]string, 0, len(r.CostByDVCStage))
		for s := range r.CostByDVCStage {
			stages = append(stages, s)
		}
		sort.Strings(stages)
		for _, s := range stages {
			line(fmt.Sprintf("  %-20s $%.4f", s, r.CostByDVCStage[s]))
		}
	}
	line("")
	line("Data Management Plan")
	line(sep)
	dmp := r.DataManagementPlan
	line(fmt.Sprintf("  Storage      : %s", dmp.StorageProvider))
	line(fmt.Sprintf("  Retention    : %s", dmp.RetentionPolicy))
	line(fmt.Sprintf("  Access ctrl  : %s", dmp.AccessControls))
	line(fmt.Sprintf("  Backup       : %s", dmp.BackupStrategy))
	line(fmt.Sprintf("  Data format  : %s", dmp.DataFormat))
	line(fmt.Sprintf("  Provenance   : %s", dmp.ReproducibilityNote))
	if dmp.ComplianceNotes != "" {
		line(fmt.Sprintf("  Notes        : %s", dmp.ComplianceNotes))
	}

	return string(b)
}

func joinStrings(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += ", " + s
	}
	return out
}
