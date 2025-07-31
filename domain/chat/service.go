package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dzuura/satu-lemari/domain/ai"
	"github.com/dzuura/satu-lemari/domain/cache"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/database"
	"github.com/dzuura/satu-lemari/domain/item"
	"github.com/dzuura/satu-lemari/domain/logging"
	"github.com/dzuura/satu-lemari/domain/user"
)

// ChatService handles chat interactions
type ChatService struct {
	config          *config.Config
	repo            ChatRepositoryInterface
	cache           *cache.RedisCache
	aiService       *ai.AIServiceManager
	itemService     *item.ItemService
	userService     *user.UserService
	logger          *logging.Logger
	sessionDuration time.Duration
}

// NewChatService creates a new chat service instance
func NewChatService(
	cfg *config.Config,
	db *database.Database,
	cache *cache.RedisCache,
	aiService *ai.AIServiceManager,
	itemService *item.ItemService,
	userService *user.UserService,
) *ChatService {
	logger := logging.GetLogger()
	repo := NewChatRepository(cfg, logger)

	return &ChatService{
		config:          cfg,
		repo:            repo,
		cache:           cache,
		aiService:       aiService,
		itemService:     itemService,
		userService:     userService,
		logger:          logger,
		sessionDuration: 24 * time.Hour, // Sessions expire after 24 hours
	}
}

// StartSession creates a new chat session
func (s *ChatService) StartSession(ctx context.Context, userID string, language string) (*ChatSession, error) {
	s.logger.Info("Starting new chat session", map[string]interface{}{
		"user_id":  userID,
		"language": language,
	})

	session := &ChatSession{
		ID:           uuid.New().String(),
		UserID:       userID,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		Context: map[string]interface{}{
			"language": language,
		},
		IsActive: true,
	}

	// Store session in cache for quick access
	if s.cache != nil {
		sessionKey := fmt.Sprintf("chat_session:%s", session.ID)
		sessionData, _ := json.Marshal(session)
		s.cache.Set(ctx, sessionKey, string(sessionData), s.sessionDuration)
	}

	// Store session in database for persistence
	err := s.storeSession(ctx, session)
	if err != nil {
		s.logger.Error("Failed to store chat session", map[string]interface{}{
			"error":      err.Error(),
			"session_id": session.ID,
		})
		return nil, fmt.Errorf("failed to create chat session: %v", err)
	}

	// Send welcome message
	welcomeMsg := s.generateWelcomeMessage(language)
	_, err = s.storeMessage(ctx, session.ID, "assistant", welcomeMsg, nil)
	if err != nil {
		s.logger.Warn("Failed to store welcome message", map[string]interface{}{
			"error":      err.Error(),
			"session_id": session.ID,
		})
	}

	return session, nil
}

