package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/models"
)

// RecommendationService provides AI-powered recommendations based on real item data
type RecommendationService struct {
	config     *config.Config
	geminiAI   *GeminiService
	httpClient *http.Client
}

// UserBehavior represents user interaction patterns
type UserBehavior struct {
	UserID          string                 `json:"user_id"`
	ViewedItems     []string               `json:"viewed_items"`
	RequestedItems  []string               `json:"requested_items"`
	PreferredSizes  []string               `json:"preferred_sizes"`
	PreferredColors []string               `json:"preferred_colors"`
	PreferredTypes  []string               `json:"preferred_types"`
	CategoryHistory map[string]int         `json:"category_history"`
	LastActivity    time.Time              `json:"last_activity"`
	Preferences     map[string]interface{} `json:"preferences"`
}

// RecommendationRequest represents a request for recommendations
type RecommendationRequest struct {
	UserID      string                 `json:"user_id"`
	Context     string                 `json:"context"` // "browse", "search", "profile"
	Query       string                 `json:"query,omitempty"`
	Filters     *models.ItemFilter     `json:"filters,omitempty"`
	Limit       int                    `json:"limit"`
	ExcludeIDs  []string               `json:"exclude_ids,omitempty"`
	UserProfile map[string]interface{} `json:"user_profile,omitempty"`
}

