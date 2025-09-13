package httpadmin

import (
	"fmt"
	"html/template"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"

	"shop-bot/internal/auth"
	"shop-bot/internal/broadcast"
	"shop-bot/internal/config"
	logger "shop-bot/internal/log"
	"shop-bot/internal/middleware"
	"shop-bot/internal/notification"
	payment "shop-bot/internal/payment/epay"
	"shop-bot/internal/security"
	"shop-bot/internal/store"
	"shop-bot/internal/ticket"
)

type Server struct {
	adminToken   string
	db           *gorm.DB
	bot          *tgbotapi.BotAPI
	epay         *payment.Client
	config       *config.Config
	configManager *config.Manager
	broadcast    *broadcast.Service
	notification *notification.Service
	ticketService *ticket.Service
	jwtService   *auth.JWTService

	// Security services
	passwordService  *auth.PasswordService
	rateLimiter      *auth.RateLimiter
	sessionManager   *auth.SessionManager
	dataSecurity     *security.DataSecurity
	securityLogger   *security.SecurityLogger
}

func NewServer(adminToken string, db *gorm.DB) *Server {
	// Load config for payment
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		return &Server{
			adminToken: adminToken,
			db:         db,
		}
	}
	
	// Initialize bot API for sending messages
	var bot *tgbotapi.BotAPI
	if cfg.BotToken != "" {
		bot, err = tgbotapi.NewBotAPI(cfg.BotToken)
		if err != nil {
			logger.Error("Failed to init bot API", "error", err)
		}
	}
	
	// Initialize epay client
	var epayClient *payment.Client
	if cfg.EpayPID != "" && cfg.EpayKey != "" && cfg.EpayGateway != "" {
		epayClient = payment.NewClient(cfg.EpayPID, cfg.EpayKey, cfg.EpayGateway)
	}
	
	// Initialize broadcast service
	var broadcastService *broadcast.Service
	if bot != nil {
		broadcastService = broadcast.NewService(db, bot)
	}
	
	// Initialize notification service
	var notificationService *notification.Service
	if bot != nil && cfg != nil {
		notificationService = notification.NewService(bot, cfg, db)
	}
	
	// Initialize ticket service
	var ticketService *ticket.Service
	if bot != nil {
		ticketService = ticket.NewService(db, bot)
	}
	
	// Initialize JWT service
	var jwtService *auth.JWTService
	if cfg != nil {
		jwtConfig := &auth.JWTConfig{
			SecretKey:        cfg.JWTSecret,
			TokenExpiry:      time.Duration(cfg.JWTExpiry) * time.Hour,
			RefreshExpiry:    time.Duration(cfg.JWTRefreshExpiry) * 24 * time.Hour,
			Issuer:          "shop-bot-admin",
			LegacyToken:     adminToken,
			EnableLegacyAuth: cfg.EnableLegacyAuth,
		}
		jwtService = auth.NewJWTService(jwtConfig)
	}
	
	// Initialize security services
	var passwordService *auth.PasswordService
	var rateLimiter *auth.RateLimiter
	var sessionManager *auth.SessionManager
	var dataSecurity *security.DataSecurity
	var securityLogger *security.SecurityLogger
	
	if cfg != nil {
		// Password service
		if cfg.EnablePasswordPolicy {
			passwordConfig := &auth.PasswordConfig{
				MinLength:      cfg.PasswordMinLength,
				RequireUpper:   cfg.PasswordRequireUpper,
				RequireLower:   cfg.PasswordRequireLower,
				RequireDigit:   cfg.PasswordRequireDigit,
				RequireSpecial: cfg.PasswordRequireSpecial,
				BcryptCost:     12,
			}
			passwordService = auth.NewPasswordService(passwordConfig)
		}
		
		// Rate limiter for login attempts
		rateLimiterConfig := &auth.RateLimiterConfig{
			MaxAttempts:     cfg.LoginMaxAttempts,
			LockoutDuration: time.Duration(cfg.LoginLockoutMinutes) * time.Minute,
			WindowDuration:  5 * time.Minute,
			CleanupInterval: 10 * time.Minute,
		}
		rateLimiter = auth.NewRateLimiter(rateLimiterConfig)
		
		// Session manager
		sessionConfig := &auth.SessionConfig{
			MaxConcurrent:        cfg.SessionMaxConcurrent,
			SessionTimeout:       time.Duration(cfg.SessionTimeoutHours) * time.Hour,
			IdleTimeout:          time.Duration(cfg.SessionIdleMinutes) * time.Minute,
			EnableIPCheck:        cfg.EnableIPValidation,
			EnableUserAgentCheck: cfg.EnableUserAgentCheck,
		}
		sessionManager = auth.NewSessionManager(sessionConfig)
		
		// Data security
		if ds, err := security.NewDataSecurity(cfg.DataEncryptionKey); err == nil {
			dataSecurity = ds
		} else {
			logger.Error("Failed to initialize data security", "error", err)
		}
		
		// Security logger
		if cfg.EnableSecurityLogging {
			securityLogger = security.NewSecurityLogger(true, cfg.MaskSensitiveData)
		}
	}
	
	return &Server{
		adminToken:      adminToken,
		db:              db,
		bot:             bot,
		epay:            epayClient,
		config:          cfg,
		broadcast:       broadcastService,
		notification:    notificationService,
		ticketService:   ticketService,
		jwtService:      jwtService,
		passwordService: passwordService,
		rateLimiter:     rateLimiter,
		sessionManager:  sessionManager,
		dataSecurity:    dataSecurity,
		securityLogger:  securityLogger,
	}
}

