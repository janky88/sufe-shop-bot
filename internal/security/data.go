package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var (
	ErrInvalidInput      = errors.New("invalid input")
	ErrDecryptionFailed  = errors.New("decryption failed")
	ErrEncryptionFailed  = errors.New("encryption failed")
)

// DataSecurity provides data encryption and validation
type DataSecurity struct {
	encryptionKey []byte
}

// NewDataSecurity creates a new data security instance
func NewDataSecurity(key string) (*DataSecurity, error) {
	if key == "" {
		// Generate a random key if not provided
		keyBytes := make([]byte, 32)
		if _, err := rand.Read(keyBytes); err != nil {
			return nil, err
		}
		key = hex.EncodeToString(keyBytes)
	}
	
	// Ensure key is 32 bytes
	hash := sha256.Sum256([]byte(key))
	
	return &DataSecurity{
		encryptionKey: hash[:],
	}, nil
}

// Encrypt encrypts sensitive data
func (ds *DataSecurity) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(ds.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}
	
	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}
	
	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}
	
	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	
	// Encode to base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts sensitive data
func (ds *DataSecurity) Decrypt(ciphertext string) (string, error) {
	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}
	
	block, err := aes.NewCipher(ds.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}
	
	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}
	
	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("%w: ciphertext too short", ErrDecryptionFailed)
	}
	
	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	
	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}
	
	return string(plaintext), nil
}

// HashData creates a secure hash of data
func (ds *DataSecurity) HashData(data string) string {
	hash := sha256.Sum256([]byte(data + string(ds.encryptionKey)))
	return hex.EncodeToString(hash[:])
}

// Input validation functions

// ValidateEmail validates email format
func ValidateEmail(email string) error {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("%w: invalid email format", ErrInvalidInput)
	}
	return nil
}

// ValidatePhoneNumber validates phone number format
func ValidatePhoneNumber(phone string) error {
	// Remove all non-numeric characters
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, phone)
	
	// Check length (basic validation)
	if len(cleaned) < 10 || len(cleaned) > 15 {
		return fmt.Errorf("%w: invalid phone number length", ErrInvalidInput)
	}
	
	return nil
}

// ValidateURL validates URL format
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid URL format", ErrInvalidInput)
	}
	
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%w: URL must have scheme and host", ErrInvalidInput)
	}
	
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: only HTTP(S) URLs are allowed", ErrInvalidInput)
	}
	
	return nil
}

// SanitizeInput removes potentially dangerous characters
func SanitizeInput(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")
	
	// Trim whitespace
	input = strings.TrimSpace(input)
	
	// Remove control characters
	sanitized := strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return -1
		}
		return r
	}, input)
	
	return sanitized
}

// ValidateAlphanumeric validates that input contains only alphanumeric characters
func ValidateAlphanumeric(input string) error {
	for _, r := range input {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return fmt.Errorf("%w: input must be alphanumeric", ErrInvalidInput)
		}
	}
	return nil
}

// ValidateNumeric validates that input contains only numeric characters
func ValidateNumeric(input string) error {
	for _, r := range input {
		if !unicode.IsDigit(r) {
			return fmt.Errorf("%w: input must be numeric", ErrInvalidInput)
		}
	}
	return nil
}

// ValidateLength validates input length
func ValidateLength(input string, min, max int) error {
	length := len(input)
	if length < min {
		return fmt.Errorf("%w: input too short (minimum %d characters)", ErrInvalidInput, min)
	}
	if max > 0 && length > max {
		return fmt.Errorf("%w: input too long (maximum %d characters)", ErrInvalidInput, max)
	}
	return nil
}

// ValidateNoSQL checks for potential SQL injection patterns
func ValidateNoSQL(input string) error {
	// Common SQL injection patterns
	sqlPatterns := []string{
		"(?i)(union.*select)",
		"(?i)(select.*from)",
		"(?i)(insert.*into)",
		"(?i)(delete.*from)",
		"(?i)(update.*set)",
		"(?i)(drop.*table)",
		"(?i)(create.*table)",
		"(?i)(alter.*table)",
		"(?i)(exec.*\\()",
		"(?i)(execute.*\\()",
		"(?i)(script.*>)",
		"(?i)(<.*script)",
		"'.*or.*'='",
		"\".*or.*\"=\"",
		"--",
		"/\\*.*\\*/",
		"xp_.*",
		"sp_.*",
	}
	
	for _, pattern := range sqlPatterns {
		matched, err := regexp.MatchString(pattern, input)
		if err == nil && matched {
			return fmt.Errorf("%w: potential SQL injection detected", ErrInvalidInput)
		}
	}
	
	return nil
}

// ValidateNoXSS checks for potential XSS patterns
func ValidateNoXSS(input string) error {
	// Common XSS patterns
	xssPatterns := []string{
		"<script",
		"</script",
		"javascript:",
		"onerror=",
		"onload=",
		"onclick=",
		"onmouseover=",
		"<iframe",
		"<object",
		"<embed",
		"<link",
		"<meta",
		"<style",
		"expression\\(",
		"vbscript:",
		"data:text/html",
	}
	
	lowerInput := strings.ToLower(input)
	for _, pattern := range xssPatterns {
		if strings.Contains(lowerInput, pattern) {
			return fmt.Errorf("%w: potential XSS detected", ErrInvalidInput)
		}
	}
	
	return nil
}

// EscapeHTML escapes HTML special characters
func EscapeHTML(input string) string {
	replacements := map[string]string{
		"&":  "&amp;",
		"<":  "&lt;",
		">":  "&gt;",
		"\"": "&quot;",
		"'":  "&#39;",
		"/":  "&#x2F;",
	}
	
	output := input
	for old, new := range replacements {
		output = strings.ReplaceAll(output, old, new)
	}
	
	return output
}

// MaskSensitiveData masks sensitive data for logging
func MaskSensitiveData(data string, visibleChars int) string {
	if len(data) <= visibleChars {
		return strings.Repeat("*", len(data))
	}
	
	if visibleChars <= 0 {
		return strings.Repeat("*", len(data))
	}
	
	return data[:visibleChars] + strings.Repeat("*", len(data)-visibleChars)
}

// MaskEmail masks email address
func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return MaskSensitiveData(email, 3)
	}
	
	username := parts[0]
	domain := parts[1]
	
	if len(username) <= 3 {
		username = strings.Repeat("*", len(username))
	} else {
		username = username[:2] + strings.Repeat("*", len(username)-2)
	}
	
	domainParts := strings.Split(domain, ".")
	if len(domainParts) >= 2 {
		domainParts[0] = domainParts[0][:1] + strings.Repeat("*", len(domainParts[0])-1)
		domain = strings.Join(domainParts, ".")
	}
	
	return username + "@" + domain
}

// MaskPhoneNumber masks phone number
func MaskPhoneNumber(phone string) string {
	// Keep only last 4 digits visible
	if len(phone) <= 4 {
		return strings.Repeat("*", len(phone))
	}
	
	return strings.Repeat("*", len(phone)-4) + phone[len(phone)-4:]
}