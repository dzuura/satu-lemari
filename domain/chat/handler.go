package chat

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/dzuura/satu-lemari/domain/common"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/logging"
)

// ChatHandler handles chat-related HTTP requests
type ChatHandler struct {
	chatService *ChatService
	logger      *logging.Logger
}

// NewChatHandler creates a new chat handler
func NewChatHandler(chatService *ChatService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		logger:      logging.GetLogger(),
	}
}

// RegisterRoutes registers chat routes
func (h *ChatHandler) RegisterRoutes(router *mux.Router, authMiddleware interface{}) {
	// Protected routes (require authentication)
	protected := router.PathPrefix("/chat").Subrouter()

	// Apply auth middleware if available
	if middleware, ok := authMiddleware.(interface {
		RequireAuth(http.Handler) http.Handler
	}); ok {
		protected.Use(middleware.RequireAuth)
	}

	protected.HandleFunc("/start", h.StartChat).Methods("POST")
	protected.HandleFunc("/send", h.SendMessage).Methods("POST")
	protected.HandleFunc("/history/{sessionId}", h.GetHistory).Methods("GET")
	protected.HandleFunc("/sessions", h.GetUserSessions).Methods("GET")
	protected.HandleFunc("/sessions/{sessionId}", h.DeleteSession).Methods("DELETE")
	protected.HandleFunc("/sessions/{sessionId}/messages", h.DeleteMessages).Methods("DELETE")
	protected.HandleFunc("/sessions/{sessionId}/messages/all", h.DeleteAllMessages).Methods("DELETE")
	protected.HandleFunc("/history/all", h.DeleteAllUserHistory).Methods("DELETE")

	// Public routes
	router.HandleFunc("/chat/suggestions", h.GetSuggestions).Methods("GET")

	// Debug route to test if chat routes are working
	router.HandleFunc("/chat/health", h.HealthCheck).Methods("GET")
}

// StartChat starts a new chat session
func (h *ChatHandler) StartChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	var req StartChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("Invalid request body for start chat", map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Validate language
	if req.Language == "" {
		req.Language = "id" // Default to Indonesian
	}

	session, err := h.chatService.StartSession(ctx, userID, req.Language)
	if err != nil {
		h.logger.Error("Failed to start chat session", map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to start chat session"),
			common.GenerateTraceID())
		return
	}

	h.logger.Info("Chat session started", map[string]interface{}{
		"session_id": session.ID,
		"user_id":    userID,
	})

	common.WriteSuccessResponse(w, session, "Chat session started successfully")
}

// SendMessage sends a message in a chat session
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("Invalid request body for send message", map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	// Validate message
	if err := h.chatService.validateMessage(req.Message); err != nil {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, err.Error()),
			common.GenerateTraceID())
		return
	}

	// Check if topic is allowed
	if !h.chatService.isTopicAllowed(req.Message) {
		response := &ChatResponse{
			SessionID:     req.SessionID,
			Message:       "Maaf, saya hanya dapat membantu dengan topik yang berkaitan dengan pakaian, donasi, sewa, dan perawatan pakaian. Apakah ada yang bisa saya bantu terkait hal tersebut?",
			MessageType:   "text",
			CanContinue:   true,
			SessionActive: true,
			QuickReplies: []QuickReply{
				{Text: "Donasi pakaian", Payload: "donation_help", Icon: "💝"},
				{Text: "Sewa pakaian", Payload: "rental_help", Icon: "👗"},
				{Text: "Tips perawatan", Payload: "care_tips", Icon: "🧺"},
				{Text: "Cari pakaian", Payload: "search_help", Icon: "🔍"},
			},
		}
		common.WriteSuccessResponse(w, response, "Message processed")
		return
	}

	response, err := h.chatService.SendMessage(ctx, &req)
	if err != nil {
		h.logger.Error("Failed to send message", map[string]interface{}{
			"error":      err.Error(),
			"user_id":    userID,
			"session_id": req.SessionID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to process message"),
			common.GenerateTraceID())
		return
	}

	h.logger.Info("Message processed", map[string]interface{}{
		"session_id": req.SessionID,
		"user_id":    userID,
	})

	common.WriteSuccessResponse(w, response, "Message processed successfully")
}

// GetHistory retrieves chat history for a session
func (h *ChatHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]

	// Parse pagination parameters
	limit := 50 // Default limit
	offset := 0 // Default offset

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	history, err := h.chatService.GetHistory(ctx, sessionID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get chat history", map[string]interface{}{
			"error":      err.Error(),
			"user_id":    userID,
			"session_id": sessionID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to get chat history"),
			common.GenerateTraceID())
		return
	}

	h.logger.Info("Chat history retrieved", map[string]interface{}{
		"session_id": sessionID,
		"user_id":    userID,
		"count":      len(history.Messages),
	})

	common.WriteSuccessResponse(w, history, "Chat history retrieved successfully")
}

// GetUserSessions retrieves all chat sessions for a user
func (h *ChatHandler) GetUserSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	// Parse pagination parameters
	limit := 20 // Default limit
	offset := 0 // Default offset

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 50 {
			limit = parsedLimit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	sessions, total, err := h.chatService.getUserSessions(ctx, userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get user sessions", map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to get chat sessions"),
			common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"sessions": sessions,
		"total":    total,
		"has_more": offset+len(sessions) < total,
	}

	h.logger.Info("User sessions retrieved", map[string]interface{}{
		"user_id": userID,
		"count":   len(sessions),
		"total":   total,
	})

	common.WriteSuccessResponse(w, response, "Chat sessions retrieved successfully")
}