// NewRecommendationService creates a new recommendation service
func NewRecommendationService(cfg *config.Config) *RecommendationService {
	return &RecommendationService{
		config:     cfg,
		geminiAI:   NewGeminiService(cfg),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// GetPersonalizedRecommendations generates personalized recommendations for a user
func (r *RecommendationService) GetPersonalizedRecommendations(ctx context.Context, req *RecommendationRequest) ([]*Recommendation, error) {
	// Get user behavior data
	userBehavior, err := r.getUserBehavior(req.UserID)
	if err != nil {
		log.Printf("Failed to get user behavior: %v", err)
		userBehavior = &UserBehavior{UserID: req.UserID}
	}

	// Get available items based on context and filters
	availableItems, err := r.getAvailableItems(req.Filters, req.ExcludeIDs)
	if err != nil {
		log.Printf("Failed to get available items: %v", err)
		return nil, err
	}

	// Enhance user profile with behavior data
	enhancedProfile := r.enhanceUserProfile(req.UserProfile, userBehavior)

	// Generate recommendations using Gemini AI
	recommendations, err := r.geminiAI.GenerateRecommendations(ctx, enhancedProfile, availableItems)
	if err != nil {
		log.Printf("Gemini AI failed, using fallback: %v", err)
		recommendations = r.generateFallbackRecommendations(userBehavior, availableItems, req.Limit)
	}

	// Post-process and rank recommendations
	rankedRecommendations := r.rankRecommendations(recommendations, userBehavior, req.Context)

	// Limit results
	if req.Limit > 0 && len(rankedRecommendations) > req.Limit {
		rankedRecommendations = rankedRecommendations[:req.Limit]
	}

	return rankedRecommendations, nil
}

// GetSimilarItems finds items similar to a given item
func (r *RecommendationService) GetSimilarItems(ctx context.Context, itemID string, limit int) ([]*Recommendation, error) {
	// Get the reference item
	referenceItem, err := r.getItemByID(itemID)
	if err != nil {
		return nil, err
	}

	// Get all available items
	availableItems, err := r.getAvailableItems(nil, []string{itemID})
	if err != nil {
		return nil, err
	}

	// Find similar items using AI
	prompt := r.buildSimilarityPrompt(referenceItem, availableItems)
	response, err := r.geminiAI.callGeminiAPI(ctx, prompt)
	if err != nil {
		log.Printf("Gemini AI failed for similarity: %v", err)
		return r.generateSimilarityFallback(referenceItem, availableItems, limit), nil
	}

	recommendations, err := r.geminiAI.parseRecommendationResponse(response, availableItems)
	if err != nil {
		return r.generateSimilarityFallback(referenceItem, availableItems, limit), nil
	}

	// Limit results
	if limit > 0 && len(recommendations) > limit {
		recommendations = recommendations[:limit]
	}

	return recommendations, nil
}

// GetTrendingRecommendations gets trending items based on recent activity
func (r *RecommendationService) GetTrendingRecommendations(ctx context.Context, limit int) ([]*Recommendation, error) {
	// Get trending items (most requested/viewed recently)
	trendingItems, err := r.getTrendingItems(limit * 2) // Get more to filter
	if err != nil {
		return nil, err
	}

	// Use AI to analyze trends and generate recommendations
	prompt := r.buildTrendingPrompt(trendingItems)
	response, err := r.geminiAI.callGeminiAPI(ctx, prompt)
	if err != nil {
		log.Printf("Gemini AI failed for trending: %v", err)
		return r.generateTrendingFallback(trendingItems, limit), nil
	}

	recommendations, err := r.geminiAI.parseRecommendationResponse(response, trendingItems)
	if err != nil {
		return r.generateTrendingFallback(trendingItems, limit), nil
	}

	// Limit results
	if limit > 0 && len(recommendations) > limit {
		recommendations = recommendations[:limit]
	}

	return recommendations, nil
}

// getUserBehavior retrieves user behavior data from database
func (r *RecommendationService) getUserBehavior(userID string) (*UserBehavior, error) {
	// This would typically query user interaction history from database
	// For now, return a basic structure
	return &UserBehavior{
		UserID:          userID,
		ViewedItems:     []string{},
		RequestedItems:  []string{},
		PreferredSizes:  []string{"M", "L"},
		PreferredColors: []string{"black", "white", "blue"},
		PreferredTypes:  []string{"donation", "rental"},
		CategoryHistory: make(map[string]int),
		LastActivity:    time.Now(),
		Preferences:     make(map[string]interface{}),
	}, nil
}

// getAvailableItems retrieves available items from database
func (r *RecommendationService) getAvailableItems(filters *models.ItemFilter, excludeIDs []string) ([]map[string]interface{}, error) {
	// Build query URL
	url := fmt.Sprintf("%s/rest/v1/items?select=id,name,description,category_id,type,status,price,images,size,color,condition,partner_id,total_quantity,available_quantity,created_at,updated_at", r.config.SupabaseURL)

	// Add basic filters
	url += "&status=eq.active"
	url += "&available_quantity=gt.0"

	// Add custom filters if provided
	if filters != nil {
		if filters.Type != nil {
			url += fmt.Sprintf("&type=eq.%s", *filters.Type)
		}
		if filters.Size != nil {
			url += fmt.Sprintf("&size=eq.%s", *filters.Size)
		}
		if filters.Color != nil {
			url += fmt.Sprintf("&color=ilike.*%s*", *filters.Color)
		}
		if filters.Condition != nil {
			url += fmt.Sprintf("&condition=eq.%s", *filters.Condition)
		}
		if filters.MinPrice != nil {
			url += fmt.Sprintf("&price=gte.%f", *filters.MinPrice)
		}
		if filters.MaxPrice != nil {
			url += fmt.Sprintf("&price=lte.%f", *filters.MaxPrice)
		}
	}

	// Limit results for performance
	url += "&limit=50"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", r.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseServiceRoleKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}

	// Filter out excluded IDs
	var filteredItems []map[string]interface{}
	excludeMap := make(map[string]bool)
	for _, id := range excludeIDs {
		excludeMap[id] = true
	}

	for _, item := range items {
		if itemID, ok := item["id"].(string); ok && !excludeMap[itemID] {
			filteredItems = append(filteredItems, item)
		}
	}

	return filteredItems, nil
}

// getItemByID retrieves a specific item by ID
func (r *RecommendationService) getItemByID(itemID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s&select=*", r.config.SupabaseURL, itemID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", r.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseServiceRoleKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("item not found")
	}

	return items[0], nil
}

// getTrendingItems gets items with high recent activity
func (r *RecommendationService) getTrendingItems(limit int) ([]map[string]interface{}, error) {
	// For now, get recent items as trending
	// In production, this would analyze request/view patterns
	url := fmt.Sprintf("%s/rest/v1/items?select=*&status=eq.active&available_quantity=gt.0&order=created_at.desc&limit=%d", r.config.SupabaseURL, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", r.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseServiceRoleKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}

	return items, nil
}

// enhanceUserProfile combines user profile with behavior data
func (r *RecommendationService) enhanceUserProfile(profile map[string]interface{}, behavior *UserBehavior) map[string]interface{} {
	enhanced := make(map[string]interface{})

	// Copy original profile
	for k, v := range profile {
		enhanced[k] = v
	}

	// Add behavior insights
	enhanced["preferred_sizes"] = behavior.PreferredSizes
	enhanced["preferred_colors"] = behavior.PreferredColors
	enhanced["preferred_types"] = behavior.PreferredTypes
	enhanced["category_history"] = behavior.CategoryHistory
	enhanced["interaction_count"] = len(behavior.ViewedItems) + len(behavior.RequestedItems)

	return enhanced
}

// rankRecommendations sorts recommendations by relevance and context
func (r *RecommendationService) rankRecommendations(recommendations []*Recommendation, behavior *UserBehavior, context string) []*Recommendation {
	// Sort by score descending
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Score > recommendations[j].Score
	})

	// Apply context-specific adjustments
	for _, rec := range recommendations {
		switch context {
		case "browse":
			// Boost popular categories
			if category, ok := rec.Data["category"].(string); ok {
				if count, exists := behavior.CategoryHistory[category]; exists && count > 2 {
					rec.Score += 0.1
				}
			}
		case "search":
			// Boost exact matches
			rec.Score += 0.05
		case "profile":
			// Boost personalized matches
			rec.Score += 0.15
		}
	}

	// Re-sort after adjustments
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Score > recommendations[j].Score
	})

	return recommendations
}