// SendMessage processes a user message and returns bot response
func (s *ChatService) SendMessage(ctx context.Context, req *SendMessageRequest) (*ChatResponse, error) {
	s.logger.Info("Processing chat message", map[string]interface{}{
		"session_id": req.SessionID,
		"message":    req.Message,
	})

	// Get session
	session, err := s.getSession(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %v", err)
	}

	// Check if session is still active
	if !session.IsActive || time.Since(session.LastActivity) > s.sessionDuration {
		return &ChatResponse{
			SessionID:     req.SessionID,
			Message:       "Sesi chat telah berakhir. Silakan mulai sesi baru.",
			MessageType:   "text",
			Timestamp:     time.Now(),
			CanContinue:   false,
			SessionActive: false,
		}, nil
	}

	// Store user message
	_, err = s.storeMessage(ctx, req.SessionID, "user", req.Message, req.Context)
	if err != nil {
		s.logger.Error("Failed to store user message", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Parse intent from message
	intent, err := s.parseIntent(ctx, req.Message, req.Context)
	if err != nil {
		s.logger.Warn("Failed to parse intent", map[string]interface{}{
			"error":   err.Error(),
			"message": req.Message,
		})
	}

	// Generate response based on intent
	response, err := s.generateResponse(ctx, session, req.Message, intent, req.Context)
	if err != nil {
		s.logger.Error("Failed to generate response", map[string]interface{}{
			"error": err.Error(),
		})
		response = s.generateFallbackResponse(req.SessionID)
	}

	// Store bot response
	_, err = s.storeMessage(ctx, req.SessionID, "assistant", response.Message, response.Metadata)
	if err != nil {
		s.logger.Error("Failed to store bot response", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Update session activity
	session.LastActivity = time.Now()
	if req.Context != nil && req.Context.Additional != nil {
		// Update session context with new information
		for k, v := range req.Context.Additional {
			session.Context[k] = v
		}
	}
	s.updateSession(ctx, session)

	return response, nil
}

// GetHistory retrieves chat history for a session (works even for expired sessions)
func (s *ChatService) GetHistory(ctx context.Context, sessionID string, limit, offset int) (*ChatHistoryResponse, error) {
	// Users can access chat history even after session expires
	// This is important for user experience and data retention
	messages, total, err := s.getMessages(ctx, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat history: %v", err)
	}

	return &ChatHistoryResponse{
		SessionID: sessionID,
		Messages:  messages,
		Total:     total,
		HasMore:   offset+len(messages) < total,
	}, nil
}

// GetActiveSession retrieves only active (non-expired) sessions
func (s *ChatService) GetActiveSession(ctx context.Context, sessionID string) (*ChatSession, error) {
	session, err := s.getSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Check if session is expired (24 hours)
	if time.Since(session.LastActivity) > 24*time.Hour {
		// Mark session as inactive but don't delete it
		session.IsActive = false
		s.updateSession(ctx, session)
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

// IsSessionExpired checks if a session has expired
func (s *ChatService) IsSessionExpired(session *ChatSession) bool {
	return time.Since(session.LastActivity) > 24*time.Hour
}

// GetSuggestions returns chat suggestions
func (s *ChatService) GetSuggestions(ctx context.Context) *ChatSuggestionsResponse {
	suggestions := GetChatSuggestions()
	return &suggestions
}

// DeleteHistory deletes chat history for a session (hard delete)
func (s *ChatService) DeleteHistory(ctx context.Context, sessionID string, userID string) error {
	// Verify session belongs to user
	session, err := s.getSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %v", err)
	}

	if session.UserID != userID {
		return fmt.Errorf("unauthorized: session does not belong to user")
	}

	// Delete all messages in the session first
	err = s.repo.DeleteAllMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session messages: %v", err)
	}

	// Delete the session itself
	err = s.repo.DeleteSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session: %v", err)
	}

	// Clear from cache
	if s.cache != nil {
		sessionKey := fmt.Sprintf("chat_session:%s", sessionID)
		s.cache.Delete(ctx, sessionKey)
	}

	s.logger.Info("Chat session and messages hard deleted", map[string]interface{}{
		"session_id": sessionID,
		"user_id":    userID,
	})

	return nil
}

// parseIntent parses user intent from message
func (s *ChatService) parseIntent(ctx context.Context, message string, context *ChatContext) (*Intent, error) {
	// Use AI service if available
	if s.aiService != nil && s.aiService.IsGeminiAvailable() {
		return s.parseIntentWithAI(ctx, message, context)
	}

	// Fallback to rule-based parsing
	return s.parseIntentRuleBased(message, context), nil
}

// parseIntentWithAI uses AI service to parse intent
func (s *ChatService) parseIntentWithAI(_ context.Context, message string, context *ChatContext) (*Intent, error) {
	// Create prompt for intent parsing
	_ = s.buildIntentPrompt(message, context)

	// Call AI service (implementation would depend on AI service interface)
	// For now, fallback to rule-based
	return s.parseIntentRuleBased(message, context), nil
}

// parseIntentRuleBased uses rule-based approach to parse intent
func (s *ChatService) parseIntentRuleBased(message string, _ *ChatContext) *Intent {
	msgLower := strings.ToLower(message)

	intent := &Intent{
		Entities:   make(map[string]interface{}),
		Confidence: 0.7, // Default confidence for rule-based
	}

	// Donation intent
	if strings.Contains(msgLower, "donasi") || strings.Contains(msgLower, "sumbang") ||
		strings.Contains(msgLower, "beri") || strings.Contains(msgLower, "kasih") {
		intent.Type = "donation"
		intent.Action = "help"
		intent.Confidence = 0.8
	}

	// Rental intent
	if strings.Contains(msgLower, "sewa") || strings.Contains(msgLower, "pinjam") ||
		strings.Contains(msgLower, "rental") {
		intent.Type = "rental"
		intent.Action = "help"
		intent.Confidence = 0.8
	}

	// Search intent
	if strings.Contains(msgLower, "cari") || strings.Contains(msgLower, "ada") ||
		strings.Contains(msgLower, "tersedia") {
		intent.Type = "search"
		intent.Action = "find_items"
		intent.Confidence = 0.7
	}

	// Education intent
	if strings.Contains(msgLower, "cara") || strings.Contains(msgLower, "bagaimana") ||
		strings.Contains(msgLower, "tips") || strings.Contains(msgLower, "rawat") {
		intent.Type = "education"
		intent.Action = "learn"
		intent.Confidence = 0.7
	}

	// Help intent
	if strings.Contains(msgLower, "help") || strings.Contains(msgLower, "bantuan") ||
		strings.Contains(msgLower, "tolong") {
		intent.Type = "help"
		intent.Action = "general"
		intent.Confidence = 0.9
	}

	// Default to help if no specific intent detected
	if intent.Type == "" {
		intent.Type = "help"
		intent.Action = "general"
		intent.Confidence = 0.5
	}

	return intent
}

// DeleteMessages deletes specific messages from a chat session
func (s *ChatService) DeleteMessages(ctx context.Context, sessionID string, messageIDs []string, userID string) error {
	// Verify session belongs to user
	session, err := s.getSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %v", err)
	}

	if session.UserID != userID {
		return fmt.Errorf("unauthorized: session does not belong to user")
	}

	// Delete specific messages using repository
	err = s.repo.DeleteMessages(ctx, sessionID, messageIDs)
	if err != nil {
		return fmt.Errorf("failed to delete messages: %v", err)
	}

	s.logger.Info("Messages deleted", map[string]interface{}{
		"session_id":  sessionID,
		"user_id":     userID,
		"message_ids": messageIDs,
		"count":       len(messageIDs),
	})

	return nil
}

// DeleteAllMessages deletes all messages from a chat session
func (s *ChatService) DeleteAllMessages(ctx context.Context, sessionID string, userID string) error {
	// Verify session belongs to user
	session, err := s.getSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %v", err)
	}

	if session.UserID != userID {
		return fmt.Errorf("unauthorized: session does not belong to user")
	}

	// Delete all messages in the session using repository
	err = s.repo.DeleteAllMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete all messages: %v", err)
	}

	// Clear cache
	s.clearSessionCache(sessionID)

	s.logger.Info("All messages deleted", map[string]interface{}{
		"session_id": sessionID,
		"user_id":    userID,
	})

	return nil
}