// DeleteSession permanently deletes a chat session and all its messages
func (h *ChatHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]

	err := h.chatService.DeleteHistory(ctx, sessionID, userID)
	if err != nil {
		h.logger.Error("Failed to delete chat session", map[string]interface{}{
			"error":      err.Error(),
			"user_id":    userID,
			"session_id": sessionID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to delete chat session"),
			common.GenerateTraceID())
		return
	}

	h.logger.Info("Chat session deleted", map[string]interface{}{
		"session_id": sessionID,
		"user_id":    userID,
	})

	common.WriteSuccessResponse(w, map[string]interface{}{
		"session_id": sessionID,
		"status":     "permanently_deleted",
	}, "Chat session and all messages permanently deleted")
}

// GetSuggestions returns chat suggestions (public endpoint)
func (h *ChatHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	suggestions := h.chatService.GetSuggestions(ctx)

	h.logger.Info("Chat suggestions retrieved", map[string]interface{}{
		"count": len(suggestions.Suggestions),
	})

	common.WriteSuccessResponse(w, suggestions, "Chat suggestions retrieved successfully")
}

// DeleteMessages deletes specific messages from a chat session
func (h *ChatHandler) DeleteMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]

	var req struct {
		MessageIDs []string `json:"message_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("Invalid request body for delete messages", map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Invalid request body"),
			common.GenerateTraceID())
		return
	}

	if len(req.MessageIDs) == 0 {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInvalidInput, "Message IDs are required"),
			common.GenerateTraceID())
		return
	}

	err := h.chatService.DeleteMessages(ctx, sessionID, req.MessageIDs, userID)
	if err != nil {
		h.logger.Error("Failed to delete messages", map[string]interface{}{
			"error":      err.Error(),
			"user_id":    userID,
			"session_id": sessionID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to delete messages"),
			common.GenerateTraceID())
		return
	}

	h.logger.Info("Messages deleted", map[string]interface{}{
		"session_id":  sessionID,
		"user_id":     userID,
		"message_ids": req.MessageIDs,
	})

	common.WriteSuccessResponse(w, map[string]interface{}{
		"session_id":    sessionID,
		"deleted_count": len(req.MessageIDs),
		"message_ids":   req.MessageIDs,
	}, "Messages deleted successfully")
}

// DeleteAllMessages deletes all messages from a chat session
func (h *ChatHandler) DeleteAllMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]

	err := h.chatService.DeleteAllMessages(ctx, sessionID, userID)
	if err != nil {
		h.logger.Error("Failed to delete all messages", map[string]interface{}{
			"error":      err.Error(),
			"user_id":    userID,
			"session_id": sessionID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to delete all messages"),
			common.GenerateTraceID())
		return
	}

	h.logger.Info("All messages deleted", map[string]interface{}{
		"session_id": sessionID,
		"user_id":    userID,
	})

	common.WriteSuccessResponse(w, map[string]interface{}{
		"session_id": sessionID,
		"status":     "all_messages_deleted",
	}, "All messages deleted successfully")
}

// DeleteAllUserHistory deletes all chat history for the authenticated user
func (h *ChatHandler) DeleteAllUserHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrUnauthorized, "Unauthorized"),
			common.GenerateTraceID())
		return
	}

	err := h.chatService.DeleteAllUserHistory(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to delete all user history", map[string]interface{}{
			"error":   err.Error(),
			"user_id": userID,
		})
		appError.WriteErrorResponse(w,
			appError.New(appError.ErrInternal, "Failed to delete all chat history"),
			common.GenerateTraceID())
		return
	}

	h.logger.Info("All user history deleted", map[string]interface{}{
		"user_id": userID,
	})

	common.WriteSuccessResponse(w, map[string]interface{}{
		"user_id": userID,
		"status":  "all_history_deleted",
	}, "All chat history deleted successfully")
}

// HealthCheck is a debug endpoint to test if chat routes are working
func (h *ChatHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "ok",
		"service":   "chat",
		"timestamp": time.Now().UTC(),
		"routes": map[string]string{
			"start_chat":          "POST /api/v1/chat/start",
			"send_message":        "POST /api/v1/chat/send",
			"get_history":         "GET /api/v1/chat/history/{sessionId}",
			"get_sessions":        "GET /api/v1/chat/sessions",
			"delete_session":      "DELETE /api/v1/chat/sessions/{sessionId}",
			"delete_messages":     "DELETE /api/v1/chat/sessions/{sessionId}/messages",
			"delete_all_messages": "DELETE /api/v1/chat/sessions/{sessionId}/messages/all",
			"delete_all_history":  "DELETE /api/v1/chat/history/all",
			"get_suggestions":     "GET /api/v1/chat/suggestions",
		},
		"auth_required": []string{
			"start_chat", "send_message", "get_history", "get_sessions",
			"delete_session", "delete_messages", "delete_all_messages", "delete_all_history",
		},
		"public": []string{"get_suggestions", "health_check"},
	}

	h.logger.Info("Chat health check accessed", map[string]interface{}{
		"remote_addr": r.RemoteAddr,
		"user_agent":  r.UserAgent(),
	})

	common.WriteSuccessResponse(w, response, "Chat service is healthy")
}
