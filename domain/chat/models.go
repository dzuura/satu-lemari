package chat

import (
	"context"
	"fmt"
	"time"
)

// ChatSession represents a chat session
type ChatSession struct {
	ID           string                 `json:"id" db:"id"`
	UserID       string                 `json:"user_id" db:"user_id"` // Firebase UID
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	LastActivity time.Time              `json:"last_activity" db:"last_activity"`
	Context      map[string]interface{} `json:"context" db:"context"`
	IsActive     bool                   `json:"is_active" db:"is_active"`
}

// Message represents a chat message
type Message struct {
	ID        string                 `json:"id" db:"id"`
	SessionID string                 `json:"session_id" db:"session_id"`
	Role      string                 `json:"role" db:"role"` // user/assistant
	Content   string                 `json:"content" db:"content"`
	Timestamp time.Time              `json:"timestamp" db:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
}

// ChatContext represents the context for a chat message
type ChatContext struct {
	Page         string                 `json:"page,omitempty"`
	ItemID       string                 `json:"item_id,omitempty"`
	Action       string                 `json:"action,omitempty"`
	UserLocation *UserLocation          `json:"user_location,omitempty"`
	Additional   map[string]interface{} `json:"additional,omitempty"`
}

// UserLocation represents user's location context
type UserLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	City      string  `json:"city,omitempty"`
}

// StartChatRequest represents request to start a chat session
type StartChatRequest struct {
	Language string `json:"language,omitempty" validate:"omitempty,oneof=id en"`
}

// SendMessageRequest represents request to send a message
type SendMessageRequest struct {
	SessionID string       `json:"session_id" validate:"required"`
	Message   string       `json:"message" validate:"required,max=1000"`
	Context   *ChatContext `json:"context,omitempty"`
}

// ChatResponse represents the chatbot's response
type ChatResponse struct {
	SessionID     string                 `json:"session_id"`
	Message       string                 `json:"message"`
	MessageType   string                 `json:"message_type"` // text, quick_reply, item_card, etc.
	Suggestions   []string               `json:"suggestions,omitempty"`
	QuickReplies  []QuickReply           `json:"quick_replies,omitempty"`
	ItemCards     []ItemCard             `json:"item_cards,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
	CanContinue   bool                   `json:"can_continue"`
	SessionActive bool                   `json:"session_active"`
}

// QuickReply represents a quick reply option
type QuickReply struct {
	Text    string `json:"text"`
	Payload string `json:"payload"`
	Icon    string `json:"icon,omitempty"`
}

// ItemCard represents an item card in chat response
type ItemCard struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Size        string   `json:"size"`
	Condition   string   `json:"condition"`
	Type        string   `json:"type"` // donation/rental
	Price       *float64 `json:"price,omitempty"`
	Images      []string `json:"images"`
	PartnerName string   `json:"partner_name"`
	Distance    *float64 `json:"distance,omitempty"`
}

// ChatHistoryResponse represents chat history response
type ChatHistoryResponse struct {
	SessionID string    `json:"session_id"`
	Messages  []Message `json:"messages"`
	Total     int       `json:"total"`
	HasMore   bool      `json:"has_more"`
}

// ChatSuggestionsResponse represents chat suggestions
type ChatSuggestionsResponse struct {
	Suggestions []ChatSuggestion `json:"suggestions"`
	Categories  []string         `json:"categories"`
}

// ChatSuggestion represents a chat suggestion
type ChatSuggestion struct {
	Text        string `json:"text"`
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
}

// Intent represents the parsed intent from user message
type Intent struct {
	Type       string                 `json:"type"`       // donation, rental, search, help, education
	Category   string                 `json:"category"`   // clothing category
	Action     string                 `json:"action"`     // specific action
	Entities   map[string]interface{} `json:"entities"`   // extracted entities
	Confidence float64                `json:"confidence"` // confidence score
}

