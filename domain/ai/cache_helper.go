package ai

import (
	"context"
	"crypto/md5"
	"fmt"
	"log"
	"time"

	"github.com/dzuura/satu-lemari/domain/cache"
)

// AICacheHelper provides caching functionality for AI Service
type AICacheHelper struct {
	cache *cache.RedisCache
}

// NewAICacheHelper creates a new AI cache helper
func NewAICacheHelper(redisCache *cache.RedisCache) *AICacheHelper {
	return &AICacheHelper{cache: redisCache}
}

// Cache durations for AI-related data (AI operations are expensive, cache longer)
const (
	// AI Analysis - very expensive, cache for 1 hour
	AIAnalysisCacheDuration = 60 * time.Minute
	
	// AI Recommendations - expensive, cache for 20 minutes
	AIRecommendationCacheDuration = 20 * time.Minute
	
	// Trending recommendations - expensive AI + database, cache for 15 minutes
	TrendingRecommendationCacheDuration = 15 * time.Minute
	
	// Similar items - moderate cost, cache for 10 minutes
	SimilarItemsCacheDuration = 10 * time.Minute
	
	// Smart listing - expensive AI analysis, cache for 30 minutes
	SmartListingCacheDuration = 30 * time.Minute
	
	// Intent parsing - moderate cost, cache for 5 minutes
	IntentParsingCacheDuration = 5 * time.Minute
	
	// Search suggestions - lightweight, cache for 3 minutes
	SearchSuggestionsCacheDuration = 3 * time.Minute
)

// Cache key generators
func (h *AICacheHelper) generateSmartListingKey(description string, images []string) string {
	// Create hash of description + image count for caching
	content := fmt.Sprintf("desc:%s:images:%d", description, len(images))
	hash := fmt.Sprintf("%x", md5.Sum([]byte(content)))
	return h.cache.GenerateKey("ai:smart_listing", hash)
}

func (h *AICacheHelper) generateIntentKey(query string) string {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(query)))
	return h.cache.GenerateKey("ai:intent", hash)
}

func (h *AICacheHelper) generateSuggestionsKey(query string, limit int) string {
	content := fmt.Sprintf("q:%s:limit:%d", query, limit)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(content)))
	return h.cache.GenerateKey("ai:suggestions", hash)
}

func (h *AICacheHelper) generateTrendingKey(limit int) string {
	return h.cache.GenerateKey("ai:trending", fmt.Sprintf("limit:%d", limit))
}

func (h *AICacheHelper) generateSimilarItemsKey(itemID string, limit int) string {
	return h.cache.GenerateKey("ai:similar", itemID, fmt.Sprintf("limit:%d", limit))
}

func (h *AICacheHelper) generatePersonalizedKey(userID string, limit int) string {
	return h.cache.GenerateKey("ai:personalized", userID, fmt.Sprintf("limit:%d", limit))
}

// Cache operations for smart listing
func (h *AICacheHelper) CacheSmartListing(ctx context.Context, description string, images []string, result interface{}) error {
	key := h.generateSmartListingKey(description, images)
	if err := h.cache.Set(ctx, key, result, SmartListingCacheDuration); err != nil {
		log.Printf("Failed to cache smart listing result: %v", err)
		return err
	}
	log.Printf("Smart listing result cached for %v", SmartListingCacheDuration)
	return nil
}

func (h *AICacheHelper) GetSmartListing(ctx context.Context, description string, images []string, dest interface{}) error {
	key := h.generateSmartListingKey(description, images)
	if err := h.cache.Get(ctx, key, dest); err != nil {
		return err
	}
	log.Printf("Smart listing result retrieved from cache")
	return nil
}

// Cache operations for intent parsing
func (h *AICacheHelper) CacheIntent(ctx context.Context, query string, result interface{}) error {
	key := h.generateIntentKey(query)
	if err := h.cache.Set(ctx, key, result, IntentParsingCacheDuration); err != nil {
		log.Printf("Failed to cache intent result: %v", err)
		return err
	}
	log.Printf("Intent result cached for %v", IntentParsingCacheDuration)
	return nil
}

func (h *AICacheHelper) GetIntent(ctx context.Context, query string, dest interface{}) error {
	key := h.generateIntentKey(query)
	if err := h.cache.Get(ctx, key, dest); err != nil {
		return err
	}
	log.Printf("Intent result retrieved from cache")
	return nil
}

// Cache operations for search suggestions
func (h *AICacheHelper) CacheSuggestions(ctx context.Context, query string, limit int, result interface{}) error {
	key := h.generateSuggestionsKey(query, limit)
	if err := h.cache.Set(ctx, key, result, SearchSuggestionsCacheDuration); err != nil {
		log.Printf("Failed to cache suggestions: %v", err)
		return err
	}
	log.Printf("Suggestions cached for %v", SearchSuggestionsCacheDuration)
	return nil
}

