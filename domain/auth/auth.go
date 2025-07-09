package auth

import (
	"net/http"
	"github.com/dzuura/satu-lemari/domain/config"

	"github.com/supabase-community/gotrue-go"
)

type AuthService struct {
	config *config.Config
	client gotrue.Client
}

func NewAuthService(cfg *config.Config) *AuthService {
	client := gotrue.New(cfg.SupabaseURL, cfg.SupabaseKey)
	return &AuthService{config: cfg, client: client}
}

func (s *AuthService) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/login", s.Login).Methods("POST")
	r.HandleFunc("/register", s.Register).Methods("POST")
	r.HandleFunc("/google-login", s.GoogleLogin).Methods("POST")
}

func (s *AuthService) Login(w http.ResponseWriter, r *http.Request) {
	// Placeholder for login logic
	w.Write([]byte("Login endpoint"))
}

func (s *AuthService) Register(w http.ResponseWriter, r *http.Request) {
	// Placeholder for register logic
	w.Write([]byte("Register endpoint"))
}

func (s *AuthService) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	// Placeholder for Google login logic
	w.Write([]byte("Google login endpoint"))
}