package store

import (
	"context"
	"errors"
	"fmt"
	"time"
	
	"gorm.io/gorm"
)

var (
	ErrNoStock = errors.New("no available codes in stock")
	ErrClaimFailed = errors.New("failed to claim code")
)

// CountAvailableCodes returns the number of unsold codes for a product
func CountAvailableCodes(db *gorm.DB, productID uint) (int64, error) {
	var count int64
	err := db.Model(&Code{}).
		Where("product_id = ? AND is_sold = ?", productID, false).
		Count(&count).Error
	return count, err
}

// ClaimOneCodeTx claims one available code for an order with concurrency safety
func ClaimOneCodeTx(ctx context.Context, db *gorm.DB, productID uint, orderID uint) (string, error) {
	var claimedCode string
	
	err := db.Transaction(func(tx *gorm.DB) error {
		if IsPostgres(db) {
			// PostgreSQL: Use FOR UPDATE SKIP LOCKED for better concurrency
			var code Code
			err := tx.Raw(`
				SELECT * FROM codes 
				WHERE product_id = ? AND is_sold = false 
				LIMIT 1 
				FOR UPDATE SKIP LOCKED
			`, productID).Scan(&code).Error
			
			if err != nil {
				if err == gorm.ErrRecordNotFound {
					return ErrNoStock
				}
				return err
			}
			
			// Update the code as sold
			result := tx.Model(&Code{}).
				Where("id = ?", code.ID).
				Updates(map[string]interface{}{
					"is_sold": true,
					"sold_at": gorm.Expr("NOW()"),
					"order_id": orderID,
				})
				
			if result.Error != nil {
				return result.Error
			}
			
			claimedCode = code.Code
			
		} else {
			// SQLite: Use UPDATE with LIMIT and check affected rows
			result := tx.Exec(`
				UPDATE codes 
				SET is_sold = 1, sold_at = CURRENT_TIMESTAMP, order_id = ?
				WHERE id IN (
					SELECT id FROM codes 
					WHERE product_id = ? AND is_sold = 0 
					LIMIT 1
				)
			`, orderID, productID)
			
			if result.Error != nil {
				return result.Error
			}
			
			if result.RowsAffected == 0 {
				return ErrNoStock
			}
			
			// Fetch the claimed code
			var code Code
			err := tx.Where("order_id = ?", orderID).First(&code).Error
			if err != nil {
				return fmt.Errorf("failed to fetch claimed code: %w", err)
			}
			
			claimedCode = code.Code
		}
		
		return nil
	})
	
	if err != nil {
		return "", err
	}
	
	return claimedCode, nil
}

// GetProduct fetches a product by ID
func GetProduct(db *gorm.DB, productID uint) (*Product, error) {
	var product Product
	err := db.First(&product, productID).Error
	return &product, err
}

// GetActiveProducts returns all active products
func GetActiveProducts(db *gorm.DB) ([]Product, error) {
	var products []Product
	err := db.Where("is_active = ?", true).Find(&products).Error
	return products, err
}

// GetOrCreateUser gets existing user or creates new one
func GetOrCreateUser(db *gorm.DB, tgUserID int64, username string) (*User, error) {
	var user User
	
	err := db.Where("tg_user_id = ?", tgUserID).First(&user).Error
	if err == nil {
		return &user, nil
	}
	
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	
	// Create new user
	user = User{
		TgUserID: tgUserID,
		Username: username,
		Language: "en",
	}
	
	if err := db.Create(&user).Error; err != nil {
		return nil, err
	}
	
	return &user, nil
}

// CreateOrder creates a new order
func CreateOrder(db *gorm.DB, userID, productID uint, amountCents int) (*Order, error) {
	order := &Order{
		UserID:        userID,
		ProductID:     productID,
		AmountCents:   amountCents,
		PaymentAmount: amountCents, // Initially same as amount, will be updated if balance is used
		Status:        "pending",
	}
	
	if err := db.Create(order).Error; err != nil {
		return nil, err
	}
	
	return order, nil
}

// CreateOrderWithBalance creates an order with balance deduction
func CreateOrderWithBalance(db *gorm.DB, userID, productID uint, amountCents int, useBalance bool) (*Order, error) {
	var order *Order
	
	err := db.Transaction(func(tx *gorm.DB) error {
		// Get user balance
		var user User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		
		balanceUsed := 0
		paymentAmount := amountCents
		
		if useBalance && user.BalanceCents > 0 {
			// Calculate how much balance can be used
			if user.BalanceCents >= amountCents {
				balanceUsed = amountCents
				paymentAmount = 0
			} else {
				balanceUsed = user.BalanceCents
				paymentAmount = amountCents - user.BalanceCents
			}
		}
		
		// Create order
		order = &Order{
			UserID:        userID,
			ProductID:     productID,
			AmountCents:   amountCents,
			BalanceUsed:   balanceUsed,
			PaymentAmount: paymentAmount,
			Status:        "pending",
		}
		
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		
		// If using balance, deduct it immediately
		if balanceUsed > 0 {
			if err := AddBalance(tx, userID, -balanceUsed, "purchase", 
				fmt.Sprintf("Order #%d", order.ID), nil, &order.ID); err != nil {
				return err
			}
			
			// If payment amount is 0, mark order as paid
			if paymentAmount == 0 {
				order.Status = "paid"
				now := time.Now()
				order.PaidAt = &now
				if err := tx.Save(order).Error; err != nil {
					return err
				}
			}
		}
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	return order, nil
}