func (h *AICacheHelper) GetSuggestions(ctx context.Context, query string, limit int, dest interface{}) error {
	key := h.generateSuggestionsKey(query, limit)
	if err := h.cache.Get(ctx, key, dest); err != nil {
		return err
	}
	log.Printf("Suggestions retrieved from cache")
	return nil
}

// Cache operations for trending recommendations
func (h *AICacheHelper) CacheTrending(ctx context.Context, limit int, result interface{}) error {
	key := h.generateTrendingKey(limit)
	if err := h.cache.Set(ctx, key, result, TrendingRecommendationCacheDuration); err != nil {
		log.Printf("Failed to cache trending recommendations: %v", err)
		return err
	}
	log.Printf("Trending recommendations cached for %v", TrendingRecommendationCacheDuration)
	return nil
}

func (h *AICacheHelper) GetTrending(ctx context.Context, limit int, dest interface{}) error {
	key := h.generateTrendingKey(limit)
	if err := h.cache.Get(ctx, key, dest); err != nil {
		return err
	}
	log.Printf("Trending recommendations retrieved from cache")
	return nil
}

// Cache operations for similar items
func (h *AICacheHelper) CacheSimilarItems(ctx context.Context, itemID string, limit int, result interface{}) error {
	key := h.generateSimilarItemsKey(itemID, limit)
	if err := h.cache.Set(ctx, key, result, SimilarItemsCacheDuration); err != nil {
		log.Printf("Failed to cache similar items: %v", err)
		return err
	}
	log.Printf("Similar items cached for %v", SimilarItemsCacheDuration)
	return nil
}

func (h *AICacheHelper) GetSimilarItems(ctx context.Context, itemID string, limit int, dest interface{}) error {
	key := h.generateSimilarItemsKey(itemID, limit)
	if err := h.cache.Get(ctx, key, dest); err != nil {
		return err
	}
	log.Printf("Similar items retrieved from cache")
	return nil
}

// Cache operations for personalized recommendations
func (h *AICacheHelper) CachePersonalized(ctx context.Context, userID string, limit int, result interface{}) error {
	key := h.generatePersonalizedKey(userID, limit)
	if err := h.cache.Set(ctx, key, result, AIRecommendationCacheDuration); err != nil {
		log.Printf("Failed to cache personalized recommendations: %v", err)
		return err
	}
	log.Printf("Personalized recommendations cached for %v", AIRecommendationCacheDuration)
	return nil
}

func (h *AICacheHelper) GetPersonalized(ctx context.Context, userID string, limit int, dest interface{}) error {
	key := h.generatePersonalizedKey(userID, limit)
	if err := h.cache.Get(ctx, key, dest); err != nil {
		return err
	}
	log.Printf("Personalized recommendations retrieved from cache")
	return nil
}

// Cache invalidation methods
func (h *AICacheHelper) InvalidateUserRecommendations(ctx context.Context, userID string) error {
	// Invalidate personalized recommendations for user
	// In production, you might want to use Redis SCAN to find all user-related keys
	log.Printf("User AI recommendations invalidation requested for user %s", userID)
	return nil
}

func (h *AICacheHelper) InvalidateTrendingCache(ctx context.Context) error {
	// Invalidate all trending caches when new items are added
	// In production, use pattern matching to delete all trending keys
	log.Printf("Trending recommendations cache invalidation requested")
	return nil
}

func (h *AICacheHelper) InvalidateItemRelatedCache(ctx context.Context, itemID string) error {
	// Invalidate similar items cache for this item
	// Invalidate trending cache (item might affect trending)
	log.Printf("Item-related AI cache invalidation requested for item %s", itemID)
	return nil
}

// Helper method to check if caching should be used
func (h *AICacheHelper) ShouldUseCache() bool {
	return h.cache != nil
}

// Health check for AI cache
func (h *AICacheHelper) HealthCheck(ctx context.Context) error {
	if h.cache == nil {
		return fmt.Errorf("AI cache not initialized")
	}
	
	testKey := "ai:health:check"
	testValue := "ok"
	
	// Test write
	if err := h.cache.Set(ctx, testKey, testValue, 1*time.Minute); err != nil {
		return fmt.Errorf("AI cache write failed: %v", err)
	}
	
	// Test read
	var result string
	if err := h.cache.Get(ctx, testKey, &result); err != nil {
		return fmt.Errorf("AI cache read failed: %v", err)
	}
	
	if result != testValue {
		return fmt.Errorf("AI cache data mismatch: expected %s, got %s", testValue, result)
	}
	
	// Cleanup
	h.cache.Delete(ctx, testKey)
	
	return nil
}