// generateFallbackRecommendations creates recommendations without AI
func (r *RecommendationService) generateFallbackRecommendations(behavior *UserBehavior, items []map[string]interface{}, limit int) []*Recommendation {
	var recommendations []*Recommendation

	// Simple scoring based on user preferences
	for i, item := range items {
		if limit > 0 && len(recommendations) >= limit {
			break
		}

		score := 0.5
		reason := "Item populer"

		// Boost based on size preference
		if size, ok := item["size"].(string); ok {
			for _, prefSize := range behavior.PreferredSizes {
				if size == prefSize {
					score += 0.2
					reason = "Sesuai dengan ukuran yang Anda sukai"
					break
				}
			}
		}

		// Boost based on color preference
		if color, ok := item["color"].(string); ok {
			for _, prefColor := range behavior.PreferredColors {
				if strings.Contains(strings.ToLower(color), strings.ToLower(prefColor)) {
					score += 0.15
					reason = "Sesuai dengan warna favorit Anda"
					break
				}
			}
		}

		// Boost based on type preference
		if itemType, ok := item["type"].(string); ok {
			for _, prefType := range behavior.PreferredTypes {
				if itemType == prefType {
					score += 0.1
					break
				}
			}
		}

		itemTypeIndo := "donasi"
		if itemType, ok := item["type"].(string); ok && itemType == "rental" {
			itemTypeIndo = "rental"
		}

		// Get images array, handle both []interface{} and []string
		var images []string
		if imgArray, ok := item["images"].([]interface{}); ok {
			for _, img := range imgArray {
				if imgStr, ok := img.(string); ok {
					images = append(images, imgStr)
				}
			}
		} else if imgArray, ok := item["images"].([]string); ok {
			images = imgArray
		}

		recommendation := &Recommendation{
			Type:        "item_recommendation",
			Title:       fmt.Sprintf("Rekomendasi: %s", item["name"]),
			Description: fmt.Sprintf("Item %s yang bagus untuk Anda berdasarkan preferensi", itemTypeIndo),
			Reason:      reason,
			Score:       score,
			Data: map[string]interface{}{
				"item_id":  item["id"],
				"name":     item["name"],
				"images":   images,
				"category": item["category_id"],
				"rank":     i + 1,
			},
		}

		recommendations = append(recommendations, recommendation)
	}

	return recommendations
}

// generateSimilarityFallback creates similarity recommendations without AI
func (r *RecommendationService) generateSimilarityFallback(referenceItem map[string]interface{}, items []map[string]interface{}, limit int) []*Recommendation {
	var recommendations []*Recommendation

	refCategory := referenceItem["category_id"]
	refSize := referenceItem["size"]
	refType := referenceItem["type"]

	for _, item := range items {
		if limit > 0 && len(recommendations) >= limit {
			break
		}

		score := 0.3
		reason := "Item serupa"

		// Same category
		if item["category_id"] == refCategory {
			score += 0.4
			reason = "Kategori yang sama"
		}

		// Same size
		if item["size"] == refSize {
			score += 0.2
		}

		// Same type
		if item["type"] == refType {
			score += 0.1
		}

		// Get images array, handle both []interface{} and []string
		var images []string
		if imgArray, ok := item["images"].([]interface{}); ok {
			for _, img := range imgArray {
				if imgStr, ok := img.(string); ok {
					images = append(images, imgStr)
				}
			}
		} else if imgArray, ok := item["images"].([]string); ok {
			images = imgArray
		}

		recommendation := &Recommendation{
			Type:        "similar_item",
			Title:       fmt.Sprintf("Mirip: %s", item["name"]),
			Description: fmt.Sprintf("Mirip dengan %s berdasarkan kategori dan karakteristik", referenceItem["name"]),
			Reason:      reason,
			Score:       score,
			Data: map[string]interface{}{
				"item_id":    item["id"],
				"name":       item["name"],
				"images":     images,
				"category":   item["category_id"],
				"similarity": score,
			},
		}

		recommendations = append(recommendations, recommendation)
	}

	// Sort by similarity score
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Score > recommendations[j].Score
	})

	return recommendations
}

