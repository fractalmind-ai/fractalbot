package channels

import (
	"context"
	"fmt"
	"log"
)

// WebhookHandler handles Telegram webhook updates
type WebhookHandler struct {
	bot *TelegramBot
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(bot *TelegramBot) *WebhookHandler {
	return &WebhookHandler{bot: bot}
}

// ServeHTTP handles incoming webhook requests
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Verify secret (TODO: add webhook secret verification)
	// secret := r.URL.Query().Get("secret")

	// Decode JSON update
	var update tgbotapi.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		log.Printf("Failed to decode webhook update: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Process update
	if err := h.bot.HandleUpdate(&update); err != nil {
		log.Printf("Error processing webhook update: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