// DeleteAllUserHistory deletes all chat history for a user
func (s *ChatService) DeleteAllUserHistory(ctx context.Context, userID string) error {
	// Delete all messages for user using repository
	err := s.repo.DeleteAllUserMessages(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user messages: %v", err)
	}

	// Delete all sessions for user using repository
	err = s.repo.DeleteAllUserSessions(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %v", err)
	}

	s.logger.Info("All user history deleted", map[string]interface{}{
		"user_id": userID,
	})

	// Clear cache for user sessions
	if s.cache != nil {
		userSessionsKey := fmt.Sprintf("user_sessions:%s", userID)
		s.cache.Delete(ctx, userSessionsKey)
	}

	return nil
}

// clearSessionCache clears session cache
func (s *ChatService) clearSessionCache(sessionID string) {
	if s.cache != nil {
		sessionKey := fmt.Sprintf("chat_session:%s", sessionID)
		s.cache.Delete(context.Background(), sessionKey)
	}
}

// Helper functions

// getLanguageFromSession extracts language from session context
func (s *ChatService) getLanguageFromSession(session *ChatSession) string {
	if lang, ok := session.Context["language"].(string); ok {
		return lang
	}
	return "id" // Default to Indonesian
}

// buildIntentPrompt builds prompt for AI intent parsing
func (s *ChatService) buildIntentPrompt(message string, context *ChatContext) string {
	prompt := fmt.Sprintf(`
Analisis pesan pengguna berikut dan ekstrak intent untuk platform donasi/sewa pakaian:

Pesan: "%s"

Konteks:
- Platform: SatuLemari (donasi dan sewa pakaian)
- Topik yang diizinkan: donasi, sewa, pencarian item, perawatan pakaian, fashion berkelanjutan, bantuan platform
- Topik yang dibatasi: topik non-pakaian, pemrosesan pembayaran, chat umum

Ekstrak:
1. Jenis intent: donation, rental, search, education, help
2. Kategori: kategori pakaian jika disebutkan
3. Aksi: aksi spesifik yang diminta
4. Entitas: entitas yang relevan (ukuran, warna, lokasi, dll.)

Respons dalam format JSON dengan key dalam bahasa Inggris.
`, message)

	if context != nil {
		if context.Page != "" {
			prompt += fmt.Sprintf("\n- Current page: %s", context.Page)
		}
		if context.ItemID != "" {
			prompt += fmt.Sprintf("\n- Current item: %s", context.ItemID)
		}
	}

	return prompt
}

