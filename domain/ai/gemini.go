package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/models"
)

// GeminiService provides AI-powered recommendations and search intent using Google Gemini API
type GeminiService struct {
	config     *config.Config
	httpClient *http.Client
	apiURL     string
}

// GeminiRequest represents the request structure for Gemini API
type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

// GeminiContent represents content in Gemini request
type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a part of content
type GeminiPart struct {
	Text string `json:"text"`
}

// GeminiResponse represents the response from Gemini API
type GeminiResponse struct {
	Candidates []GeminiCandidate `json:"candidates"`
}

// GeminiCandidate represents a candidate response
type GeminiCandidate struct {
	Content GeminiContent `json:"content"`
}

// NewGeminiService creates a new Gemini AI service
func NewGeminiService(cfg *config.Config) *GeminiService {
	return &GeminiService{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiURL:     fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", cfg.GeminiModel),
	}
}

// ParseIntent analyzes natural language query using Gemini AI
func (g *GeminiService) ParseIntent(ctx context.Context, query string) (*IntentResult, error) {
	prompt := g.buildIntentPrompt(query)

	response, err := g.callGeminiAPI(ctx, prompt)
	if err != nil {
		log.Printf("Gemini API error for intent parsing: %v", err)
		return g.fallbackIntentAnalysis(query), nil
	}

	return g.parseIntentResponse(response, query)
}

// GenerateRecommendations creates personalized recommendations using Gemini AI
func (g *GeminiService) GenerateRecommendations(ctx context.Context, userProfile map[string]interface{}, availableItems []map[string]interface{}) ([]*Recommendation, error) {
	prompt := g.buildRecommendationPrompt(userProfile, availableItems)

	response, err := g.callGeminiAPI(ctx, prompt)
	if err != nil {
		log.Printf("Gemini API error for recommendations: %v", err)
		return g.fallbackRecommendations(availableItems), nil
	}

	return g.parseRecommendationResponse(response, availableItems)
}

// GetSearchSuggestions generates intelligent search suggestions
func (g *GeminiService) GetSearchSuggestions(ctx context.Context, query string) ([]string, error) {
	prompt := g.buildSuggestionPrompt(query)

	response, err := g.callGeminiAPI(ctx, prompt)
	if err != nil {
		log.Printf("Gemini API error for suggestions: %v", err)
		return g.fallbackSuggestions(query), nil
	}

	return g.parseSuggestionResponse(response)
}

// buildIntentPrompt creates a prompt for intent analysis
func (g *GeminiService) buildIntentPrompt(query string) string {
	return fmt.Sprintf(`
Analisis query pencarian bahasa Indonesia ini untuk platform sharing fashion dan ekstrak informasi terstruktur:

Query: "%s"

Mohon berikan respons dalam format JSON object:
{
  "intent": "search|filter|browse|recommend",
  "confidence": 0.0-1.0,
  "entities": {
    "category": "string (jika disebutkan)",
    "size": "XS|S|M|L|XL|XXL (jika disebutkan)",
    "color": "string (jika disebutkan)",
    "condition": "excellent|good|fair (jika disebutkan)",
    "type": "donation|rental (jika disebutkan)",
    "price_range": "low|medium|high (jika disebutkan)",
    "brand": "string (jika disebutkan)",
    "material": "string (jika disebutkan)",
    "season": "summer|winter|spring|fall (jika disebutkan)"
  },
  "search_terms": ["kata", "kunci", "yang", "diekstrak"],
  "suggestions": ["saran", "pencarian", "terkait", "dalam", "bahasa", "indonesia"]
}

Fokus pada pola bahasa Indonesia dan terminologi fashion lokal.
Berikan suggestions dalam bahasa Indonesia yang natural.
`, query)
}

// buildRecommendationPrompt creates a prompt for generating recommendations
func (g *GeminiService) buildRecommendationPrompt(userProfile map[string]interface{}, availableItems []map[string]interface{}) string {
	userJSON, _ := json.Marshal(userProfile)
	itemsJSON, _ := json.Marshal(availableItems)

	return fmt.Sprintf(`
Buatkan rekomendasi item fashion yang dipersonalisasi untuk pengguna platform sharing fashion Indonesia.

Profil Pengguna:
%s

Item yang Tersedia:
%s

Mohon berikan respons dalam format JSON array dengan rekomendasi:
[
  {
    "item_id": "string",
    "title": "string",
    "description": "deskripsi dalam bahasa Indonesia mengapa item ini cocok",
    "reason": "alasan rekomendasi dalam bahasa Indonesia",
    "score": 0.0-1.0,
    "category": "string",
    "tags": ["tag", "relevan"]
  }
]

Pertimbangkan:
- Preferensi ukuran dan riwayat pengguna
- Kompatibilitas gaya fashion Indonesia
- Kesesuaian musim di iklim tropis Indonesia
- Ketersediaan dan kondisi item
- Keragaman dalam rekomendasi
- Preferensi fashion Indonesia dan budaya lokal

Gunakan bahasa Indonesia untuk description dan reason.
Batasi hingga 5 rekomendasi teratas.
`, string(userJSON), string(itemsJSON))
}

