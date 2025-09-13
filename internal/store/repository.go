package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"shop-bot/internal/config"
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

// GetOrCreateUserWithStatus gets existing user or creates new one, returning if user was created
func GetOrCreateUserWithStatus(db *gorm.DB, tgUserID int64, username string) (*User, bool, error) {
	var user User
	
	err := db.Where("tg_user_id = ?", tgUserID).First(&user).Error
	if err == nil {
		return &user, false, nil
	}
	
	if err != gorm.ErrRecordNotFound {
		return nil, false, err
	}
	
	// Create new user
	user = User{
		TgUserID: tgUserID,
		Username: username,
		Language: "en",
	}
	
	if err := db.Create(&user).Error; err != nil {
		return nil, false, err
	}
	
	return &user, true, nil
}

// CreateOrder creates a new order
func CreateOrder(db *gorm.DB, userID, productID uint, amountCents int) (*Order, error) {
	// Generate unique out_trade_no at creation time
	tempID := fmt.Sprintf("%d-%d-%d", userID, productID, time.Now().UnixNano())
	
	order := &Order{
		UserID:         userID,
		ProductID:      &productID,
		AmountCents:    amountCents,
		PaymentAmount:  amountCents, // Initially same as amount, will be updated if balance is used
		Status:         "pending",
		EpayOutTradeNo: tempID, // Temporary unique ID, will be updated when payment is initiated
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
		// Generate unique out_trade_no at creation time
		tempID := fmt.Sprintf("%d-%d-%d", userID, productID, time.Now().UnixNano())
		
		order = &Order{
			UserID:         userID,
			ProductID:      &productID,
			AmountCents:    amountCents,
			BalanceUsed:    balanceUsed,
			PaymentAmount:  paymentAmount,
			Status:         "pending",
			EpayOutTradeNo: tempID, // Temporary unique ID, will be updated when payment is initiated
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

// CreateDepositOrder creates a deposit order (no product)
func CreateDepositOrder(db *gorm.DB, userID uint, amountCents int) (*Order, error) {
	// Generate unique out_trade_no at creation time
	tempID := fmt.Sprintf("DEPOSIT-%d-%d", userID, time.Now().UnixNano())
	
	order := &Order{
		UserID:         userID,
		ProductID:      nil, // No product for deposit orders
		AmountCents:    amountCents,
		PaymentAmount:  amountCents,
		BalanceUsed:    0,
		Status:         "pending",
		EpayOutTradeNo: tempID, // Temporary unique ID, will be updated when payment is initiated
	}
	
	if err := db.Create(order).Error; err != nil {
		return nil, err
	}
	
	// Load associations
	if err := db.Preload("User").First(order, order.ID).Error; err != nil {
		return nil, err
	}
	
	return order, nil
}

// GetSystemSetting retrieves a system setting by key
func GetSystemSetting(db *gorm.DB, key string) (string, error) {
	var setting SystemSetting
	err := db.Where("key = ?", key).First(&setting).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return setting.Value, nil
}

// SetSystemSetting sets a system setting value
func SetSystemSetting(db *gorm.DB, key, value string) error {
	var setting SystemSetting
	err := db.Where("key = ?", key).First(&setting).Error
	
	if err == gorm.ErrRecordNotFound {
		// Create new setting
		setting = SystemSetting{
			Key:   key,
			Value: value,
		}
		return db.Create(&setting).Error
	}
	
	if err != nil {
		return err
	}
	
	// Update existing setting
	return db.Model(&setting).Update("value", value).Error
}

// GetCurrencySettings retrieves currency settings from database or config
func GetCurrencySettings(db *gorm.DB, config *config.Config) (currency string, symbol string) {
	// Try to get from database first
	if db != nil {
		if curr, err := GetSystemSetting(db, "currency"); err == nil && curr != "" {
			currency = curr
		}
		if sym, err := GetSystemSetting(db, "currency_symbol"); err == nil && sym != "" {
			symbol = sym
		}
	}
	
	// Fall back to config if not in database
	if currency == "" && config != nil {
		currency = config.Currency
	}
	if symbol == "" && config != nil {
		symbol = config.CurrencySymbol
	}
	
	// Default values
	if currency == "" {
		currency = "CNY"
	}
	if symbol == "" {
		symbol = "¥"
	}

	return currency, symbol
}

// InitializeAdminsFromConfig initializes admin users based on ADMIN_TELEGRAM_IDS config
func InitializeAdminsFromConfig(db *gorm.DB, cfg *config.Config) error {
	adminIDs := cfg.GetAdminTelegramIDs()
	if len(adminIDs) == 0 {
		return nil
	}

	for i, telegramID := range adminIDs {
		username := fmt.Sprintf("admin%d", i+1)
		if i == 0 {
			username = "admin" // First admin gets the default username
		}

		var adminUser AdminUser
		err := db.Where("telegram_id = ?", telegramID).First(&adminUser).Error

		if err == gorm.ErrRecordNotFound {
			// Create new admin user
			adminUser = AdminUser{
				Username:            username,
				Password:            "", // Password will be set via web interface
				TelegramID:          sql.NullInt64{Int64: telegramID, Valid: true},
				ReceiveNotifications: true,
				IsActive:            true,
			}

			// Check if username already exists
			var existing AdminUser
			if db.Where("username = ?", username).First(&existing).Error == nil {
				// Username exists, add suffix
				adminUser.Username = fmt.Sprintf("%s_%d", username, telegramID)
			}

			if err := db.Create(&adminUser).Error; err != nil {
				return fmt.Errorf("failed to create admin user for telegram ID %d: %w", telegramID, err)
			}

			fmt.Printf("Admin user %s created with Telegram ID: %d\n", adminUser.Username, telegramID)
		} else if err == nil {
			// Update existing admin to ensure notifications are enabled
			if !adminUser.ReceiveNotifications {
				adminUser.ReceiveNotifications = true
				if err := db.Save(&adminUser).Error; err != nil {
					return fmt.Errorf("failed to update admin user: %w", err)
				}
			}
		} else {
			return fmt.Errorf("failed to check admin user: %w", err)
		}
	}

	return nil
}

// GetActiveFAQs returns all active FAQs for a given language
func GetActiveFAQs(db *gorm.DB, language string) ([]FAQ, error) {
	var faqs []FAQ
	err := db.Where("language = ? AND is_active = ?", language, true).
		Order("sort_order ASC, id ASC").
		Find(&faqs).Error
	return faqs, err
}
