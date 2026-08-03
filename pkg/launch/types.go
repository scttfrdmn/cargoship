// Package launch provides autonomous archival for CargoShip deployed on a
// remote host such as a NAS.
//
// A ghost ship polls its configured watch paths on an interval, matches each
// file against archival rules, and uploads matches to S3 on its own. It accepts
// no inbound connections and requires nothing else to be running: the
// distributed controller this package once talked to was removed in #340, and
// the parallel agent implementation that duplicated GhostShip in #347.
package launch

import "time"

// WatchPath defines a directory a ghost ship monitors for archival candidates.
type WatchPath struct {
	Path            string        `json:"path" yaml:"path"`
	IncludePatterns []string      `json:"include_patterns" yaml:"include_patterns"`
	ExcludePatterns []string      `json:"exclude_patterns" yaml:"exclude_patterns"`
	MinAge          time.Duration `json:"min_age" yaml:"min_age"`
	StorageClass    string        `json:"storage_class" yaml:"storage_class"`
	Recursive       bool          `json:"recursive" yaml:"recursive"`
}

// JobState represents the state of an archival job.
type JobState string

const (
	JobStatePending   JobState = "pending"
	JobStateRunning   JobState = "running"
	JobStateCompleted JobState = "completed"
	JobStateFailed    JobState = "failed"
	JobStateCancelled JobState = "cancelled"
)

// TestResults contains the results of a throughput test run.
//
// This and NetworkUtilization are consumed by cmd/cargoship-test, which builds
// them directly. They were declared alongside the astrapi launcher deleted in
// #347; only the types were ever reachable.
type TestResults struct {
	TestType              string              `json:"test_type"`
	Success               bool                `json:"success"`
	TotalFiles            int                 `json:"total_files"`
	ProcessedFiles        int                 `json:"processed_files"`
	TotalBytes            int64               `json:"total_bytes"`
	ProcessedBytes        int64               `json:"processed_bytes"`
	Duration              time.Duration       `json:"duration"`
	AverageThroughputMBps float64             `json:"average_throughput_mbps"`
	PeakThroughputMBps    float64             `json:"peak_throughput_mbps"`
	OptimizationStats     interface{}         `json:"optimization_stats,omitempty"`
	NetworkUtilization    *NetworkUtilization `json:"network_utilization,omitempty"`
	ErrorCount            int                 `json:"error_count"`
	Errors                []string            `json:"errors,omitempty"`
}

// NetworkUtilization provides detailed network performance metrics.
type NetworkUtilization struct {
	LocalNetworkMbps   float64 `json:"local_network_mbps"`  // local network (10Gbps)
	InternetMbps       float64 `json:"internet_mbps"`       // Internet to AWS (5Gbps)
	LocalEfficiency    float64 `json:"local_efficiency"`    // % of 10Gbps utilized
	InternetEfficiency float64 `json:"internet_efficiency"` // % of 5Gbps utilized
	OptimalPathUsed    bool    `json:"optimal_path_used"`   // whether the optimal path was used
}
