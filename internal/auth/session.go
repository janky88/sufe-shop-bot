package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"
	
	logger "shop-bot/internal/log"
)

var (
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session expired")
	ErrConcurrentLimit    = errors.New("concurrent session limit exceeded")
	ErrAnomalousActivity  = errors.New("anomalous activity detected")
)

// SessionInfo holds session information
type SessionInfo struct {
	ID          string
	UserID      string
	Username    string
	Role        string
	CreatedAt   time.Time
	LastAccess  time.Time
	ExpiresAt   time.Time
	IPAddress   string
	UserAgent   string
	IsActive    bool
}

// SessionConfig holds session configuration
type SessionConfig struct {
	MaxConcurrent      int           // Max concurrent sessions per user
	SessionTimeout     time.Duration // Session timeout
	IdleTimeout        time.Duration // Idle timeout
	EnableIPCheck      bool          // Enable IP address validation
	EnableUserAgentCheck bool        // Enable user agent validation
}

// DefaultSessionConfig returns default session configuration
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		MaxConcurrent:        3,
		SessionTimeout:       24 * time.Hour,
		IdleTimeout:          2 * time.Hour,
		EnableIPCheck:        true,
		EnableUserAgentCheck: true,
	}
}

// SessionManager manages user sessions
type SessionManager struct {
	config       *SessionConfig
	sessions     map[string]*SessionInfo     // sessionID -> session
	userSessions map[string]map[string]bool  // userID -> set of sessionIDs
	mu           sync.RWMutex
	stopClean    chan bool
}

// NewSessionManager creates a new session manager
func NewSessionManager(config *SessionConfig) *SessionManager {
	if config == nil {
		config = DefaultSessionConfig()
	}
	
	sm := &SessionManager{
		config:       config,
		sessions:     make(map[string]*SessionInfo),
		userSessions: make(map[string]map[string]bool),
		stopClean:    make(chan bool),
	}
	
	// Start cleanup goroutine
	go sm.cleanupLoop()
	
	return sm
}

// CreateSession creates a new session
func (sm *SessionManager) CreateSession(userID, username, role, ipAddress, userAgent string) (*SessionInfo, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	// Check concurrent session limit
	if sm.config.MaxConcurrent > 0 {
		userSessionIDs, exists := sm.userSessions[userID]
		if exists {
			activeCount := 0
			for sessionID := range userSessionIDs {
				if session, ok := sm.sessions[sessionID]; ok && session.IsActive {
					activeCount++
				}
			}
			
			if activeCount >= sm.config.MaxConcurrent {
				// Find and remove oldest session
				var oldestID string
				var oldestTime time.Time
				
				for sessionID := range userSessionIDs {
					if session, ok := sm.sessions[sessionID]; ok && session.IsActive {
						if oldestID == "" || session.CreatedAt.Before(oldestTime) {
							oldestID = sessionID
							oldestTime = session.CreatedAt
						}
					}
				}
				
				if oldestID != "" {
					sm.invalidateSessionUnsafe(oldestID)
					logger.Warn("Session removed due to concurrent limit", 
						"userID", userID, "removedSessionID", oldestID)
				}
			}
		}
	}
	
	// Generate session ID
	sessionID := generateSessionID()
	
	// Create session
	now := time.Now()
	session := &SessionInfo{
		ID:         sessionID,
		UserID:     userID,
		Username:   username,
		Role:       role,
		CreatedAt:  now,
		LastAccess: now,
		ExpiresAt:  now.Add(sm.config.SessionTimeout),
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		IsActive:   true,
	}
	
	// Store session
	sm.sessions[sessionID] = session
	
	// Update user sessions
	if sm.userSessions[userID] == nil {
		sm.userSessions[userID] = make(map[string]bool)
	}
	sm.userSessions[userID][sessionID] = true
	
	logger.Info("Session created", 
		"sessionID", sessionID, "userID", userID, "ip", ipAddress)
	
	return session, nil
}

