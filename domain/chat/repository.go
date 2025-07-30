package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/logging"
)

// ChatRepositoryInterface defines the interface for chat data operations
type ChatRepositoryInterface interface {
	// Session operations
	CreateSession(ctx context.Context, session *ChatSession) error
	GetSession(ctx context.Context, sessionID string) (*ChatSession, error)
	GetUserSessions(ctx context.Context, userID string, limit, offset int) ([]ChatSession, int, error)
	UpdateSession(ctx context.Context, sessionID string, lastActivity time.Time, context map[string]interface{}, isActive bool) error
	DeleteSession(ctx context.Context, sessionID string) error
	DeleteAllUserSessions(ctx context.Context, userID string) error

	// Message operations
	CreateMessage(ctx context.Context, message *Message) error
	GetMessages(ctx context.Context, sessionID string, limit, offset int) ([]Message, int, error)
	DeleteMessages(ctx context.Context, sessionID string, messageIDs []string) error
	DeleteAllMessages(ctx context.Context, sessionID string) error
	DeleteAllUserMessages(ctx context.Context, userID string) error

	// Count operations
	CountMessages(ctx context.Context, sessionID string) (int, error)
	CountSessions(ctx context.Context, userID string) (int, error)
}

// ChatRepository implements ChatRepositoryInterface using Supabase REST API
type ChatRepository struct {
	config     *config.Config
	httpClient *http.Client
	logger     *logging.Logger
}