// KnowledgeBase represents knowledge base entry
type KnowledgeBase struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Topic       string   `json:"topic"`
	Question    string   `json:"question"`
	Answer      string   `json:"answer"`
	Keywords    []string `json:"keywords"`
	RelatedFAQs []string `json:"related_faqs,omitempty"`
}

// ChatStats represents chat statistics
type ChatStats struct {
	TotalSessions    int `json:"total_sessions"`
	ActiveSessions   int `json:"active_sessions"`
	TotalMessages    int `json:"total_messages"`
	AvgSessionLength int `json:"avg_session_length"` // in minutes
}

// Response generation methods

// generateResponse generates a response based on intent and context
func (s *ChatService) generateResponse(ctx context.Context, session *ChatSession, message string, intent *Intent, context *ChatContext) (*ChatResponse, error) {
	response := &ChatResponse{
		SessionID:     session.ID,
		MessageType:   "text",
		Timestamp:     time.Now(),
		CanContinue:   true,
		SessionActive: true,
		Metadata:      make(map[string]interface{}),
	}

	language := s.getLanguageFromSession(session)

	switch intent.Type {
	case "donation":
		return s.generateDonationResponse(ctx, response, intent, context, language)
	case "rental":
		return s.generateRentalResponse(ctx, response, intent, context, language)
	case "search":
		return s.generateSearchResponse(ctx, response, message, intent, context, language)
	case "education":
		return s.generateEducationResponse(ctx, response, message, intent, context, language)
	case "help":
		return s.generateHelpResponse(ctx, response, intent, context, language)
	default:
		return s.generateDefaultResponse(ctx, response, language)
	}
}

// generateDonationResponse generates response for donation-related queries
func (s *ChatService) generateDonationResponse(_ context.Context, response *ChatResponse, _ *Intent, _ *ChatContext, language string) (*ChatResponse, error) {
	kb := SearchKnowledgeBase("donasi", "donation")

	if len(kb) > 0 {
		response.Message = kb[0].Answer
		response.MessageType = "text"

		// Add quick replies for donation-related actions
		response.QuickReplies = []QuickReply{
			{Text: "Cara mendonasikan pakaian", Payload: "donation_process", Icon: "📝"},
			{Text: "Kuota donasi mingguan", Payload: "donation_quota", Icon: "📊"},
			{Text: "Kondisi pakaian yang diterima", Payload: "item_condition", Icon: "👕"},
			{Text: "Lihat item saya", Payload: "my_items", Icon: "👔"},
		}
	} else {
		if language == "en" {
			response.Message = "I can help you with clothing donations. You can donate clothes in good condition to help others in need. Would you like to know more about the donation process?"
		} else {
			response.Message = "Saya dapat membantu Anda dengan donasi pakaian. Anda dapat mendonasikan pakaian dalam kondisi baik untuk membantu orang yang membutuhkan. Apakah Anda ingin tahu lebih lanjut tentang proses donasi?"
		}
	}

	return response, nil
}

// generateRentalResponse generates response for rental-related queries
func (s *ChatService) generateRentalResponse(_ context.Context, response *ChatResponse, _ *Intent, _ *ChatContext, language string) (*ChatResponse, error) {
	kb := SearchKnowledgeBase("sewa", "rental")

	if len(kb) > 0 {
		response.Message = kb[0].Answer
		response.MessageType = "text"

		// Add quick replies for rental-related actions
		response.QuickReplies = []QuickReply{
			{Text: "Cara menyewa pakaian", Payload: "rental_process", Icon: "🔄"},
			{Text: "Durasi sewa", Payload: "rental_duration", Icon: "⏰"},
			{Text: "Cari pakaian sewa", Payload: "search_rental", Icon: "🔍"},
			{Text: "Riwayat sewa saya", Payload: "my_rentals", Icon: "📋"},
		}
	} else {
		if language == "en" {
			response.Message = "I can help you with clothing rentals. You can rent clothes for special occasions or events. Would you like to search for available rental items?"
		} else {
			response.Message = "Saya dapat membantu Anda dengan sewa pakaian. Anda dapat menyewa pakaian untuk acara khusus atau event. Apakah Anda ingin mencari item sewa yang tersedia?"
		}
	}

	return response, nil
}

