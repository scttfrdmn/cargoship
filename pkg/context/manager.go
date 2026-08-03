// Package context provides execution context management for CargoShip CLI
package context

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	homedir "github.com/mitchellh/go-homedir"
)

// ExecutionContext represents the current execution environment
type ExecutionContext string

const (
	ContextLocal ExecutionContext = "local" // Local filesystem operations
	ContextAgent ExecutionContext = "agent" // Launch agent environment
	ContextREPL  ExecutionContext = "repl"  // Interactive shell mode
)

// ContextInfo holds context state information
type ContextInfo struct {
	Current       ExecutionContext `json:"current"`
	LastUsed      time.Time        `json:"last_used"`
	Version       string           `json:"version"`
	AgentEndpoint string           `json:"agent_endpoint,omitempty"`
	WorkingDir    string           `json:"working_dir,omitempty"`
}

// Manager handles context persistence and switching
type Manager struct {
	contextFile string
	current     *ContextInfo
	logger      *slog.Logger
}

// NewManager creates a new context manager
func NewManager(logger *slog.Logger) *Manager {
	home, err := homedir.Dir()
	if err != nil {
		// Fallback to current directory if home detection fails
		home = "."
	}

	contextFile := filepath.Join(home, ".cargoship-context")

	return &Manager{
		contextFile: contextFile,
		logger:      logger.With("component", "context-manager"),
	}
}

// Load reads the current context from the dot file
func (m *Manager) Load() (*ContextInfo, error) {
	// If already loaded, return cached version
	if m.current != nil {
		return m.current, nil
	}

	// Check if context file exists
	if _, err := os.Stat(m.contextFile); os.IsNotExist(err) {
		// First run - create default context
		m.logger.Info("First run detected, creating default context", "context", ContextLocal)
		return m.createDefaultContext()
	}

	// Read and parse context file
	data, err := os.ReadFile(m.contextFile)
	if err != nil {
		m.logger.Warn("Failed to read context file, using default", "error", err)
		return m.createDefaultContext()
	}

	var ctx ContextInfo
	if err := json.Unmarshal(data, &ctx); err != nil {
		m.logger.Warn("Failed to parse context file, recreating", "error", err)
		return m.createDefaultContext()
	}

	// Validate context
	if !isValidContext(ctx.Current) {
		m.logger.Warn("Invalid context in file, resetting to local", "context", ctx.Current)
		ctx.Current = ContextLocal
	}

	// Update last used time
	ctx.LastUsed = time.Now()

	m.current = &ctx
	m.logger.Debug("Loaded context from file", "context", ctx.Current, "file", m.contextFile)

	return m.current, nil
}

// Save persists the current context to the dot file
func (m *Manager) Save() error {
	if m.current == nil {
		return fmt.Errorf("no context to save")
	}

	// Update metadata
	m.current.LastUsed = time.Now()
	m.current.Version = "0.3.0" // Could be imported from version

	// Marshal to JSON with pretty formatting
	data, err := json.MarshalIndent(m.current, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal context: %w", err)
	}

	// Write to file with appropriate permissions
	if err := os.WriteFile(m.contextFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write context file: %w", err)
	}

	m.logger.Debug("Saved context to file", "context", m.current.Current, "file", m.contextFile)
	return nil
}

// SwitchTo changes the current context and persists it
func (m *Manager) SwitchTo(ctx ExecutionContext) error {
	if !isValidContext(ctx) {
		return fmt.Errorf("invalid context: %s", ctx)
	}

	// Load current context if not loaded
	if m.current == nil {
		if _, err := m.Load(); err != nil {
			return fmt.Errorf("failed to load current context: %w", err)
		}
	}

	previousContext := m.current.Current
	m.current.Current = ctx

	// Update working directory if switching to local
	if ctx == ContextLocal {
		if wd, err := os.Getwd(); err == nil {
			m.current.WorkingDir = wd
		}
	}

	// Save the updated context
	if err := m.Save(); err != nil {
		// Rollback on save failure
		m.current.Current = previousContext
		return fmt.Errorf("failed to save context switch: %w", err)
	}

	m.logger.Info("Context switched", "from", previousContext, "to", ctx)
	return nil
}

// Current returns the current execution context
func (m *Manager) Current() ExecutionContext {
	ctx, err := m.Load()
	if err != nil {
		m.logger.Warn("Failed to load context, defaulting to local", "error", err)
		return ContextLocal
	}
	return ctx.Current
}

// SetEndpoint updates the connection endpoint for the agent context
func (m *Manager) SetEndpoint(endpoint string) error {
	if m.current == nil {
		if _, err := m.Load(); err != nil {
			return err
		}
	}

	switch m.current.Current {
	case ContextAgent:
		m.current.AgentEndpoint = endpoint
	default:
		return fmt.Errorf("endpoints not applicable for context: %s", m.current.Current)
	}

	return m.Save()
}

// GetEndpoint returns the current endpoint for the agent context
func (m *Manager) GetEndpoint() string {
	if m.current == nil {
		return ""
	}

	switch m.current.Current {
	case ContextAgent:
		return m.current.AgentEndpoint
	default:
		return ""
	}
}

// IsFirstRun checks if this is the first time CargoShip is being run
func (m *Manager) IsFirstRun() bool {
	_, err := os.Stat(m.contextFile)
	return os.IsNotExist(err)
}

// Reset removes the context file and resets to default
func (m *Manager) Reset() error {
	if err := os.Remove(m.contextFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove context file: %w", err)
	}

	m.current = nil
	m.logger.Info("Context reset to default")
	return nil
}

// GetContextFile returns the path to the context file
func (m *Manager) GetContextFile() string {
	return m.contextFile
}

// createDefaultContext creates the initial context for first-time users
func (m *Manager) createDefaultContext() (*ContextInfo, error) {
	wd, _ := os.Getwd() // Ignore error, will be empty string

	ctx := &ContextInfo{
		Current:    ContextLocal,
		LastUsed:   time.Now(),
		Version:    "0.3.0",
		WorkingDir: wd,
	}

	m.current = ctx

	// Save the default context
	if err := m.Save(); err != nil {
		m.logger.Warn("Failed to save default context", "error", err)
		// Continue anyway, context will work in memory
	}

	return ctx, nil
}

// isValidContext validates if a context string is valid
func isValidContext(ctx ExecutionContext) bool {
	switch ctx {
	case ContextLocal, ContextAgent, ContextREPL:
		return true
	default:
		return false
	}
}

// GetAvailableContexts returns all valid execution contexts
func GetAvailableContexts() []ExecutionContext {
	return []ExecutionContext{
		ContextLocal,
		ContextAgent,
		ContextREPL,
	}
}

// FormatContext returns a human-readable description of a context
func FormatContext(ctx ExecutionContext) string {
	switch ctx {
	case ContextLocal:
		return "Local filesystem operations and archive creation"
	case ContextAgent:
		return "Launch agent monitoring and management"
	case ContextREPL:
		return "Interactive shell mode with command discovery"
	default:
		return "Unknown context"
	}
}
