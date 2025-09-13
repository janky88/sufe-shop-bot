package config

import (
	"strings"
	"sync"

	"gorm.io/gorm"
	logger "shop-bot/internal/log"
)

// Manager manages configuration loading and reloading
type Manager struct {
	config *Config
	db     *gorm.DB
	mu     sync.RWMutex
}

// NewManager creates a new configuration manager
func NewManager(cfg *Config, db *gorm.DB) *Manager {
	return &Manager{
		config: cfg,
		db:     db,
	}
}

// GetConfig returns the current configuration (thread-safe)
func (m *Manager) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// LoadFromDatabase loads configuration from database
func (m *Manager) LoadFromDatabase() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load system settings from database using raw SQL
	settings := make(map[string]string)

	// Query all system settings
	rows, err := m.db.Raw("SELECT key, value FROM system_settings").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	// Scan results
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		settings[key] = value
	}

	// Update configuration with database values
	if val, ok := settings["admin_token"]; ok && val != "" {
		m.config.AdminToken = val
		logger.Info("Loaded admin_token from database")
	}

	if val, ok := settings["bot_token"]; ok && val != "" {
		m.config.BotToken = val
		logger.Info("Loaded bot_token from database")
	}

	if val, ok := settings["admin_telegram_ids"]; ok {
		m.config.AdminTelegramIDs = val
		m.config.AdminChatIDs = m.config.GetAdminTelegramIDs()
		logger.Info("Loaded admin_telegram_ids from database", "ids", val)
	}

	if val, ok := settings["epay_pid"]; ok {
		m.config.EpayPID = val
		logger.Info("Loaded epay_pid from database")
	}

	if val, ok := settings["epay_key"]; ok && val != "" {
		m.config.EpayKey = val
		logger.Info("Loaded epay_key from database")
	}

	if val, ok := settings["epay_gateway"]; ok {
		m.config.EpayGateway = val
		logger.Info("Loaded epay_gateway from database")
	}

	if val, ok := settings["base_url"]; ok {
		m.config.BaseURL = val
		logger.Info("Loaded base_url from database")
	}

	if val, ok := settings["currency"]; ok {
		m.config.Currency = val
		logger.Info("Loaded currency from database", "currency", val)
	}

	if val, ok := settings["currency_symbol"]; ok {
		m.config.CurrencySymbol = val
		logger.Info("Loaded currency_symbol from database", "symbol", val)
	}

	return nil
}

// ReloadConfig reloads configuration from database
func (m *Manager) ReloadConfig() error {
	logger.Info("Reloading configuration from database")
	return m.LoadFromDatabase()
}

// UpdateAndReload updates configuration in database and reloads
func (m *Manager) UpdateAndReload(updates map[string]string) error {
	// Update database using raw SQL
	tx := m.db.Begin()

	for key, value := range updates {
		// Skip masked values
		if strings.Contains(value, "*") && (key == "admin_token" || key == "bot_token" || key == "epay_key") {
			continue
		}

		// Check if setting exists
		var count int64
		tx.Raw("SELECT COUNT(*) FROM system_settings WHERE key = ?", key).Scan(&count)

		if count > 0 {
			// Update existing setting
			if err := tx.Exec("UPDATE system_settings SET value = ?, updated_at = NOW() WHERE key = ?", value, key).Error; err != nil {
				tx.Rollback()
				return err
			}
		} else {
			// Insert new setting
			if err := tx.Exec("INSERT INTO system_settings (key, value, created_at, updated_at) VALUES (?, ?, NOW(), NOW())", key, value).Error; err != nil {
				tx.Rollback()
				return err
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	// Reload configuration
	return m.ReloadConfig()
}