// extractSearchParams extracts search parameters from user message
func (s *ChatService) extractSearchParams(message string) map[string]interface{} {
	params := make(map[string]interface{})
	msgLower := strings.ToLower(message)

	// Extract clothing categories
	categories := map[string]string{
		"formal":      "Pakaian Formal",
		"kasual":      "Pakaian Kasual",
		"casual":      "Pakaian Kasual",
		"olahraga":    "Pakaian Olahraga",
		"sport":       "Pakaian Olahraga",
		"tradisional": "Pakaian Tradisional",
		"traditional": "Pakaian Tradisional",
		"aksesoris":   "Aksesoris",
		"accessories": "Aksesoris",
		"sepatu":      "Alas Kaki",
		"shoes":       "Alas Kaki",
		"celana":      "Celana",
		"pants":       "Celana",
	}

	for keyword, category := range categories {
		if strings.Contains(msgLower, keyword) {
			params["category"] = category
			break
		}
	}

	// Extract sizes
	sizes := []string{"xs", "s", "m", "l", "xl", "xxl", "xxxl"}
	for _, size := range sizes {
		if strings.Contains(msgLower, size) {
			params["size"] = strings.ToUpper(size)
			break
		}
	}

	// Extract colors
	colors := map[string]string{
		"merah":  "Merah",
		"red":    "Merah",
		"biru":   "Biru",
		"blue":   "Biru",
		"hijau":  "Hijau",
		"green":  "Hijau",
		"kuning": "Kuning",
		"yellow": "Kuning",
		"hitam":  "Hitam",
		"black":  "Hitam",
		"putih":  "Putih",
		"white":  "Putih",
		"abu":    "Abu-abu",
		"gray":   "Abu-abu",
		"grey":   "Abu-abu",
		"coklat": "Coklat",
		"brown":  "Coklat",
		"pink":   "Pink",
		"ungu":   "Ungu",
		"purple": "Ungu",
		"orange": "Orange",
		"oren":   "Orange",
	}

	for keyword, color := range colors {
		if strings.Contains(msgLower, keyword) {
			params["color"] = color
			break
		}
	}

	// Extract type (donation/rental)
	if strings.Contains(msgLower, "sewa") || strings.Contains(msgLower, "rental") || strings.Contains(msgLower, "pinjam") {
		params["type"] = "rental"
	} else if strings.Contains(msgLower, "donasi") || strings.Contains(msgLower, "gratis") || strings.Contains(msgLower, "free") {
		params["type"] = "donation"
	}

	// Extract condition
	conditions := map[string]string{
		"excellent": "Excellent",
		"bagus":     "Good",
		"good":      "Good",
		"baik":      "Good",
		"fair":      "Fair",
		"cukup":     "Fair",
	}

	for keyword, condition := range conditions {
		if strings.Contains(msgLower, keyword) {
			params["condition"] = condition
			break
		}
	}

	return params
}

// searchItems searches for items based on parameters
func (s *ChatService) searchItems(_ context.Context, _ map[string]interface{}, _, _ *float64) ([]ItemCard, error) {
	// For now, return empty slice since we need to check item service interface
	// This will be implemented based on actual item service methods
	// When implemented, make sure to format condition using formatConditionForDisplay()

	// Example of how to format condition when returning ItemCard:
	// item.Condition = formatConditionForDisplay(originalCondition)

	return []ItemCard{}, nil
}

// validateMessage validates chat message content
func (s *ChatService) validateMessage(message string) error {
	if len(strings.TrimSpace(message)) == 0 {
		return fmt.Errorf("message cannot be empty")
	}

	if len(message) > 1000 {
		return fmt.Errorf("message too long (max 1000 characters)")
	}

	// Check for inappropriate content (basic implementation)
	inappropriateWords := []string{
		"spam", "scam", "fraud", "hack", "illegal",
	}

	msgLower := strings.ToLower(message)
	for _, word := range inappropriateWords {
		if strings.Contains(msgLower, word) {
			return fmt.Errorf("message contains inappropriate content")
		}
	}

	return nil
}

