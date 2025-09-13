package httpadmin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"shop-bot/internal/store"
	logger "shop-bot/internal/log"
	payment "shop-bot/internal/payment/epay"
)

// handleSettings shows the settings page
func (s *Server) handleSettings(c *gin.Context) {
	// Get currency settings
	currency, symbol := store.GetCurrencySettings(s.db, nil)

	// Get order settings
	orderSettings, err := store.GetSettingsMap(s.db)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Failed to load settings",
		})
		return
	}

	// Get order statistics
	orderStats, err := store.GetOrderStats(s.db)
	if err != nil {
		orderStats = make(map[string]int64)
	}

	// Get core settings from config
	coreSettings := gin.H{
		"admin_token": strings.Repeat("*", 20), // Mask the token
		"bot_token": strings.Repeat("*", 20), // Mask the token
		"admin_telegram_ids": s.config.AdminTelegramIDs,
	}

	// Get payment settings from config
	paymentSettings := gin.H{
		"epay_pid": s.config.EpayPID,
		"epay_key": strings.Repeat("*", 20), // Mask the key
		"epay_gateway": s.config.EpayGateway,
		"base_url": s.config.BaseURL,
	}

	// Get currency list
	currencies := []struct {
		Code   string
		Symbol string
		Name   string
	}{
		{"CNY", "¥", "人民币"},
		{"USD", "$", "美元"},
		{"EUR", "€", "欧元"},
		{"GBP", "£", "英镑"},
		{"JPY", "¥", "日元"},
		{"HKD", "$", "港币"},
		{"TWD", "NT$", "新台币"},
		{"KRW", "₩", "韩元"},
		{"SGD", "$", "新加坡元"},
		{"AUD", "$", "澳元"},
		{"CAD", "$", "加元"},
		{"THB", "฿", "泰铢"},
		{"MYR", "RM", "马来西亚令吉"},
		{"PHP", "₱", "菲律宾比索"},
		{"IDR", "Rp", "印尼盾"},
		{"VND", "₫", "越南盾"},
		{"INR", "₹", "印度卢比"},
		{"RUB", "₽", "俄罗斯卢布"},
		{"BRL", "R$", "巴西雷亚尔"},
		{"MXN", "$", "墨西哥比索"},
	}

	c.HTML(http.StatusOK, "settings.html", gin.H{
		"currency":      currency,
		"symbol":        symbol,
		"currencies":    currencies,
		"orderSettings": orderSettings,
		"orderStats":    orderStats,
		"coreSettings": coreSettings,
		"paymentSettings": paymentSettings,
	})
}

// handleSaveSettings saves settings via API
func (s *Server) handleSaveSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	
	// Save each setting
	for key, value := range req {
		var description, settingType string
		
		switch key {
		case "order_expire_hours":
			description = "订单过期时间（小时）"
			settingType = "int"
			// Validate
			if hours, err := strconv.Atoi(value); err != nil || hours < 1 || hours > 168 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expire hours"})
				return
			}
		case "order_cleanup_days":
			description = "清理过期订单的天数"
			settingType = "int"
			// Validate
			if days, err := strconv.Atoi(value); err != nil || days < 1 || days > 365 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cleanup days"})
				return
			}
		case "enable_auto_expire":
			description = "启用订单自动过期"
			settingType = "bool"
			if value != "true" && value != "false" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid boolean value"})
				return
			}
		case "enable_auto_cleanup":
			description = "启用过期订单自动清理"
			settingType = "bool"
			if value != "true" && value != "false" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid boolean value"})
				return
			}
		default:
			continue // Skip unknown settings
		}
		
		if err := store.SetSetting(s.db, key, value, description, settingType); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save setting"})
			return
		}
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Settings saved successfully"})
}

// handleExpireOrders manually triggers order expiration
func (s *Server) handleExpireOrders(c *gin.Context) {
	if err := store.ExpirePendingOrders(s.db); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// Get count of expired orders for feedback
	count, _ := store.GetExpiredOrdersCount(s.db)
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Orders expired successfully",
		"count":   count,
	})
}