// NewServerWithApp creates a new server with application reference
func NewServerWithApp(adminToken string, app interface{}) *Server {
	// Use reflection to extract fields from app
	appValue := reflect.ValueOf(app)
	if appValue.Kind() == reflect.Ptr {
		appValue = appValue.Elem()
	}
	
	server := &Server{
		adminToken: adminToken,
	}
	
	// Try to get DB field
	if dbField := appValue.FieldByName("DB"); dbField.IsValid() {
		if db, ok := dbField.Interface().(*gorm.DB); ok {
			server.db = db
		}
	}
	
	// Try to get Config field
	if cfgField := appValue.FieldByName("Config"); cfgField.IsValid() {
		if cfg, ok := cfgField.Interface().(*config.Config); ok {
			server.config = cfg

			// Initialize payment client
			if cfg.EpayPID != "" && cfg.EpayKey != "" {
				server.epay = payment.NewClient(cfg.EpayPID, cfg.EpayKey, cfg.EpayGateway)
			}
		}
	}

	// Try to get ConfigManager field
	if cfgManagerField := appValue.FieldByName("ConfigManager"); cfgManagerField.IsValid() {
		if cfgManager, ok := cfgManagerField.Interface().(*config.Manager); ok {
			server.configManager = cfgManager
		}
	}
	
	// Try to get Bot field and extract API
	if botField := appValue.FieldByName("Bot"); botField.IsValid() && !botField.IsNil() {
		if method := botField.MethodByName("GetAPI"); method.IsValid() {
			if results := method.Call(nil); len(results) > 0 {
				if api, ok := results[0].Interface().(*tgbotapi.BotAPI); ok {
					server.bot = api
				}
			}
		}
	}
	
	// Try to get Broadcast field
	if broadcastField := appValue.FieldByName("Broadcast"); broadcastField.IsValid() {
		if bc, ok := broadcastField.Interface().(*broadcast.Service); ok {
			server.broadcast = bc
		}
	}
	
	// Initialize notification service if we have bot and config
	if server.bot != nil && server.config != nil {
		server.notification = notification.NewService(server.bot, server.config, server.db)
	}
	
	// Initialize ticket service
	if server.bot != nil && server.db != nil {
		server.ticketService = ticket.NewService(server.db, server.bot)
	}
	
	// Initialize JWT service
	if server.config != nil {
		jwtConfig := &auth.JWTConfig{
			SecretKey:        server.config.JWTSecret,
			TokenExpiry:      time.Duration(server.config.JWTExpiry) * time.Hour,
			RefreshExpiry:    time.Duration(server.config.JWTRefreshExpiry) * 24 * time.Hour,
			Issuer:          "shop-bot-admin",
			LegacyToken:     server.adminToken,
			EnableLegacyAuth: server.config.EnableLegacyAuth,
		}
		server.jwtService = auth.NewJWTService(jwtConfig)
		
		// Initialize security services
		cfg := server.config
		
		// Password service
		if cfg.EnablePasswordPolicy {
			passwordConfig := &auth.PasswordConfig{
				MinLength:      cfg.PasswordMinLength,
				RequireUpper:   cfg.PasswordRequireUpper,
				RequireLower:   cfg.PasswordRequireLower,
				RequireDigit:   cfg.PasswordRequireDigit,
				RequireSpecial: cfg.PasswordRequireSpecial,
				BcryptCost:     12,
			}
			server.passwordService = auth.NewPasswordService(passwordConfig)
		}
		
		// Rate limiter for login attempts
		rateLimiterConfig := &auth.RateLimiterConfig{
			MaxAttempts:     cfg.LoginMaxAttempts,
			LockoutDuration: time.Duration(cfg.LoginLockoutMinutes) * time.Minute,
			WindowDuration:  5 * time.Minute,
			CleanupInterval: 10 * time.Minute,
		}
		server.rateLimiter = auth.NewRateLimiter(rateLimiterConfig)
		
		// Session manager
		sessionConfig := &auth.SessionConfig{
			MaxConcurrent:        cfg.SessionMaxConcurrent,
			SessionTimeout:       time.Duration(cfg.SessionTimeoutHours) * time.Hour,
			IdleTimeout:          time.Duration(cfg.SessionIdleMinutes) * time.Minute,
			EnableIPCheck:        cfg.EnableIPValidation,
			EnableUserAgentCheck: cfg.EnableUserAgentCheck,
		}
		server.sessionManager = auth.NewSessionManager(sessionConfig)
		
		// Data security
		if ds, err := security.NewDataSecurity(cfg.DataEncryptionKey); err == nil {
			server.dataSecurity = ds
		} else {
			logger.Error("Failed to initialize data security", "error", err)
		}
		
		// Security logger
		if cfg.EnableSecurityLogging {
			server.securityLogger = security.NewSecurityLogger(true, cfg.MaskSensitiveData)
		}
	}
	
	return server
}