// isTopicAllowed checks if the topic is allowed for the chatbot
func (s *ChatService) isTopicAllowed(message string) bool {
	msgLower := strings.ToLower(message)

	// Allowed topics
	allowedKeywords := []string{
		"donasi", "donation", "sewa", "rental", "pakaian", "clothing", "clothes",
		"baju", "shirt", "celana", "pants", "sepatu", "shoes", "aksesoris", "accessories",
		"perawatan", "care", "cuci", "wash", "setrika", "iron", "tips", "cara", "how",
		"sustainable", "berkelanjutan", "fashion", "style", "outfit", "lemari", "wardrobe",
		"kategori", "category", "ukuran", "size", "warna", "color", "kondisi", "condition",
		"cari", "search", "bantuan", "help", "akun", "account", "profil", "profile",
	}

	for _, keyword := range allowedKeywords {
		if strings.Contains(msgLower, keyword) {
			return true
		}
	}

	// Restricted topics
	restrictedKeywords := []string{
		"payment", "bayar", "harga", "price", "money", "uang", "transfer", "bank",
		"politik", "politics", "agama", "religion", "seks", "sex", "dating", "pacaran",
		"obat", "medicine", "drug", "illegal", "gambling", "judi",
	}

	for _, keyword := range restrictedKeywords {
		if strings.Contains(msgLower, keyword) {
			return false
		}
	}

	return true // Allow by default if no specific keywords found
}

// Database operations

// storeSession stores a chat session in the database
func (s *ChatService) storeSession(ctx context.Context, session *ChatSession) error {
	err := s.repo.CreateSession(ctx, session)
	if err != nil {
		return fmt.Errorf("failed to store session: %v", err)
	}

	return nil
}

// getSession retrieves a chat session from cache or database
func (s *ChatService) getSession(ctx context.Context, sessionID string) (*ChatSession, error) {
	// Try cache first
	if s.cache != nil {
		sessionKey := fmt.Sprintf("chat_session:%s", sessionID)
		var session ChatSession
		err := s.cache.Get(ctx, sessionKey, &session)
		if err == nil {
			return &session, nil
		}
	}

	// Fallback to database using repository
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %v", err)
	}

	// Update cache
	if s.cache != nil {
		sessionKey := fmt.Sprintf("chat_session:%s", sessionID)
		sessionData, _ := json.Marshal(session)
		s.cache.Set(ctx, sessionKey, string(sessionData), s.sessionDuration)
	}

	return session, nil
}

// updateSession updates a chat session
func (s *ChatService) updateSession(ctx context.Context, session *ChatSession) error {
	err := s.repo.UpdateSession(ctx, session.ID, session.LastActivity, session.Context, session.IsActive)
	if err != nil {
		return fmt.Errorf("failed to update session: %v", err)
	}

	// Update cache
	if s.cache != nil {
		sessionKey := fmt.Sprintf("chat_session:%s", session.ID)
		sessionData, _ := json.Marshal(session)
		s.cache.Set(ctx, sessionKey, string(sessionData), s.sessionDuration)
	}

	return nil
}

// storeMessage stores a chat message in the database
func (s *ChatService) storeMessage(ctx context.Context, sessionID, role, content string, metadata interface{}) (*Message, error) {
	message := &Message{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}

	if metadata != nil {
		if metadataMap, ok := metadata.(map[string]interface{}); ok {
			message.Metadata = metadataMap
		} else {
			message.Metadata = make(map[string]interface{})
		}
	} else {
		message.Metadata = make(map[string]interface{})
	}

	err := s.repo.CreateMessage(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("failed to store message: %v", err)
	}

	return message, nil
}

// getMessages retrieves chat messages for a session
func (s *ChatService) getMessages(ctx context.Context, sessionID string, limit, offset int) ([]Message, int, error) {
	// Use repository method
	messages, total, err := s.repo.GetMessages(ctx, sessionID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get messages: %v", err)
	}

	return messages, total, nil
}

