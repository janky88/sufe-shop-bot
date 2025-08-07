package store

import (
	"fmt"
	logger "shop-bot/internal/log"
	"gorm.io/gorm"
)

// SeedData creates initial test data if database is empty
func SeedData(db *gorm.DB) error {
	// Check if we already have products
	var count int64
	if err := db.Model(&Product{}).Count(&count).Error; err != nil {
		return err
	}
	
	if count > 0 {
		logger.Info("Database already has products, skipping seed")
		return nil
	}
	
	logger.Info("Seeding database with test data")
	
	// Create test products
	products := []Product{
		{
			Name:       "Azure FT US Outlook",
			PriceCents: 400, // $4.00
			IsActive:   true,
		},
		{
			Name:       "Google Voice Number",
			PriceCents: 1200, // $12.00
			IsActive:   true,
		},
	}
	
	for _, p := range products {
		if err := db.Create(&p).Error; err != nil {
			return fmt.Errorf("failed to create product %s: %w", p.Name, err)
		}
		
		// Create test codes for each product
		codes := generateTestCodes(p.ID, 10)
		if err := db.Create(&codes).Error; err != nil {
			return fmt.Errorf("failed to create codes for product %s: %w", p.Name, err)
		}
		
		logger.Info("Created product with codes", "product", p.Name, "codes", len(codes))
	}
	
	return nil
}

func generateTestCodes(productID uint, count int) []Code {
	codes := make([]Code, count)
	for i := 0; i < count; i++ {
		codes[i] = Code{
			ProductID: productID,
			Code:      fmt.Sprintf("TEST-%d-%04d", productID, i+1),
			IsSold:    false,
		}
	}
	return codes
}