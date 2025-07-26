package category

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dzuura/satu-lemari/domain/cache"
	"github.com/dzuura/satu-lemari/domain/models"
)

// CategoryCacheHelper provides caching functionality for Category Service
type CategoryCacheHelper struct {
	cache *cache.RedisCache
}

// NewCategoryCacheHelper creates a new category cache helper
func NewCategoryCacheHelper(redisCache *cache.RedisCache) *CategoryCacheHelper {
	return &CategoryCacheHelper{cache: redisCache}
}

// Cache durations for category-related data
const (
	CategoryListCacheDuration    = 30 * time.Minute // Categories rarely change
	CategoryDetailsCacheDuration = 20 * time.Minute
	CategorySearchCacheDuration  = 10 * time.Minute
)

// Cache key generators
func (h *CategoryCacheHelper) generateCategoryListKey(search, isActive string, page, limit int) string {
	key := "list"
	if search != "" {
		key += ":search:" + search
	}
	if isActive != "" {
		key += ":active:" + isActive
	}
	key += ":page:" + string(rune(page)) + ":limit:" + string(rune(limit))
	return h.cache.GenerateKey(cache.KeyCategoryList, key)
}

func (h *CategoryCacheHelper) generateCategoryDetailsKey(categoryID string) string {
	return h.cache.GenerateKey(cache.KeyCategoryList, "details", categoryID)
}

func (h *CategoryCacheHelper) generateAllCategoriesKey() string {
	return h.cache.GenerateKey(cache.KeyCategoryList, "all")
}

// Cache operations for category list
func (h *CategoryCacheHelper) CacheCategoryList(ctx context.Context, search, isActive string, page, limit int, categories []models.Category, total int) error {
	key := h.generateCategoryListKey(search, isActive, page, limit)

	listResult := struct {
		Categories []models.Category `json:"categories"`
		Total      int               `json:"total"`
	}{
		Categories: categories,
		Total:      total,
	}

	if err := h.cache.Set(ctx, key, listResult, CategoryListCacheDuration); err != nil {
		log.Printf("Failed to cache category list: %v", err)
		return err
	}
	log.Printf("Category list cached for %v (key: %s)", CategoryListCacheDuration, key)
	return nil
}

func (h *CategoryCacheHelper) GetCategoryList(ctx context.Context, search, isActive string, page, limit int) ([]models.Category, int, error) {
	key := h.generateCategoryListKey(search, isActive, page, limit)

	var listResult struct {
		Categories []models.Category `json:"categories"`
		Total      int               `json:"total"`
	}

	if err := h.cache.Get(ctx, key, &listResult); err != nil {
		return nil, 0, err
	}

	log.Printf("Category list retrieved from cache (key: %s)", key)
	return listResult.Categories, listResult.Total, nil
}

// Cache operations for all categories (commonly used)
func (h *CategoryCacheHelper) CacheAllCategories(ctx context.Context, categories []models.Category) error {
	key := h.generateAllCategoriesKey()

	if err := h.cache.Set(ctx, key, categories, CategoryListCacheDuration); err != nil {
		log.Printf("Failed to cache all categories: %v", err)
		return err
	}
	log.Printf("All categories cached for %v", CategoryListCacheDuration)
	return nil
}

func (h *CategoryCacheHelper) GetAllCategories(ctx context.Context) ([]models.Category, error) {
	key := h.generateAllCategoriesKey()

	var categories []models.Category
	if err := h.cache.Get(ctx, key, &categories); err != nil {
		return nil, err
	}

	log.Printf("All categories retrieved from cache")
	return categories, nil
}

// Cache operations for category details
func (h *CategoryCacheHelper) CacheCategoryDetails(ctx context.Context, categoryID string, category *models.Category) error {
	key := h.generateCategoryDetailsKey(categoryID)

	if err := h.cache.Set(ctx, key, category, CategoryDetailsCacheDuration); err != nil {
		log.Printf("Failed to cache category details %s: %v", categoryID, err)
		return err
	}
	log.Printf("Category details %s cached for %v", categoryID, CategoryDetailsCacheDuration)
	return nil
}

