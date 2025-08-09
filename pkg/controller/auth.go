package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AuthManager manages agent authentication and authorization
type AuthManager struct {
	// JWT configuration
	jwtSecret []byte
	jwtIssuer string
	jwtExpiry time.Duration

	// RSA keys for signing
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey

	// Agent registry for role checking
	agents map[string]*RegisteredAgent
	roles  map[string]*AgentRole
	logger *slog.Logger
	mu     sync.RWMutex
}

// RegisteredAgent represents a registered agent with authentication details
type RegisteredAgent struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	PublicKey    string            `json:"public_key"`
	Role         string            `json:"role"`
	Capabilities []string          `json:"capabilities"`
	Metadata     map[string]string `json:"metadata"`
	RegisteredAt time.Time         `json:"registered_at"`
	LastAuth     time.Time         `json:"last_auth"`
	Enabled      bool              `json:"enabled"`
}

// AgentRole defines permissions and capabilities for agent roles
type AgentRole struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Permissions  []Permission           `json:"permissions"`
	Capabilities map[string]interface{} `json:"capabilities"`
	MaxSessions  int                    `json:"max_sessions"`
	SessionTTL   time.Duration          `json:"session_ttl"`
}

// Permission represents a specific permission for agents
type Permission string

const (
	PermissionRead          Permission = "read"
	PermissionWrite         Permission = "write"
	PermissionExecute       Permission = "execute"
	PermissionArchive       Permission = "archive"
	PermissionRestore       Permission = "restore"
	PermissionConfig        Permission = "config"
	PermissionMetrics       Permission = "metrics"
	PermissionAdministrator Permission = "administrator"
)