// toInt64 converts interface{} to int64
func toInt64(v interface{}) (int64, error) {
	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case int32:
		return int64(val), nil
	case uint:
		return int64(val), nil
	case uint32:
		return int64(val), nil
	case uint64:
		return int64(val), nil
	case float64:
		return int64(val), nil
	case float32:
		return int64(val), nil
	case string:
		return strconv.ParseInt(val, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()
	
	// Get currency settings
	_, currencySymbol := store.GetCurrencySettings(s.db, s.config)
	
	// Add template functions BEFORE loading templates
	r.SetFuncMap(template.FuncMap{
		"divf": func(a, b interface{}) float64 {
			af, _ := toFloat64(a)
			bf, _ := toFloat64(b)
			if bf == 0 {
				return 0
			}
			return af / bf
		},
		"addf": func(a, b interface{}) float64 {
			af, _ := toFloat64(a)
			bf, _ := toFloat64(b)
			return af + bf
		},
		"subf": func(a, b interface{}) float64 {
			af, _ := toFloat64(a)
			bf, _ := toFloat64(b)
			return af - bf
		},
		"int": func(a interface{}) int {
			f, _ := toFloat64(a)
			return int(f)
		},
		"seq": func(start, end int) []int {
			var result []int
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
			return result
		},
		"currency": func() string {
			return currencySymbol
		},
		"plus": func(a, b interface{}) int64 {
			ai, _ := toInt64(a)
			bi, _ := toInt64(b)
			return ai + bi
		},
		"minus": func(a, b interface{}) int64 {
			ai, _ := toInt64(a)
			bi, _ := toInt64(b)
			return ai - bi
		},
		"multiply": func(a, b interface{}) int64 {
			ai, _ := toInt64(a)
			bi, _ := toInt64(b)
			return ai * bi
		},
	})
	
	// Load HTML templates AFTER setting functions
	r.LoadHTMLGlob("templates/*")

	// Add middleware
	r.Use(s.requestLogger())
	r.Use(RecoveryMiddleware())  // Add panic recovery before error handler
	r.Use(ErrorHandlerMiddleware())
	
	// Set up all routes
	s.SetupRoutes(r)

	return r
}

// SetupRoutes sets up routes on an existing router
func (s *Server) SetupRoutes(r *gin.Engine) {
	// Apply global security middleware if configured
	if s.config != nil {
		// Rate limiting
		if s.config.EnableRateLimit {
			r.Use(middleware.RateLimitMiddleware(
				s.config.RateLimitRequests,
				time.Duration(s.config.RateLimitWindowMinutes)*time.Minute,
				s.config.RateLimitMessage,
			))
		}
		
		// Security headers
		if s.config.EnableSecurityHeaders {
			securityConfig := &middleware.SecurityConfig{
				EnableSecurityHeaders: true,
				HSTS:                 s.config.EnableHSTS,
				HSTSMaxAge:          s.config.HSTSMaxAge,
				ContentTypeNosniff:  true,
				XFrameOptions:       "SAMEORIGIN",
				XSSProtection:       true,
			}
			r.Use(middleware.SecurityHeadersMiddleware(securityConfig))
		}
		
		// CSRF protection for forms
		if s.config.EnableCSRF {
			// Apply CSRF middleware selectively (not on all routes)
			// We'll add it to specific routes that need it
		}
	}
	
	// Static files (CSS, JS)
	r.Static("/static", "./static")
	
	// Health check
	r.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Metrics
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Root path - login page (only show if not authenticated)
	r.GET("/", func(c *gin.Context) {
		// Check if user is already authenticated
		token := c.GetHeader("Authorization")
		if token == "Bearer "+s.adminToken {
			c.Redirect(http.StatusFound, "/admin/")
			return
		}
		
		// Check cookie
		cookie, err := c.Cookie("admin_token")
		if err == nil && cookie == s.adminToken {
			c.Redirect(http.StatusFound, "/admin/")
			return
		}
		
		// Show login page
		s.handleLoginPage(c)
	})
	
	// API routes
	r.POST("/api/login", s.handleLogin)
	r.POST("/api/logout", s.handleLogout)
	r.POST("/api/refresh", s.handleRefreshToken)

	// Payment webhook routes
	r.POST("/payment/epay/notify", s.handleEpayNotify)
	r.GET("/payment/return", s.handlePaymentReturn)
	
	// Test bot endpoint (protected)
	r.POST("/admin/test-bot/:user_id", s.authMiddleware(), s.handleTestBot)

	// Admin routes (protected)
	adminGroup := r.Group("/admin", s.authMiddleware())
	{
		// Product management
		adminGroup.GET("/products", s.handleProductList)
		adminGroup.GET("/products/test", func(c *gin.Context) {
			c.HTML(http.StatusOK, "product_test.html", nil)
		})
		adminGroup.POST("/products", s.handleProductCreate)
		adminGroup.PUT("/products/:id", s.handleProductUpdate)
		adminGroup.DELETE("/products/:id", s.handleProductDelete)
		adminGroup.PUT("/products/:id/restore", s.handleProductRestore)
		adminGroup.DELETE("/products/:id/permanent", s.handleProductPermanentDelete)
		adminGroup.GET("/products/:id/codes", s.handleProductCodes)
		adminGroup.POST("/products/:id/codes/upload", s.handleCodesUpload)
		adminGroup.DELETE("/codes/:id", s.handleCodeDelete)
		adminGroup.GET("/codes/template", s.handleCodeTemplate)

		// Order management
		adminGroup.GET("/orders", s.handleOrderList)
		
		// User management
		adminGroup.GET("/users", s.handleUserList)
		adminGroup.GET("/users/:id", s.handleUserDetail)

		// Recharge card management
		adminGroup.GET("/recharge-cards", s.handleRechargeCardList)
		adminGroup.POST("/recharge-cards/generate", s.handleRechargeCardGenerate)
		adminGroup.DELETE("/recharge-cards/:id", s.handleRechargeCardDelete)
		adminGroup.GET("/recharge-cards/:id/usage", s.handleRechargeCardUsage)

		// Template management
		adminGroup.GET("/templates", s.handleTemplateList)
		adminGroup.POST("/templates/:id", s.handleTemplateUpdate)

		// System settings
		adminGroup.GET("/settings", s.handleSettingsList)
		adminGroup.POST("/settings", s.handleSettingsUpdate)
		
		// FAQ management
		adminGroup.GET("/faq", s.handleFAQList)
		adminGroup.POST("/faq", s.handleFAQCreate)
		adminGroup.PUT("/faq/:id", s.handleFAQUpdate)
		adminGroup.DELETE("/faq/:id", s.handleFAQDelete)
		adminGroup.PUT("/faq/:id/sort", s.handleFAQSort)
		adminGroup.POST("/faq/init", s.handleFAQInit)
		
		// Broadcast management
		adminGroup.GET("/broadcast", s.handleBroadcastList)
		adminGroup.POST("/broadcast", s.handleBroadcastCreate)
		adminGroup.POST("/broadcast/send", s.handleBroadcastSend)  // Add this route for AJAX requests
		adminGroup.GET("/broadcast/:id", s.handleBroadcastDetail)
		
		// Ticket management
		adminGroup.GET("/tickets", s.handleTicketList)
		adminGroup.GET("/tickets/:id", s.handleTicketDetail)
		adminGroup.POST("/tickets/:id/reply", s.handleTicketReply)
		adminGroup.PUT("/tickets/:id/status", s.handleTicketStatusUpdate)
		adminGroup.PUT("/tickets/:id/assign", s.handleTicketAssign)
		adminGroup.GET("/ticket-templates", s.handleTicketTemplates)

		// Admin profile
		adminGroup.GET("/profile/telegram", s.handleGetAdminTelegram)
		adminGroup.POST("/profile/telegram", s.handleSetAdminTelegram)
		adminGroup.POST("/ticket-templates", s.handleTicketTemplateCreate)
		adminGroup.PUT("/ticket-templates/:id", s.handleTicketTemplateUpdate)
		adminGroup.DELETE("/ticket-templates/:id", s.handleTicketTemplateDelete)
		
		// Order maintenance APIs
		adminGroup.POST("/api/settings", s.handleSaveSettings)
		adminGroup.POST("/api/settings/core", s.handleSaveCoreSettings)
		adminGroup.POST("/api/settings/payment", s.handleSavePaymentSettings)
		adminGroup.POST("/api/orders/cleanup", s.handleCleanupOrders)

		// Dashboard
		adminGroup.GET("/", s.handleAdminDashboard)
	}
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get client IP and User-Agent for session validation
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()

		// Check session first if session manager is available
		if s.sessionManager != nil {
			sessionID, err := c.Cookie("session_id")
			if err == nil && sessionID != "" {
				session, err := s.sessionManager.ValidateSession(sessionID, clientIP, userAgent)
				if err == nil {
					// Valid session found
					c.Set("session_id", sessionID)
					c.Set("user_id", session.UserID)
					c.Set("username", session.Username)
					c.Set("role", session.Role)
					c.Next()
					return
				}
			}
		}

		// Get token from various sources
		var token string

		// 1. Check Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		// 2. Check cookie if no header token
		if token == "" {
			if cookie, err := c.Cookie("admin_token"); err == nil {
				token = cookie
			}
		}

		// 3. Validate token
		if token != "" {
			// First try JWT validation
			if s.jwtService != nil {
				claims, err := s.jwtService.ValidateToken(token)
				if err == nil {
					// Store claims in context for later use
					c.Set("user_claims", claims)
					c.Set("user_id", claims.UserID)
					c.Set("username", claims.Username)
					c.Set("role", claims.Role)

					// Log data access if security logger is available
					if s.securityLogger != nil {
						s.securityLogger.LogDataAccess(
							claims.UserID,
							claims.Username,
							c.Request.URL.Path,
							c.Request.Method,
						)
					}

					c.Next()
					return
				}
				// Log JWT validation error for debugging
				if err != auth.ErrInvalidToken {
					logger.Debug("JWT validation failed", "error", err)
				}
			}

			// Fall back to legacy token check for backward compatibility
			if token == s.adminToken {
				// Set default admin claims for legacy token
				c.Set("user_id", "admin")
				c.Set("username", "admin")
				c.Set("role", "admin")
				c.Next()
				return
			}
		}

		// Authentication failed
		// Log unauthorized access
		if s.securityLogger != nil {
			s.securityLogger.LogAccessDenied(
				"",
				"",
				c.Request.URL.Path,
				"no_valid_authentication",
			)
		}

		// If it's an API request or AJAX request, return 401
		if strings.HasPrefix(c.Request.URL.Path, "/api/") ||
		   c.GetHeader("X-Requested-With") == "XMLHttpRequest" ||
		   strings.Contains(c.GetHeader("Accept"), "application/json") {
			JSONError(c, NewUnauthorizedError("Authentication required"))
			c.Abort()
			return
		}

		// Otherwise redirect to login page
		c.Redirect(http.StatusFound, "/")
		c.Abort()
	}
}