// generateTrendingFallback creates trending recommendations without AI
func (r *RecommendationService) generateTrendingFallback(items []map[string]interface{}, limit int) []*Recommendation {
	var recommendations []*Recommendation

	for i, item := range items {
		if limit > 0 && len(recommendations) >= limit {
			break
		}

		// Score based on recency and availability
		score := 0.7 + float64(len(items)-i)/float64(len(items))*0.3

		// Get images array, handle both []interface{} and []string
		var images []string
		if imgArray, ok := item["images"].([]interface{}); ok {
			for _, img := range imgArray {
				if imgStr, ok := img.(string); ok {
					images = append(images, imgStr)
				}
			}
		} else if imgArray, ok := item["images"].([]string); ok {
			images = imgArray
		}

		recommendation := &Recommendation{
			Type:        "trending_item",
			Title:       fmt.Sprintf("Trending: %s", item["name"]),
			Description: "Item populer saat ini di kalangan pengguna SatuLemari",
			Reason:      "Sedang trending berdasarkan aktivitas terbaru",
			Score:       score,
			Data: map[string]interface{}{
				"item_id":  item["id"],
				"name":     item["name"],
				"images":   images,
				"category": item["category_id"],
				"trending": true,
			},
		}

		recommendations = append(recommendations, recommendation)
	}

	return recommendations
}

// buildSimilarityPrompt creates prompt for finding similar items
func (r *RecommendationService) buildSimilarityPrompt(referenceItem map[string]interface{}, availableItems []map[string]interface{}) string {
	refJSON, _ := json.Marshal(referenceItem)
	itemsJSON, _ := json.Marshal(availableItems)

	return fmt.Sprintf(`
Temukan item yang mirip dengan item referensi ini dalam konteks fashion Indonesia:

Item Referensi:
%s

Item yang Tersedia:
%s

Mohon berikan respons dalam format JSON array dengan item yang paling mirip:
[
  {
    "item_id": "string",
    "title": "Mirip: [nama item]",
    "description": "Mengapa item ini mirip dalam bahasa Indonesia",
    "reason": "Faktor kesamaan spesifik dalam bahasa Indonesia",
    "score": 0.0-1.0,
    "category": "string",
    "tags": ["kesamaan", "faktor"]
  }
]

Pertimbangkan:
- Kategori yang sama atau kategori terkait
- Gaya dan desain yang mirip
- Ukuran yang kompatibel
- Rentang harga yang mirip
- Tipe yang sama (donasi/rental)
- Kompatibilitas warna
- Kesamaan kondisi
- Kesesuaian dengan iklim Indonesia

Gunakan bahasa Indonesia untuk description dan reason.
Batasi hingga 5 item paling mirip.
`, string(refJSON), string(itemsJSON))
}

// buildTrendingPrompt creates prompt for trending analysis
func (r *RecommendationService) buildTrendingPrompt(items []map[string]interface{}) string {
	itemsJSON, _ := json.Marshal(items)

	return fmt.Sprintf(`
Analisis item-item terbaru ini dan identifikasi pola trending dalam konteks fashion Indonesia:

Item Terbaru:
%s

Mohon berikan respons dalam format JSON array dengan rekomendasi trending:
[
  {
    "item_id": "string",
    "title": "Trending: [nama item]",
    "description": "Mengapa item ini sedang trending dalam bahasa Indonesia",
    "reason": "Analisis trend dalam bahasa Indonesia",
    "score": 0.0-1.0,
    "category": "string",
    "tags": ["trending", "faktor-faktor"]
  }
]

Pertimbangkan:
- Kategori yang populer di Indonesia
- Trend musiman di iklim tropis
- Pola gaya fashion Indonesia
- Trend harga di pasar Indonesia
- Preferensi tipe (donasi vs rental) di Indonesia
- Trend warna yang disukai di Indonesia
- Popularitas ukuran di Indonesia

Fokus pada trend fashion Indonesia dan preferensi lokal.
Gunakan bahasa Indonesia untuk description dan reason.
Batasi hingga 5 item trending teratas.
`, string(itemsJSON))
}
