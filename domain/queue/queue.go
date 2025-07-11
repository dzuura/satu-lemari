package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/dzuura/satu-lemari/domain/auth"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/item"
	"github.com/redis/go-redis/v9"
	"github.com/gorilla/mux"
)

type QueueService struct {
	config      *config.Config
	redis       *redis.Client
	itemService *item.ItemService
}

type QueueItem struct {
	UserID    string `json:"user_id"`
	ItemID    string `json:"item_id"`
	Type      string `json:"type"` // "donation" or "rental"
	Timestamp int64  `json:"timestamp"`
}

func NewQueueService(cfg *config.Config) *QueueService {
	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})
	itemService := item.NewItemService(cfg)
	return &QueueService{config: cfg, redis: redisClient, itemService: itemService}
}

func (s *QueueService) RegisterRoutes(r *mux.Router) {
	authMiddleware := auth.NewAuthService(s.config).Middleware
	r.Handle("/queue/request", authMiddleware(http.HandlerFunc(s.RequestItem))).Methods("POST")
}

func (s *QueueService) RequestItem(w http.ResponseWriter, r *http.Request) {
	var queueItem QueueItem
	if err := json.NewDecoder(r.Body).Decode(&queueItem); err != nil {
		http.Error(w, error.NewBadRequestError(err.Error()).Message, http.StatusBadRequest)
		return
	}

	role := r.Context().Value("role").(string)
	if role != "user" {
		http.Error(w, error.NewUnauthorizedError("Only users can request items").Message, http.StatusUnauthorized)
		return
	}

	queueKey := "queue:" + queueItem.Type
	err := s.redis.RPush(context.Background(), queueKey, queueItem).Err()
	if err != nil {
		http.Error(w, error.NewInternalServerError(err.Error()).Message, http.StatusInternalServerError)
		return
	}

	// Check queue and update item status
	s.processQueue(queueKey)
	w.Write([]byte("Request queued"))
}

func (s *QueueService) processQueue(queueKey string) {
	for {
		queueItem, err := s.redis.LPop(context.Background(), queueKey).Result()
		if err == redis.Nil {
			break // Queue is empty
		} else if err != nil {
			log.Printf("Error popping queue: %v", err)
			break
		}

		var item QueueItem
		if err := json.Unmarshal([]byte(queueItem), &item); err != nil {
			log.Printf("Error unmarshaling queue item: %v", err)
			continue
		}

		// Fetch item from Supabase or cache
		url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s", s.config.SupabaseURL, item.ItemID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Printf("Error creating request: %v", err)
			continue
		}

		req.Header.Add("apikey", s.config.SupabaseKey)
		req.Header.Add("Authorization", "Bearer "+s.config.SupabaseKey)
		req.Header.Add("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error fetching item: %v", err)
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading response: %v", err)
			continue
		}

		var items []map[string]interface{}
		if err := json.Unmarshal(body, &items); err != nil {
			log.Printf("Error parsing item: %v", err)
			continue
		}

		if len(items) > 0 {
			quantity := int(items[0]["quantity"].(float64))
			if quantity > 0 {
				// Update item status to "not available" and decrease quantity
				err = s.itemService.UpdateItemStatus(item.ItemID, "not available")
				if err == nil {
					// Decrease quantity (simplified logic)
					newQuantity := quantity - 1
					updateData := map[string]interface{}{
						"quantity": newQuantity,
					}
					jsonData, _ := json.Marshal(updateData)
					url := fmt.Sprintf("%s/rest/v1/items?id=eq.%s", s.config.SupabaseURL, item.ItemID)
					req, _ := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
					req.Header.Add("apikey", s.config.SupabaseKey)
					req.Header.Add("Authorization", "Bearer "+s.config.SupabaseKey)
					req.Header.Add("Content-Type", "application/json")
					client.Do(req)
				}
			}
		}
		time.Sleep(1 * time.Second) // Prevent race conditions
	}
}
