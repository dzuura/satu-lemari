package ai

import (
	"github.com/dzuura/satu-lemari/domain/models"
	"github.com/google/uuid"
)

// SmartListingResult represents the result of AI smart listing analysis
// Fields aligned with actual database schema
type SmartListingResult struct {
	// Core fields that exist in database
	CategoryID     *uuid.UUID `json:"category_id,omitempty"`     // Maps to categories table
	Name           string     `json:"name"`                      // Item name
	Description    string     `json:"description"`               // Item description
	Size           string     `json:"size"`                      // Size (required in DB)
	Color          *string    `json:"color,omitempty"`           // Color (optional in DB)
	Condition      string     `json:"condition"`                 // excellent|good|fair
	Type           string     `json:"type"`                      // donation|rental
	SuggestedPrice *float64   `json:"suggested_price,omitempty"` // Price for rental items

	// AI analysis metadata
	ConfidenceScore float64 `json:"confidence_score"` // AI confidence (0.0-1.0)
	AnalysisNotes   string  `json:"analysis_notes"`   // AI analysis notes

	// Additional AI insights (not stored in DB but useful for analysis)
	CategoryName    string  `json:"category_name,omitempty"`    // Human-readable category name
	SizeConfidence  float64 `json:"size_confidence,omitempty"`  // Confidence in size detection
	ColorConfidence float64 `json:"color_confidence,omitempty"` // Confidence in color detection
}

// IntentResult represents the result of intent analysis
type IntentResult struct {
	Intent      string                 `json:"intent"` // search, filter, browse, etc.
	Confidence  float64                `json:"confidence"`
	Entities    map[string]interface{} `json:"entities"`    // extracted data
	Filters     *models.ItemFilter     `json:"filters"`     // converted filters
	Query       string                 `json:"query"`       // original query
	Suggestions []string               `json:"suggestions"` // search suggestions
}