// buildSuggestionPrompt creates a prompt for search suggestions
func (g *GeminiService) buildSuggestionPrompt(query string) string {
	return fmt.Sprintf(`
Buatkan saran pencarian yang cerdas untuk query fashion Indonesia ini: "%s"

Mohon berikan respons dalam format JSON array dengan 5-8 saran pencarian terkait:
[
  "saran 1",
  "saran 2",
  "saran 3"
]

Pertimbangkan:
- Terminologi fashion Indonesia
- Kategori dan gaya terkait
- Variasi ukuran dan warna
- Alternatif brand
- Item musiman untuk iklim Indonesia
- Trend fashion populer di Indonesia

Buat saran yang spesifik dan dapat ditindaklanjuti untuk platform sharing fashion Indonesia.
Gunakan bahasa Indonesia yang natural dan mudah dipahami.
`, query)
}

// callGeminiAPI makes a request to Gemini API
func (g *GeminiService) callGeminiAPI(ctx context.Context, prompt string) (string, error) {
	if g.config.GeminiAPIKey == "" {
		return "", fmt.Errorf("gemini API key not configured")
	}

	request := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{Text: prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	url := fmt.Sprintf("%s?key=%s", g.apiURL, g.config.GeminiAPIKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call Gemini API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from gemini API")
	}

	return geminiResp.Candidates[0].Content.Parts[0].Text, nil
}

// parseIntentResponse parses Gemini response for intent analysis
func (g *GeminiService) parseIntentResponse(response, originalQuery string) (*IntentResult, error) {
	// Extract JSON from response
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}") + 1

	if jsonStart == -1 || jsonEnd <= jsonStart {
		return g.fallbackIntentAnalysis(originalQuery), nil
	}

	jsonStr := response[jsonStart:jsonEnd]

	var result struct {
		Intent      string                 `json:"intent"`
		Confidence  float64                `json:"confidence"`
		Entities    map[string]interface{} `json:"entities"`
		SearchTerms []string               `json:"search_terms"`
		Suggestions []string               `json:"suggestions"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		log.Printf("Failed to parse intent JSON: %v", err)
		return g.fallbackIntentAnalysis(originalQuery), nil
	}

	return &IntentResult{
		Intent:      result.Intent,
		Confidence:  result.Confidence,
		Entities:    result.Entities,
		Filters:     g.convertEntitiesToFilters(result.Entities),
		Query:       originalQuery,
		Suggestions: result.Suggestions,
	}, nil
}

// parseRecommendationResponse parses Gemini response for recommendations
func (g *GeminiService) parseRecommendationResponse(response string, availableItems []map[string]interface{}) ([]*Recommendation, error) {
	// Extract JSON array from response
	jsonStart := strings.Index(response, "[")
	jsonEnd := strings.LastIndex(response, "]") + 1

	if jsonStart == -1 || jsonEnd <= jsonStart {
		return g.fallbackRecommendations(availableItems), nil
	}

	jsonStr := response[jsonStart:jsonEnd]

	var results []struct {
		ItemID      string   `json:"item_id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Reason      string   `json:"reason"`
		Score       float64  `json:"score"`
		Category    string   `json:"category"`
		Tags        []string `json:"tags"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		log.Printf("Failed to parse recommendation JSON: %v", err)
		return g.fallbackRecommendations(availableItems), nil
	}

	// Create a map for quick item lookup
	itemMap := make(map[string]map[string]interface{})
	for _, item := range availableItems {
		if itemID, ok := item["id"].(string); ok {
			itemMap[itemID] = item
		}
	}

	var recommendations []*Recommendation
	for _, result := range results {
		// Get item details from availableItems
		var itemName string
		var images []string
		if item, exists := itemMap[result.ItemID]; exists {
			if name, ok := item["name"].(string); ok {
				itemName = name
			}
			// Handle images array
			if imgArray, ok := item["images"].([]interface{}); ok {
				for _, img := range imgArray {
					if imgStr, ok := img.(string); ok {
						images = append(images, imgStr)
					}
				}
			} else if imgArray, ok := item["images"].([]string); ok {
				images = imgArray
			}
		}

		recommendation := &Recommendation{
			Type:        "item_recommendation",
			Title:       result.Title,
			Description: result.Description,
			Reason:      result.Reason,
			Score:       result.Score,
			Data: map[string]interface{}{
				"item_id":  result.ItemID,
				"name":     itemName,
				"images":   images,
				"category": result.Category,
				"tags":     result.Tags,
			},
		}
		recommendations = append(recommendations, recommendation)
	}

	return recommendations, nil
}

// parseSuggestionResponse parses Gemini response for suggestions
func (g *GeminiService) parseSuggestionResponse(response string) ([]string, error) {
	// Extract JSON array from response
	jsonStart := strings.Index(response, "[")
	jsonEnd := strings.LastIndex(response, "]") + 1

	if jsonStart == -1 || jsonEnd <= jsonStart {
		return g.fallbackSuggestions(""), nil
	}

	jsonStr := response[jsonStart:jsonEnd]

	var suggestions []string
	if err := json.Unmarshal([]byte(jsonStr), &suggestions); err != nil {
		log.Printf("Failed to parse suggestion JSON: %v", err)
		return g.fallbackSuggestions(""), nil
	}

	return suggestions, nil
}

// convertEntitiesToFilters converts extracted entities to ItemFilter
func (g *GeminiService) convertEntitiesToFilters(entities map[string]interface{}) *models.ItemFilter {
	filter := &models.ItemFilter{}

	// Convert size
	if size, ok := entities["size"].(string); ok && size != "" {
		filter.Size = &size
	}

	// Convert color
	if color, ok := entities["color"].(string); ok && color != "" {
		filter.Color = &color
	}

	// Convert condition
	if condition, ok := entities["condition"].(string); ok && condition != "" {
		filter.Condition = &condition
	}

	// Convert type
	if itemType, ok := entities["type"].(string); ok && itemType != "" {
		filter.Type = &itemType
	}

	// Convert price range to actual values
	if priceRange, ok := entities["price_range"].(string); ok {
		switch priceRange {
		case "low":
			maxPrice := 50000.0
			filter.MaxPrice = &maxPrice
		case "medium":
			minPrice := 50000.0
			maxPrice := 200000.0
			filter.MinPrice = &minPrice
			filter.MaxPrice = &maxPrice
		case "high":
			minPrice := 200000.0
			filter.MinPrice = &minPrice
		}
	}

	// Convert search terms (brand, category, etc.)
	if brand, ok := entities["brand"].(string); ok && brand != "" {
		filter.Search = &brand
	} else if category, ok := entities["category"].(string); ok && category != "" {
		filter.Search = &category
	}

	return filter
}

// fallbackIntentAnalysis provides rule-based fallback for intent analysis
func (g *GeminiService) fallbackIntentAnalysis(query string) *IntentResult {
	query = strings.ToLower(query)

	// Simple intent classification
	intent := "search"
	confidence := 0.6

	// Check for filter keywords
	filterKeywords := []string{"ukuran", "warna", "kondisi", "harga", "brand", "merek", "filter"}
	for _, keyword := range filterKeywords {
		if strings.Contains(query, keyword) {
			intent = "filter"
			confidence = 0.7
			break
		}
	}

	// Check for recommendation keywords
	recommendKeywords := []string{"rekomendasi", "saran", "recommend", "suggest"}
	for _, keyword := range recommendKeywords {
		if strings.Contains(query, keyword) {
			intent = "recommend"
			confidence = 0.75
			break
		}
	}

	// Extract basic entities
	entities := make(map[string]interface{})

	// Size detection
	sizePatterns := map[string]string{
		"xs": "XS", "s": "S", "m": "M", "l": "L", "xl": "XL", "xxl": "XXL",
	}
	for pattern, size := range sizePatterns {
		if strings.Contains(query, pattern) {
			entities["size"] = size
			break
		}
	}

	// Color detection
	colorKeywords := map[string]string{
		"hitam": "black", "putih": "white", "merah": "red", "biru": "blue",
		"hijau": "green", "kuning": "yellow", "pink": "pink",
	}
	for pattern, color := range colorKeywords {
		if strings.Contains(query, pattern) {
			entities["color"] = color
			break
		}
	}

	return &IntentResult{
		Intent:      intent,
		Confidence:  confidence,
		Entities:    entities,
		Filters:     g.convertEntitiesToFilters(entities),
		Query:       query,
		Suggestions: g.fallbackSuggestions(query),
	}
}

// fallbackRecommendations provides simple fallback recommendations
func (g *GeminiService) fallbackRecommendations(availableItems []map[string]interface{}) []*Recommendation {
	var recommendations []*Recommendation

	// Generate simple recommendations from available items
	for i := 0; i < 3 && i < len(availableItems); i++ {
		item := availableItems[i]

		recommendation := &Recommendation{
			Type:        "item_recommendation",
			Title:       fmt.Sprintf("Recommended %s", item["name"]),
			Description: fmt.Sprintf("Popular item in %s category", item["category"]),
			Reason:      "Based on popularity and availability",
			Score:       0.7,
			Data: map[string]interface{}{
				"item_id":  item["id"],
				"category": item["category"],
			},
		}
		recommendations = append(recommendations, recommendation)
	}

	return recommendations
}

// fallbackSuggestions provides basic fallback suggestions
func (g *GeminiService) fallbackSuggestions(query string) []string {
	suggestions := []string{
		"coba cari dengan kata kunci yang lebih spesifik",
		"gunakan filter untuk hasil yang lebih akurat",
		"lihat kategori pakaian populer",
		"cari berdasarkan ukuran yang sesuai",
		"filter berdasarkan kondisi barang",
	}

	// Add query-specific suggestions
	if strings.Contains(query, "hoodie") {
		suggestions = append(suggestions, "hoodie oversize", "hoodie vintage", "hoodie branded")
	}
	if strings.Contains(query, "kemeja") {
		suggestions = append(suggestions, "kemeja formal", "kemeja casual", "kemeja vintage")
	}

	return suggestions
}

// Close closes the service
func (g *GeminiService) Close() error {
	return nil
}
