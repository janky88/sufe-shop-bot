package auth

import (
	"fmt"
	"sync"
	"time"
)

// LoginAttempt represents a login attempt
type LoginAttempt struct {
	Count      int
	LastAttempt time.Time
	LockedUntil time.Time
}

// RateLimiterConfig holds rate limiter configuration
type RateLimiterConfig struct {
	MaxAttempts     int           // Maximum attempts before lockout
	LockoutDuration time.Duration // How long to lock after max attempts
	WindowDuration  time.Duration // Time window for counting attempts
	CleanupInterval time.Duration // How often to clean up old entries
}

// DefaultRateLimiterConfig returns default rate limiter configuration
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		MaxAttempts:     5,
		LockoutDuration: 15 * time.Minute,
		WindowDuration:  5 * time.Minute,
		CleanupInterval: 10 * time.Minute,
	}
}

// RateLimiter implements login attempt rate limiting
type RateLimiter struct {
	config    *RateLimiterConfig
	attempts  map[string]*LoginAttempt
	mu        sync.RWMutex
	stopClean chan bool
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config *RateLimiterConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}
	
	rl := &RateLimiter{
		config:    config,
		attempts:  make(map[string]*LoginAttempt),
		stopClean: make(chan bool),
	}
	
	// Start cleanup goroutine
	go rl.cleanupLoop()
	
	return rl
}

// CheckAttempt checks if an identifier can make an attempt
func (rl *RateLimiter) CheckAttempt(identifier string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	attempt, exists := rl.attempts[identifier]
	
	// Create new attempt record if doesn't exist
	if !exists {
		rl.attempts[identifier] = &LoginAttempt{
			Count:       0,
			LastAttempt: now,
		}
		return true, 0
	}
	
	// Check if currently locked out
	if !attempt.LockedUntil.IsZero() && now.Before(attempt.LockedUntil) {
		return false, attempt.LockedUntil.Sub(now)
	}
	
	// Reset count if outside window
	if now.Sub(attempt.LastAttempt) > rl.config.WindowDuration {
		attempt.Count = 0
		attempt.LockedUntil = time.Time{}
	}
	
	// Check if at limit
	if attempt.Count >= rl.config.MaxAttempts {
		attempt.LockedUntil = now.Add(rl.config.LockoutDuration)
		return false, rl.config.LockoutDuration
	}
	
	return true, 0
}

// RecordAttempt records a login attempt
func (rl *RateLimiter) RecordAttempt(identifier string, success bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	attempt, exists := rl.attempts[identifier]
	
	if !exists {
		attempt = &LoginAttempt{
			LastAttempt: now,
		}
		rl.attempts[identifier] = attempt
	}
	
	attempt.LastAttempt = now
	
	if success {
		// Reset on successful login
		attempt.Count = 0
		attempt.LockedUntil = time.Time{}
	} else {
		// Increment failed attempts
		attempt.Count++
		
		// Lock if exceeded max attempts
		if attempt.Count >= rl.config.MaxAttempts {
			attempt.LockedUntil = now.Add(rl.config.LockoutDuration)
		}
	}
}

// ResetAttempts resets attempts for an identifier
func (rl *RateLimiter) ResetAttempts(identifier string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	delete(rl.attempts, identifier)
}

// GetAttemptInfo returns information about current attempts
func (rl *RateLimiter) GetAttemptInfo(identifier string) (attempts int, lockedUntil time.Time, exists bool) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	attempt, exists := rl.attempts[identifier]
	if !exists {
		return 0, time.Time{}, false
	}
	
	return attempt.Count, attempt.LockedUntil, true
}

// cleanupLoop periodically removes old entries
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopClean:
			return
		}
	}
}

// cleanup removes old entries
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	for id, attempt := range rl.attempts {
		// Remove if:
		// 1. No recent attempts and not locked
		// 2. Lockout has expired
		if attempt.LockedUntil.IsZero() && now.Sub(attempt.LastAttempt) > rl.config.WindowDuration*2 {
			delete(rl.attempts, id)
		} else if !attempt.LockedUntil.IsZero() && now.After(attempt.LockedUntil.Add(rl.config.WindowDuration)) {
			delete(rl.attempts, id)
		}
	}
}

// Stop stops the rate limiter cleanup
func (rl *RateLimiter) Stop() {
	close(rl.stopClean)
}

// FormatLockoutMessage formats a user-friendly lockout message
func FormatLockoutMessage(remaining time.Duration) string {
	if remaining < time.Minute {
		return fmt.Sprintf("Too many failed attempts. Please try again in %d seconds.", int(remaining.Seconds()))
	}
	
	minutes := int(remaining.Minutes())
	if minutes == 1 {
		return "Too many failed attempts. Please try again in 1 minute."
	}
	
	return fmt.Sprintf("Too many failed attempts. Please try again in %d minutes.", minutes)
}