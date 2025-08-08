package httpadmin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"shop-bot/internal/store"
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