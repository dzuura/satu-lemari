package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/dzuura/satu-lemari/domain/config"
)

// SimpleSmartListingService provides smart listing without external AI dependencies
type SimpleSmartListingService struct {
	config *config.Config
}

// NewSimpleSmartListingService creates a new simple smart listing service
func NewSimpleSmartListingService(cfg *config.Config) *SimpleSmartListingService {
	return &SimpleSmartListingService{
		config: cfg,
	}
}

// AutoListItems analyzes images and generates complete item listing using rule-based logic
func (s *SimpleSmartListingService) AutoListItems(ctx context.Context, images [][]byte, basicInfo map[string]string) (*SmartListingResult, error) {
	description := basicInfo["description"]
	if description == "" {
		description = "Item fashion"
	}

	// Use basic AI service for analysis
	basicAI, err := NewAIService(s.config)
	if err != nil {
		return s.fallbackAnalysis(basicInfo), nil
	}

	analysis, err := basicAI.AnalyzeItem(ctx, description, nil)
	if err != nil {
		return s.fallbackAnalysis(basicInfo), nil
	}

	// Convert to SmartListingResult with enhanced logic
	result := &SmartListingResult{
		Category:        analysis.Category,
		Subcategory:     analysis.Subcategory,
		Size:            analysis.Size,
		Condition:       analysis.Condition,
		Material:        analysis.Material,
		Brand:           analysis.Brand,
		Season:          analysis.Season,
		Color:           s.detectColor(description),
		SuggestedPrice:  s.suggestPrice(analysis.Category, analysis.Condition, analysis.Material),
		Description:     s.generateDescription(analysis, description),
		Tags:            analysis.Tags,
		ConfidenceScore: analysis.ConfidenceScore,
		AnalysisNotes:   "Rule-based analysis with enhanced pricing and description",
	}

	return result, nil
}

// detectColor extracts color information from description
func (s *SimpleSmartListingService) detectColor(description string) string {
	description = strings.ToLower(description)

	colorPatterns := map[string]string{
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

	for pattern, color := range colorPatterns {
		if strings.Contains(description, pattern) {
			return color
		}
	}

	return "unknown"
}

// suggestPrice suggests price based on category, condition, and material
func (s *SimpleSmartListingService) suggestPrice(category, condition, material string) float64 {
	basePrice := 50000.0

	// Category multiplier
	switch strings.ToLower(category) {
	case "accessories":
		basePrice = 30000
	case "tops":
		basePrice = 40000
	case "bottoms":
		basePrice = 50000
	case "dresses":
		basePrice = 80000
	case "outerwear":
		basePrice = 100000
	}

	// Condition multiplier
	switch strings.ToLower(condition) {
	case "excellent":
		basePrice *= 1.2
	case "good":
		basePrice *= 1.0
	case "fair":
		basePrice *= 0.8
	case "poor":
		basePrice *= 0.6
	}

	// Material multiplier
	switch strings.ToLower(material) {
	case "silk":
		basePrice *= 1.5
	case "wool":
		basePrice *= 1.3
	case "denim":
		basePrice *= 1.1
	case "cotton":
		basePrice *= 1.0
	case "polyester":
		basePrice *= 0.9
	}

	return basePrice
}

// generateDescription creates an attractive description
func (s *SimpleSmartListingService) generateDescription(analysis *ItemAnalysis, userDesc string) string {
	if userDesc != "" && userDesc != "Item fashion" {
		return userDesc
	}

	template := "Barang %s dalam kondisi %s. Terbuat dari bahan %s yang nyaman dipakai. Cocok untuk musim %s."

	subcategory := analysis.Subcategory
	if subcategory == "" {
		subcategory = "fashion"
	}

	condition := analysis.Condition
	if condition == "" {
		condition = "baik"
	}

	material := analysis.Material
	if material == "" {
		material = "cotton"
	}

	season := analysis.Season
	if season == "" {
		season = "semua musim"
	}

	return fmt.Sprintf(template, subcategory, condition, material, season)
}

// fallbackAnalysis provides basic analysis when AI is not available
func (s *SimpleSmartListingService) fallbackAnalysis(basicInfo map[string]string) *SmartListingResult {
	description := basicInfo["description"]
	if description == "" {
		description = "Item dalam kondisi baik"
	}

	return &SmartListingResult{
		Category:        "unknown",
		Subcategory:     "unknown",
		Size:            "M",
		Condition:       "good",
		Material:        "cotton",
		Brand:           "unknown",
		Season:          "all-season",
		Color:           "unknown",
		SuggestedPrice:  50000,
		Description:     description,
		Tags:            []string{"fashion", "clothing"},
		ConfidenceScore: 0.5,
		AnalysisNotes:   "Basic fallback analysis",
	}
}

// Close closes the service
func (s *SimpleSmartListingService) Close() error {
	return nil
}
 