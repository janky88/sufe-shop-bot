package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var (
	ErrCardMaxUsesReached    = errors.New("recharge card has reached maximum uses")
	ErrCardMaxUsesPerUserReached = errors.New("you have reached the maximum uses for this card")
)

// UseRechargeCardV2 uses a recharge card with usage limits
func UseRechargeCardV2(db *gorm.DB, userID uint, cardCode string) (*RechargeCard, error) {
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
		
		// Check expiration
		if card.ExpiresAt != nil && card.ExpiresAt.Before(time.Now()) {
			return ErrCardExpired
		}
		
		// Check if card has reached max total uses
		if card.MaxUses > 0 && card.UsedCount >= card.MaxUses {
			return ErrCardMaxUsesReached
		}
		
		// Check user's usage for this card
		var usage RechargeCardUsage
		err := tx.Where("recharge_card_id = ? AND user_id = ?", card.ID, userID).
			First(&usage).Error
		
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		
		// Check if user has reached max uses for this card
		if err == nil && card.MaxUsesPerUser > 0 && usage.UseCount >= card.MaxUsesPerUser {
			return ErrCardMaxUsesPerUserReached
		}
		
		// Update or create usage record
		if err == gorm.ErrRecordNotFound {
			// First time use by this user
			usage = RechargeCardUsage{
				RechargeCardID: card.ID,
				UserID:         userID,
				UseCount:       1,
				LastUsedAt:     time.Now(),
			}
			if err := tx.Create(&usage).Error; err != nil {
				return err
			}
		} else {
			// Update existing usage
			usage.UseCount++
			usage.LastUsedAt = time.Now()
			if err := tx.Save(&usage).Error; err != nil {
				return err
			}
		}
		
		// Update card's total used count
		card.UsedCount++
		if card.UsedCount == 1 {
			// For backward compatibility, update old fields on first use
			card.IsUsed = true
			card.UsedByUserID = &userID
			now := time.Now()
			card.UsedAt = &now
		}
		
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

// GenerateRechargeCards generates multiple unique recharge cards
func GenerateRechargeCards(db *gorm.DB, count int, amountCents int, maxUses int, maxUsesPerUser int, expiresAt *time.Time) ([]RechargeCard, error) {
	cards := make([]RechargeCard, 0, count)
	codeMap := make(map[string]bool)
	
	// Check existing codes to avoid duplicates
	var existingCodes []string
	db.Model(&RechargeCard{}).Pluck("code", &existingCodes)
	for _, code := range existingCodes {
		codeMap[code] = true
	}
	
	// Generate unique codes
	for i := 0; i < count; i++ {
		var code string
		for {
			code = GenerateRechargeCardCode("RC")
			if !codeMap[code] {
				codeMap[code] = true
				break
			}
		}
		
		card := RechargeCard{
			Code:           code,
			AmountCents:    amountCents,
			MaxUses:        maxUses,
			MaxUsesPerUser: maxUsesPerUser,
			UsedCount:      0,
			IsUsed:         false,
			ExpiresAt:      expiresAt,
		}
		cards = append(cards, card)
	}
	
	// Batch create cards
	if err := db.Create(&cards).Error; err != nil {
		return nil, err
	}
	
	return cards, nil
}

// GetRechargeCards returns paginated recharge cards
func GetRechargeCards(db *gorm.DB, limit, offset int, showUsed bool) ([]RechargeCard, int64, error) {
	var cards []RechargeCard
	var total int64
	
	query := db.Model(&RechargeCard{})
	if !showUsed {
		query = query.Where("used_count < max_uses OR max_uses = 0")
	}
	
	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	
	// Get paginated results
	err := query.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("UsedBy").
		Find(&cards).Error
	
	return cards, total, err
}

// GetRechargeCardUsages returns usage details for a specific card
func GetRechargeCardUsages(db *gorm.DB, cardID uint) ([]RechargeCardUsage, error) {
	var usages []RechargeCardUsage
	err := db.Where("recharge_card_id = ?", cardID).
		Preload("User").
		Order("last_used_at DESC").
		Find(&usages).Error
	return usages, err
}

// DeleteRechargeCard deletes an unused recharge card
func DeleteRechargeCard(db *gorm.DB, cardID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var card RechargeCard
		if err := tx.First(&card, cardID).Error; err != nil {
			return err
		}
		
		// Only allow deletion if not used
		if card.UsedCount > 0 {
			return errors.New("cannot delete used recharge card")
		}
		
		return tx.Delete(&card).Error
	})
}

// GetRechargeCardStatsV2 returns detailed recharge card statistics
func GetRechargeCardStatsV2(db *gorm.DB) (total, active, fullyUsed, expired int64, err error) {
	// Total cards
	err = db.Model(&RechargeCard{}).Count(&total).Error
	if err != nil {
		return
	}
	
	// Active cards (not fully used and not expired)
	err = db.Model(&RechargeCard{}).
		Where("(used_count < max_uses OR max_uses = 0) AND (expires_at IS NULL OR expires_at > ?)", time.Now()).
		Count(&active).Error
	if err != nil {
		return
	}
	
	// Fully used cards
	err = db.Model(&RechargeCard{}).
		Where("max_uses > 0 AND used_count >= max_uses").
		Count(&fullyUsed).Error
	if err != nil {
		return
	}
	
	// Expired cards
	err = db.Model(&RechargeCard{}).
		Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Count(&expired).Error
	
	return
}