// handleLoginPage serves the login page
func (s *Server) handleLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", nil)
}

// handleLogin processes login request
func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, NewBadRequestError("Invalid request format", err))
		return
	}
	
	// Get client IP and User-Agent
	clientIP := c.ClientIP()
	userAgent := c.Request.UserAgent()
	
	// Check rate limiting if enabled
	if s.rateLimiter != nil {
		allowed, remaining := s.rateLimiter.CheckAttempt(clientIP)
		if !allowed {
			// Log security event
			if s.securityLogger != nil {
				s.securityLogger.LogRateLimited(clientIP, userAgent, "/api/login")
			}
			JSONError(c, NewTooManyRequestsError(auth.FormatLockoutMessage(remaining)))
			return
		}
	}
	
	// Verify token against admin token
	if req.Token != s.adminToken {
		// Record failed attempt
		if s.rateLimiter != nil {
			s.rateLimiter.RecordAttempt(clientIP, false)
		}
		
		// Log failed login
		if s.securityLogger != nil {
			s.securityLogger.LogLoginFailed("admin", clientIP, userAgent, "invalid_token")
		}
		
		JSONError(c, NewUnauthorizedError("Invalid credentials"))
		return
	}
	
	// Record successful attempt
	if s.rateLimiter != nil {
		s.rateLimiter.RecordAttempt(clientIP, true)
	}
	
	// Create session if session manager is available
	var sessionID string
	if s.sessionManager != nil {
		session, err := s.sessionManager.CreateSession("admin", "admin", "admin", clientIP, userAgent)
		if err != nil {
			logger.Error("Failed to create session", "error", err)
		} else {
			sessionID = session.ID
		}
	}
	
	// Generate JWT token if JWT service is available
	var responseToken string
	var refreshToken string
	
	if s.jwtService != nil {
		// Generate JWT tokens
		token, err := s.jwtService.GenerateToken("admin", "admin", "admin")
		if err != nil {
			logger.Error("Failed to generate JWT token", "error", err)
			// Fall back to legacy token
			responseToken = s.adminToken
		} else {
			responseToken = token
			
			// Generate refresh token
			refresh, err := s.jwtService.GenerateRefreshToken("admin")
			if err != nil {
				logger.Error("Failed to generate refresh token", "error", err)
			} else {
				refreshToken = refresh
			}
		}
	} else {
		// Use legacy token
		responseToken = s.adminToken
	}
	
	// Log successful login
	if s.securityLogger != nil {
		s.securityLogger.LogLogin("admin", "admin", clientIP, userAgent)
	}
	
	// Set cookie with the token
	c.SetCookie("admin_token", responseToken, 86400*7, "/", "", false, true) // 7 days
	
	// Set session cookie if available
	if sessionID != "" {
		c.SetCookie("session_id", sessionID, 86400, "/", "", false, true) // 1 day
	}
	
	// Return tokens in response
	response := gin.H{
		"success": true,
		"token":   responseToken,
	}
	
	if refreshToken != "" {
		response["refresh_token"] = refreshToken
		// Also set refresh token as httpOnly cookie
		c.SetCookie("refresh_token", refreshToken, 86400*7, "/", "", false, true)
	}
	
	if sessionID != "" {
		response["session_id"] = sessionID
	}
	
	c.JSON(http.StatusOK, response)
}

