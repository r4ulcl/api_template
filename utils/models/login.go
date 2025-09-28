package models

import "time"

// Role represents the user's role in the system.
type Role string

const (
	// AdminRole represents an administrator with higher privileges.
	AdminRole Role = "admin" // @Enum admin

	// UserRole represents a regular user with standard privileges.
	UserRole Role = "user" // @Enum user
)

// User represents a system user.
//
// It contains authentication details and metadata like creation and update timestamps.
type User struct {
	// Primary key
	Username string `gorm:"primaryKey;column:username" json:"username"`

	// Authentication
	Password string `gorm:"column:password" json:"password"` // never returned in API responses

	// Authorization
	Role Role `gorm:"column:role" json:"role"`

	// Contact
	Email         string `gorm:"column:email;type:varchar(191);uniqueIndex" json:"email"`
	EmailVerified bool   `gorm:"column:email_verified" json:"email_verified"`

	// API keys assigned to the user (stored as JSON in DB) for scripting
	APIKeys []string `json:"api_keys" gorm:"type:json;serializer:json"`

	// Audit
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	LastUpdate time.Time `gorm:"column:last_update;autoUpdateTime" json:"last_update"`
	CreatedBy  string    `gorm:"column:created_by" json:"created_by"`
	EditedBy   string    `gorm:"column:edited_by" json:"edited_by"`
}

// UpdateUserRequest represents the payload for updating a user, including an optional new password.
type UpdateUser struct {
	User               // embed all the User fields
	NewPassword string `json:"new_password" example:"new_password"`
}
