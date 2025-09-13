package security

import (
	"fmt"
	"strings"
	"time"
	
	logger "shop-bot/internal/log"
)

// EventType represents security event types
type EventType string

const (
	EventLogin           EventType = "login"
	EventLoginFailed     EventType = "login_failed"
	EventLogout          EventType = "logout"
	EventSessionCreated  EventType = "session_created"
	EventSessionExpired  EventType = "session_expired"
	EventRateLimited     EventType = "rate_limited"
	EventSuspiciousIP    EventType = "suspicious_ip"
	EventPasswordChanged EventType = "password_changed"
	EventAccessDenied    EventType = "access_denied"
	EventDataAccess      EventType = "data_access"
	EventDataModified    EventType = "data_modified"
	EventSecurityAlert   EventType = "security_alert"
)

// SecurityEvent represents a security-related event
type SecurityEvent struct {
	Type        EventType
	UserID      string
	Username    string
	IPAddress   string
	UserAgent   string
	Resource    string
	Action      string
	Result      string
	Details     map[string]interface{}
	Timestamp   time.Time
}

// SecurityLogger handles security event logging
type SecurityLogger struct {
	enableDetailedLogging bool
	maskSensitiveData     bool
}

// NewSecurityLogger creates a new security logger
func NewSecurityLogger(enableDetailed, maskSensitive bool) *SecurityLogger {
	return &SecurityLogger{
		enableDetailedLogging: enableDetailed,
		maskSensitiveData:     maskSensitive,
	}
}

// LogEvent logs a security event
func (sl *SecurityLogger) LogEvent(event SecurityEvent) {
	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	// Build log fields
	fields := []interface{}{
		"event_type", event.Type,
		"timestamp", event.Timestamp.Format(time.RFC3339),
	}
	
	// Add user info if available
	if event.UserID != "" {
		fields = append(fields, "user_id", event.UserID)
	}
	if event.Username != "" {
		username := event.Username
		if sl.maskSensitiveData {
			username = MaskSensitiveData(username, 3)
		}
		fields = append(fields, "username", username)
	}
	
	// Add request info
	if event.IPAddress != "" {
		fields = append(fields, "ip_address", event.IPAddress)
	}
	if event.UserAgent != "" {
		fields = append(fields, "user_agent", event.UserAgent)
	}
	
	// Add action details
	if event.Resource != "" {
		fields = append(fields, "resource", event.Resource)
	}
	if event.Action != "" {
		fields = append(fields, "action", event.Action)
	}
	if event.Result != "" {
		fields = append(fields, "result", event.Result)
	}
	
	// Add additional details if enabled
	if sl.enableDetailedLogging && event.Details != nil {
		for key, value := range event.Details {
			// Mask sensitive fields
			if sl.maskSensitiveData && isSensitiveField(key) {
				if strVal, ok := value.(string); ok {
					value = MaskSensitiveData(strVal, 4)
				}
			}
			fields = append(fields, key, value)
		}
	}
	
	// Determine log level based on event type
	switch event.Type {
	case EventLoginFailed, EventRateLimited, EventSuspiciousIP, EventAccessDenied:
		logger.Warn(fmt.Sprintf("Security Event: %s", event.Type), fields...)
	case EventSecurityAlert:
		logger.Error(fmt.Sprintf("Security Alert: %s", event.Type), fields...)
	default:
		logger.Info(fmt.Sprintf("Security Event: %s", event.Type), fields...)
	}
}

// LogLogin logs a successful login
func (sl *SecurityLogger) LogLogin(userID, username, ipAddress, userAgent string) {
	sl.LogEvent(SecurityEvent{
		Type:      EventLogin,
		UserID:    userID,
		Username:  username,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Result:    "success",
	})
}

// LogLoginFailed logs a failed login attempt
func (sl *SecurityLogger) LogLoginFailed(username, ipAddress, userAgent, reason string) {
	sl.LogEvent(SecurityEvent{
		Type:      EventLoginFailed,
		Username:  username,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Result:    "failed",
		Details: map[string]interface{}{
			"reason": reason,
		},
	})
}

// LogRateLimited logs rate limiting events
func (sl *SecurityLogger) LogRateLimited(ipAddress, userAgent, resource string) {
	sl.LogEvent(SecurityEvent{
		Type:      EventRateLimited,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Resource:  resource,
		Result:    "blocked",
	})
}

// LogAccessDenied logs access denial events
func (sl *SecurityLogger) LogAccessDenied(userID, username, resource, reason string) {
	sl.LogEvent(SecurityEvent{
		Type:     EventAccessDenied,
		UserID:   userID,
		Username: username,
		Resource: resource,
		Result:   "denied",
		Details: map[string]interface{}{
			"reason": reason,
		},
	})
}

// LogDataAccess logs data access events
func (sl *SecurityLogger) LogDataAccess(userID, username, resource, action string) {
	sl.LogEvent(SecurityEvent{
		Type:     EventDataAccess,
		UserID:   userID,
		Username: username,
		Resource: resource,
		Action:   action,
		Result:   "success",
	})
}

// LogSecurityAlert logs security alerts
func (sl *SecurityLogger) LogSecurityAlert(alertType, description string, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["alert_type"] = alertType
	details["description"] = description
	
	sl.LogEvent(SecurityEvent{
		Type:    EventSecurityAlert,
		Result:  "alert",
		Details: details,
	})
}

// isSensitiveField checks if a field name indicates sensitive data
func isSensitiveField(fieldName string) bool {
	sensitiveFields := []string{
		"password", "token", "secret", "key", "email", "phone",
		"credit_card", "ssn", "api_key", "private_key",
	}
	
	fieldLower := strings.ToLower(fieldName)
	for _, sensitive := range sensitiveFields {
		if strings.Contains(fieldLower, sensitive) {
			return true
		}
	}
	
	return false
}

// SecurityAudit represents an audit trail entry
type SecurityAudit struct {
	ID          string
	UserID      string
	Username    string
	Action      string
	Resource    string
	OldValue    string
	NewValue    string
	IPAddress   string
	UserAgent   string
	Timestamp   time.Time
}

// LogAudit logs an audit trail entry
func (sl *SecurityLogger) LogAudit(audit SecurityAudit) {
	if audit.Timestamp.IsZero() {
		audit.Timestamp = time.Now()
	}
	
	fields := []interface{}{
		"audit_id", audit.ID,
		"user_id", audit.UserID,
		"username", audit.Username,
		"action", audit.Action,
		"resource", audit.Resource,
		"timestamp", audit.Timestamp.Format(time.RFC3339),
	}
	
	if audit.IPAddress != "" {
		fields = append(fields, "ip_address", audit.IPAddress)
	}
	
	if audit.UserAgent != "" {
		fields = append(fields, "user_agent", audit.UserAgent)
	}
	
	// Mask sensitive values if needed
	if sl.maskSensitiveData {
		if audit.OldValue != "" {
			fields = append(fields, "old_value", MaskSensitiveData(audit.OldValue, 10))
		}
		if audit.NewValue != "" {
			fields = append(fields, "new_value", MaskSensitiveData(audit.NewValue, 10))
		}
	} else {
		if audit.OldValue != "" {
			fields = append(fields, "old_value", audit.OldValue)
		}
		if audit.NewValue != "" {
			fields = append(fields, "new_value", audit.NewValue)
		}
	}
	
	logger.Info("Security Audit", fields...)
}