package httpadmin

import (
	"context"
	"crypto/md5"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"
	
	"shop-bot/internal/config"
	logger "shop-bot/internal/log"
	"shop-bot/internal/payment/epay"
	"shop-bot/internal/store"
	"shop-bot/internal/bot/messages"
	"shop-bot/internal/metrics"
	"shop-bot/internal/broadcast"
)

type Server struct {
	adminToken string
	db         *gorm.DB
	bot        *tgbotapi.BotAPI
	epay       *epay.Client
	config     *config.Config
	broadcast  *broadcast.Service
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
	var epayClient *epay.Client
	if cfg.EpayPID != "" && cfg.EpayKey != "" && cfg.EpayGateway != "" {
		epayClient = epay.NewClient(cfg.EpayPID, cfg.EpayKey, cfg.EpayGateway)
	}
	
	// Initialize broadcast service
	var broadcastService *broadcast.Service
	if bot != nil {
		broadcastService = broadcast.NewService(db, bot)
	}
	
	return &Server{
		adminToken: adminToken,
		db:         db,
		bot:        bot,
		epay:       epayClient,
		config:     cfg,
		broadcast:  broadcastService,
	}
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()
	
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

	// Webhook route for Telegram (no auth required)
	webhook := r.Group("/webhook")
	{
		webhook.POST("/:token", s.handleWebhook)
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
		
		// Admin dashboard
		admin.GET("/", s.handleAdminDashboard)
	}

	return r
}

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

func (s *Server) handleEpayNotify(c *gin.Context) {
	metrics.PaymentCallbacksReceived.Inc()
	
	// Parse form data
	if err := c.Request.ParseForm(); err != nil {
		traceID := c.GetString("trace_id")
		logger.Error("Failed to parse form", "error", err, "trace_id", traceID)
		metrics.PaymentCallbacksFailed.Inc()
		c.String(http.StatusBadRequest, "fail")
		return
	}
	
	params := c.Request.Form
	traceID := c.GetString("trace_id")
	logger.Info("Received payment callback", "params", params, "trace_id", traceID)
	
	// Verify signature
	if s.epay == nil || !s.epay.VerifyNotify(params) {
		logger.Error("Invalid callback signature")
		c.String(http.StatusBadRequest, "fail")
		return
	}
	
	// Parse notification
	notify := epay.ParseNotify(params)
	
	// Check trade status
	if notify.TradeStatus != "TRADE_SUCCESS" {
		logger.Info("Trade not successful", "status", notify.TradeStatus)
		c.String(http.StatusOK, "success")
		return
	}
	
	// Find order by out_trade_no
	var order store.Order
	if err := s.db.Preload("User").Preload("Product").Where("epay_out_trade_no = ?", notify.OutTradeNo).First(&order).Error; err != nil {
		logger.Error("Order not found", "out_trade_no", notify.OutTradeNo, "error", err)
		c.String(http.StatusBadRequest, "fail")
		return
	}
	
	// Check if already paid (idempotency)
	if order.Status != "pending" {
		logger.Info("Order already processed", "order_id", order.ID, "status", order.Status)
		c.String(http.StatusOK, "success")
		return
	}
	
	// Verify amount
	notifyMoney, _ := strconv.ParseFloat(notify.Money, 64)
	if int(notifyMoney*100) != order.AmountCents {
		logger.Error("Amount mismatch", "expected", order.AmountCents, "received", notifyMoney*100)
		c.String(http.StatusBadRequest, "fail")
		return
	}
	
	// Start transaction to update order and claim code
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Update order status
		now := time.Now()
		updates := map[string]interface{}{
			"status":        "paid",
			"epay_trade_no": notify.TradeNo,
			"paid_at":       &now,
		}
		
		if err := tx.Model(&order).Updates(updates).Error; err != nil {
			return err
		}
		
		// Track metric
		metrics.OrdersPaid.Inc()
		metrics.RevenueTotal.WithLabelValues(order.Product.Name).Add(float64(order.AmountCents))
		
		// Try to claim a code
		ctx := context.Background()
		code, err := store.ClaimOneCodeTx(ctx, tx, order.ProductID, order.ID)
		if err != nil {
			if err == store.ErrNoStock {
				// Update status to paid_no_stock
				if err := tx.Model(&order).Update("status", "paid_no_stock").Error; err != nil {
					return err
				}
				
				// Track no stock metric
				metrics.OrdersNoStock.Inc()
				
				// Send alert to admin
				go s.alertAdminNoStock(&order)
				
				// Send message to user about no stock
				go s.sendNoStockMessage(&order)
				
				return nil // Transaction successful, but no stock
			}
			return err
		}
		
		// Update order status to delivered
		if err := tx.Model(&order).Update("status", "delivered").Error; err != nil {
			return err
		}
		
		// Track delivered metric
		metrics.OrdersDelivered.Inc()
		
		// Send code to user
		go s.sendCodeToUser(&order, code)
		
		return nil
	})
	
	if err != nil {
		logger.Error("Failed to process payment", "error", err, "order_id", order.ID)
		metrics.PaymentCallbacksFailed.Inc()
		c.String(http.StatusInternalServerError, "fail")
		return
	}
	
	logger.Info("Payment processed successfully", "order_id", order.ID)
	c.String(http.StatusOK, "success")
}

