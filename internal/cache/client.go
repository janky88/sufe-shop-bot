package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	
	logger "shop-bot/internal/log"
)

// Client represents a cache client
type Client struct {
	redis  *redis.Client
	prefix string
}

// NewClient creates a new cache client
func NewClient(redisURL string) (*Client, error) {
	if redisURL == "" {
		return &Client{}, nil // No cache
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}

	client := redis.NewClient(opt)
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	logger.Info("Connected to Redis cache")
	
	return &Client{
		redis:  client,
		prefix: "shopbot:",
	}, nil
}

// IsEnabled checks if cache is enabled
func (c *Client) IsEnabled() bool {
	return c.redis != nil
}

// Get retrieves a value from cache
func (c *Client) Get(ctx context.Context, key string, value interface{}) error {
	if !c.IsEnabled() {
		return redis.Nil
	}

	data, err := c.redis.Get(ctx, c.prefix+key).Bytes()
	if err != nil {
		return err
	}

	return json.Unmarshal(data, value)
}

// Set stores a value in cache
func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if !c.IsEnabled() {
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return c.redis.Set(ctx, c.prefix+key, data, ttl).Err()
}

// Delete removes a value from cache
func (c *Client) Delete(ctx context.Context, key string) error {
	if !c.IsEnabled() {
		return nil
	}

	return c.redis.Del(ctx, c.prefix+key).Err()
}

// DeletePattern removes all keys matching a pattern
func (c *Client) DeletePattern(ctx context.Context, pattern string) error {
	if !c.IsEnabled() {
		return nil
	}

	iter := c.redis.Scan(ctx, 0, c.prefix+pattern, 0).Iterator()
	var keys []string
	
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	
	if err := iter.Err(); err != nil {
		return err
	}
	
	if len(keys) > 0 {
		return c.redis.Del(ctx, keys...).Err()
	}
	
	return nil
}

// Close closes the cache connection
func (c *Client) Close() error {
	if c.redis != nil {
		return c.redis.Close()
	}
	return nil
}

// Cache keys
const (
	KeyUserPrefix     = "user:"
	KeyProductPrefix  = "product:"
	KeyProductList    = "products:list"
	KeyStockPrefix    = "stock:"
	KeyGroupPrefix    = "group:"
	KeyActiveGroups   = "groups:active"
	CacheTTLUser      = 5 * time.Minute
	CacheTTLProduct   = 10 * time.Minute
	CacheTTLStock     = 1 * time.Minute
	CacheTTLGroup     = 5 * time.Minute
)

// GetUserKey returns cache key for user
func GetUserKey(tgUserID int64) string {
	return fmt.Sprintf("%s%d", KeyUserPrefix, tgUserID)
}

// GetProductKey returns cache key for product
func GetProductKey(productID uint) string {
	return fmt.Sprintf("%s%d", KeyProductPrefix, productID)
}

// GetStockKey returns cache key for product stock
func GetStockKey(productID uint) string {
	return fmt.Sprintf("%s%d", KeyStockPrefix, productID)
}

// GetGroupKey returns cache key for group
func GetGroupKey(tgGroupID int64) string {
	return fmt.Sprintf("%s%d", KeyGroupPrefix, tgGroupID)
}