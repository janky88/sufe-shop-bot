package app

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	
	"shop-bot/internal/bot"
	"shop-bot/internal/broadcast"
	"shop-bot/internal/cache"
	"shop-bot/internal/config"
	"shop-bot/internal/httpadmin"
	logger "shop-bot/internal/log"
	"shop-bot/internal/worker"
)

// Application holds all application components
type Application struct {
	Config      *config.Config
	DB          *gorm.DB
	Cache       *cache.Client
	Bot         *bot.Bot
	Broadcast   *broadcast.Service
	AdminServer *httpadmin.Server
	RetryWorker *worker.RetryWorker
	
	httpServer  *http.Server
	wg          sync.WaitGroup
}

// New creates a new application instance
func New(cfg *config.Config, db *gorm.DB) (*Application, error) {
	// Initialize cache
	cacheClient, err := cache.NewClient(cfg.GetRedisURL())
	if err != nil {
		logger.Warn("Failed to init cache, running without cache", "error", err)
		cacheClient = &cache.Client{} // Empty cache client
	}
	
	// Initialize Telegram bot
	botInstance, err := bot.New(cfg.BotToken, db)
	if err != nil {
		return nil, fmt.Errorf("failed to init bot: %w", err)
	}
	
	// Initialize broadcast service
	broadcastService := broadcast.NewService(db, botInstance.GetAPI())
	
	// Initialize retry worker
	retryWorker := worker.NewRetryWorker(db, botInstance.GetAPI())
	
	// Create application
	app := &Application{
		Config:      cfg,
		DB:          db,
		Cache:       cacheClient,
		Bot:         botInstance,
		Broadcast:   broadcastService,
		RetryWorker: retryWorker,
	}
	
	// Initialize HTTP admin server with access to bot
	app.AdminServer = httpadmin.NewServerWithApp(cfg.AdminToken, app)
	
	return app, nil
}

// Start starts all application components
func (app *Application) Start(ctx context.Context) error {
	// Start bot (polling or webhook mode)
	if !app.Config.UseWebhook {
		app.wg.Add(1)
		go func() {
			defer app.wg.Done()
			logger.Info("Starting Telegram bot in polling mode")
			if err := app.Bot.Start(ctx); err != nil {
				logger.Error("Bot stopped with error", "error", err)
			}
		}()
	} else {
		// In webhook mode, just set the webhook
		if err := app.Bot.SetWebhook(app.Config.WebhookURL + "/webhook/" + app.Bot.GetAPI().Token); err != nil {
			return fmt.Errorf("failed to set webhook: %w", err)
		}
		logger.Info("Webhook set", "url", app.Config.WebhookURL)
	}
	
	// Start HTTP server
	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		app.startHTTPServer(ctx)
	}()
	
	// Start retry worker
	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		logger.Info("Starting retry worker")
		app.RetryWorker.Start(ctx)
	}()
	
	return nil
}

// startHTTPServer starts the HTTP server
func (app *Application) startHTTPServer(ctx context.Context) {
	router := app.setupRouter()
	
	addr := ":8080"
	if app.Config.UseWebhook && app.Config.WebhookPort > 0 {
		addr = fmt.Sprintf(":%d", app.Config.WebhookPort)
	}
	
	app.httpServer = &http.Server{
		Addr:    addr,
		Handler: router,
	}
	
	logger.Info("Starting HTTP server", "addr", addr)
	
	go func() {
		<-ctx.Done()
		app.httpServer.Shutdown(context.Background())
	}()
	
	if err := app.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("HTTP server error", "error", err)
	}
}

// setupRouter sets up all HTTP routes
func (app *Application) setupRouter() *gin.Engine {
	r := gin.Default()
	
	// Add all admin routes
	app.AdminServer.SetupRoutes(r)
	
	// Add webhook route if enabled
	if app.Config.UseWebhook {
		r.POST("/webhook/:token", app.handleWebhook)
	}
	
	return r
}

// handleWebhook handles Telegram webhook updates
func (app *Application) handleWebhook(c *gin.Context) {
	token := c.Param("token")
	if token != app.Bot.GetAPI().Token {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	
	var update tgbotapi.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		logger.Error("Failed to parse webhook update", "error", err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	
	// Process update asynchronously
	go app.Bot.HandleWebhookUpdate(update)
	
	c.Status(http.StatusOK)
}

// Wait waits for all components to finish
func (app *Application) Wait() {
	app.wg.Wait()
}

// Shutdown gracefully shuts down the application
func (app *Application) Shutdown(ctx context.Context) error {
	logger.Info("Shutting down application...")
	
	// Shutdown HTTP server
	if app.httpServer != nil {
		if err := app.httpServer.Shutdown(ctx); err != nil {
			logger.Error("HTTP server shutdown error", "error", err)
		}
	}
	
	// Close cache
	if app.Cache != nil {
		app.Cache.Close()
	}
	
	return nil
}

// GetDB returns the database instance
func (app *Application) GetDB() *gorm.DB {
	return app.DB
}

// GetBot returns the bot instance
func (app *Application) GetBot() interface{ GetAPI() *tgbotapi.BotAPI } {
	return app.Bot
}

// GetBroadcast returns the broadcast service
func (app *Application) GetBroadcast() *broadcast.Service {
	return app.Broadcast
}

// GetConfig returns the configuration
func (app *Application) GetConfig() *config.Config {
	return app.Config
}