package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// UserManager handles user authorization and validation
type UserManager struct {
	allowedIDs map[int64]bool
	// In production, store authorized users in database
}

// NewUserManager creates a new user manager
func NewUserManager(allowedUsers []int64) *UserManager {
	allowedIDs := make(map[int64]bool)
	for _, id := range allowedUsers {
		allowedIDs[id] = true
	}

	return &UserManager{
		allowedIDs: allowedIDs,
	}
}

// Authorize checks if a user is authorized
func (m *UserManager) Authorize(userID int64) bool {
	return m.allowedIDs[userID]
}

// AddUser adds a user to allowlist
func (m *UserManager) AddUser(userID int64) {
	m.allowedIDs[userID] = true
	log.Printf("✅ User added to allowlist: %d", userID)
}

// RemoveUser removes a user from allowlist
func (m *UserManager) RemoveUser(userID int64) {
	delete(m.allowedIDs, userID)
	log.Printf("❌ User removed from allowlist: %d", userID)
}

// GetAllowedUsers returns list of allowed users
func (m *UserManager) GetAllowedUsers() []int64 {
	users := make([]int64, 0, len(m.allowedIDs))
	for id := range m.allowedIDs {
		users = append(users, id)
	}

	return users
}

// HandleAddUserCommand handles /adduser command
func HandleAddUserCommand(ctx context.Context, botToken, chatID, adminID, userID int64, newUserID int64) error {
	// Verify admin is authorized
	if adminID != 5088760910 {
		return fmt.Errorf("unauthorized: only admin can add users")
	}

	// TODO: Add to database
	log.Printf("👤 Adding user %d to allowlist", newUserID)

	return nil
}

// HandleRemoveUserCommand handles /removeuser command
func HandleRemoveUserCommand(ctx context.Context, botToken, chatID, adminID, userID int64, targetUserID int64) error {
	// Verify admin is authorized
	if adminID != 5088760910 {
		return fmt.Errorf("unauthorized: only admin can remove users")
	}

	// TODO: Remove from database
	log.Printf("👤 Removing user %d from allowlist", targetUserID)

	return nil
}

// SendAdminResponse sends a response to admin
func SendAdminResponse(ctx context.Context, botToken, chatID int64, text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}