// handleLogout processes logout request
func (s *Server) handleLogout(c *gin.Context) {
	// Clear cookies
	c.SetCookie("admin_token", "", -1, "/", "", false, true)
	c.SetCookie("refresh_token", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleRefreshToken refreshes the access token using a refresh token
func (s *Server) handleRefreshToken(c *gin.Context) {
	// Check if JWT service is available
	if s.jwtService == nil {
		JSONError(c, NewInternalError(fmt.Errorf("JWT service not available")))
		return
	}
	
	var refreshToken string
	
	// Try to get refresh token from request body
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err == nil && req.RefreshToken != "" {
		refreshToken = req.RefreshToken
	}
	
	// Try to get from cookie if not in body
	if refreshToken == "" {
		if cookie, err := c.Cookie("refresh_token"); err == nil {
			refreshToken = cookie
		}
	}
	
	if refreshToken == "" {
		JSONError(c, NewBadRequestError("Refresh token required", nil))
		return
	}
	
	// Generate new access token
	newToken, err := s.jwtService.RefreshToken(refreshToken)
	if err != nil {
		JSONError(c, NewUnauthorizedError("Invalid refresh token"))
		return
	}
	
	// Set new token in cookie
	c.SetCookie("admin_token", newToken, 86400*7, "/", "", false, true)
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token":   newToken,
	})
}

