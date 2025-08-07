package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Order metrics
	OrdersCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_bot_orders_created_total",
		Help: "The total number of orders created",
	})
	
	OrdersPaid = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_bot_orders_paid_total",
		Help: "The total number of orders paid",
	})
	
	OrdersDelivered = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_bot_orders_delivered_total",
		Help: "The total number of orders delivered",
	})
	
	OrdersNoStock = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_bot_orders_no_stock_total",
		Help: "The total number of orders with no stock after payment",
	})
	
	// Revenue metric
	RevenueTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shop_bot_revenue_cents_total",
		Help: "The total revenue in cents",
	}, []string{"product"})
	
	// Stock metrics
	StockAvailable = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shop_bot_stock_available",
		Help: "Current available stock per product",
	}, []string{"product_id", "product_name"})
	
	// Payment callback metrics
	PaymentCallbacksReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_bot_payment_callbacks_received_total",
		Help: "The total number of payment callbacks received",
	})
	
	PaymentCallbacksFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_bot_payment_callbacks_failed_total",
		Help: "The total number of payment callbacks that failed",
	})
	
	// Bot message metrics
	BotMessagesReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shop_bot_messages_received_total",
		Help: "The total number of messages received by the bot",
	}, []string{"type"}) // type: command, callback, text
	
	BotMessagesSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_bot_messages_sent_total",
		Help: "The total number of messages sent by the bot",
	})
	
	// HTTP request metrics
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "shop_bot_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})
	
	// Database metrics
	DBQueriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "shop_bot_db_queries_total",
		Help: "The total number of database queries",
	}, []string{"operation"}) // operation: select, insert, update, delete
	
	// Active users gauge
	ActiveUsers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "shop_bot_active_users",
		Help: "Number of active users in the last 24 hours",
	})
)