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

	// Audit
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	LastUpdate time.Time `gorm:"column:last_update;autoUpdateTime" json:"last_update"`
	CreatedBy  string    `gorm:"column:created_by" json:"created_by"`
	EditedBy   string    `gorm:"column:edited_by" json:"edited_by"`
}

// models/api_key.go
type APIKey struct {
	Token     string     `gorm:"primaryKey;column:token;size:512" json:"token"`
	Description  string     `gorm:"column:description" json:"description"`
	Username  string     `gorm:"column:username;index;not null" json:"username"`
	Enabled   bool       `gorm:"column:enabled;default:true" json:"enabled"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	LastUsed  *time.Time `gorm:"column:last_used" json:"last_used,omitempty"`
}

func (APIKey) TableName() string { return "api_key" } // singular

// UpdateUserRequest represents the payload for updating a user, including an optional new password.
type UpdateUser struct {
	User               // embed all the User fields
	NewPassword string `json:"new_password" example:"new_password"`
}

// UpdateUserWhitelist defines which JSON fields are accepted per role.
var UpdateUserWhitelist = map[Role]map[string]bool{
	UserRole: {
		"password":     true, // current password input
		"new_password": true,
		"email":        true, // allow users to change email
	},
	AdminRole: {
		"new_password":   true,
		"role":           true,
		"email":          true,
		"email_verified": true,
	},
}