// handleTestBot tests sending a message to a user
func (s *Server) handleTestBot(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		JSONError(c, NewBadRequestError("Invalid user ID format", err))
		return
	}
	
	if s.bot == nil {
		JSONError(c, NewInternalError(fmt.Errorf("bot service not initialized")))
		return
	}
	
	// Log bot info
	logger.Info("Test bot", "bot_username", s.bot.Self.UserName, "bot_id", s.bot.Self.ID, "target_user", userID)
	
	// Send test message
	testMsg := "🔔 测试消息 / Test Message\n\n这是一条测试消息，用于验证机器人连接。\nThis is a test message to verify bot connection."
	msg := tgbotapi.NewMessage(userID, testMsg)
	msg.ParseMode = "Markdown"
	
	resp, err := s.bot.Send(msg)
	if err != nil {
		logger.Error("Failed to send test message", "error", err, "user_id", userID, "error_type", fmt.Sprintf("%T", err))
		if apiErr, ok := err.(*tgbotapi.Error); ok {
			// Telegram API specific error
			JSONError(c, AppError{
				Code:       ErrCodeExternalService,
				Message:    "Failed to send message via Telegram",
				Details:    fmt.Sprintf("Telegram error: %s (code: %d)", apiErr.Message, apiErr.Code),
				HTTPStatus: http.StatusBadRequest,
				Err:        err,
			})
			return
		}
		JSONError(c, NewExternalServiceError("Telegram Bot API", err))
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message_id": resp.MessageID,
		"chat_id": resp.Chat.ID,
		"bot_username": s.bot.Self.UserName,
	})
}
