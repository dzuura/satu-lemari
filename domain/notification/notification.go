package notification

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"github.com/gorilla/mux"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/error"

	firebase "firebase.google.com/go/v4"
	fcm "firebase.google.com/go/v4/messaging"
)

type NotificationService struct {
	config *config.Config
	client *fcm.Client
}

type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Token string `json:"token"`
}

func NewNotificationService(cfg *config.Config) *NotificationService {
	app, err := firebase.NewApp(context.Background(), &firebase.Config{
		ProjectID: cfg.FirebaseProjectID,
	})
	if err != nil {
		log.Fatalf("error initializing app: %v", err)
	}
	client, err := app.Messaging(context.Background())
	if err != nil {
		log.Fatalf("error getting Messaging client: %v", err)
	}
	return &NotificationService{config: cfg, client: client}
}

func (s *NotificationService) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/notify", s.SendNotification).Methods("POST")
}

func (s *NotificationService) SendNotification(w http.ResponseWriter, r *http.Request) {
	var notif Notification
	if err := json.NewDecoder(r.Body).Decode(&notif); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response, err := s.client.Send(context.Background(), &fcm.Message{
		Notification: &fcm.Notification{
			Title: notif.Title,
			Body:  notif.Body,
		},
		Token: notif.Token,
	})
	if err != nil {
		http.Error(w, error.NewInternalServerError(err.Error()).Message, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(response)
}