// getUserSessions retrieves all sessions for a user
func (s *ChatService) getUserSessions(ctx context.Context, userID string, limit, offset int) ([]ChatSession, int, error) {
	// Use repository method
	sessions, total, err := s.repo.GetUserSessions(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get sessions: %v", err)
	}

	return sessions, total, nil
}

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

	// Try AI service first if available
	if s.aiService != nil && s.aiService.IsGeminiAvailable() {
		aiResponse, err := s.generateAIResponse(ctx, message, intent, context, language)
		if err == nil && aiResponse != "" {
			response.Message = aiResponse
			s.addContextualQuickReplies(response, intent, language)
			return response, nil
		}

		s.logger.Warn("AI service failed, falling back to rule-based response", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Fallback to rule-based responses
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

// generateAIResponse generates response using AI service
func (s *ChatService) generateAIResponse(ctx context.Context, message string, intent *Intent, context *ChatContext, language string) (string, error) {
	// Build comprehensive prompt for AI
	prompt := s.buildAIPrompt(message, intent, context, language)

	// Prepare context data for AI
	data := map[string]interface{}{
		"intent":   intent,
		"context":  context,
		"language": language,
		"platform": "SatuLemari",
		"message":  message,
	}

	// Generate response using AI service
	response, err := s.aiService.GenerateResponse(ctx, prompt, data)
	if err != nil {
		return "", fmt.Errorf("AI service error: %v", err)
	}

	return response, nil
}

// buildAIPrompt builds comprehensive prompt for AI response generation
func (s *ChatService) buildAIPrompt(message string, intent *Intent, context *ChatContext, language string) string {
	var prompt strings.Builder

	// System context based on language
	if language == "en" {
		prompt.WriteString("You are an AI assistant for SatuLemari, a sustainable clothing donation and rental platform in Indonesia. ")
		prompt.WriteString("Provide helpful, accurate, and relevant responses to user questions. ")
		prompt.WriteString("Use natural and friendly English.\n\n")
	} else {
		prompt.WriteString("Kamu adalah asisten AI untuk SatuLemari, platform donasi dan sewa pakaian berkelanjutan di Indonesia. ")
		prompt.WriteString("Berikan respons yang membantu, akurat, dan relevan dengan pertanyaan pengguna. ")
		prompt.WriteString("Gunakan bahasa Indonesia yang natural dan ramah.\n\n")
	}

	// Platform information based on language
	if language == "en" {
		prompt.WriteString("SatuLemari Platform Information:\n")
		prompt.WriteString("- Platform for clothing donation and rental\n")
		prompt.WriteString("- Supports sustainable fashion\n")
		prompt.WriteString("- Users can donate or rent clothing\n")
		prompt.WriteString("- Weekly quota system for donations\n")
		prompt.WriteString("- Various categories: formal, casual, sports, traditional, accessories\n")
		prompt.WriteString("- Clothing conditions: Excellent, Good, Fair\n\n")
	} else {
		prompt.WriteString("Informasi Platform SatuLemari:\n")
		prompt.WriteString("- Platform untuk donasi dan sewa pakaian\n")
		prompt.WriteString("- Mendukung fashion berkelanjutan\n")
		prompt.WriteString("- Users dapat mendonasikan atau menyewakan pakaian\n")
		prompt.WriteString("- Sistem kuota mingguan untuk donasi\n")
		prompt.WriteString("- Berbagai kategori: formal, kasual, olahraga, tradisional, aksesoris\n")
		// Use formatConditionForDisplay to show proper formatting
		excellentFormatted := formatConditionForDisplay("excellent")
		goodFormatted := formatConditionForDisplay("good")
		fairFormatted := formatConditionForDisplay("fair")
		prompt.WriteString(fmt.Sprintf("- Kondisi pakaian: %s (Excellent), %s (Good), %s (Fair)\n\n",
			excellentFormatted, goodFormatted, fairFormatted))
	}

	// Intent context
	if intent != nil {
		prompt.WriteString(fmt.Sprintf("Intent yang terdeteksi: %s\n", intent.Type))
		if intent.Action != "" {
			prompt.WriteString(fmt.Sprintf("Aksi: %s\n", intent.Action))
		}
	}

	// Context information
	if context != nil {
		if context.Page != "" {
			prompt.WriteString(fmt.Sprintf("Halaman saat ini: %s\n", context.Page))
		}
		if context.ItemID != "" {
			prompt.WriteString(fmt.Sprintf("Item yang sedang dilihat: %s\n", context.ItemID))
		}
		if context.UserLocation != nil {
			prompt.WriteString(fmt.Sprintf("Lokasi user: lat=%f, lng=%f\n",
				context.UserLocation.Latitude, context.UserLocation.Longitude))
		}
	}

	// Guidelines based on language
	if language == "en" {
		prompt.WriteString("\nResponse guidelines:\n")
		prompt.WriteString("- Answer questions directly and informatively\n")
		prompt.WriteString("- For donations: explain process, quota, accepted conditions (Excellent, Good, Fair)\n")
		prompt.WriteString("- For rentals: explain rental process, duration, clothing conditions\n")
		prompt.WriteString("- For search: help with categories, sizes, colors, conditions\n")
		prompt.WriteString("- For care: provide practical tips\n")
		prompt.WriteString("- Promote sustainable fashion practices\n")
		prompt.WriteString("- If unsure, direct to customer service\n")
		prompt.WriteString("- Maximum 200 words, use short paragraphs\n\n")
	} else {
		excellentFormatted := formatConditionForDisplay("excellent")
		goodFormatted := formatConditionForDisplay("good")
		fairFormatted := formatConditionForDisplay("fair")
		prompt.WriteString("\nPanduan respons:\n")
		prompt.WriteString("- Jawab pertanyaan secara langsung dan informatif\n")
		prompt.WriteString(fmt.Sprintf("- Jika tentang donasi: jelaskan proses, kuota, kondisi yang diterima (%s, %s, %s)\n",
			excellentFormatted, goodFormatted, fairFormatted))
		prompt.WriteString("- Jika tentang sewa: jelaskan cara sewa, durasi, kondisi pakaian\n")
		prompt.WriteString("- Jika tentang pencarian: bantu dengan kategori, ukuran, warna, kondisi\n")
		prompt.WriteString("- Jika tentang perawatan: berikan tips praktis\n")
		prompt.WriteString("- Promosikan praktik fashion berkelanjutan\n")
		prompt.WriteString(fmt.Sprintf("- Gunakan istilah kondisi: %s, %s, %s (bukan Excellent, Good, Fair)\n",
			excellentFormatted, goodFormatted, fairFormatted))
		prompt.WriteString("- Jika tidak tahu pasti, arahkan ke customer service\n")
		prompt.WriteString("- Maksimal 200 kata, gunakan paragraf pendek\n\n")
	}

	// User message
	prompt.WriteString(fmt.Sprintf("Pertanyaan pengguna: \"%s\"\n\n", message))
	prompt.WriteString("Respons:")

	return prompt.String()
}

// formatConditionForDisplay formats clothing condition from English to Indonesian for display
func formatConditionForDisplay(condition string) string {
	switch strings.ToLower(condition) {
	case "excellent":
		return "Sangat Baik"
	case "good":
		return "Baik"
	case "fair":
		return "Cukup"
	default:
		return condition // Return original if not recognized
	}
}

// addContextualQuickReplies adds contextual quick replies based on intent
func (s *ChatService) addContextualQuickReplies(response *ChatResponse, intent *Intent, language string) {
	switch intent.Type {
	case "donation":
		if language == "en" {
			response.QuickReplies = []QuickReply{
				{Text: "How to donate clothes", Payload: "donation_process", Icon: "📝"},
				{Text: "Weekly donation quota", Payload: "donation_quota", Icon: "📊"},
				{Text: "Accepted clothing conditions", Payload: "item_condition", Icon: "👕"},
				{Text: "View my items", Payload: "my_items", Icon: "👔"},
			}
		} else {
			response.QuickReplies = []QuickReply{
				{Text: "Cara mendonasikan pakaian", Payload: "donation_process", Icon: "📝"},
				{Text: "Kuota donasi mingguan", Payload: "donation_quota", Icon: "📊"},
				{Text: "Kondisi pakaian yang diterima", Payload: "item_condition", Icon: "👕"},
				{Text: "Lihat item saya", Payload: "my_items", Icon: "👔"},
			}
		}
	case "rental":
		if language == "en" {
			response.QuickReplies = []QuickReply{
				{Text: "How to rent clothes", Payload: "rental_process", Icon: "🔄"},
				{Text: "Rental duration", Payload: "rental_duration", Icon: "⏰"},
				{Text: "Search rental clothes", Payload: "search_rental", Icon: "🔍"},
				{Text: "My rental history", Payload: "my_rentals", Icon: "📋"},
			}
		} else {
			response.QuickReplies = []QuickReply{
				{Text: "Cara menyewa pakaian", Payload: "rental_process", Icon: "🔄"},
				{Text: "Durasi sewa", Payload: "rental_duration", Icon: "⏰"},
				{Text: "Cari pakaian sewa", Payload: "search_rental", Icon: "🔍"},
				{Text: "Riwayat sewa saya", Payload: "my_rentals", Icon: "📋"},
			}
		}
	case "search":
		response.QuickReplies = []QuickReply{
			{Text: "Lihat semua kategori", Payload: "view_categories", Icon: "📂"},
			{Text: "Pakaian formal", Payload: "search_formal", Icon: "👔"},
			{Text: "Pakaian kasual", Payload: "search_casual", Icon: "👕"},
			{Text: "Aksesoris", Payload: "search_accessories", Icon: "👜"},
		}
	case "education":
		response.QuickReplies = []QuickReply{
			{Text: "Perawatan pakaian", Payload: "clothing_care", Icon: "🧺"},
			{Text: "Fashion berkelanjutan", Payload: "sustainable_fashion", Icon: "♻️"},
			{Text: "Tips styling", Payload: "styling_tips", Icon: "💄"},
			{Text: "Organisasi lemari", Payload: "wardrobe_organization", Icon: "🗂️"},
		}
	default:
		response.QuickReplies = []QuickReply{
			{Text: "Donasi", Payload: "donation_help", Icon: "💝"},
			{Text: "Sewa", Payload: "rental_help", Icon: "👗"},
			{Text: "Cari pakaian", Payload: "search_help", Icon: "🔍"},
			{Text: "Tips perawatan", Payload: "care_tips", Icon: "🧺"},
		}
	}
}

// generateWelcomeMessage generates welcome message based on language
func (s *ChatService) generateWelcomeMessage(language string) string {
	if language == "en" {
		return "Hello! I'm SatuLemari assistant. I can help you with clothing donations, rentals, care tips, and sustainable fashion. How can I assist you today?"
	}

	return "Halo! Saya asisten SatuLemari. Saya dapat membantu Anda dengan donasi pakaian, sewa, tips perawatan, dan fashion berkelanjutan. Ada yang bisa saya bantu?"
}

// generateDonationResponse generates response for donation-related queries (fallback)
func (s *ChatService) generateDonationResponse(_ context.Context, response *ChatResponse, _ *Intent, _ *ChatContext, language string) (*ChatResponse, error) {
	// Try knowledge base as last resort
	kb := SearchKnowledgeBase("donasi", "donation")

	if len(kb) > 0 {
		response.Message = kb[0].Answer
	} else {
		if language == "en" {
			response.Message = "I can help you with clothing donations. You can donate clothes in good condition to help others in need. Would you like to know more about the donation process?"
		} else {
			response.Message = "Saya dapat membantu Anda dengan donasi pakaian. Anda dapat mendonasikan pakaian dalam kondisi baik untuk membantu orang yang membutuhkan. Apakah Anda ingin tahu lebih lanjut tentang proses donasi?"
		}
	}

	response.QuickReplies = []QuickReply{
		{Text: "Cara mendonasikan pakaian", Payload: "donation_process", Icon: "📝"},
		{Text: "Kuota donasi mingguan", Payload: "donation_quota", Icon: "📊"},
		{Text: "Kondisi pakaian yang diterima", Payload: "item_condition", Icon: "👕"},
		{Text: "Lihat item saya", Payload: "my_items", Icon: "👔"},
	}

	return response, nil
}

// generateRentalResponse generates response for rental-related queries (fallback)
func (s *ChatService) generateRentalResponse(_ context.Context, response *ChatResponse, _ *Intent, _ *ChatContext, language string) (*ChatResponse, error) {
	// Try knowledge base as last resort
	kb := SearchKnowledgeBase("sewa", "rental")

	if len(kb) > 0 {
		response.Message = kb[0].Answer
	} else {
		if language == "en" {
			response.Message = "I can help you with clothing rentals. You can rent clothes for special occasions or events. Would you like to search for available rental items?"
		} else {
			response.Message = "Saya dapat membantu Anda dengan sewa pakaian. Anda dapat menyewa pakaian untuk acara khusus atau event. Apakah Anda ingin mencari item sewa yang tersedia?"
		}
	}

	response.QuickReplies = []QuickReply{
		{Text: "Cara menyewa pakaian", Payload: "rental_process", Icon: "🔄"},
		{Text: "Durasi sewa", Payload: "rental_duration", Icon: "⏰"},
		{Text: "Cari pakaian sewa", Payload: "search_rental", Icon: "🔍"},
		{Text: "Riwayat sewa saya", Payload: "my_rentals", Icon: "📋"},
	}

	return response, nil
}

// generateSearchResponse generates response for search-related queries (fallback)
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

// generateEducationResponse generates response for education-related queries (fallback)
func (s *ChatService) generateEducationResponse(_ context.Context, response *ChatResponse, message string, _ *Intent, _ *ChatContext, language string) (*ChatResponse, error) {
	// Search knowledge base for education content
	kb := SearchKnowledgeBase(message, "education")

	if len(kb) > 0 {
		response.Message = kb[0].Answer

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

// generateHelpResponse generates response for help-related queries (fallback)
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
