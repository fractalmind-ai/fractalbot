package channels

import "log"

// UserManager handles user authorization and validation
type UserManager struct {
	allowedIDs map[int64]bool
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
