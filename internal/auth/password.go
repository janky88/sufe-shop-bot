package auth

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrPasswordTooShort = errors.New("password must be at least 8 characters long")
	ErrPasswordNoUpper  = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLower  = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoDigit  = errors.New("password must contain at least one digit")
	ErrPasswordNoSpecial = errors.New("password must contain at least one special character")
	ErrPasswordCommon   = errors.New("password is too common")
	ErrPasswordInvalid  = errors.New("invalid password")
)

// PasswordConfig holds password policy configuration
type PasswordConfig struct {
	MinLength       int
	RequireUpper    bool
	RequireLower    bool
	RequireDigit    bool
	RequireSpecial  bool
	BcryptCost     int
}

// DefaultPasswordConfig returns default password configuration
func DefaultPasswordConfig() *PasswordConfig {
	return &PasswordConfig{
		MinLength:      8,
		RequireUpper:   true,
		RequireLower:   true,
		RequireDigit:   true,
		RequireSpecial: true,
		BcryptCost:    bcrypt.DefaultCost,
	}
}

// PasswordService handles password operations
type PasswordService struct {
	config *PasswordConfig
}

// NewPasswordService creates a new password service
func NewPasswordService(config *PasswordConfig) *PasswordService {
	if config == nil {
		config = DefaultPasswordConfig()
	}
	return &PasswordService{config: config}
}

// ValidatePassword checks if password meets complexity requirements
func (s *PasswordService) ValidatePassword(password string) error {
	if len(password) < s.config.MinLength {
		return ErrPasswordTooShort
	}
	
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}
	
	if s.config.RequireUpper && !hasUpper {
		return ErrPasswordNoUpper
	}
	if s.config.RequireLower && !hasLower {
		return ErrPasswordNoLower
	}
	if s.config.RequireDigit && !hasDigit {
		return ErrPasswordNoDigit
	}
	if s.config.RequireSpecial && !hasSpecial {
		return ErrPasswordNoSpecial
	}
	
	// Check common passwords
	if isCommonPassword(password) {
		return ErrPasswordCommon
	}
	
	return nil
}

// HashPassword hashes a password using bcrypt
func (s *PasswordService) HashPassword(password string) (string, error) {
	// Validate password first
	if err := s.ValidatePassword(password); err != nil {
		return "", err
	}
	
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), s.config.BcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	
	return string(bytes), nil
}

// ComparePassword compares a password with its hash
func (s *PasswordService) ComparePassword(hashedPassword, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrPasswordInvalid
		}
		return fmt.Errorf("failed to compare password: %w", err)
	}
	return nil
}

// GetPasswordStrength returns a score from 0-100 indicating password strength
func (s *PasswordService) GetPasswordStrength(password string) int {
	score := 0
	
	// Length score (max 30)
	length := len(password)
	if length >= 8 {
		score += 10
	}
	if length >= 12 {
		score += 10
	}
	if length >= 16 {
		score += 10
	}
	
	// Character diversity score (max 40)
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	uniqueChars := make(map[rune]bool)
	
	for _, char := range password {
		uniqueChars[char] = true
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}
	
	if hasUpper {
		score += 10
	}
	if hasLower {
		score += 10
	}
	if hasDigit {
		score += 10
	}
	if hasSpecial {
		score += 10
	}
	
	// Uniqueness score (max 30)
	uniqueRatio := float64(len(uniqueChars)) / float64(len(password))
	if uniqueRatio > 0.6 {
		score += 10
	}
	if uniqueRatio > 0.7 {
		score += 10
	}
	if uniqueRatio > 0.8 {
		score += 10
	}
	
	// Cap at 100
	if score > 100 {
		score = 100
	}
	
	return score
}

// isCommonPassword checks if password is in common passwords list
func isCommonPassword(password string) bool {
	// This is a simplified check. In production, use a comprehensive list
	commonPasswords := []string{
		"password", "password123", "123456", "12345678", "qwerty", "abc123",
		"monkey", "1234567", "letmein", "trustno1", "dragon", "baseball",
		"111111", "iloveyou", "master", "sunshine", "ashley", "bailey",
		"passw0rd", "shadow", "123123", "654321", "superman", "qazwsx",
		"michael", "football", "password1", "password12", "password123",
	}
	
	lowerPassword := strings.ToLower(password)
	for _, common := range commonPasswords {
		if lowerPassword == common {
			return true
		}
	}
	
	return false
}