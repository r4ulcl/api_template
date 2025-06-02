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
	// Username is the unique identifier for the user.
	// It serves as the primary key in the database.
	Username string `gorm:"primaryKey" json:"username"`

	// Password stores the hashed password for authentication.
	// The JSON tag omits this field in API responses for security reasons.
	Password string `json:"password"`

	// Role defines the user's permissions, either "admin" or "user".
	Role Role `json:"role"`

	// CreatedAt is the timestamp of when the user was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the timestamp of the last modification to the user record.
	UpdatedAt time.Time `json:"updated_at"`
}
