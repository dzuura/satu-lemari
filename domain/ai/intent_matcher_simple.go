package ai

import (
	"context"
	"strings"

	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/models"
)

// SimpleIntentMatcherService provides intent matching without external AI dependencies
type SimpleIntentMatcherService struct {
	config *config.Config
}

// NewSimpleIntentMatcherService creates a new simple intent matcher service
func NewSimpleIntentMatcherService(cfg *config.Config) *SimpleIntentMatcherService {
	return &SimpleIntentMatcherService{
		config: cfg,
	}
}

// ParseIntent analyzes natural language query and extracts intent and entities
func (s *SimpleIntentMatcherService) ParseIntent(ctx context.Context, query string) (*IntentResult, error) {
	return s.fallbackIntentAnalysis(query), nil
}

// fallbackIntentAnalysis provides rule-based intent analysis
func (s *SimpleIntentMatcherService) fallbackIntentAnalysis(query string) *IntentResult {
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

	// Check for browse keywords
	browseKeywords := []string{"lihat", "browse", "semua", "apa", "ada", "tampilkan"}
	for _, keyword := range browseKeywords {
		if strings.Contains(query, keyword) {
			intent = "browse"
			confidence = 0.8
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
		"xs": "XS", "extra small": "XS", "ekstra kecil": "XS",
		"s": "S", "small": "S", "kecil": "S",
		"m": "M", "medium": "M", "sedang": "M",
		"l": "L", "large": "L", "besar": "L",
		"xl": "XL", "extra large": "XL", "ekstra besar": "XL",
		"xxl": "XXL", "2xl": "XXL",
	}

	for pattern, size := range sizePatterns {
		if strings.Contains(query, pattern) {
			entities["size"] = size
			break
		}
	}

	// Category detection
	categoryPatterns := map[string]string{
		"baju": "tops", "kaos": "tops", "shirt": "tops", "kemeja": "tops", "blouse": "tops",
		"celana": "bottoms", "jeans": "bottoms", "pants": "bottoms", "rok": "bottoms", "skirt": "bottoms",
		"dress": "dresses", "gaun": "dresses", "gown": "dresses",
		"jaket": "outerwear", "coat": "outerwear", "mantel": "outerwear", "cardigan": "outerwear",
		"tas": "accessories", "bag": "accessories", "dompet": "accessories", "sepatu": "accessories", "shoes": "accessories",
	}

	for pattern, category := range categoryPatterns {
		if strings.Contains(query, pattern) {
			entities["category"] = category
			break
		}
	}

	// Price detection
	if strings.Contains(query, "murah") || strings.Contains(query, "cheap") || strings.Contains(query, "harga rendah") {
		entities["max_price"] = 50000.0
	} else if strings.Contains(query, "mahal") || strings.Contains(query, "expensive") || strings.Contains(query, "harga tinggi") {
		entities["min_price"] = 100000.0
	} else if strings.Contains(query, "gratis") || strings.Contains(query, "free") {
		entities["max_price"] = 10000.0
	}

	// Color detection
	colorKeywords := map[string]string{
		"hitam": "black", "black": "black",
		"putih": "white", "white": "white",
		"merah": "red", "red": "red",
		"biru": "blue", "blue": "blue",
		"hijau": "green", "green": "green",
		"kuning": "yellow", "yellow": "yellow",
		"pink": "pink", "merah muda": "pink",
		"abu-abu": "gray", "gray": "gray", "grey": "gray",
		"coklat": "brown", "brown": "brown",
		"ungu": "purple", "purple": "purple",
		"orange": "orange", "jingga": "orange",
	}

	for pattern, color := range colorKeywords {
		if strings.Contains(query, pattern) {
			entities["color"] = color
			break
		}
	}

	// Condition detection
	conditionPatterns := map[string]string{
		"baru": "excellent", "new": "excellent", "mint": "excellent",
		"baik": "good", "good": "good", "bagus": "good",
		"cukup": "fair", "fair": "fair", "lumayan": "fair",
		"rusak": "poor", "poor": "poor", "jelek": "poor",
	}

	for pattern, condition := range conditionPatterns {
		if strings.Contains(query, pattern) {
			entities["condition"] = condition
			break
		}
	}

	// Material detection
	materialPatterns := map[string]string{
		"katun": "cotton", "cotton": "cotton",
		"polyester": "polyester", "poliester": "polyester",
		"wol": "wool", "wool": "wool",
		"sutra": "silk", "silk": "silk",
		"denim": "denim", "jeans": "denim",
	}

	for pattern, material := range materialPatterns {
		if strings.Contains(query, pattern) {
			entities["material"] = material
			break
		}
	}

	// Season detection
	seasonPatterns := map[string]string{
		"musim panas": "summer", "summer": "summer", "panas": "summer",
		"musim dingin": "winter", "winter": "winter", "dingin": "winter",
		"musim semi": "spring", "spring": "spring",
		"musim gugur": "fall", "fall": "fall", "autumn": "fall",
	}

	for pattern, season := range seasonPatterns {
		if strings.Contains(query, pattern) {
			entities["season"] = season
			break
		}
	}

	// Brand detection (simple)
	brandKeywords := []string{"nike", "adidas", "puma", "reebok", "converse", "vans", "uniqlo", "zara", "h&m"}
	for _, brand := range brandKeywords {
		if strings.Contains(query, brand) {
			entities["brand"] = brand
			break
		}
	}

	return &IntentResult{
		Intent:      intent,
		Confidence:  confidence,
		Entities:    entities,
		Filters:     s.convertEntitiesToFilters(entities),
		Query:       query,
		Suggestions: s.getFallbackSuggestions(query),
	}
}

// convertEntitiesToFilters converts extracted entities to ItemFilter
func (s *SimpleIntentMatcherService) convertEntitiesToFilters(entities map[string]interface{}) *models.ItemFilter {
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

	// Convert price range
	if minPrice, ok := entities["min_price"].(float64); ok && minPrice > 0 {
		filter.MinPrice = &minPrice
	}
	if maxPrice, ok := entities["max_price"].(float64); ok && maxPrice > 0 {
		filter.MaxPrice = &maxPrice
	}

	// Convert search terms (brand, category, etc.)
	if brand, ok := entities["brand"].(string); ok && brand != "" {
		filter.Search = &brand
	}

	return filter
}

// GetSearchSuggestions generates search suggestions
func (s *SimpleIntentMatcherService) GetSearchSuggestions(ctx context.Context, query string) ([]string, error) {
	return s.getFallbackSuggestions(query), nil
}

// getFallbackSuggestions provides basic suggestions
func (s *SimpleIntentMatcherService) getFallbackSuggestions(query string) []string {
	query = strings.ToLower(query)

	suggestions := []string{
		"coba cari dengan kata kunci yang lebih spesifik",
		"gunakan filter untuk hasil yang lebih akurat",
	}

	// Add category-specific suggestions
	if strings.Contains(query, "baju") || strings.Contains(query, "kaos") {
		suggestions = append(suggestions, "kemeja formal", "kaos casual", "blouse wanita")
	} else if strings.Contains(query, "celana") {
		suggestions = append(suggestions, "celana jeans", "celana formal", "celana pendek")
	} else if strings.Contains(query, "dress") {
		suggestions = append(suggestions, "dress casual", "dress formal", "dress pesta")
	} else if strings.Contains(query, "jaket") {
		suggestions = append(suggestions, "jaket denim", "jaket kulit", "cardigan")
	} else if strings.Contains(query, "tas") {
		suggestions = append(suggestions, "tas ransel", "tas tangan", "dompet")
	}

	// Add price-based suggestions
	if strings.Contains(query, "murah") {
		suggestions = append(suggestions, "harga di bawah 50 ribu", "barang second murah")
	} else if strings.Contains(query, "mahal") {
		suggestions = append(suggestions, "brand premium", "barang original")
	}

	// Add size-based suggestions
	if strings.Contains(query, "ukuran") || strings.Contains(query, "size") {
		suggestions = append(suggestions, "ukuran S", "ukuran M", "ukuran L", "ukuran XL")
	}

	return suggestions
}

// Close closes the service
func (s *SimpleIntentMatcherService) Close() error {
	return nil
}
 