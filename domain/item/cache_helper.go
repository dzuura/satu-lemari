package item

import (
	"context"
	"crypto/md5"
	"fmt"
	"log"
	"time"

	"github.com/dzuura/satu-lemari/domain/cache"
	"github.com/dzuura/satu-lemari/domain/models"
)

// ItemCacheHelper provides caching functionality for Item Service
type ItemCacheHelper struct {
	cache *cache.RedisCache
}

// NewItemCacheHelper creates a new item cache helper
func NewItemCacheHelper(redisCache *cache.RedisCache) *ItemCacheHelper {
	return &ItemCacheHelper{cache: redisCache}
}

// Cache durations for item-related data
const (
	ItemDetailsCacheDuration = 10 * time.Minute
	ItemSearchCacheDuration  = 3 * time.Minute
	ItemListCacheDuration    = 5 * time.Minute
)

// Cache key generators
func (h *ItemCacheHelper) generateItemDetailsKey(itemID string) string {
	return h.cache.GenerateKey(cache.KeyItemDetails, itemID)
}

func (h *ItemCacheHelper) generateSearchResultKey(filters SearchFilters, page, limit int) string {
	// Create a hash of the search parameters for consistent caching
	filterStr := fmt.Sprintf("%+v:page:%d:limit:%d", filters, page, limit)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(filterStr)))
	return h.cache.GenerateKey(cache.KeySearchResult, hash)
}

func (h *ItemCacheHelper) generateItemListKey(page, limit int, partnerID string) string {
	key := fmt.Sprintf("list:page:%d:limit:%d", page, limit)
	if partnerID != "" {
		key += ":partner:" + partnerID
	}
	return h.cache.GenerateKey(cache.KeyItemDetails, key)
}

// Cache operations for item details
func (h *ItemCacheHelper) CacheItemDetails(ctx context.Context, itemID string, item *models.Item) error {
	key := h.generateItemDetailsKey(itemID)
	if err := h.cache.Set(ctx, key, item, ItemDetailsCacheDuration); err != nil {
		log.Printf("Failed to cache item details %s: %v", itemID, err)
		return err
	}
	log.Printf("Item details %s cached for %v", itemID, ItemDetailsCacheDuration)
	return nil
}

func (h *ItemCacheHelper) GetItemDetails(ctx context.Context, itemID string) (*models.Item, error) {
	key := h.generateItemDetailsKey(itemID)
	var item models.Item
	if err := h.cache.Get(ctx, key, &item); err != nil {
		return nil, err
	}
	log.Printf("Item details %s retrieved from cache", itemID)
	return &item, nil
}

// Cache operations for search results
func (h *ItemCacheHelper) CacheSearchResults(ctx context.Context, filters SearchFilters, page, limit int, items []models.Item, total int) error {
	key := h.generateSearchResultKey(filters, page, limit)
	
	searchResult := struct {
		Items []models.Item `json:"items"`
		Total int           `json:"total"`
	}{
		Items: items,
		Total: total,
	}
	
	if err := h.cache.Set(ctx, key, searchResult, ItemSearchCacheDuration); err != nil {
		log.Printf("Failed to cache search results: %v", err)
		return err
	}
	log.Printf("Search results cached for %v (key: %s)", ItemSearchCacheDuration, key)
	return nil
}

func (h *ItemCacheHelper) GetSearchResults(ctx context.Context, filters SearchFilters, page, limit int) ([]models.Item, int, error) {
	key := h.generateSearchResultKey(filters, page, limit)
	
	var searchResult struct {
		Items []models.Item `json:"items"`
		Total int           `json:"total"`
	}
	
	if err := h.cache.Get(ctx, key, &searchResult); err != nil {
		return nil, 0, err
	}
	
	log.Printf("Search results retrieved from cache (key: %s)", key)
	return searchResult.Items, searchResult.Total, nil
}

