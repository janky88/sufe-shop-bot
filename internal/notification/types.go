package notification

import (
	"time"
)

// Priority represents notification priority
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

// Notification represents a notification to be sent
type Notification struct {
	ID        string                 `json:"id"`
	Type      EventType              `json:"type"`
	Priority  Priority               `json:"priority"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt time.Time              `json:"created_at"`
	Retries   int                    `json:"retries"`
	LastError string                 `json:"last_error,omitempty"`
}

// Channel represents a notification channel
type Channel interface {
	Send(notification *Notification) error
	Name() string
	IsEnabled() bool
}

// Queue represents a notification queue
type Queue interface {
	Push(notification *Notification) error
	Process()
	Stop()
}

// Config represents notification configuration
type NotificationConfig struct {
	Enabled         bool
	MaxRetries      int
	RetryDelay      time.Duration
	RateLimit       int           // notifications per minute
	RateLimitWindow time.Duration // time window for rate limiting
	AdminChatIDs    []int64
}

// Result represents the result of sending a notification
type Result struct {
	Success   bool
	Error     error
	Timestamp time.Time
	Channel   string
}