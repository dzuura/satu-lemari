package item

import (
	"encoding/json"
	"github.com/dzuura/satu-lemari/domain/config"
	"net/http"

	"github.com/go-redis/redis/v8"
	"github.com/supabase-community/gotrue-go"
)

type ItemService struct {
	config   *config.Config
	supabase *gotrue.Client
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
	storageClient := storage.NewClient(cfg.SupabaseURL, cfg.SupabaseKey)
	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})
	return &ItemService{config: cfg, supabase: supabase, storage: storageClient, redis: redisClient}
}

func (s *ItemService) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/items", s.CreateItem).Methods("POST")
	r.HandleFunc("/items/{id}", s.GetItem).Methods("GET")
}

func (s *ItemService) CreateItem(w http.ResponseWriter, r *http.Request) {
	var item Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Placeholder for creating item in Supabase and storing image in Supabase Storage
	w.Write([]byte("Item created"))
}

func (s *ItemService) GetItem(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	// Placeholder for getting item from cache or Supabase
	w.Write([]byte("Get item with ID: " + id))
}
