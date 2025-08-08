package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	
	"gorm.io/gorm"
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrCardNotFound        = errors.New("recharge card not found")
	ErrCardAlreadyUsed     = errors.New("recharge card already used")
	ErrCardExpired         = errors.New("recharge card expired")
)

// AddBalance adds balance to user account with transaction record
func AddBalance(db *gorm.DB, userID uint, amountCents int, txType string, description string, rechargeCardID *uint, orderID *uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Lock user record for update
		var user User
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&user, userID).Error; err != nil {
			return err
		}
		
		// Calculate new balance
		newBalance := user.BalanceCents + amountCents
		if newBalance < 0 {
			return ErrInsufficientBalance
		}
		
		// Update user balance
		if err := tx.Model(&user).Update("balance_cents", newBalance).Error; err != nil {
			return err
		}
		
		// Create transaction record
		balanceTx := BalanceTransaction{
			UserID:         userID,
			Type:           txType,
			AmountCents:    amountCents,
			BalanceAfter:   newBalance,
			RechargeCardID: rechargeCardID,
			OrderID:        orderID,
			Description:    description,
		}
		
		if err := tx.Create(&balanceTx).Error; err != nil {
			return err
		}
		
		return nil
	})
}

// UseRechargeCard uses a recharge card to top up balance
func UseRechargeCard(db *gorm.DB, userID uint, cardCode string) (*RechargeCard, error) {
	var card RechargeCard
	
	err := db.Transaction(func(tx *gorm.DB) error {
		// Find and lock the card
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("code = ?", cardCode).
			First(&card).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrCardNotFound
			}
			return err
		}
		
		// Check if already used
		if card.IsUsed {
			return ErrCardAlreadyUsed
		}
		
		// Check expiration
		if card.ExpiresAt != nil && card.ExpiresAt.Before(time.Now()) {
			return ErrCardExpired
		}
		
		// Mark card as used
		now := time.Now()
		card.IsUsed = true
		card.UsedByUserID = &userID
		card.UsedAt = &now
		
		if err := tx.Save(&card).Error; err != nil {
			return err
		}
		
		// Add balance to user
		if err := AddBalance(tx, userID, card.AmountCents, "recharge", 
			fmt.Sprintf("Recharge card: %s", cardCode), &card.ID, nil); err != nil {
			return err
		}
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	return &card, nil
}

// GetUserBalance returns user's current balance
func GetUserBalance(db *gorm.DB, userID uint) (int, error) {
	var user User
	if err := db.Select("balance_cents").First(&user, userID).Error; err != nil {
		return 0, err
	}
	return user.BalanceCents, nil
}

// GetBalanceTransactions returns user's balance transaction history
func GetBalanceTransactions(db *gorm.DB, userID uint, limit, offset int) ([]BalanceTransaction, error) {
	var transactions []BalanceTransaction
	err := db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("RechargeCard").
		Preload("Order").
		Find(&transactions).Error
	return transactions, err
}

// CreateRechargeCards creates multiple recharge cards
func CreateRechargeCards(db *gorm.DB, cards []RechargeCard) error {
	return db.Create(&cards).Error
}

// GetRechargeCardStats returns recharge card statistics
func GetRechargeCardStats(db *gorm.DB) (total, used, expired int64, err error) {
	err = db.Model(&RechargeCard{}).Count(&total).Error
	if err != nil {
		return
	}
	
	err = db.Model(&RechargeCard{}).Where("is_used = ?", true).Count(&used).Error
	if err != nil {
		return
	}
	
	err = db.Model(&RechargeCard{}).
		Where("is_used = ? AND expires_at IS NOT NULL AND expires_at < ?", false, time.Now()).
		Count(&expired).Error
	
	return
}

// GenerateRechargeCardCode generates a unique recharge card code
func GenerateRechargeCardCode(prefix string) string {
	// Generate 8 random bytes
	b := make([]byte, 8)
	rand.Read(b)
	
	// Convert to hex string and make uppercase
	code := strings.ToUpper(hex.EncodeToString(b))
	
	// Format as PREFIX-XXXX-XXXX-XXXX-XXXX
	formatted := fmt.Sprintf("%s-%s-%s-%s-%s",
		prefix,
		code[0:4],
		code[4:8],
		code[8:12],
		code[12:16],
	)
	
	return formatted
}