// Claims represents JWT claims for agent authentication
type Claims struct {
	AgentID     string   `json:"agent_id"`
	AgentName   string   `json:"agent_name"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	SessionID   string   `json:"session_id"`
	jwt.RegisteredClaims
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret           string        `json:"jwt_secret" yaml:"jwt_secret"`
	JWTIssuer           string        `json:"jwt_issuer" yaml:"jwt_issuer"`
	JWTExpiry           time.Duration `json:"jwt_expiry" yaml:"jwt_expiry"`
	EnableRSASigning    bool          `json:"enable_rsa_signing" yaml:"enable_rsa_signing"`
	RequireClientCert   bool          `json:"require_client_cert" yaml:"require_client_cert"`
	MaxSessionsPerAgent int           `json:"max_sessions_per_agent" yaml:"max_sessions_per_agent"`
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(config *AuthConfig, logger *slog.Logger) (*AuthManager, error) {
	if config == nil {
		config = &AuthConfig{
			JWTIssuer:           "cargoship-controller",
			JWTExpiry:           24 * time.Hour,
			EnableRSASigning:    true,
			MaxSessionsPerAgent: 3,
		}
	}

	// Generate JWT secret if not provided
	jwtSecret := []byte(config.JWTSecret)
	if len(jwtSecret) == 0 {
		jwtSecret = make([]byte, 32)
		if _, err := rand.Read(jwtSecret); err != nil {
			return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		logger.Info("Generated random JWT secret for session")
	}

	am := &AuthManager{
		jwtSecret: jwtSecret,
		jwtIssuer: config.JWTIssuer,
		jwtExpiry: config.JWTExpiry,
		agents:    make(map[string]*RegisteredAgent),
		roles:     make(map[string]*AgentRole),
		logger:    logger.With("component", "auth-manager"),
	}

	// Generate RSA keys if enabled
	if config.EnableRSASigning {
		if err := am.generateRSAKeys(); err != nil {
			return nil, fmt.Errorf("failed to generate RSA keys: %w", err)
		}
	}

	// Initialize default roles
	am.initializeDefaultRoles()

	logger.Info("Authentication manager initialized",
		"jwt_issuer", config.JWTIssuer,
		"jwt_expiry", config.JWTExpiry,
		"rsa_signing", config.EnableRSASigning)

	return am, nil
}

// generateRSAKeys generates RSA key pair for JWT signing
func (am *AuthManager) generateRSAKeys() error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate RSA private key: %w", err)
	}

	am.privateKey = privateKey
	am.publicKey = &privateKey.PublicKey

	am.logger.Info("Generated RSA key pair for JWT signing")
	return nil
}

// initializeDefaultRoles creates default agent roles
func (am *AuthManager) initializeDefaultRoles() {
	// Agent role - basic archival operations
	agentRole := &AgentRole{
		Name:        "agent",
		Description: "Standard launch agent with archival capabilities",
		Permissions: []Permission{
			PermissionRead,
			PermissionWrite,
			PermissionArchive,
			PermissionMetrics,
		},
		Capabilities: map[string]interface{}{
			"max_concurrent_jobs": 5,
			"max_file_size":       "10GB",
			"allowed_operations":  []string{"archive", "compress", "upload"},
		},
		MaxSessions: 3,
		SessionTTL:  24 * time.Hour,
	}

	// Admin role - full access
	adminRole := &AgentRole{
		Name:        "admin",
		Description: "Administrator agent with full access",
		Permissions: []Permission{
			PermissionRead,
			PermissionWrite,
			PermissionExecute,
			PermissionArchive,
			PermissionRestore,
			PermissionConfig,
			PermissionMetrics,
			PermissionAdministrator,
		},
		Capabilities: map[string]interface{}{
			"max_concurrent_jobs": 20,
			"max_file_size":       "unlimited",
			"allowed_operations":  []string{"*"},
		},
		MaxSessions: 10,
		SessionTTL:  7 * 24 * time.Hour,
	}

	// Read-only role - monitoring and metrics
	readOnlyRole := &AgentRole{
		Name:        "readonly",
		Description: "Read-only access for monitoring agents",
		Permissions: []Permission{
			PermissionRead,
			PermissionMetrics,
		},
		Capabilities: map[string]interface{}{
			"max_concurrent_jobs": 0,
			"allowed_operations":  []string{"status", "metrics"},
		},
		MaxSessions: 5,
		SessionTTL:  12 * time.Hour,
	}

	am.roles["agent"] = agentRole
	am.roles["admin"] = adminRole
	am.roles["readonly"] = readOnlyRole

	am.logger.Info("Initialized default agent roles", "roles", []string{"agent", "admin", "readonly"})
}

// RegisterAgent registers a new agent with the authentication system
func (am *AuthManager) RegisterAgent(id, name, publicKey, role string, capabilities []string, metadata map[string]string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Validate role exists
	if _, exists := am.roles[role]; !exists {
		return fmt.Errorf("role '%s' does not exist", role)
	}

	// Check if agent already exists
	if _, exists := am.agents[id]; exists {
		return fmt.Errorf("agent with ID '%s' already registered", id)
	}

	// Validate public key format
	if publicKey != "" {
		if err := am.validatePublicKey(publicKey); err != nil {
			return fmt.Errorf("invalid public key: %w", err)
		}
	}

	agent := &RegisteredAgent{
		ID:           id,
		Name:         name,
		PublicKey:    publicKey,
		Role:         role,
		Capabilities: capabilities,
		Metadata:     metadata,
		RegisteredAt: time.Now(),
		Enabled:      true,
	}

	am.agents[id] = agent

	am.logger.Info("Agent registered successfully",
		"agent_id", id,
		"agent_name", name,
		"role", role,
		"capabilities", capabilities)

	return nil
}

// AuthenticateAgent authenticates an agent and returns a JWT token
func (am *AuthManager) AuthenticateAgent(agentID, signature string) (string, error) {
	am.mu.RLock()
	agent, exists := am.agents[agentID]
	am.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("agent not found: %s", agentID)
	}

	if !agent.Enabled {
		return "", fmt.Errorf("agent is disabled: %s", agentID)
	}

	// Verify signature if public key is provided
	if agent.PublicKey != "" {
		if err := am.verifySignature(agentID, signature, agent.PublicKey); err != nil {
			return "", fmt.Errorf("signature verification failed: %w", err)
		}
	}

	// Get role permissions
	role, exists := am.roles[agent.Role]
	if !exists {
		return "", fmt.Errorf("role not found: %s", agent.Role)
	}

	// Generate session ID
	sessionID := am.generateSessionID()

	// Create JWT claims
	claims := &Claims{
		AgentID:     agentID,
		AgentName:   agent.Name,
		Role:        agent.Role,
		Permissions: am.permissionsToStrings(role.Permissions),
		SessionID:   sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    am.jwtIssuer,
			Subject:   agentID,
			Audience:  []string{"cargoship"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(role.SessionTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	// Create and sign token
	var token string
	var err error

	if am.privateKey != nil {
		// Use RSA signing
		jwtToken := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token, err = jwtToken.SignedString(am.privateKey)
	} else {
		// Use HMAC signing
		jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		token, err = jwtToken.SignedString(am.jwtSecret)
	}

	if err != nil {
		return "", fmt.Errorf("failed to sign JWT token: %w", err)
	}

	// Update last auth time
	am.mu.Lock()
	agent.LastAuth = time.Now()
	am.mu.Unlock()

	am.logger.Info("Agent authenticated successfully",
		"agent_id", agentID,
		"session_id", sessionID,
		"role", agent.Role)

	return token, nil
}

// ValidateToken validates a JWT token and returns the claims
func (am *AuthManager) ValidateToken(tokenString string) (*Claims, error) {
	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		switch token.Method.(type) {
		case *jwt.SigningMethodRSA:
			if am.publicKey == nil {
				return nil, fmt.Errorf("RSA public key not available")
			}
			return am.publicKey, nil
		case *jwt.SigningMethodHMAC:
			return am.jwtSecret, nil
		default:
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Validate token and extract claims
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		// Check if agent still exists and is enabled
		am.mu.RLock()
		agent, exists := am.agents[claims.AgentID]
		am.mu.RUnlock()

		if !exists {
			return nil, fmt.Errorf("agent no longer exists: %s", claims.AgentID)
		}

		if !agent.Enabled {
			return nil, fmt.Errorf("agent is disabled: %s", claims.AgentID)
		}

		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// HasPermission checks if an agent has a specific permission
func (am *AuthManager) HasPermission(claims *Claims, permission Permission) bool {
	am.mu.RLock()
	role, exists := am.roles[claims.Role]
	am.mu.RUnlock()

	if !exists {
		return false
	}

	for _, p := range role.Permissions {
		if p == permission || p == PermissionAdministrator {
			return true
		}
	}

	return false
}

// GetAgentCapabilities returns the capabilities for an agent's role
func (am *AuthManager) GetAgentCapabilities(claims *Claims) map[string]interface{} {
	am.mu.RLock()
	role, exists := am.roles[claims.Role]
	am.mu.RUnlock()

	if !exists {
		return map[string]interface{}{}
	}

	return role.Capabilities
}

// Helper methods

func (am *AuthManager) validatePublicKey(publicKey string) error {
	// Decode base64 public key
	keyBytes, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return fmt.Errorf("invalid base64 encoding: %w", err)
	}

	// Parse PEM block
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return fmt.Errorf("invalid PEM format")
	}

	// Parse public key
	_, err = x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}

	return nil
}

func (am *AuthManager) verifySignature(agentID, signature, publicKey string) error {
	// TODO: Implement actual signature verification
	// For now, just check signature is not empty
	if signature == "" {
		return fmt.Errorf("signature required")
	}
	return nil
}

func (am *AuthManager) generateSessionID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes) // Ignore error - rand.Read always succeeds for slice
	return base64.URLEncoding.EncodeToString(bytes)
}

func (am *AuthManager) permissionsToStrings(permissions []Permission) []string {
	result := make([]string, len(permissions))
	for i, p := range permissions {
		result[i] = string(p)
	}
	return result
}

// GetRegisteredAgents returns all registered agents
func (am *AuthManager) GetRegisteredAgents() map[string]*RegisteredAgent {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make(map[string]*RegisteredAgent)
	for k, v := range am.agents {
		result[k] = v
	}
	return result
}

// GetRoles returns all available roles
func (am *AuthManager) GetRoles() map[string]*AgentRole {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make(map[string]*AgentRole)
	for k, v := range am.roles {
		result[k] = v
	}
	return result
}
