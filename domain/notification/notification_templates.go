package notification

import (
	"fmt"
	"strings"
)

// NotificationTemplates contains all predefined notification templates
var NotificationTemplates = map[string]*NotificationTemplate{
	// 1. User membuat request (confirmation to user)
	"request_created": {
		ID:       "request_created",
		Title:    "Permintaan Berhasil Dibuat",
		Message:  "Permintaan {type} untuk {item_name} telah berhasil dibuat dan sedang menunggu persetujuan partner.",
		Type:     "request_created",
		Channels: []string{"mobile", "web"},
		Priority: "normal",
	},

	// 1b. New request notification to partner
	"new_request": {
		ID:       "new_request",
		Title:    "Permintaan Baru",
		Message:  "Ada permintaan {type} baru untuk {item_name}. Silakan periksa dan berikan persetujuan.",
		Type:     "new_request",
		Channels: []string{"mobile", "web"},
		Priority: "high",
	},

	// 2. Partner update request - Approved
	"request_approved": {
		ID:       "request_approved",
		Title:    "Permintaan Disetujui",
		Message:  "Permintaan {type} untuk {item_name} telah disetujui. Silakan hubungi partner untuk pengambilan.",
		Type:     "request_approved",
		Channels: []string{"mobile", "web"},
		Priority: "high",
	},

	// 3. Partner update request - Rejected
	"request_rejected": {
		ID:       "request_rejected",
		Title:    "Permintaan Ditolak",
		Message:  "Permintaan {type} untuk {item_name} telah ditolak oleh partner.",
		Type:     "request_rejected",
		Channels: []string{"mobile", "web"},
		Priority: "high",
	},

	// 4. Partner update request - Completed
	"request_completed": {
		ID:       "request_completed",
		Title:    "Permintaan Selesai",
		Message:  "Permintaan {type} untuk {item_name} telah selesai. Terima kasih!",
		Type:     "request_completed",
		Channels: []string{"mobile", "web"},
		Priority: "normal",
	},

	// 5. Admin system update
	"system_update": {
		ID:       "system_update",
		Title:    "Update Sistem",
		Message:  "Ada update penting dari sistem SatuLemari. Tap untuk melihat detail.",
		Type:     "system_update",
		Channels: []string{"mobile", "web"},
		Priority: "normal",
	},

	// 6. Admin promotion
	"promotion": {
		ID:       "promotion",
		Title:    "Promo Spesial",
		Message:  "Ada promo menarik untuk Anda! Jangan sampai terlewat.",
		Type:     "promotion",
		Channels: []string{"mobile", "web"},
		Priority: "low",
	},

	// Additional templates for other scenarios
	"item_available": {
		ID:       "item_available",
		Title:    "Item Tersedia",
		Message:  "Item {item_name} yang Anda cari sekarang tersedia!",
		Type:     "item_available",
		Channels: []string{"mobile", "web"},
		Priority: "normal",
	},

	"item_out_of_stock": {
		ID:       "item_out_of_stock",
		Title:    "Item Habis",
		Message:  "Maaf, item {item_name} sedang tidak tersedia.",
		Type:     "item_out_of_stock",
		Channels: []string{"mobile", "web"},
		Priority: "low",
	},

	"reminder_return": {
		ID:       "reminder_return",
		Title:    "Pengingat Pengembalian",
		Message:  "Jangan lupa mengembalikan {item_name} sebelum {due_date}.",
		Type:     "reminder_return",
		Channels: []string{"mobile", "web"},
		Priority: "high",
	},

	"overdue_item": {
		ID:       "overdue_item",
		Title:    "Item Terlambat",
		Message:  "Item {item_name} sudah melewati batas waktu pengembalian.",
		Type:     "overdue_item",
		Channels: []string{"mobile", "web", "email"},
		Priority: "urgent",
	},

	"welcome": {
		ID:       "welcome",
		Title:    "Selamat Datang!",
		Message:  "Selamat datang di SatuLemari! Mari mulai berbagi dan meminjam barang dengan mudah.",
		Type:     "welcome",
		Channels: []string{"mobile", "web"},
		Priority: "normal",
	},

	"quota_reset": {
		ID:       "quota_reset",
		Title:    "Kuota Direset",
		Message:  "Kuota peminjaman Anda telah direset. Anda dapat meminjam lagi!",
		Type:     "quota_reset",
		Channels: []string{"mobile", "web"},
		Priority: "normal",
	},

	"quota_exceeded": {
		ID:       "quota_exceeded",
		Title:    "Kuota Terlampaui",
		Message:  "Anda telah mencapai batas maksimal peminjaman. Kembalikan item untuk meminjam lagi.",
		Type:     "quota_exceeded",
		Channels: []string{"mobile", "web"},
		Priority: "high",
	},

	// Additional templates from service.go
	"item_received": {
		ID:       "item_received",
		Title:    "Barang Diterima",
		Message:  "Barang {item_name} telah berhasil diterima. Terima kasih telah menggunakan SatuLemari!",
		Type:     "transaction_update",
		Channels: []string{"mobile", "web"},
		Priority: "normal",
	},

	"item_returned": {
		ID:       "item_returned",
		Title:    "Barang Dikembalikan",
		Message:  "Barang {item_name} telah berhasil dikembalikan. Terima kasih telah menggunakan SatuLemari!",
		Type:     "transaction_update",
		Channels: []string{"mobile", "web"},
		Priority: "normal",
	},

	"new_item_available": {
		ID:       "new_item_available",
		Title:    "Barang Baru Tersedia",
		Message:  "Ada barang baru {item_name} yang sesuai dengan preferensi Anda. Segera cek sekarang!",
		Type:     "item_update",
		Channels: []string{"mobile", "web", "email"},
		Priority: "normal",
	},

	"reminder_pickup": {
		ID:       "reminder_pickup",
		Title:    "Pengingat Pengambilan",
		Message:  "Jangan lupa untuk mengambil barang {item_name} yang telah disetujui dalam 24 jam ke depan.",
		Type:     "reminder",
		Channels: []string{"mobile", "web", "email"},
		Priority: "high",
	},
}

