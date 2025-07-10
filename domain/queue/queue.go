package queue

import (
	"context"
	"encoding/json"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/error"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"net/http"
)

type QueueService struct {
	config *config.Config
	redis  *redis.Client
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
	return &QueueService{config: cfg, redis: redisClient}
}

func (s *QueueService) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/queue/request", s.RequestItem).Methods("POST")
}

func (s *QueueService) RequestItem(w http.ResponseWriter, r *http.Request) {
	var queueItem QueueItem
	if err := json.NewDecoder(r.Body).Decode(&queueItem); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	queueKey := "queue:" + queueItem.Type
	err := s.redis.RPush(context.Background(), queueKey, queueItem).Err()
	if err != nil {
		http.Error(w, error.NewInternalServerError(err.Error()).Message, http.StatusInternalServerError)
		return
	}
	w.Write([]byte("Request queued"))
}
