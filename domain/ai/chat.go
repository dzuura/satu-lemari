package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/logging"
)

// GenerateResponse generates an AI response for the chatbot
func (s *AIServiceManager) GenerateResponse(ctx context.Context, prompt string, data map[string]interface{}) (string, error) {
	logger := logging.GetLogger()

	if !s.IsGeminiAvailable() {
		return s.getFallbackResponse(prompt)
	}

	// Convert data to JSON for context
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	// Build complete prompt with context
	fullPrompt := prompt + "\nContext: " + string(dataJSON)

	// Use AIService to generate response
	response, err := s.geminiService.GenerateContent(ctx, fullPrompt)
	if err != nil {
		logger.Error("Failed to generate AI response", map[string]interface{}{
			"error": err.Error(),
		})
		return s.getFallbackResponse(prompt)
	}

	return response, nil
}

// getFallbackResponse provides a basic response when AI service is unavailable
func (s *AIServiceManager) getFallbackResponse(_ string) (string, error) {
	return "I apologize, but I'm currently unable to provide a detailed response. Please try again later or contact support for assistance.", nil
}

// ChatService handles chat functionality
type ChatService struct {
	geminiService *GeminiService
	config        *config.Config
}

// NewChatService creates a new chat service
func NewChatService(cfg *config.Config, geminiService *GeminiService) *ChatService {
	return &ChatService{
		geminiService: geminiService,
		config:        cfg,
	}
}

// GetSuggestions returns a list of suggested chat messages based on the current context
func (m *AIServiceManager) GetSuggestions() ([]string, error) {
	// Return static suggestions for now
	return []string{
		"Bagaimana cara menyumbangkan pakaian di SatuLemari?",
		"Jenis pakaian apa saja yang diterima?",
		"Bagaimana proses penyewaan bekerja?",
		"Saya butuh bantuan untuk menemukan ukuran yang tepat",
		"Tips and trick dalam merawat pakaian",
	}, nil
}

// GenerateResponseForChat generates an AI response for chat (ChatService method)
func (s *ChatService) GenerateResponse(ctx context.Context, prompt string, data map[string]interface{}) (string, error) {
	response, err := s.geminiService.GenerateContent(ctx, buildChatPrompt(prompt, data))
	if err != nil {
		return getFallbackResponseStatic(), nil
	}
	return response, nil
}

// buildChatPrompt creates a contextualized prompt for chat
func buildChatPrompt(message string, data map[string]interface{}) string {
	var sb strings.Builder

	// Add system context in Indonesian
	sb.WriteString("Kamu adalah asisten AI untuk SatuLemari, platform donasi dan sewa pakaian. ")
	sb.WriteString("Berikan respons yang membantu dan ringkas tentang pakaian, fashion, dan layanan platform kami. ")
	sb.WriteString("Gunakan nada yang ramah dan profesional, serta tekankan praktik fashion berkelanjutan. ")

	// Add platform context
	sb.WriteString("Tentang SatuLemari:\n")
	sb.WriteString("- Platform untuk donasi dan sewa pakaian\n")
	sb.WriteString("- Fokus pada fashion berkelanjutan dan circular economy\n")
	sb.WriteString("- Membantu mengurangi limbah tekstil\n")
	sb.WriteString("- Komunitas berbagi pakaian\n\n")

	// Add message context if available
	if data != nil {
		if topic, ok := data["topic"].(string); ok {
			sb.WriteString(fmt.Sprintf("Konteks pembahasan: %s\n", topic))
		}
		if intent, ok := data["intent"].(string); ok {
			sb.WriteString(fmt.Sprintf("Maksud pengguna: %s\n", intent))
		}
		if category, ok := data["category"].(string); ok {
			sb.WriteString(fmt.Sprintf("Kategori: %s\n", category))
		}
	}

	// Add guidelines
	sb.WriteString("Panduan respons:\n")
	sb.WriteString("- Gunakan bahasa Indonesia yang natural dan ramah\n")
	sb.WriteString("- Berikan informasi yang akurat tentang platform\n")
	sb.WriteString("- Jika tidak tahu, arahkan ke customer service\n")
	sb.WriteString("- Promosikan praktik fashion berkelanjutan\n")
	sb.WriteString("- Berikan saran yang praktis dan actionable\n\n")

	// Add the user's message
	sb.WriteString("Pengguna: ")
	sb.WriteString(message)
	sb.WriteString("\n\nAsisten: ")

	return sb.String()
}

// getFallbackResponseStatic provides a default response when AI is unavailable
func getFallbackResponseStatic() string {
	return "Saya mohon maaf, tetapi saat ini saya tidak dapat memberikan tanggapan yang detail. Silakan coba lagi nanti atau hubungi contact support untuk bantuan."
}
