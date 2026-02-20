package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/scttfrdmn/cargoship/pkg/launch"
)

var (
	port     = flag.Int("port", 8080, "Server port")
	logLevel = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

// Server manages the launch API server
type Server struct {
	logger *slog.Logger
	jobs   map[string]*JobStatus
	jobsMu sync.RWMutex
}

// JobStatus tracks the status of a running job
type JobStatus struct {
	JobID       string              `json:"job_id"`
	Status      string              `json:"status"` // pending, running, completed, failed
	Command     []string            `json:"command"`
	Environment map[string]string   `json:"environment"`
	StartTime   time.Time           `json:"start_time"`
	EndTime     *time.Time          `json:"end_time,omitempty"`
	Duration    *time.Duration      `json:"duration,omitempty"`
	Results     *launch.TestResults `json:"results,omitempty"`
	Logs        []string            `json:"logs,omitempty"`
	Error       string              `json:"error,omitempty"`
	Process     *exec.Cmd           `json:"-"`
	ctx         context.Context     `json:"-"`
	cancel      context.CancelFunc  `json:"-"`
}

// JobSubmission represents a job submission request
type JobSubmission struct {
	Image       string                 `json:"image"`
	Command     []string               `json:"command"`
	Environment map[string]string      `json:"environment"`
	Volumes     []map[string]string    `json:"volumes"`
	Resources   map[string]interface{} `json:"resources"`
	NetworkMode string                 `json:"network_mode"`
	Timeout     float64                `json:"timeout"`
}

func main() {
	flag.Parse()

	// Setup logging
	var logLevelVar slog.Level
	switch *logLevel {
	case "debug":
		logLevelVar = slog.LevelDebug
	case "info":
		logLevelVar = slog.LevelInfo
	case "warn":
		logLevelVar = slog.LevelWarn
	case "error":
		logLevelVar = slog.LevelError
	default:
		logLevelVar = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevelVar,
	}))

	server := &Server{
		logger: logger,
		jobs:   make(map[string]*JobStatus),
	}

	// Setup routes
	r := mux.NewRouter()
	r.HandleFunc("/health", server.healthHandler).Methods("GET")
	r.HandleFunc("/api/v1/launch", server.launchHandler).Methods("POST")
	r.HandleFunc("/api/v1/jobs/{jobId}", server.jobStatusHandler).Methods("GET")
	r.HandleFunc("/api/v1/jobs/{jobId}/logs", server.jobLogsHandler).Methods("GET")
	r.HandleFunc("/api/v1/jobs/{jobId}/cancel", server.cancelJobHandler).Methods("POST")
	r.HandleFunc("/api/v1/jobs", server.listJobsHandler).Methods("GET")

	// Add CORS middleware
	r.Use(corsMiddleware)

	addr := fmt.Sprintf(":%d", *port)
	logger.Info("starting launch server", "address", addr)

	srv := &http.Server{
		Handler:      r,
		Addr:         addr,
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
	}); err != nil {
		s.logger.Error("failed to encode health response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) launchHandler(w http.ResponseWriter, r *http.Request) {
	var submission JobSubmission
	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Generate job ID
	jobID := fmt.Sprintf("job-%d", time.Now().UnixNano())

	// Create job context with timeout
	timeout := time.Duration(submission.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute // Default timeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Create job status
	job := &JobStatus{
		JobID:       jobID,
		Status:      "pending",
		Command:     submission.Command,
		Environment: submission.Environment,
		StartTime:   time.Now(),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Store job
	s.jobsMu.Lock()
	s.jobs[jobID] = job
	s.jobsMu.Unlock()

	s.logger.Info("job submitted", "job_id", jobID, "command", submission.Command)

	// Start job execution
	go s.executeJob(job)

	// Return job ID
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"job_id": jobID,
	}); err != nil {
		s.logger.Error("failed to encode launch response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) jobStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["jobId"]

	s.jobsMu.RLock()
	job, exists := s.jobs[jobID]
	s.jobsMu.RUnlock()

	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// Create response (exclude internal fields)
	response := &launch.LaunchResponse{
		JobID:     job.JobID,
		Status:    job.Status,
		Results:   job.Results,
		Logs:      job.Logs,
		Error:     job.Error,
		StartTime: job.StartTime,
		EndTime:   job.EndTime,
		Duration:  job.Duration,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("failed to encode job status response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) jobLogsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["jobId"]

	s.jobsMu.RLock()
	job, exists := s.jobs[jobID]
	logs := make([]string, len(job.Logs))
	copy(logs, job.Logs)
	s.jobsMu.RUnlock()

	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string][]string{
		"logs": logs,
	}); err != nil {
		s.logger.Error("failed to encode logs response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) cancelJobHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["jobId"]

	s.jobsMu.Lock()
	job, exists := s.jobs[jobID]
	s.jobsMu.Unlock()

	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if job.Status == "running" && job.cancel != nil {
		job.cancel()
		s.logger.Info("job cancelled", "job_id", jobID)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "cancelled",
	}); err != nil {
		s.logger.Error("failed to encode cancellation response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) listJobsHandler(w http.ResponseWriter, r *http.Request) {
	s.jobsMu.RLock()
	jobs := make([]*launch.LaunchResponse, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, &launch.LaunchResponse{
			JobID:     job.JobID,
			Status:    job.Status,
			StartTime: job.StartTime,
			EndTime:   job.EndTime,
			Duration:  job.Duration,
		})
	}
	s.jobsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"jobs": jobs,
	}); err != nil {
		s.logger.Error("failed to encode jobs list response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) executeJob(job *JobStatus) {
	s.logger.Info("starting job execution", "job_id", job.JobID)

	// Update job status
	s.jobsMu.Lock()
	job.Status = "running"
	s.jobsMu.Unlock()

	// Execute the command
	cmd := exec.CommandContext(job.ctx, job.Command[0], job.Command[1:]...)

	// Set environment variables.
	// Job-supplied vars are filtered through a denylist to prevent callers
	// from hijacking dynamic linker or credential env vars (CWE-454).
	cmd.Env = os.Environ()
	for key, value := range job.Environment {
		if isSafeEnvKey(key) {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		} else {
			s.logger.Warn("Rejected unsafe environment variable in job", "job_id", job.JobID, "key", key)
		}
	}

	// Capture output
	output, err := cmd.CombinedOutput()

	// Calculate duration
	endTime := time.Now()
	duration := endTime.Sub(job.StartTime)

	s.jobsMu.Lock()
	job.EndTime = &endTime
	job.Duration = &duration

	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		job.Logs = append(job.Logs, string(output))
		s.logger.Error("job failed", "job_id", job.JobID, "error", err)
	} else {
		job.Status = "completed"
		job.Logs = append(job.Logs, string(output))

		// Try to parse results from output (assuming JSON output)
		var results launch.TestResults
		if err := json.Unmarshal(output, &results); err == nil {
			job.Results = &results
		}

		s.logger.Info("job completed", "job_id", job.JobID, "duration", duration)
	}
	s.jobsMu.Unlock()

	// Cancel context
	job.cancel()
}

// isSafeEnvKey returns true if the environment variable name is safe to pass
// to a subprocess. Rejects names that are empty, contain "=", or match
// security-sensitive prefixes/names (dynamic linker, credential vars).
func isSafeEnvKey(key string) bool {
	if key == "" || strings.ContainsRune(key, '=') {
		return false
	}
	// Denylist: dynamic linker hijack vectors and credential vars
	denied := []string{
		"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES",
		"DYLD_LIBRARY_PATH", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_SECURITY_TOKEN",
	}
	upper := strings.ToUpper(key)
	for _, d := range denied {
		if upper == d {
			return false
		}
	}
	return true
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
