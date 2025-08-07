package store

import (
	"errors"
	"gorm.io/gorm"
)

var (
	ErrOrderNotFound = errors.New("order not found")
	ErrUnauthorized  = errors.New("unauthorized access")
)

// GetUserOrders retrieves orders for a specific user
func GetUserOrders(db *gorm.DB, userID uint, limit, offset int) ([]Order, error) {
	var orders []Order
	err := db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("Product").
		Find(&orders).Error
	return orders, err
}

// GetUserOrder retrieves a specific order for a user
func GetUserOrder(db *gorm.DB, userID uint, orderID uint) (*Order, error) {
	var order Order
	err := db.Where("id = ? AND user_id = ?", orderID, userID).
		Preload("Product").
		First(&order).Error
	
	if err == gorm.ErrRecordNotFound {
		return nil, ErrOrderNotFound
	}
	
	return &order, err
}

// GetUserOrderCount returns total order count for a user
func GetUserOrderCount(db *gorm.DB, userID uint) (int64, error) {
	var count int64
	err := db.Model(&Order{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// GetUserOrderStats returns order statistics for a user
func GetUserOrderStats(db *gorm.DB, userID uint) (totalOrders, deliveredOrders int64, totalSpent int, err error) {
	// Total orders
	err = db.Model(&Order{}).Where("user_id = ?", userID).Count(&totalOrders).Error
	if err != nil {
		return
	}
	
	// Delivered orders
	err = db.Model(&Order{}).Where("user_id = ? AND status = ?", userID, "delivered").Count(&deliveredOrders).Error
	if err != nil {
		return
	}
	
	// Total spent
	var result struct {
		Total int
	}
	err = db.Model(&Order{}).
		Select("COALESCE(SUM(amount_cents), 0) as total").
		Where("user_id = ? AND status IN (?, ?)", userID, "paid", "delivered").
		Scan(&result).Error
	totalSpent = result.Total
	
	return
}

// SearchUserOrders searches orders by code content
func SearchUserOrders(db *gorm.DB, userID uint, searchTerm string) ([]Order, error) {
	var orders []Order
	
	// First find codes that match the search term
	var codeOrderIDs []uint
	err := db.Model(&Code{}).
		Select("order_id").
		Where("code LIKE ? AND order_id IS NOT NULL", "%"+searchTerm+"%").
		Pluck("order_id", &codeOrderIDs).Error
	
	if err != nil {
		return nil, err
	}
	
	// Then get orders that belong to the user
	if len(codeOrderIDs) > 0 {
		err = db.Where("user_id = ? AND id IN ?", userID, codeOrderIDs).
			Order("created_at DESC").
			Preload("Product").
			Find(&orders).Error
	}
	
	return orders, err
}