// NewChatRepository creates a new chat repository
func NewChatRepository(cfg *config.Config, logger *logging.Logger) ChatRepositoryInterface {
	return &ChatRepository{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

// CreateSession creates a new chat session
func (r *ChatRepository) CreateSession(ctx context.Context, session *ChatSession) error {
	url := fmt.Sprintf("%s/rest/v1/chat_sessions", r.config.SupabaseURL)

	sessionData := map[string]interface{}{
		"id":            session.ID,
		"user_id":       session.UserID,
		"created_at":    session.CreatedAt.Format(time.RFC3339),
		"last_activity": session.LastActivity.Format(time.RFC3339),
		"context":       session.Context,
		"is_active":     session.IsActive,
	}

	jsonData, err := json.Marshal(sessionData)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create session failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetSession retrieves a chat session by ID
func (r *ChatRepository) GetSession(ctx context.Context, sessionID string) (*ChatSession, error) {
	url := fmt.Sprintf("%s/rest/v1/chat_sessions?id=eq.%s&limit=1", r.config.SupabaseURL, sessionID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get session failed with status %d: %s", resp.StatusCode, string(body))
	}

	var sessions []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("failed to decode session: %v", err)
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("session not found")
	}

	return r.mapToSession(sessions[0])
}

// GetUserSessions retrieves all sessions for a user with pagination
func (r *ChatRepository) GetUserSessions(ctx context.Context, userID string, limit, offset int) ([]ChatSession, int, error) {
	// Get total count first
	total, err := r.CountSessions(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	// Get sessions with pagination
	url := fmt.Sprintf("%s/rest/v1/chat_sessions?user_id=eq.%s&is_active=eq.true&order=last_activity.desc&limit=%d&offset=%d",
		r.config.SupabaseURL, userID, limit, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("get sessions failed with status %d: %s", resp.StatusCode, string(body))
	}

	var sessionsData []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&sessionsData); err != nil {
		return nil, 0, fmt.Errorf("failed to decode sessions: %v", err)
	}

	var sessions []ChatSession
	for _, data := range sessionsData {
		session, err := r.mapToSession(data)
		if err != nil {
			r.logger.Warn("Failed to map session data", map[string]interface{}{
				"error": err.Error(),
				"data":  data,
			})
			continue
		}
		sessions = append(sessions, *session)
	}

	return sessions, total, nil
}

// UpdateSession updates a chat session
func (r *ChatRepository) UpdateSession(ctx context.Context, sessionID string, lastActivity time.Time, context map[string]interface{}, isActive bool) error {
	url := fmt.Sprintf("%s/rest/v1/chat_sessions?id=eq.%s", r.config.SupabaseURL, sessionID)

	updateData := map[string]interface{}{
		"last_activity": lastActivity.Format(time.RFC3339),
		"context":       context,
		"is_active":     isActive,
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("failed to marshal update data: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update session failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteSession deletes a chat session
func (r *ChatRepository) DeleteSession(ctx context.Context, sessionID string) error {
	url := fmt.Sprintf("%s/rest/v1/chat_sessions?id=eq.%s", r.config.SupabaseURL, sessionID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete session failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteAllUserSessions deletes all sessions for a user
func (r *ChatRepository) DeleteAllUserSessions(ctx context.Context, userID string) error {
	url := fmt.Sprintf("%s/rest/v1/chat_sessions?user_id=eq.%s", r.config.SupabaseURL, userID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete sessions failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CreateMessage creates a new chat message
func (r *ChatRepository) CreateMessage(ctx context.Context, message *Message) error {
	url := fmt.Sprintf("%s/rest/v1/chat_messages", r.config.SupabaseURL)

	messageData := map[string]interface{}{
		"id":         message.ID,
		"session_id": message.SessionID,
		"role":       message.Role,
		"content":    message.Content,
		"timestamp":  message.Timestamp.Format(time.RFC3339),
		"metadata":   message.Metadata,
	}

	jsonData, err := json.Marshal(messageData)
	if err != nil {
		return fmt.Errorf("failed to marshal message data: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create message failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetMessages retrieves messages for a session with pagination
func (r *ChatRepository) GetMessages(ctx context.Context, sessionID string, limit, offset int) ([]Message, int, error) {
	// Get total count first
	total, err := r.CountMessages(ctx, sessionID)
	if err != nil {
		return nil, 0, err
	}

	// Get messages with pagination
	url := fmt.Sprintf("%s/rest/v1/chat_messages?session_id=eq.%s&order=timestamp.asc&limit=%d&offset=%d",
		r.config.SupabaseURL, sessionID, limit, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get messages: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("get messages failed with status %d: %s", resp.StatusCode, string(body))
	}

	var messagesData []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&messagesData); err != nil {
		return nil, 0, fmt.Errorf("failed to decode messages: %v", err)
	}

	var messages []Message
	for _, data := range messagesData {
		message, err := r.mapToMessage(data)
		if err != nil {
			r.logger.Warn("Failed to map message data", map[string]interface{}{
				"error": err.Error(),
				"data":  data,
			})
			continue
		}
		messages = append(messages, *message)
	}

	return messages, total, nil
}

// DeleteMessages deletes specific messages
func (r *ChatRepository) DeleteMessages(ctx context.Context, sessionID string, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	// Quote each UUID for Supabase REST API
	quotedIDs := make([]string, len(messageIDs))
	for i, id := range messageIDs {
		quotedIDs[i] = fmt.Sprintf("\"%s\"", id)
	}

	// Build filter for multiple IDs: id=in.(id1,id2,id3) with session_id filter
	idsFilter := fmt.Sprintf("id=in.(%s)", strings.Join(quotedIDs, ","))
	url := fmt.Sprintf("%s/rest/v1/chat_messages?session_id=eq.%s&%s",
		r.config.SupabaseURL, sessionID, idsFilter)

	r.logger.Info("Deleting specific messages", map[string]interface{}{
		"session_id":  sessionID,
		"message_ids": messageIDs,
		"url":         url,
	})

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete messages: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		r.logger.Error("Delete messages failed", map[string]interface{}{
			"status_code": resp.StatusCode,
			"response":    string(body),
			"url":         url,
		})
		return fmt.Errorf("delete messages failed with status %d: %s", resp.StatusCode, string(body))
	}

	r.logger.Info("Messages deleted successfully", map[string]interface{}{
		"session_id":  sessionID,
		"message_ids": messageIDs,
		"status_code": resp.StatusCode,
	})

	return nil
}

// DeleteAllMessages deletes all messages in a session
func (r *ChatRepository) DeleteAllMessages(ctx context.Context, sessionID string) error {
	url := fmt.Sprintf("%s/rest/v1/chat_messages?session_id=eq.%s", r.config.SupabaseURL, sessionID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete messages: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete messages failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteAllUserMessages deletes all messages for a user (across all sessions)
func (r *ChatRepository) DeleteAllUserMessages(ctx context.Context, userID string) error {
	// First get all session IDs for the user
	url := fmt.Sprintf("%s/rest/v1/chat_sessions?user_id=eq.%s&select=id", r.config.SupabaseURL, userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("get user sessions failed with status %d: %s", resp.StatusCode, string(body))
	}

	var sessions []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return fmt.Errorf("failed to decode sessions: %v", err)
	}

	// Delete messages for each session
	for _, session := range sessions {
		sessionID := getString(session, "id")
		if sessionID != "" {
			if err := r.DeleteAllMessages(ctx, sessionID); err != nil {
				r.logger.Warn("Failed to delete messages for session", map[string]interface{}{
					"session_id": sessionID,
					"error":      err.Error(),
				})
			}
		}
	}

	return nil
}

// CountMessages counts messages in a session
func (r *ChatRepository) CountMessages(ctx context.Context, sessionID string) (int, error) {
	url := fmt.Sprintf("%s/rest/v1/chat_messages?session_id=eq.%s&select=count",
		r.config.SupabaseURL, sessionID)

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)
	req.Header.Set("Prefer", "count=exact")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %v", err)
	}
	defer resp.Body.Close()

	return r.parseCountFromHeader(resp.Header.Get("Content-Range")), nil
}

// CountSessions counts sessions for a user
func (r *ChatRepository) CountSessions(ctx context.Context, userID string) (int, error) {
	url := fmt.Sprintf("%s/rest/v1/chat_sessions?user_id=eq.%s&is_active=eq.true&select=count",
		r.config.SupabaseURL, userID)

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", r.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+r.config.SupabaseKey)
	req.Header.Set("Prefer", "count=exact")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to count sessions: %v", err)
	}
	defer resp.Body.Close()

	return r.parseCountFromHeader(resp.Header.Get("Content-Range")), nil
}

// Helper function to map REST API response to ChatSession
func (r *ChatRepository) mapToSession(data map[string]interface{}) (*ChatSession, error) {
	session := &ChatSession{
		ID:       getString(data, "id"),
		UserID:   getString(data, "user_id"),
		IsActive: getBool(data, "is_active"),
	}

	// Parse timestamps
	if createdAtStr := getString(data, "created_at"); createdAtStr != "" {
		if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			session.CreatedAt = createdAt
		}
	}

	if lastActivityStr := getString(data, "last_activity"); lastActivityStr != "" {
		if lastActivity, err := time.Parse(time.RFC3339, lastActivityStr); err == nil {
			session.LastActivity = lastActivity
		}
	}

	// Parse context
	if contextInterface, ok := data["context"]; ok && contextInterface != nil {
		if contextMap, ok := contextInterface.(map[string]interface{}); ok {
			session.Context = contextMap
		} else {
			session.Context = make(map[string]interface{})
		}
	} else {
		session.Context = make(map[string]interface{})
	}

	return session, nil
}

// Helper function to map REST API response to Message
func (r *ChatRepository) mapToMessage(data map[string]interface{}) (*Message, error) {
	message := &Message{
		ID:        getString(data, "id"),
		SessionID: getString(data, "session_id"),
		Role:      getString(data, "role"),
		Content:   getString(data, "content"),
	}

	// Parse timestamp
	if timestampStr := getString(data, "timestamp"); timestampStr != "" {
		if timestamp, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			message.Timestamp = timestamp
		}
	}

	// Parse metadata
	if metadataInterface, ok := data["metadata"]; ok && metadataInterface != nil {
		if metadataMap, ok := metadataInterface.(map[string]interface{}); ok {
			message.Metadata = metadataMap
		} else {
			message.Metadata = make(map[string]interface{})
		}
	} else {
		message.Metadata = make(map[string]interface{})
	}

	return message, nil
}

// Helper function to parse count from Content-Range header
func (r *ChatRepository) parseCountFromHeader(contentRange string) int {
	if contentRange == "" {
		return 0
	}

	// Parse count from "0-9/10" format
	parts := strings.Split(contentRange, "/")
	if len(parts) != 2 {
		return 0
	}

	count := 0
	fmt.Sscanf(parts[1], "%d", &count)
	return count
}

// Helper functions for safe type conversion
func getString(data map[string]interface{}, key string) string {
	if value, ok := data[key]; ok && value != nil {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func getBool(data map[string]interface{}, key string) bool {
	if value, ok := data[key]; ok && value != nil {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return false
}