// generateWelcomeMessage generates welcome message based on language
func (s *ChatService) generateWelcomeMessage(language string) string {
	if language == "en" {
		return "Hello! I'm SatuLemari assistant. I can help you with clothing donations, rentals, care tips, and sustainable fashion. How can I assist you today?"
	}

	return "Halo! Saya asisten SatuLemari. Saya dapat membantu Anda dengan donasi pakaian, sewa, tips perawatan, dan fashion berkelanjutan. Ada yang bisa saya bantu?"
}

// generateSearchResponse generates response for search-related queries
func (s *ChatService) generateSearchResponse(ctx context.Context, response *ChatResponse, message string, _ *Intent, context *ChatContext, language string) (*ChatResponse, error) {
	// Extract search parameters from message
	searchParams := s.extractSearchParams(message)

	// Get user location if available
	var userLat, userLng *float64
	if context != nil && context.UserLocation != nil {
		userLat = &context.UserLocation.Latitude
		userLng = &context.UserLocation.Longitude
	}

	// Search for items
	items, err := s.searchItems(ctx, searchParams, userLat, userLng)
	if err != nil {
		s.logger.Warn("Failed to search items", map[string]interface{}{
			"error": err.Error(),
		})

		if language == "en" {
			response.Message = "I'm having trouble searching for items right now. Please try again later or browse our categories."
		} else {
			response.Message = "Saya mengalami kesulitan mencari item saat ini. Silakan coba lagi nanti atau jelajahi kategori kami."
		}
		return response, nil
	}

	if len(items) == 0 {
		if language == "en" {
			response.Message = "I couldn't find any items matching your search. Try adjusting your criteria or browse our available categories."
		} else {
			response.Message = "Saya tidak dapat menemukan item yang sesuai dengan pencarian Anda. Coba sesuaikan kriteria atau jelajahi kategori yang tersedia."
		}

		response.QuickReplies = []QuickReply{
			{Text: "Lihat semua kategori", Payload: "view_categories", Icon: "📂"},
			{Text: "Pakaian formal", Payload: "search_formal", Icon: "👔"},
			{Text: "Pakaian kasual", Payload: "search_casual", Icon: "👕"},
			{Text: "Aksesoris", Payload: "search_accessories", Icon: "👜"},
		}
	} else {
		if language == "en" {
			response.Message = fmt.Sprintf("I found %d items matching your search:", len(items))
		} else {
			response.Message = fmt.Sprintf("Saya menemukan %d item yang sesuai dengan pencarian Anda:", len(items))
		}

		response.MessageType = "item_cards"
		response.ItemCards = items

		if len(items) >= 5 {
			response.QuickReplies = []QuickReply{
				{Text: "Lihat lebih banyak", Payload: "view_more_items", Icon: "➕"},
				{Text: "Filter hasil", Payload: "filter_results", Icon: "🔍"},
			}
		}
	}

	return response, nil
}

