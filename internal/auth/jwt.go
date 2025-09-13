package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
	
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
	ErrInvalidClaims = errors.New("invalid claims")
)

// JWTConfig holds JWT configuration
type JWTConfig struct {
	SecretKey       string
	TokenExpiry     time.Duration
	RefreshExpiry   time.Duration
	Issuer          string
	// For backward compatibility
	LegacyToken     string
	EnableLegacyAuth bool
}

// Claims represents JWT claims
type Claims struct {
	jwt.RegisteredClaims
	UserID   string `json:"uid,omitempty"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
}

// JWTService handles JWT operations
type JWTService struct {
	config *JWTConfig
}

// NewJWTService creates a new JWT service
func NewJWTService(config *JWTConfig) *JWTService {
	// Generate a secret key if not provided
	if config.SecretKey == "" {
		config.SecretKey = generateSecretKey()
	}
	
	// Set default expiry times
	if config.TokenExpiry == 0 {
		config.TokenExpiry = 24 * time.Hour // 24 hours
	}
	if config.RefreshExpiry == 0 {
		config.RefreshExpiry = 7 * 24 * time.Hour // 7 days
	}
	if config.Issuer == "" {
		config.Issuer = "shop-bot-admin"
	}
	
	return &JWTService{
		config: config,
	}
}

// GenerateToken generates a new JWT token
func (s *JWTService) GenerateToken(userID, username, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.TokenExpiry)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        generateTokenID(),
		},
		UserID:   userID,
		Username: username,
		Role:     role,
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.SecretKey))
}

// GenerateRefreshToken generates a new refresh token
func (s *JWTService) GenerateRefreshToken(userID string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    s.config.Issuer,
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(now.Add(s.config.RefreshExpiry)),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        generateTokenID(),
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.SecretKey))
}

// ValidateToken validates and parses a JWT token
func (s *JWTService) ValidateToken(tokenString string) (*Claims, error) {
	// For backward compatibility, check if it's the legacy token
	if s.config.EnableLegacyAuth && tokenString == s.config.LegacyToken {
		// Return a special claim for legacy token
		return &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:  s.config.Issuer,
				Subject: "admin",
			},
			UserID:   "admin",
			Username: "admin",
			Role:     "admin",
		}, nil
	}
	
	// Parse JWT token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.SecretKey), nil
	})
	
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}
	
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidClaims
	}
	
	return claims, nil
}

// RefreshToken creates a new access token from a refresh token
func (s *JWTService) RefreshToken(refreshTokenString string) (string, error) {
	// Parse refresh token
	token, err := jwt.ParseWithClaims(refreshTokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.SecretKey), nil
	})
	
	if err != nil {
		return "", err
	}
	
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return "", ErrInvalidClaims
	}
	
	// Generate new access token
	// In a real system, you'd fetch user details from database
	return s.GenerateToken(claims.Subject, "admin", "admin")
}

// IsLegacyToken checks if the provided token is the legacy admin token
func (s *JWTService) IsLegacyToken(token string) bool {
	return s.config.EnableLegacyAuth && token == s.config.LegacyToken
}

// Helper functions

func generateSecretKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}