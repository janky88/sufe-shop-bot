package httpadmin

import (
	"html/template"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	
	"shop-bot/internal/broadcast"
	"shop-bot/internal/config"
	logger "shop-bot/internal/log"
	"shop-bot/internal/payment/epay"
)

// App interface to avoid circular dependency
type App interface {
	GetDB() *gorm.DB
	GetBot() interface{ GetAPI() *tgbotapi.BotAPI }
	GetBroadcast() *broadcast.Service
	GetConfig() *config.Config
}

// Server represents the HTTP admin server
type Server struct {
	adminToken string
	app        App
	
	// Direct references for convenience
	db         *gorm.DB
	bot        *tgbotapi.BotAPI
	broadcast  *broadcast.Service
	config     *config.Config
	epay       *epay.Client
}

// NewServerWithApp creates a new server with app instance
func NewServerWithApp(adminToken string, app App) *Server {
	s := &Server{
		adminToken: adminToken,
		app:        app,
		db:         app.GetDB(),
		config:     app.GetConfig(),
		broadcast:  app.GetBroadcast(),
	}
	
	// Get bot API
	if botInstance := app.GetBot(); botInstance != nil {
		s.bot = botInstance.GetAPI()
	}
	
	// Initialize epay client
	cfg := app.GetConfig()
	if cfg.EpayPID != "" && cfg.EpayKey != "" && cfg.EpayGateway != "" {
		s.epay = epay.NewClient(cfg.EpayPID, cfg.EpayKey, cfg.EpayGateway)
	}
	
	return s
}

// SetupRoutes sets up all HTTP routes
func (s *Server) SetupRoutes(r *gin.Engine) {
	// Load HTML templates
	r.LoadHTMLGlob("templates/*")
	
	// Add template functions
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
	})
	
	// Health check
	r.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	
	// Metrics endpoint
	r.GET("/metrics", func(c *gin.Context) {
		promhttp.HandlerFor(
			prometheus.DefaultGatherer,
			promhttp.HandlerOpts{},
		).ServeHTTP(c.Writer, c.Request)
	})
	
	// Add request logging middleware
	r.Use(s.requestLogger())
	
	// Payment routes (no auth required for callbacks)
	payment := r.Group("/payment")
	{
		payment.POST("/epay/notify", s.handleEpayNotify)
		payment.GET("/return", s.handlePaymentReturn)
	}
	
	// Admin routes with auth
	admin := r.Group("/admin")
	admin.Use(s.authMiddleware())
	{
		// Product management
		admin.GET("/products", s.handleProductList)
		admin.POST("/products", s.handleProductCreate)
		admin.PUT("/products/:id", s.handleProductUpdate)
		admin.DELETE("/products/:id", s.handleProductDelete)
		
		// Inventory management
		admin.GET("/products/:id/codes", s.handleProductCodes)
		admin.POST("/products/:id/codes/upload", s.handleCodesUpload)
		
		// Order management
		admin.GET("/orders", s.handleOrderList)
		
		// Recharge card management
		admin.GET("/recharge-cards", s.handleRechargeCardList)
		admin.POST("/recharge-cards/generate", s.handleRechargeCardGenerate)
		admin.DELETE("/recharge-cards/:id", s.handleRechargeCardDelete)
		
		// Message template management
		admin.GET("/templates", s.handleTemplateList)
		admin.POST("/templates/:id", s.handleTemplateUpdate)
		
		// Broadcast management
		admin.GET("/broadcast", s.handleBroadcastPage)
		admin.POST("/broadcast/send", s.handleBroadcastSend)
		admin.GET("/broadcast/history", s.handleBroadcastHistory)
		
		// Admin dashboard
		admin.GET("/", s.handleAdminDashboard)
	}
}

// authMiddleware checks admin token
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token != "Bearer "+s.adminToken {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

// toFloat64 converts interface to float64
func toFloat64(i interface{}) (float64, error) {
	switch v := i.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	default:
		return 0, nil
	}
}