// generateEducationResponse generates response for education-related queries
func (s *ChatService) generateEducationResponse(_ context.Context, response *ChatResponse, message string, _ *Intent, _ *ChatContext, language string) (*ChatResponse, error) {
	// Search knowledge base for education content
	kb := SearchKnowledgeBase(message, "education")

	if len(kb) > 0 {
		response.Message = kb[0].Answer
		response.MessageType = "text"

		// Add related topics as quick replies
		response.QuickReplies = []QuickReply{
			{Text: "Perawatan katun", Payload: "fabric_care_cotton", Icon: "🧵"},
			{Text: "Perawatan sutra", Payload: "fabric_care_silk", Icon: "✨"},
			{Text: "Menghilangkan noda", Payload: "stain_removal", Icon: "🧽"},
			{Text: "Fast fashion", Payload: "fast_fashion_impact", Icon: "🌍"},
			{Text: "Tips outfit", Payload: "outfit_combinations", Icon: "👗"},
		}

		// Add related FAQs if available
		if len(kb[0].RelatedFAQs) > 0 {
			response.Suggestions = kb[0].RelatedFAQs
		}
	} else {
		if language == "en" {
			response.Message = "I can help you learn about clothing care, sustainable fashion, and styling tips. What would you like to know more about?"
		} else {
			response.Message = "Saya dapat membantu Anda belajar tentang perawatan pakaian, fashion berkelanjutan, dan tips styling. Apa yang ingin Anda ketahui lebih lanjut?"
		}

		response.QuickReplies = []QuickReply{
			{Text: "Perawatan pakaian", Payload: "clothing_care", Icon: "🧺"},
			{Text: "Fashion berkelanjutan", Payload: "sustainable_fashion", Icon: "♻️"},
			{Text: "Tips styling", Payload: "styling_tips", Icon: "💄"},
			{Text: "Organisasi lemari", Payload: "wardrobe_organization", Icon: "🗂️"},
		}
	}

	return response, nil
}

// generateHelpResponse generates response for help-related queries
func (s *ChatService) generateHelpResponse(_ context.Context, response *ChatResponse, _ *Intent, _ *ChatContext, language string) (*ChatResponse, error) {
	if language == "en" {
		response.Message = "I'm here to help! I can assist you with:\n\n• Clothing donations and rentals\n• Item search and recommendations\n• Clothing care and maintenance tips\n• Sustainable fashion education\n• Platform features and account management\n\nWhat would you like to know more about?"
	} else {
		response.Message = "Saya di sini untuk membantu! Saya dapat membantu Anda dengan:\n\n• Donasi dan sewa pakaian\n• Pencarian dan rekomendasi item\n• Tips perawatan pakaian\n• Edukasi fashion berkelanjutan\n• Fitur platform dan manajemen akun\n\nApa yang ingin Anda ketahui lebih lanjut?"
	}

	response.QuickReplies = []QuickReply{
		{Text: "Donasi pakaian", Payload: "donation_help", Icon: "💝"},
		{Text: "Sewa pakaian", Payload: "rental_help", Icon: "👗"},
		{Text: "Cari pakaian", Payload: "search_help", Icon: "🔍"},
		{Text: "Tips perawatan", Payload: "care_tips", Icon: "🧺"},
		{Text: "Akun saya", Payload: "account_help", Icon: "👤"},
	}

	return response, nil
}

// generateDefaultResponse generates default fallback response
func (s *ChatService) generateDefaultResponse(_ context.Context, response *ChatResponse, language string) (*ChatResponse, error) {
	if language == "en" {
		response.Message = "I'm not sure I understand. Could you please rephrase your question? I can help with clothing donations, rentals, care tips, and more."
	} else {
		response.Message = "Saya tidak yakin saya mengerti. Bisakah Anda mengulang pertanyaan Anda? Saya dapat membantu dengan donasi pakaian, sewa, tips perawatan, dan lainnya."
	}

	response.QuickReplies = []QuickReply{
		{Text: "Bantuan umum", Payload: "general_help", Icon: "❓"},
		{Text: "Donasi", Payload: "donation_help", Icon: "💝"},
		{Text: "Sewa", Payload: "rental_help", Icon: "👗"},
		{Text: "Edukasi", Payload: "education_help", Icon: "📚"},
	}

	return response, nil
}

// generateFallbackResponse generates fallback response for errors
func (s *ChatService) generateFallbackResponse(sessionID string) *ChatResponse {
	return &ChatResponse{
		SessionID:     sessionID,
		Message:       "Maaf, terjadi kesalahan saat memproses pesan Anda. Silakan coba lagi.",
		MessageType:   "text",
		Timestamp:     time.Now(),
		CanContinue:   true,
		SessionActive: true,
	}
}