// GetTemplate returns a notification template by ID
func GetTemplate(templateID string) (*NotificationTemplate, bool) {
	template, exists := NotificationTemplates[templateID]
	return template, exists
}

// CreateNotificationFromTemplate creates a notification from template with data substitution
func CreateNotificationFromTemplate(templateID string, data map[string]interface{}) (*NotificationTemplate, error) {
	template, exists := GetTemplate(templateID)
	if !exists {
		return nil, fmt.Errorf("template with ID '%s' not found", templateID)
	}

	// Create a copy of the template
	result := &NotificationTemplate{
		ID:       template.ID,
		Title:    template.Title,
		Message:  template.Message,
		Type:     template.Type,
		Channels: template.Channels,
		Priority: template.Priority,
		Data:     data,
	}

	// Substitute placeholders in title and message
	result.Title = substitutePlaceholders(result.Title, data)
	result.Message = substitutePlaceholders(result.Message, data)

	return result, nil
}

// substitutePlaceholders replaces {key} placeholders with values from data
func substitutePlaceholders(text string, data map[string]interface{}) string {
	result := text
	for key, value := range data {
		placeholder := fmt.Sprintf("{%s}", key)
		if valueStr, ok := value.(string); ok {
			result = strings.ReplaceAll(result, placeholder, valueStr)
		} else {
			result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
		}
	}
	return result
}

// Helper functions for common notification scenarios

// CreateRequestNotification creates notification for request-related events
func CreateRequestNotification(eventType, requestType, itemName, requestID string) (*NotificationTemplate, error) {
	data := map[string]interface{}{
		"type":       requestType,
		"item_name":  itemName,
		"request_id": requestID,
	}

	return CreateNotificationFromTemplate(eventType, data)
}

// CreateItemNotification creates notification for item-related events
func CreateItemNotification(eventType, itemName, itemID string, additionalData map[string]interface{}) (*NotificationTemplate, error) {
	data := map[string]interface{}{
		"item_name": itemName,
		"item_id":   itemID,
	}

	// Merge additional data
	for k, v := range additionalData {
		data[k] = v
	}

	return CreateNotificationFromTemplate(eventType, data)
}

// CreateSystemNotification creates notification for system events
func CreateSystemNotification(eventType, customTitle, customMessage string, data map[string]interface{}) (*NotificationTemplate, error) {
	template, exists := GetTemplate(eventType)
	if !exists {
		return nil, fmt.Errorf("template with ID '%s' not found", eventType)
	}

	result := &NotificationTemplate{
		ID:       template.ID,
		Title:    customTitle,
		Message:  customMessage,
		Type:     template.Type,
		Channels: template.Channels,
		Priority: template.Priority,
		Data:     data,
	}

	return result, nil
}

// GetAllTemplates returns all available templates
func GetAllTemplates() map[string]*NotificationTemplate {
	return NotificationTemplates
}

// ValidateTemplate validates if a template has all required fields
func ValidateTemplate(template *NotificationTemplate) error {
	if template.ID == "" {
		return fmt.Errorf("template ID is required")
	}
	if template.Title == "" {
		return fmt.Errorf("template title is required")
	}
	if template.Message == "" {
		return fmt.Errorf("template message is required")
	}
	if template.Type == "" {
		return fmt.Errorf("template type is required")
	}
	if len(template.Channels) == 0 {
		return fmt.Errorf("template must have at least one channel")
	}
	return nil
}
