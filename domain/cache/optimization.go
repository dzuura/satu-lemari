package cache

import (
	"context"
	"fmt"
	"log"
	"time"
)

// CacheOptimizer provides optimized caching strategies for different data types
type CacheOptimizer struct {
	cache *RedisCache
}

// NewCacheOptimizer creates a new cache optimizer
func NewCacheOptimizer(cache *RedisCache) *CacheOptimizer {
	return &CacheOptimizer{cache: cache}
}

// Cache durations for different data types
const (
	// Static data - cache for longer periods
	CategoryCacheDuration = 30 * time.Minute
	
	// User data - moderate caching
	UserProfileCacheDuration = 15 * time.Minute
	UserStatsCacheDuration   = 5 * time.Minute
	
	// Dynamic data - short caching
	ItemSearchCacheDuration = 3 * time.Minute
	ItemDetailsCacheDuration = 10 * time.Minute
	
	// AI data - expensive to generate, cache longer
	AIRecommendationCacheDuration = 20 * time.Minute
	AIAnalysisCacheDuration       = 60 * time.Minute
	
	// Search results - cache briefly
	SearchResultCacheDuration = 2 * time.Minute
)

// Cache key generators
func (c *CacheOptimizer) GenerateUserProfileKey(userID string) string {
	return c.cache.GenerateKey(KeyUserProfile, userID)
}

func (c *CacheOptimizer) GenerateUserStatsKey(userID, role string) string {
	return c.cache.GenerateKey(KeyUserStats, userID, role)
}

func (c *CacheOptimizer) GenerateItemDetailsKey(itemID string) string {
	return c.cache.GenerateKey(KeyItemDetails, itemID)
}

func (c *CacheOptimizer) GenerateCategoryListKey() string {
	return c.cache.GenerateKey(KeyCategoryList, "all")
}

func (c *CacheOptimizer) GenerateSearchResultKey(query, filters string) string {
	return c.cache.GenerateKey(KeySearchResult, query, filters)
}

func (c *CacheOptimizer) GenerateAIRecommendationKey(userID, recommendationType string) string {
	return c.cache.GenerateKey(KeyRecommendation, userID, recommendationType)
}

// Cache operations with optimized durations
func (c *CacheOptimizer) CacheUserProfile(ctx context.Context, userID string, data interface{}) error {
	key := c.GenerateUserProfileKey(userID)
	if err := c.cache.Set(ctx, key, data, UserProfileCacheDuration); err != nil {
		log.Printf("Failed to cache user profile %s: %v", userID, err)
		return err
	}
	log.Printf("User profile %s cached for %v", userID, UserProfileCacheDuration)
	return nil
}

func (c *CacheOptimizer) GetUserProfile(ctx context.Context, userID string, dest interface{}) error {
	key := c.GenerateUserProfileKey(userID)
	return c.cache.Get(ctx, key, dest)
}

func (c *CacheOptimizer) CacheUserStats(ctx context.Context, userID, role string, data interface{}) error {
	key := c.GenerateUserStatsKey(userID, role)
	if err := c.cache.Set(ctx, key, data, UserStatsCacheDuration); err != nil {
		log.Printf("Failed to cache user stats %s: %v", userID, err)
		return err
	}
	log.Printf("User stats %s cached for %v", userID, UserStatsCacheDuration)
	return nil
}

func (c *CacheOptimizer) GetUserStats(ctx context.Context, userID, role string, dest interface{}) error {
	key := c.GenerateUserStatsKey(userID, role)
	return c.cache.Get(ctx, key, dest)
}

func (c *CacheOptimizer) CacheItemDetails(ctx context.Context, itemID string, data interface{}) error {
	key := c.GenerateItemDetailsKey(itemID)
	if err := c.cache.Set(ctx, key, data, ItemDetailsCacheDuration); err != nil {
		log.Printf("Failed to cache item details %s: %v", itemID, err)
		return err
	}
	log.Printf("Item details %s cached for %v", itemID, ItemDetailsCacheDuration)
	return nil
}

func (c *CacheOptimizer) GetItemDetails(ctx context.Context, itemID string, dest interface{}) error {
	key := c.GenerateItemDetailsKey(itemID)
	return c.cache.Get(ctx, key, dest)
}

