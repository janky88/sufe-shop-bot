package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	
	"shop-bot/internal/cache"
	logger "shop-bot/internal/log"
)

// CachedStore wraps store operations with caching
type CachedStore struct {
	db    *gorm.DB
	cache *cache.Client
}

// NewCachedStore creates a new cached store
func NewCachedStore(db *gorm.DB, cache *cache.Client) *CachedStore {
	return &CachedStore{
		db:    db,
		cache: cache,
	}
}

// GetOrCreateUserCached gets user with caching
func (s *CachedStore) GetOrCreateUserCached(ctx context.Context, tgUserID int64, username string) (*User, error) {
	// Try cache first
	cacheKey := cache.GetUserKey(tgUserID)
	var user User
	
	if err := s.cache.Get(ctx, cacheKey, &user); err == nil {
		return &user, nil
	}
	
	// Get from database
	dbUser, err := GetOrCreateUser(s.db, tgUserID, username)
	if err != nil {
		return nil, err
	}
	
	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, dbUser, cache.CacheTTLUser); err != nil {
		logger.Error("Failed to cache user", "error", err, "user_id", tgUserID)
	}
	
	return dbUser, nil
}

// GetProductCached gets product with caching
func (s *CachedStore) GetProductCached(ctx context.Context, productID uint) (*Product, error) {
	// Try cache first
	cacheKey := cache.GetProductKey(productID)
	var product Product
	
	if err := s.cache.Get(ctx, cacheKey, &product); err == nil {
		return &product, nil
	}
	
	// Get from database
	dbProduct, err := GetProduct(s.db, productID)
	if err != nil {
		return nil, err
	}
	
	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, dbProduct, cache.CacheTTLProduct); err != nil {
		logger.Error("Failed to cache product", "error", err, "product_id", productID)
	}
	
	return dbProduct, nil
}

// GetActiveProductsCached gets active products with caching
func (s *CachedStore) GetActiveProductsCached(ctx context.Context) ([]Product, error) {
	// Try cache first
	var products []Product
	
	if err := s.cache.Get(ctx, cache.KeyProductList, &products); err == nil {
		return products, nil
	}
	
	// Get from database
	dbProducts, err := GetActiveProducts(s.db)
	if err != nil {
		return nil, err
	}
	
	// Cache the result
	if err := s.cache.Set(ctx, cache.KeyProductList, dbProducts, cache.CacheTTLProduct); err != nil {
		logger.Error("Failed to cache product list", "error", err)
	}
	
	return dbProducts, nil
}

// CountAvailableCodesCached gets stock count with caching
func (s *CachedStore) CountAvailableCodesCached(ctx context.Context, productID uint) (int64, error) {
	// Try cache first
	cacheKey := cache.GetStockKey(productID)
	var count int64
	
	if err := s.cache.Get(ctx, cacheKey, &count); err == nil {
		return count, nil
	}
	
	// Get from database
	dbCount, err := CountAvailableCodes(s.db, productID)
	if err != nil {
		return 0, err
	}
	
	// Cache the result (short TTL for stock)
	if err := s.cache.Set(ctx, cacheKey, dbCount, cache.CacheTTLStock); err != nil {
		logger.Error("Failed to cache stock count", "error", err, "product_id", productID)
	}
	
	return dbCount, nil
}

// InvalidateProductCache invalidates product-related caches
func (s *CachedStore) InvalidateProductCache(ctx context.Context, productID uint) {
	// Delete specific product cache
	s.cache.Delete(ctx, cache.GetProductKey(productID))
	
	// Delete product list cache
	s.cache.Delete(ctx, cache.KeyProductList)
	
	// Delete stock cache
	s.cache.Delete(ctx, cache.GetStockKey(productID))
}

// InvalidateUserCache invalidates user cache
func (s *CachedStore) InvalidateUserCache(ctx context.Context, tgUserID int64) {
	s.cache.Delete(ctx, cache.GetUserKey(tgUserID))
}

// GetGroupCached gets group with caching
func (s *CachedStore) GetGroupCached(ctx context.Context, tgGroupID int64) (*Group, error) {
	// Try cache first
	cacheKey := cache.GetGroupKey(tgGroupID)
	var group Group
	
	if err := s.cache.Get(ctx, cacheKey, &group); err == nil {
		return &group, nil
	}
	
	// Get from database
	dbGroup, err := GetGroup(s.db, tgGroupID)
	if err != nil {
		return nil, err
	}
	
	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, dbGroup, cache.CacheTTLGroup); err != nil {
		logger.Error("Failed to cache group", "error", err, "group_id", tgGroupID)
	}
	
	return dbGroup, nil
}

// GetActiveGroupsCached gets active groups with caching
func (s *CachedStore) GetActiveGroupsCached(ctx context.Context) ([]Group, error) {
	// Try cache first
	var groups []Group
	
	if err := s.cache.Get(ctx, cache.KeyActiveGroups, &groups); err == nil {
		return groups, nil
	}
	
	// Get from database
	dbGroups, err := GetActiveGroups(s.db)
	if err != nil {
		return nil, err
	}
	
	// Cache the result
	if err := s.cache.Set(ctx, cache.KeyActiveGroups, dbGroups, cache.CacheTTLGroup); err != nil {
		logger.Error("Failed to cache active groups", "error", err)
	}
	
	return dbGroups, nil
}

// InvalidateGroupCache invalidates group-related caches
func (s *CachedStore) InvalidateGroupCache(ctx context.Context, tgGroupID int64) {
	// Delete specific group cache
	s.cache.Delete(ctx, cache.GetGroupKey(tgGroupID))
	
	// Delete active groups cache
	s.cache.Delete(ctx, cache.KeyActiveGroups)
}