// handleCleanupOrders manually triggers order cleanup
func (s *Server) handleCleanupOrders(c *gin.Context) {
	// Get count before cleanup
	countBefore, _ := store.GetExpiredOrdersCount(s.db)
	
	if err := store.CleanupExpiredOrders(s.db); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// Get count after cleanup
	countAfter, _ := store.GetExpiredOrdersCount(s.db)
	cleanedCount := countBefore - countAfter
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Orders cleaned up successfully",
		"count":   cleanedCount,
	})
}

// handleSaveCoreSettings saves core system settings
func (s *Server) handleSaveCoreSettings(c *gin.Context) {
	var req struct {
		AdminToken        string `json:"admin_token"`
		BotToken          string `json:"bot_token"`
		AdminTelegramIDs  string `json:"admin_telegram_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	// Build update map
	updates := make(map[string]string)

	// Add updates only for non-masked values
	if req.AdminToken != "" && !strings.Contains(req.AdminToken, "*") {
		updates["admin_token"] = req.AdminToken
	}

	if req.BotToken != "" && !strings.Contains(req.BotToken, "*") {
		updates["bot_token"] = req.BotToken
	}

	updates["admin_telegram_ids"] = req.AdminTelegramIDs

	// Update and reload configuration if config manager is available
	if s.configManager != nil {
		if err := s.configManager.UpdateAndReload(updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败: " + err.Error()})
			return
		}
	} else {
		// Fallback to direct database update
		tx := s.db.Begin()

		for key, value := range updates {
			if err := store.SetSystemSetting(tx, key, value); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败"})
				return
			}
		}

		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败"})
			return
		}
	}

	// Process admin telegram IDs to create/update admin users
	if req.AdminTelegramIDs != "" {
		adminIDs := strings.Split(req.AdminTelegramIDs, ",")
		for _, idStr := range adminIDs {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}

			telegramID, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				logger.Error("Invalid admin telegram ID", "id", idStr, "error", err)
				continue
			}

			// Check if admin user exists
			var adminUser store.AdminUser
			result := s.db.Where("telegram_id = ?", telegramID).First(&adminUser)

			if result.Error != nil {
				// Create new admin user
				adminUser = store.AdminUser{
					Username:            "admin_" + idStr,
					TelegramID:          &telegramID,
					ReceiveNotifications: true,
					IsActive:            true,
				}

				if err := s.db.Create(&adminUser).Error; err != nil {
					logger.Error("Failed to create admin user", "telegram_id", telegramID, "error", err)
				} else {
					logger.Info("Created admin user", "telegram_id", telegramID)
				}
			} else {
				// Update existing admin user
				adminUser.IsActive = true
				adminUser.ReceiveNotifications = true
				s.db.Save(&adminUser)
				logger.Info("Updated admin user", "telegram_id", telegramID)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "核心设置已保存"})
}

// handleSavePaymentSettings saves payment gateway settings
func (s *Server) handleSavePaymentSettings(c *gin.Context) {
	var req struct {
		EpayPID     string `json:"epay_pid"`
		EpayKey     string `json:"epay_key"`
		EpayGateway string `json:"epay_gateway"`
		BaseURL     string `json:"base_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	// Build update map
	updates := make(map[string]string)

	updates["epay_pid"] = req.EpayPID
	updates["epay_gateway"] = req.EpayGateway
	updates["base_url"] = req.BaseURL

	// Add epay key only if not masked
	if req.EpayKey != "" && !strings.Contains(req.EpayKey, "*") {
		updates["epay_key"] = req.EpayKey
	}

	// Update and reload configuration if config manager is available
	if s.configManager != nil {
		if err := s.configManager.UpdateAndReload(updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败: " + err.Error()})
			return
		}

		// Update payment client if configuration changed
		if s.config.EpayPID != "" && s.config.EpayKey != "" && s.config.EpayGateway != "" {
			s.epay = payment.NewClient(s.config.EpayPID, s.config.EpayKey, s.config.EpayGateway)
			logger.Info("Payment client updated with new configuration")
		}
	} else {
		// Fallback to direct database update
		tx := s.db.Begin()

		for key, value := range updates {
			if err := store.SetSystemSetting(tx, key, value); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败"})
				return
			}
		}

		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "支付设置已保存"})
}