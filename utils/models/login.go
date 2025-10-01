package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// Role represents the user's role in the system.
type Role string

const (
	//AnonymousRole Role = "anonymous"
	UserRole     Role = "user"
	ReviewerRole Role = "reviewer"
	AdminRole    Role = "admin"
	// DefaultRoleWhitelist applies to any role not explicitly listed in UpdateUserWhitelist.
	DefaultRoleWhitelist Role = "*"
)

// User represents a system user.
//
// It contains authentication details and metadata like creation and update timestamps.
type User struct {
	// Primary key
	Username string `gorm:"primaryKey;column:username" json:"username"`

	// Authentication
	Password    string `gorm:"column:password" json:"password"` // never returned in API responses
	TotpSecret  string `gorm:"column:totp_secret" json:"totp_secret,omitempty"`
	TotpEnabled bool   `gorm:"column:totp_enabled" json:"totp_enabled"`

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
	ID          uint       `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Token       string     `gorm:"primaryKey;column:token;size:512" json:"token"`
	Description string     `gorm:"column:description" json:"description"`
	Username    string     `gorm:"column:username;index;not null" json:"username"`
	Expiracy    *time.Time `gorm:"column:expiracy" json:"expiracy"`
	Enabled     bool       `gorm:"column:enabled;default:true" json:"enabled"`
	LastUsed    *time.Time `gorm:"column:last_used" json:"last_used,omitempty"`

	// Audit
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	LastUpdate time.Time `gorm:"column:last_update;autoUpdateTime" json:"last_update"`
	CreatedBy  string    `gorm:"column:created_by" json:"created_by"`
	EditedBy   string    `gorm:"column:edited_by" json:"edited_by"`
}

func (APIKey) TableName() string { return "api_key" } // singular

func (k APIKey) MarshalJSON() ([]byte, error) {
	type Alias APIKey
	masked := Alias(k)
	masked.Token = maskTokenForResponse(k.Token)
	return json.Marshal(masked)
}

func maskTokenForResponse(token string) string {
	length := len(token)
	if length <= 6 {
		if length == 0 {
			return ""
		}
		return "REDACTED"
	}
	return fmt.Sprintf("%s...%s", token[:3], token[length-3:])
}

// UpdateUserRequest represents the payload for updating a user, including an optional new password.
type UpdateUser struct {
	User                   // embed all the User fields
	NewPassword     string `json:"new_password" example:"new_password"`
	TotpSecret      string `json:"totp_secret"`
	TotpEnabled     *bool  `json:"totp_enabled"`
	TotpResetSecret bool   `json:"totp_reset_secret"`
	TotpCode        string `json:"totp_code"`
}

// UpdateUserWhitelist defines which JSON fields are accepted per role.

var UpdateUserWhitelist = map[Role]map[string]bool{
	DefaultRoleWhitelist: {
		"password":          true, // current password input
		"new_password":      true,
		"email":             true, // allow non-admins to change email
		"totp_secret":       true,
		"totp_enabled":      true,
		"totp_reset_secret": true,
		"totp_code":         true,
	},
	AdminRole: {
		"new_password":      true,
		"role":              true,
		"email":             true,
		"email_verified":    true,
		"totp_secret":       true,
		"totp_enabled":      true,
		"totp_reset_secret": true,
		"totp_code":         true,
	},
}
