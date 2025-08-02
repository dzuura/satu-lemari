package chat

import (
	"time"
)

// ChatSession represents a chat session
type ChatSession struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"user_id"`
	CreatedAt    time.Time              `json:"created_at"`
	LastActivity time.Time              `json:"last_activity"`
	Context      map[string]interface{} `json:"context"`
	IsActive     bool                   `json:"is_active"`
}

// Message represents a chat message
type Message struct {
	ID        string                 `json:"id"`
	SessionID string                 `json:"session_id"`
	Role      string                 `json:"role"` // "user" or "assistant"
	Content   string                 `json:"content"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ChatContext represents the context of a chat interaction
type ChatContext struct {
	Page         string                 `json:"page,omitempty"`
	ItemID       string                 `json:"item_id,omitempty"`
	UserLocation *UserLocation          `json:"user_location,omitempty"`
	Additional   map[string]interface{} `json:"additional,omitempty"`
}

// UserLocation represents user's location
type UserLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Intent represents parsed user intent
type Intent struct {
	Type       string                 `json:"type"`
	Action     string                 `json:"action"`
	Entities   map[string]interface{} `json:"entities"`
	Confidence float64                `json:"confidence"`
}

// QuickReply represents a quick reply option
type QuickReply struct {
	Text    string `json:"text"`
	Payload string `json:"payload"`
	Icon    string `json:"icon,omitempty"`
}

// ItemCard represents an item card in chat response
type ItemCard struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Category  string   `json:"category"`
	Size      string   `json:"size"`
	Color     string   `json:"color"`
	Condition string   `json:"condition"`
	Type      string   `json:"type"` // "donation" or "rental"
	ImageURL  string   `json:"image_url"`
	OwnerName string   `json:"owner_name"`
	Location  string   `json:"location"`
	Distance  *float64 `json:"distance,omitempty"` // in km
	Price     *float64 `json:"price,omitempty"`    // for rentals
}

// ChatResponse represents a response from the chatbot
type ChatResponse struct {
	SessionID     string                 `json:"session_id"`
	Message       string                 `json:"message"`
	MessageType   string                 `json:"message_type"` // "text", "item_cards", "image", etc.
	ItemCards     []ItemCard             `json:"item_cards,omitempty"`
	QuickReplies  []QuickReply           `json:"quick_replies,omitempty"`
	Suggestions   []string               `json:"suggestions,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
	CanContinue   bool                   `json:"can_continue"`
	SessionActive bool                   `json:"session_active"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// SendMessageRequest represents a request to send a message
type SendMessageRequest struct {
	SessionID string       `json:"session_id"`
	Message   string       `json:"message"`
	Context   *ChatContext `json:"context,omitempty"`
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
}

// ChatSuggestion represents a chat suggestion
type ChatSuggestion struct {
	Text        string `json:"text"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Category    string `json:"category"`
}

// StartSessionRequest represents a request to start a new session
type StartSessionRequest struct {
	UserID   string `json:"user_id"`
	Language string `json:"language,omitempty"`
}

// SessionResponse represents a session response
type SessionResponse struct {
	Session *ChatSession `json:"session"`
	Message string       `json:"message,omitempty"`
}

// DeleteHistoryRequest represents a request to delete chat history
type DeleteHistoryRequest struct {
	SessionID  string   `json:"session_id"`
	MessageIDs []string `json:"message_ids,omitempty"` // If empty, delete all messages
}

// ChatStats represents chat statistics
type ChatStats struct {
	TotalSessions    int `json:"total_sessions"`
	ActiveSessions   int `json:"active_sessions"`
	TotalMessages    int `json:"total_messages"`
	AvgSessionLength int `json:"avg_session_length"` // in minutes
}

// KnowledgeBase represents a knowledge base entry
type KnowledgeBase struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Topic       string   `json:"topic"`
	Question    string   `json:"question"`
	Answer      string   `json:"answer"`
	Keywords    []string `json:"keywords"`
	RelatedFAQs []string `json:"related_faqs,omitempty"`
}

// StartChatRequest represents a request to start chat
type StartChatRequest struct {
	UserID   string `json:"user_id"`
	Language string `json:"language,omitempty"`
}