func (h *CategoryCacheHelper) GetCategoryDetails(ctx context.Context, categoryID string) (*models.Category, error) {
	key := h.generateCategoryDetailsKey(categoryID)

	var category models.Category
	if err := h.cache.Get(ctx, key, &category); err != nil {
		return nil, err
	}

	log.Printf("Category details %s retrieved from cache", categoryID)
	return &category, nil
}

// Cache invalidation methods
func (h *CategoryCacheHelper) InvalidateAllCategoryCache(ctx context.Context) error {
	// Invalidate all category-related caches when categories are modified
	keys := []string{
		h.generateAllCategoriesKey(),
	}

	for _, key := range keys {
		if err := h.cache.Delete(ctx, key); err != nil {
			log.Printf("Failed to invalidate cache key %s: %v", key, err)
		}
	}

	// In production, you might want to use Redis SCAN to find and delete
	// all category list cache keys with different search/pagination parameters
	log.Printf("Category cache invalidation completed")
	return nil
}

func (h *CategoryCacheHelper) InvalidateCategoryDetails(ctx context.Context, categoryID string) error {
	key := h.generateCategoryDetailsKey(categoryID)

	if err := h.cache.Delete(ctx, key); err != nil {
		log.Printf("Failed to invalidate category details cache %s: %v", categoryID, err)
		return err
	}

	log.Printf("Category details cache invalidated for %s", categoryID)
	return nil
}

// Comprehensive invalidation for when categories are created/updated/deleted
func (h *CategoryCacheHelper) InvalidateRelatedCaches(ctx context.Context, categoryID string) error {
	// Invalidate specific category details
	if categoryID != "" {
		if err := h.InvalidateCategoryDetails(ctx, categoryID); err != nil {
			log.Printf("Failed to invalidate category details cache: %v", err)
		}
	}

	// Invalidate all category lists (since category changes affect lists)
	if err := h.InvalidateAllCategoryCache(ctx); err != nil {
		log.Printf("Failed to invalidate category list cache: %v", err)
	}

	log.Printf("Related category caches invalidated for category %s", categoryID)
	return nil
}

// Helper method to check if caching should be used
func (h *CategoryCacheHelper) ShouldUseCache() bool {
	return h.cache != nil
}

// Health check for category cache
func (h *CategoryCacheHelper) HealthCheck(ctx context.Context) error {
	if h.cache == nil {
		return fmt.Errorf("cache not initialized")
	}

	testKey := "category:health:check"
	testValue := "ok"

	// Test write
	if err := h.cache.Set(ctx, testKey, testValue, 1*time.Minute); err != nil {
		return fmt.Errorf("category cache write failed: %v", err)
	}

	// Test read
	var result string
	if err := h.cache.Get(ctx, testKey, &result); err != nil {
		return fmt.Errorf("category cache read failed: %v", err)
	}

	if result != testValue {
		return fmt.Errorf("category cache data mismatch: expected %s, got %s", testValue, result)
	}

	// Cleanup
	h.cache.Delete(ctx, testKey)

	return nil
}

// Cache category name lookup (frequently used)
func (h *CategoryCacheHelper) CacheCategoryName(ctx context.Context, categoryID, categoryName string) error {
	key := h.cache.GenerateKey("category:name", categoryID)

	if err := h.cache.Set(ctx, key, categoryName, CategoryDetailsCacheDuration); err != nil {
		log.Printf("Failed to cache category name %s: %v", categoryID, err)
		return err
	}

	return nil
}

func (h *CategoryCacheHelper) GetCategoryName(ctx context.Context, categoryID string) (string, error) {
	key := h.cache.GenerateKey("category:name", categoryID)

	var categoryName string
	if err := h.cache.Get(ctx, key, &categoryName); err != nil {
		return "", err
	}

	return categoryName, nil
}
