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

// GetUserPaidOrders retrieves only paid orders (delivered or deposit) for a specific user with pagination
func GetUserPaidOrders(db *gorm.DB, userID uint, limit, offset int) ([]Order, error) {
	var orders []Order
	err := db.Where("user_id = ? AND status IN (?, ?)", userID, "delivered", "deposit").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("Product").
		Find(&orders).Error
	return orders, err
}

// GetUserPaidOrderCount returns total paid order count for a user
func GetUserPaidOrderCount(db *gorm.DB, userID uint) (int64, error) {
	var count int64
	err := db.Model(&Order{}).
		Where("user_id = ? AND status IN (?, ?)", userID, "delivered", "deposit").
		Count(&count).Error
	return count, err
}

// GetOrderCode retrieves the code associated with an order
func GetOrderCode(db *gorm.DB, orderID uint) (string, error) {
	var code Code
	err := db.Where("order_id = ?", orderID).First(&code).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return code.Code, nil
}