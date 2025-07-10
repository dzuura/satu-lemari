package item

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/dzuura/satu-lemari/domain/config"
	"net/http"

	"github.com/go-redis/redis/v8"
	"github.com/supabase-community/gotrue-go"
	storage "github.com/supabase-community/storage-go"
)

type ItemService struct {
	config   *config.Config
	supabase gotrue.Client
	storage  *storage.Client
	redis    *redis.Client
}

type Item struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"` // "donation" or "rental"
	Category    string  `json:"category"`
	Size        string  `json:"size"`
	Color       string  `json:"color"`
	Quantity    int     `json:"quantity"`
	Price       float64 `json:"price,omitempty"`
	Description string  `json:"description"`
	Status      string  `json:"status"` // "available" or "not available"
}

func NewItemService(cfg *config.Config) *ItemService {
	supabase := gotrue.New(cfg.SupabaseURL, cfg.SupabaseKey)
	storageClient := storage.NewClient(cfg.SupabaseURL, cfg.SupabaseKey, map[string]string{})
	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})
	return &ItemService{config: cfg, supabase: supabase, storage: storageClient, redis: redisClient}
}

func (s *ItemService) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/items", s.CreateItem).Methods("POST")
	r.HandleFunc("/items/{id}", s.GetItem).Methods("GET")
	r.HandleFunc("/items/recommend", s.GetRecommendations).Methods("GET")
}

func (s *ItemService) CreateItem(w http.ResponseWriter, r *http.Request) {
	var item Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Store item in Supabase
	if item.Quantity == 0 {
		item.Status = "not available"
	} else {
		item.Status = "available"
	}
	w.Write([]byte("Item created"))
}

func (s *ItemService) GetItem(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	cachedItem, err := s.redis.Get(context.Background(), "item:"+id).Result()
	if err == nil {
		w.Write([]byte(cachedItem))
		return
	}
	// Fetch from Supabase
	w.Write([]byte("Get item with ID: " + id))
}

func (s *ItemService) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	// Placeholder for recommendation logic
	w.Write([]byte("Recommended items"))
}