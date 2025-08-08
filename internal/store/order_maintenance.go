package store

import (
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
	logger "shop-bot/internal/log"
)

// ExpirePendingOrders marks old pending orders as expired
func ExpirePendingOrders(db *gorm.DB) error {
	// Get expiration hours setting
	expireHoursStr, err := GetSetting(db, SettingOrderExpireHours)
	if err != nil {
		return err
	}
	
	expireHours, err := strconv.Atoi(expireHoursStr)
	if err != nil {
		expireHours = 24 // Default to 24 hours
	}
	
	// Check if auto-expire is enabled
	enabledStr, err := GetSetting(db, SettingEnableAutoExpire)
	if err != nil {
		return err
	}
	
	if enabledStr != "true" {
		logger.Info("Order auto-expire is disabled")
		return nil
	}
	
	// Calculate expiration time
	expirationTime := time.Now().Add(-time.Duration(expireHours) * time.Hour)
	
	// Update pending orders to expired
	result := db.Model(&Order{}).
		Where("status = ? AND created_at < ?", "pending", expirationTime).
		Update("status", "expired")
	
	if result.Error != nil {
		return fmt.Errorf("failed to expire orders: %w", result.Error)
	}
	
	if result.RowsAffected > 0 {
		logger.Info("Expired orders", "count", result.RowsAffected)
	}
	
	return nil
}

// CleanupExpiredOrders deletes old expired orders
func CleanupExpiredOrders(db *gorm.DB) error {
	// Get cleanup days setting
	cleanupDaysStr, err := GetSetting(db, SettingOrderCleanupDays)
	if err != nil {
		return err
	}
	
	cleanupDays, err := strconv.Atoi(cleanupDaysStr)
	if err != nil {
		cleanupDays = 7 // Default to 7 days
	}
	
	// Check if auto-cleanup is enabled
	enabledStr, err := GetSetting(db, SettingEnableAutoCleanup)
	if err != nil {
		return err
	}
	
	if enabledStr != "true" {
		logger.Info("Order auto-cleanup is disabled")
		return nil
	}
	
	// Calculate cleanup time
	cleanupTime := time.Now().Add(-time.Duration(cleanupDays) * 24 * time.Hour)
	
	// Delete old expired orders
	result := db.Where("status = ? AND created_at < ?", "expired", cleanupTime).Delete(&Order{})
	
	if result.Error != nil {
		return fmt.Errorf("failed to cleanup orders: %w", result.Error)
	}
	
	if result.RowsAffected > 0 {
		logger.Info("Cleaned up expired orders", "count", result.RowsAffected)
	}
	
	return nil
}

// GetOrderStats returns order statistics
func GetOrderStats(db *gorm.DB) (map[string]int64, error) {
	stats := make(map[string]int64)
	
	// Count orders by status
	statuses := []string{"pending", "paid", "delivered", "paid_no_stock", "failed_delivery", "expired"}
	
	for _, status := range statuses {
		var count int64
		if err := db.Model(&Order{}).Where("status = ?", status).Count(&count).Error; err != nil {
			return nil, err
		}
		stats[status] = count
	}
	
	// Count total orders
	var total int64
	if err := db.Model(&Order{}).Count(&total).Error; err != nil {
		return nil, err
	}
	stats["total"] = total
	
	return stats, nil
}

// GetExpiredOrdersCount returns the count of expired orders
func GetExpiredOrdersCount(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&Order{}).Where("status = ?", "expired").Count(&count).Error
	return count, err
}

// ManualExpireOrder manually expires a specific order
func ManualExpireOrder(db *gorm.DB, orderID uint) error {
	result := db.Model(&Order{}).
		Where("id = ? AND status = ?", orderID, "pending").
		Update("status", "expired")
	
	if result.Error != nil {
		return result.Error
	}
	
	if result.RowsAffected == 0 {
		return fmt.Errorf("order not found or not in pending status")
	}
	
	return nil
}