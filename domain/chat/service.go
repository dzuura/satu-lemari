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