// ValidateSession validates a session and returns session info
func (sm *SessionManager) ValidateSession(sessionID, ipAddress, userAgent string) (*SessionInfo, error) {
	sm.mu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()
	
	if !exists {
		return nil, ErrSessionNotFound
	}
	
	if !session.IsActive {
		return nil, ErrSessionExpired
	}
	
	now := time.Now()
	
	// Check if session expired
	if now.After(session.ExpiresAt) {
		sm.InvalidateSession(sessionID)
		return nil, ErrSessionExpired
	}
	
	// Check idle timeout
	if sm.config.IdleTimeout > 0 && now.Sub(session.LastAccess) > sm.config.IdleTimeout {
		sm.InvalidateSession(sessionID)
		logger.Info("Session expired due to inactivity", 
			"sessionID", sessionID, "userID", session.UserID)
		return nil, ErrSessionExpired
	}
	
	// Check for anomalous activity
	if sm.config.EnableIPCheck && session.IPAddress != ipAddress {
		logger.Warn("Session IP mismatch detected", 
			"sessionID", sessionID, "expectedIP", session.IPAddress, "actualIP", ipAddress)
		// You might want to invalidate session or just log warning
		// For now, we'll just log and continue
	}
	
	if sm.config.EnableUserAgentCheck && session.UserAgent != userAgent {
		logger.Warn("Session UserAgent mismatch detected", 
			"sessionID", sessionID, "expectedUA", session.UserAgent, "actualUA", userAgent)
		// You might want to invalidate session or just log warning
		// For now, we'll just log and continue
	}
	
	// Update last access time
	sm.mu.Lock()
	session.LastAccess = now
	sm.mu.Unlock()
	
	return session, nil
}

// InvalidateSession invalidates a session
func (sm *SessionManager) InvalidateSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	sm.invalidateSessionUnsafe(sessionID)
}

// invalidateSessionUnsafe invalidates a session without locking (must be called with lock held)
func (sm *SessionManager) invalidateSessionUnsafe(sessionID string) {
	session, exists := sm.sessions[sessionID]
	if !exists {
		return
	}
	
	session.IsActive = false
	
	// Remove from user sessions
	if userSessions, ok := sm.userSessions[session.UserID]; ok {
		delete(userSessions, sessionID)
		if len(userSessions) == 0 {
			delete(sm.userSessions, session.UserID)
		}
	}
	
	// Remove session
	delete(sm.sessions, sessionID)
	
	logger.Info("Session invalidated", 
		"sessionID", sessionID, "userID", session.UserID)
}

// InvalidateUserSessions invalidates all sessions for a user
func (sm *SessionManager) InvalidateUserSessions(userID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	userSessionIDs, exists := sm.userSessions[userID]
	if !exists {
		return
	}
	
	for sessionID := range userSessionIDs {
		sm.invalidateSessionUnsafe(sessionID)
	}
	
	logger.Info("All user sessions invalidated", "userID", userID)
}

// GetUserSessions returns all active sessions for a user
func (sm *SessionManager) GetUserSessions(userID string) []*SessionInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	var sessions []*SessionInfo
	
	userSessionIDs, exists := sm.userSessions[userID]
	if !exists {
		return sessions
	}
	
	for sessionID := range userSessionIDs {
		if session, ok := sm.sessions[sessionID]; ok && session.IsActive {
			// Create copy to avoid data races
			sessionCopy := *session
			sessions = append(sessions, &sessionCopy)
		}
	}
	
	return sessions
}

// GetActiveSessionCount returns the number of active sessions
func (sm *SessionManager) GetActiveSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	count := 0
	for _, session := range sm.sessions {
		if session.IsActive {
			count++
		}
	}
	
	return count
}

// cleanupLoop periodically removes expired sessions
func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			sm.cleanup()
		case <-sm.stopClean:
			return
		}
	}
}

// cleanup removes expired sessions
func (sm *SessionManager) cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	now := time.Now()
	var toRemove []string
	
	for sessionID, session := range sm.sessions {
		if !session.IsActive || now.After(session.ExpiresAt) ||
			(sm.config.IdleTimeout > 0 && now.Sub(session.LastAccess) > sm.config.IdleTimeout) {
			toRemove = append(toRemove, sessionID)
		}
	}
	
	for _, sessionID := range toRemove {
		sm.invalidateSessionUnsafe(sessionID)
	}
	
	if len(toRemove) > 0 {
		logger.Debug("Session cleanup completed", "removed", len(toRemove))
	}
}

// Stop stops the session manager cleanup
func (sm *SessionManager) Stop() {
	close(sm.stopClean)
}

// generateSessionID generates a secure session ID
func generateSessionID() string {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%d-%s", time.Now().UnixNano(), base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano()))))
	}
	return base64.URLEncoding.EncodeToString(b)
}