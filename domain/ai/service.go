package ai

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/models"
)

// AIServiceManager manages all AI services
type AIServiceManager struct {
	config        *config.Config
	smartListing  *SimpleSmartListingService
	intentMatcher *SimpleIntentMatcherService
	basicAI       *AIService // Rule-based service
}

// NewAIServiceManager creates a new AI service manager
func NewAIServiceManager(cfg *config.Config) (*AIServiceManager, error) {
	// Create basic AI service (rule-based)
	basicAI, err := NewAIService(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create basic AI service: %v", err)
	}

	// Create smart listing service (using simple version for now)
	smartListing := NewSimpleSmartListingService(cfg)

	// Create intent matcher service (using simple version for now)
	intentMatcher := NewSimpleIntentMatcherService(cfg)

	return &AIServiceManager{
		config:        cfg,
		smartListing:  smartListing,
		intentMatcher: intentMatcher,
		basicAI:       basicAI,
	}, nil
}

// AutoListItems uses smart listing to analyze images and generate item details
func (m *AIServiceManager) AutoListItems(ctx context.Context, images [][]byte, basicInfo map[string]string) (*SmartListingResult, error) {
	if m.smartListing != nil {
		return m.smartListing.AutoListItems(ctx, images, basicInfo)
	}

	// Fallback to basic AI service
	if m.basicAI != nil {
		description := basicInfo["description"]
		if description == "" {
			description = "Item fashion"
		}

		analysis, err := m.basicAI.AnalyzeItem(ctx, description, nil)
		if err != nil {
			return nil, err
		}

		// Convert to SmartListingResult
		result := &SmartListingResult{
			Category:        analysis.Category,
			Subcategory:     analysis.Subcategory,
			Size:            analysis.Size,
			Condition:       analysis.Condition,
			Material:        analysis.Material,
			Brand:           analysis.Brand,
			Season:          analysis.Season,
			Color:           "unknown",
			SuggestedPrice:  50000,
			Description:     analysis.Description,
			Tags:            analysis.Tags,
			ConfidenceScore: analysis.ConfidenceScore,
			AnalysisNotes:   "Basic AI analysis (fallback)",
		}

		return result, nil
	}

	return nil, fmt.Errorf("no AI service available")
}

// ParseIntent uses intent matcher to understand natural language queries
func (m *AIServiceManager) ParseIntent(ctx context.Context, query string) (*IntentResult, error) {
	if m.intentMatcher != nil {
		return m.intentMatcher.ParseIntent(ctx, query)
	}

	// Fallback to basic analysis
	return &IntentResult{
		Intent:     "search",
		Confidence: 0.5,
		Entities:   make(map[string]interface{}),
		Filters:    &models.ItemFilter{},
		Query:      query,
		Suggestions: []string{
			"coba cari dengan kata kunci yang lebih spesifik",
			"gunakan filter untuk hasil yang lebih akurat",
		},
	}, nil
}

// GetSearchSuggestions generates search suggestions
func (m *AIServiceManager) GetSearchSuggestions(ctx context.Context, query string) ([]string, error) {
	if m.intentMatcher != nil {
		return m.intentMatcher.GetSearchSuggestions(ctx, query)
	}

	// Fallback suggestions
	return []string{
		"coba cari dengan kata kunci yang lebih spesifik",
		"gunakan filter untuk hasil yang lebih akurat",
	}, nil
}

// AnalyzeItem uses basic AI service for item analysis
func (m *AIServiceManager) AnalyzeItem(ctx context.Context, description string, imageData []byte) (*ItemAnalysis, error) {
	if m.basicAI != nil {
		return m.basicAI.AnalyzeItem(ctx, description, imageData)
	}

	return nil, fmt.Errorf("no AI service available")
}

// GenerateRecommendations uses basic AI service for recommendations
func (m *AIServiceManager) GenerateRecommendations(ctx context.Context, userProfile map[string]interface{}, availableItems []map[string]interface{}) ([]*Recommendation, error) {
	if m.basicAI != nil {
		return m.basicAI.GenerateRecommendations(ctx, userProfile, availableItems)
	}

	return nil, fmt.Errorf("no AI service available")
}

// AnalyzeUserBehavior uses basic AI service for user behavior analysis
func (m *AIServiceManager) AnalyzeUserBehavior(ctx context.Context, userHistory []map[string]interface{}) (map[string]interface{}, error) {
	if m.basicAI != nil {
		return m.basicAI.AnalyzeUserBehavior(ctx, userHistory)
	}

	return nil, fmt.Errorf("no AI service available")
}