// Cache operations for item lists
func (h *ItemCacheHelper) CacheItemList(ctx context.Context, page, limit int, partnerID string, items []models.Item, total int) error {
	key := h.generateItemListKey(page, limit, partnerID)
	
	listResult := struct {
		Items []models.Item `json:"items"`
		Total int           `json:"total"`
	}{
		Items: items,
		Total: total,
	}
	
	if err := h.cache.Set(ctx, key, listResult, ItemListCacheDuration); err != nil {
		log.Printf("Failed to cache item list: %v", err)
		return err
	}
	log.Printf("Item list cached for %v (key: %s)", ItemListCacheDuration, key)
	return nil
}

func (h *ItemCacheHelper) GetItemList(ctx context.Context, page, limit int, partnerID string) ([]models.Item, int, error) {
	key := h.generateItemListKey(page, limit, partnerID)
	
	var listResult struct {
		Items []models.Item `json:"items"`
		Total int           `json:"total"`
	}
	
	if err := h.cache.Get(ctx, key, &listResult); err != nil {
		return nil, 0, err
	}
	
	log.Printf("Item list retrieved from cache (key: %s)", key)
	return listResult.Items, listResult.Total, nil
}

// Cache invalidation methods
func (h *ItemCacheHelper) InvalidateItemCache(ctx context.Context, itemID string) error {
	key := h.generateItemDetailsKey(itemID)
	if err := h.cache.Delete(ctx, key); err != nil {
		log.Printf("Failed to invalidate item cache %s: %v", itemID, err)
		return err
	}
	log.Printf("Item cache invalidated for %s", itemID)
	return nil
}

func (h *ItemCacheHelper) InvalidateSearchCache(ctx context.Context) error {
	// In a production environment, you might want to use Redis SCAN
	// to find and delete all search result keys
	log.Printf("Search cache invalidation requested")
	return nil
}

func (h *ItemCacheHelper) InvalidatePartnerItemsCache(ctx context.Context, partnerID string) error {
	// Invalidate all cached lists for this partner
	// In production, you might want to track partner-specific cache keys
	log.Printf("Partner items cache invalidation requested for partner %s", partnerID)
	return nil
}

// Batch invalidation for when items are created/updated/deleted
func (h *ItemCacheHelper) InvalidateRelatedCaches(ctx context.Context, itemID, partnerID string) error {
	// Invalidate item details
	if err := h.InvalidateItemCache(ctx, itemID); err != nil {
		log.Printf("Failed to invalidate item cache: %v", err)
	}
	
	// Invalidate search results (items might appear in search)
	if err := h.InvalidateSearchCache(ctx); err != nil {
		log.Printf("Failed to invalidate search cache: %v", err)
	}
	
	// Invalidate partner's item lists
	if partnerID != "" {
		if err := h.InvalidatePartnerItemsCache(ctx, partnerID); err != nil {
			log.Printf("Failed to invalidate partner items cache: %v", err)
		}
	}
	
	log.Printf("Related caches invalidated for item %s", itemID)
	return nil
}

// Helper method to check if caching should be used
func (h *ItemCacheHelper) ShouldUseCache() bool {
	return h.cache != nil
}

// Health check for item cache
func (h *ItemCacheHelper) HealthCheck(ctx context.Context) error {
	if h.cache == nil {
		return fmt.Errorf("cache not initialized")
	}
	
	testKey := "item:health:check"
	testValue := "ok"
	
	// Test write
	if err := h.cache.Set(ctx, testKey, testValue, 1*time.Minute); err != nil {
		return fmt.Errorf("item cache write failed: %v", err)
	}
	
	// Test read
	var result string
	if err := h.cache.Get(ctx, testKey, &result); err != nil {
		return fmt.Errorf("item cache read failed: %v", err)
	}
	
	if result != testValue {
		return fmt.Errorf("item cache data mismatch: expected %s, got %s", testValue, result)
	}
	
	// Cleanup
	h.cache.Delete(ctx, testKey)
	
	return nil
}