func (s *Server) handlePaymentReturn(c *gin.Context) {
	// Simple return page
	c.String(http.StatusOK, "Payment completed. Please check your Telegram for the delivery.")
}

func (s *Server) sendCodeToUser(order *store.Order, code string) {
	if s.bot == nil {
		logger.Error("Bot not initialized, cannot send code")
		return
	}
	
	// Get user language
	lang := messages.GetUserLanguage(order.User.Language, "")
	msgManager := messages.GetManager()
	
	message := msgManager.Format(lang, "order_paid_msg", map[string]interface{}{
		"OrderID":     order.ID,
		"ProductName": order.Product.Name,
		"Code":        code,
	})
	
	msg := tgbotapi.NewMessage(order.User.TgUserID, message)
	msg.ParseMode = "Markdown"
	
	if _, err := s.bot.Send(msg); err != nil {
		logger.Error("Failed to send code to user", "error", err, "user_id", order.User.TgUserID)
		// TODO: Store failed message for retry
	} else {
		logger.Info("Code sent to user", "order_id", order.ID, "user_id", order.User.TgUserID)
	}
}

func (s *Server) sendNoStockMessage(order *store.Order) {
	if s.bot == nil {
		return
	}
	
	// Get user language
	lang := messages.GetUserLanguage(order.User.Language, "")
	msgManager := messages.GetManager()
	
	message := msgManager.Format(lang, "paid_no_stock_msg", map[string]interface{}{
		"OrderID":     order.ID,
		"ProductName": order.Product.Name,
	})
	
	msg := tgbotapi.NewMessage(order.User.TgUserID, message)
	s.bot.Send(msg)
}

func (s *Server) alertAdminNoStock(order *store.Order) {
	logger.Warn("Product out of stock after payment", 
		"order_id", order.ID, 
		"product_id", order.ProductID,
		"product_name", order.Product.Name,
	)
	// TODO: Send notification to admin users
}

// TestCallbackParams generates test callback parameters
func TestCallbackParams(outTradeNo string, money float64) url.Values {
	params := url.Values{}
	params.Set("pid", "test_pid")
	params.Set("trade_no", fmt.Sprintf("TEST%d", time.Now().Unix()))
	params.Set("out_trade_no", outTradeNo)
	params.Set("type", "alipay")
	params.Set("name", "Test Product")
	params.Set("money", fmt.Sprintf("%.2f", money))
	params.Set("trade_status", "TRADE_SUCCESS")
	
	// Generate test signature (using "test_key" as key)
	testClient := epay.NewClient("test_pid", "test_key", "")
	sign := testClient.GenerateSign(params)
	params.Set("sign", sign)
	params.Set("sign_type", "MD5")
	
	return params
}

// GenerateSign exposes sign generation for testing
func (c *epay.Client) GenerateSign(params url.Values) string {
	// Sort parameters by key
	var keys []string
	for k := range params {
		if k != "sign" && k != "sign_type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	
	// Build sign string
	var signParts []string
	for _, k := range keys {
		if k != "" && params.Get(k) != "" {
			signParts = append(signParts, fmt.Sprintf("%s=%s", k, params.Get(k)))
		}
	}
	
	signStr := strings.Join(signParts, "&") + c.Key
	
	// Calculate MD5
	h := md5.New()
	h.Write([]byte(signStr))
	return fmt.Sprintf("%x", h.Sum(nil))
}