// IsGeminiAvailable checks if Gemini API is available
func (m *AIServiceManager) IsGeminiAvailable() bool {
	return m.config.EnableAIFeatures && m.config.GeminiAPIKey != "" &&
		(m.smartListing != nil || m.intentMatcher != nil)
}

// GetAIServiceStatus returns the status of all AI services
func (m *AIServiceManager) GetAIServiceStatus() map[string]interface{} {
	return map[string]interface{}{
		"gemini_available":    m.IsGeminiAvailable(),
		"smart_listing":       m.smartListing != nil,
		"intent_matcher":      m.intentMatcher != nil,
		"basic_ai":            m.basicAI != nil,
		"ai_features_enabled": m.config.EnableAIFeatures,
		"gemini_api_key_set":  m.config.GeminiAPIKey != "",
		"gemini_model":        m.config.GeminiModel,
	}
}

// Close closes all AI services
func (m *AIServiceManager) Close() error {
	var errors []error

	if m.smartListing != nil {
		if err := m.smartListing.Close(); err != nil {
			errors = append(errors, fmt.Errorf("smart listing close error: %v", err))
		}
	}

	if m.intentMatcher != nil {
		if err := m.intentMatcher.Close(); err != nil {
			errors = append(errors, fmt.Errorf("intent matcher close error: %v", err))
		}
	}

	if m.basicAI != nil {
		if err := m.basicAI.Close(); err != nil {
			errors = append(errors, fmt.Errorf("basic AI close error: %v", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing AI services: %v", errors)
	}

	return nil
}

// AIService handles AI operations using rule-based logic
type AIService struct {
	config *config.Config
}

// ItemAnalysis represents the result of AI analysis for an item
type ItemAnalysis struct {
	Category        string   `json:"category"`
	Subcategory     string   `json:"subcategory"`
	Size            string   `json:"size"`
	Condition       string   `json:"condition"`
	Material        string   `json:"material"`
	Brand           string   `json:"brand"`
	Season          string   `json:"season"`
	Tags            []string `json:"tags"`
	Description     string   `json:"description"`
	ConfidenceScore float64  `json:"confidence_score"`
}

// Recommendation represents an AI-generated recommendation
type Recommendation struct {
	Type        string                 `json:"type"` // user_recommendation, item_recommendation
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Reason      string                 `json:"reason"`
	Score       float64                `json:"score"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// NewAIService creates a new AI service
func NewAIService(cfg *config.Config) (*AIService, error) {
	return &AIService{
		config: cfg,
	}, nil
}

// AnalyzeItem analyzes an item based on its description using rule-based logic
func (a *AIService) AnalyzeItem(ctx context.Context, description string, imageData []byte) (*ItemAnalysis, error) {
	description = strings.ToLower(description)

	analysis := &ItemAnalysis{
		Category:        "unknown",
		Subcategory:     "unknown",
		Size:            "M",
		Condition:       "good",
		Material:        "cotton",
		Brand:           "unknown",
		Season:          "all-season",
		Tags:            []string{},
		Description:     description,
		ConfidenceScore: 0.7,
	}

	// Category detection
	if strings.Contains(description, "shirt") || strings.Contains(description, "t-shirt") || strings.Contains(description, "blouse") {
		analysis.Category = "tops"
		analysis.Subcategory = "shirt"
		analysis.Tags = append(analysis.Tags, "casual", "comfortable")
	} else if strings.Contains(description, "pants") || strings.Contains(description, "jeans") || strings.Contains(description, "trousers") {
		analysis.Category = "bottoms"
		analysis.Subcategory = "pants"
		analysis.Tags = append(analysis.Tags, "versatile", "classic")
	} else if strings.Contains(description, "dress") {
		analysis.Category = "dresses"
		analysis.Subcategory = "dress"
		analysis.Tags = append(analysis.Tags, "elegant", "feminine")
	} else if strings.Contains(description, "jacket") || strings.Contains(description, "coat") {
		analysis.Category = "outerwear"
		analysis.Subcategory = "jacket"
		analysis.Tags = append(analysis.Tags, "warm", "stylish")
	} else if strings.Contains(description, "bag") || strings.Contains(description, "purse") {
		analysis.Category = "accessories"
		analysis.Subcategory = "bag"
		analysis.Tags = append(analysis.Tags, "practical", "fashionable")
	}

	// Size detection
	if strings.Contains(description, "xs") || strings.Contains(description, "extra small") {
		analysis.Size = "XS"
	} else if strings.Contains(description, "s") || strings.Contains(description, "small") {
		analysis.Size = "S"
	} else if strings.Contains(description, "l") || strings.Contains(description, "large") {
		analysis.Size = "L"
	} else if strings.Contains(description, "xl") || strings.Contains(description, "extra large") {
		analysis.Size = "XL"
	} else if strings.Contains(description, "xxl") {
		analysis.Size = "XXL"
	}

	// Condition detection
	if strings.Contains(description, "new") || strings.Contains(description, "excellent") {
		analysis.Condition = "excellent"
		analysis.ConfidenceScore = 0.9
	} else if strings.Contains(description, "good") || strings.Contains(description, "like new") {
		analysis.Condition = "good"
		analysis.ConfidenceScore = 0.8
	} else if strings.Contains(description, "fair") || strings.Contains(description, "used") {
		analysis.Condition = "fair"
		analysis.ConfidenceScore = 0.6
	} else if strings.Contains(description, "poor") || strings.Contains(description, "worn") {
		analysis.Condition = "poor"
		analysis.ConfidenceScore = 0.4
	}

	// Material detection
	if strings.Contains(description, "cotton") {
		analysis.Material = "cotton"
	} else if strings.Contains(description, "polyester") {
		analysis.Material = "polyester"
	} else if strings.Contains(description, "wool") {
		analysis.Material = "wool"
	} else if strings.Contains(description, "silk") {
		analysis.Material = "silk"
	} else if strings.Contains(description, "denim") {
		analysis.Material = "denim"
	}

	// Season detection
	if strings.Contains(description, "summer") || strings.Contains(description, "light") {
		analysis.Season = "summer"
	} else if strings.Contains(description, "winter") || strings.Contains(description, "warm") {
		analysis.Season = "winter"
	} else if strings.Contains(description, "spring") {
		analysis.Season = "spring"
	} else if strings.Contains(description, "fall") || strings.Contains(description, "autumn") {
		analysis.Season = "fall"
	}

	// Brand detection (simple keywords)
	if strings.Contains(description, "nike") {
		analysis.Brand = "Nike"
	} else if strings.Contains(description, "adidas") {
		analysis.Brand = "Adidas"
	} else if strings.Contains(description, "zara") {
		analysis.Brand = "Zara"
	} else if strings.Contains(description, "h&m") || strings.Contains(description, "hm") {
		analysis.Brand = "H&M"
	}

	// Add more tags based on description
	if strings.Contains(description, "casual") {
		analysis.Tags = append(analysis.Tags, "casual")
	}
	if strings.Contains(description, "formal") {
		analysis.Tags = append(analysis.Tags, "formal")
	}
	if strings.Contains(description, "vintage") {
		analysis.Tags = append(analysis.Tags, "vintage")
	}
	if strings.Contains(description, "modern") {
		analysis.Tags = append(analysis.Tags, "modern")
	}

	return analysis, nil
}

// GenerateRecommendations generates personalized recommendations using simple logic
func (a *AIService) GenerateRecommendations(ctx context.Context, userProfile map[string]interface{}, availableItems []map[string]interface{}) ([]*Recommendation, error) {
	var recommendations []*Recommendation

	// Simple recommendation logic based on user profile
	userSize, _ := userProfile["size"].(string)
	userPreferences, _ := userProfile["preferences"].([]string)

	// Generate 3-5 recommendations
	for i := 0; i < 3 && i < len(availableItems); i++ {
		item := availableItems[i]

		recommendation := &Recommendation{
			Type:        "item_recommendation",
			Title:       fmt.Sprintf("Recommended %s", item["category"]),
			Description: fmt.Sprintf("This %s matches your preferences", item["category"]),
			Reason:      "Based on your size and style preferences",
			Score:       0.7 + rand.Float64()*0.2, // Random score between 0.7-0.9
			Data: map[string]interface{}{
				"item_id":  item["id"],
				"category": item["category"],
			},
		}

		// Adjust score based on size match
		if itemSize, ok := item["size"].(string); ok && itemSize == userSize {
			recommendation.Score += 0.1
			recommendation.Reason = "Perfect size match for you"
		}

		// Adjust score based on preferences
		if itemCategory, ok := item["category"].(string); ok {
			for _, pref := range userPreferences {
				if strings.Contains(strings.ToLower(itemCategory), strings.ToLower(pref)) {
					recommendation.Score += 0.05
					break
				}
			}
		}

		recommendations = append(recommendations, recommendation)
	}

	return recommendations, nil
}

// GenerateItemDescription generates an improved description for an item
func (a *AIService) GenerateItemDescription(ctx context.Context, basicInfo map[string]string) (string, error) {
	category := basicInfo["category"]
	size := basicInfo["size"]
	condition := basicInfo["condition"]
	material := basicInfo["material"]
	brand := basicInfo["brand"]
	season := basicInfo["season"]

	description := fmt.Sprintf("Beautiful %s in %s size. Made from high-quality %s material, this item is in %s condition. ",
		category, size, material, condition)

	if brand != "unknown" {
		description += fmt.Sprintf("From the renowned %s brand, ", brand)
	}

	description += fmt.Sprintf("this %s is perfect for %s weather. ", category, season)

	// Add styling suggestions based on category
	switch category {
	case "tops":
		description += "Pairs beautifully with jeans or skirts for a casual yet stylish look."
	case "bottoms":
		description += "Can be styled with various tops for different occasions."
	case "dresses":
		description += "Perfect for both casual outings and special occasions."
	case "outerwear":
		description += "Essential piece for layering and staying warm in cooler weather."
	case "accessories":
		description += "Adds the perfect finishing touch to any outfit."
	default:
		description += "A versatile piece that can be styled in multiple ways."
	}

	return description, nil
}

// AnalyzeUserBehavior analyzes user behavior patterns using simple statistics
func (a *AIService) AnalyzeUserBehavior(ctx context.Context, userHistory []map[string]interface{}) (map[string]interface{}, error) {
	analysis := map[string]interface{}{
		"preferred_categories":    []string{},
		"preferred_sizes":         []string{},
		"preferred_conditions":    []string{},
		"activity_pattern":        "occasional",
		"preferred_seasons":       []string{},
		"style_preferences":       []string{},
		"recommendation_insights": "Based on your history, we recommend items that match your preferences",
	}

	// Count categories
	categoryCount := make(map[string]int)
	sizeCount := make(map[string]int)
	conditionCount := make(map[string]int)
	seasonCount := make(map[string]int)

	for _, item := range userHistory {
		if category, ok := item["category"].(string); ok {
			categoryCount[category]++
		}
		if size, ok := item["size"].(string); ok {
			sizeCount[size]++
		}
		if condition, ok := item["condition"].(string); ok {
			conditionCount[condition]++
		}
		if season, ok := item["season"].(string); ok {
			seasonCount[season]++
		}
	}

	// Find most common categories
	var preferredCategories []string
	for category, count := range categoryCount {
		if count >= 2 { // At least 2 items in this category
			preferredCategories = append(preferredCategories, category)
		}
	}
	analysis["preferred_categories"] = preferredCategories

	// Find most common sizes
	var preferredSizes []string
	for size, count := range sizeCount {
		if count >= 2 {
			preferredSizes = append(preferredSizes, size)
		}
	}
	analysis["preferred_sizes"] = preferredSizes

	// Find most common conditions
	var preferredConditions []string
	for condition, count := range conditionCount {
		if count >= 2 {
			preferredConditions = append(preferredConditions, condition)
		}
	}
	analysis["preferred_conditions"] = preferredConditions

	// Find most common seasons
	var preferredSeasons []string
	for season, count := range seasonCount {
		if count >= 2 {
			preferredSeasons = append(preferredSeasons, season)
		}
	}
	analysis["preferred_seasons"] = preferredSeasons

	// Determine activity pattern
	historyLength := len(userHistory)
	if historyLength >= 10 {
		analysis["activity_pattern"] = "frequent"
	} else if historyLength >= 5 {
		analysis["activity_pattern"] = "occasional"
	} else {
		analysis["activity_pattern"] = "rare"
	}

	// Style preferences based on categories
	var stylePreferences []string
	if len(preferredCategories) > 0 {
		for _, category := range preferredCategories {
			switch category {
			case "tops":
				stylePreferences = append(stylePreferences, "casual")
			case "dresses":
				stylePreferences = append(stylePreferences, "elegant")
			case "outerwear":
				stylePreferences = append(stylePreferences, "practical")
			}
		}
	}
	analysis["style_preferences"] = stylePreferences

	return analysis, nil
}

// Close closes the AI service
func (a *AIService) Close() error {
	return nil
}