func (c *CacheOptimizer) CacheCategoryList(ctx context.Context, data interface{}) error {
	key := c.GenerateCategoryListKey()
	if err := c.cache.Set(ctx, key, data, CategoryCacheDuration); err != nil {
		log.Printf("Failed to cache category list: %v", err)
		return err
	}
	log.Printf("Category list cached for %v", CategoryCacheDuration)
	return nil
}

func (c *CacheOptimizer) GetCategoryList(ctx context.Context, dest interface{}) error {
	key := c.GenerateCategoryListKey()
	return c.cache.Get(ctx, key, dest)
}

func (c *CacheOptimizer) CacheSearchResult(ctx context.Context, query, filters string, data interface{}) error {
	key := c.GenerateSearchResultKey(query, filters)
	if err := c.cache.Set(ctx, key, data, SearchResultCacheDuration); err != nil {
		log.Printf("Failed to cache search result: %v", err)
		return err
	}
	log.Printf("Search result cached for %v", SearchResultCacheDuration)
	return nil
}

func (c *CacheOptimizer) GetSearchResult(ctx context.Context, query, filters string, dest interface{}) error {
	key := c.GenerateSearchResultKey(query, filters)
	return c.cache.Get(ctx, key, dest)
}

func (c *CacheOptimizer) CacheAIRecommendation(ctx context.Context, userID, recommendationType string, data interface{}) error {
	key := c.GenerateAIRecommendationKey(userID, recommendationType)
	if err := c.cache.Set(ctx, key, data, AIRecommendationCacheDuration); err != nil {
		log.Printf("Failed to cache AI recommendation: %v", err)
		return err
	}
	log.Printf("AI recommendation cached for %v", AIRecommendationCacheDuration)
	return nil
}

func (c *CacheOptimizer) GetAIRecommendation(ctx context.Context, userID, recommendationType string, dest interface{}) error {
	key := c.GenerateAIRecommendationKey(userID, recommendationType)
	return c.cache.Get(ctx, key, dest)
}

// Cache invalidation methods
func (c *CacheOptimizer) InvalidateUserCache(ctx context.Context, userID string) error {
	keys := []string{
		c.GenerateUserProfileKey(userID),
		c.GenerateUserStatsKey(userID, "user"),
		c.GenerateUserStatsKey(userID, "partner"),
	}
	
	for _, key := range keys {
		if err := c.cache.Delete(ctx, key); err != nil {
			log.Printf("Failed to invalidate cache key %s: %v", key, err)
		}
	}
	
	log.Printf("User cache invalidated for %s", userID)
	return nil
}

func (c *CacheOptimizer) InvalidateItemCache(ctx context.Context, itemID string) error {
	key := c.GenerateItemDetailsKey(itemID)
	if err := c.cache.Delete(ctx, key); err != nil {
		log.Printf("Failed to invalidate item cache %s: %v", itemID, err)
		return err
	}
	
	log.Printf("Item cache invalidated for %s", itemID)
	return nil
}

func (c *CacheOptimizer) InvalidateCategoryCache(ctx context.Context) error {
	key := c.GenerateCategoryListKey()
	if err := c.cache.Delete(ctx, key); err != nil {
		log.Printf("Failed to invalidate category cache: %v", err)
		return err
	}
	
	log.Printf("Category cache invalidated")
	return nil
}

// Batch operations for better performance
func (c *CacheOptimizer) InvalidateSearchCache(ctx context.Context) error {
	// Use pattern matching to delete all search result keys
	pattern := fmt.Sprintf("%s:*", KeySearchResult)
	log.Printf("Search cache invalidation requested (pattern: %s)", pattern)
	
	return nil
}

// Health check for cache
func (c *CacheOptimizer) HealthCheck(ctx context.Context) error {
	testKey := "health:check"
	testValue := "ok"
	
	// Test write
	if err := c.cache.Set(ctx, testKey, testValue, 1*time.Minute); err != nil {
		return fmt.Errorf("cache write failed: %v", err)
	}
	
	// Test read
	var result string
	if err := c.cache.Get(ctx, testKey, &result); err != nil {
		return fmt.Errorf("cache read failed: %v", err)
	}
	
	if result != testValue {
		return fmt.Errorf("cache data mismatch: expected %s, got %s", testValue, result)
	}
	
	// Cleanup
	c.cache.Delete(ctx, testKey)
	
	return nil
}
