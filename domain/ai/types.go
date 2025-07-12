package ai

import "github.com/dzuura/satu-lemari/domain/models"

// SmartListingResult represents the result of AI smart listing analysis
type SmartListingResult struct {
	Category        string   `json:"category"`
	Subcategory     string   `json:"subcategory"`
	Size            string   `json:"size"`
	Condition       string   `json:"condition"`
	Material        string   `json:"material"`
	Brand           string   `json:"brand"`
	Season          string   `json:"season"`
	Color           string   `json:"color"`
	SuggestedPrice  float64  `json:"suggested_price"`
	Description     string   `json:"description"`
	Tags            []string `json:"tags"`
	ConfidenceScore float64  `json:"confidence_score"`
	AnalysisNotes   string   `json:"analysis_notes"`
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
